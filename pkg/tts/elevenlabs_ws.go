package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	elevenLabsWSBaseURL = "wss://api.elevenlabs.io/v1/text-to-speech"
	keepaliveInterval   = 30 * time.Second
	reconnectBaseDelay  = 1 * time.Second
	reconnectMaxDelay   = 30 * time.Second
)

// ElevenLabsWS implements streaming TTS via WebSocket for lowest latency.
type ElevenLabsWS struct {
	config *Config
	logger *slog.Logger

	conn     *websocket.Conn
	connMu   sync.Mutex
	connected bool

	// Callbacks
	OnAudio      func(pcmData []byte) // Called for each audio chunk
	OnError      func(err error)      // Called on errors
	OnConnected  func()               // Called when connected
	OnDisconnect func()               // Called when disconnected

	// Internal state
	ctx        context.Context
	cancel     context.CancelFunc
	sendCh     chan string
	closeCh    chan struct{}
	reconnecting bool
}

// NewElevenLabsWS creates a new WebSocket-based ElevenLabs TTS provider.
func NewElevenLabsWS(opts ...Option) (*ElevenLabsWS, error) {
	cfg := DefaultConfig()
	cfg.Apply(opts...)

	if err := cfg.ValidateWithVoice(); err != nil {
		return nil, err
	}

	return &ElevenLabsWS{
		config:  cfg,
		logger:  cfg.Logger.With("component", "tts.elevenlabs_ws"),
		sendCh:  make(chan string, 100), // Buffer for text chunks
		closeCh: make(chan struct{}),
	}, nil
}

// Connect establishes the WebSocket connection (pre-warms for low latency).
func (e *ElevenLabsWS) Connect(ctx context.Context) error {
	e.ctx, e.cancel = context.WithCancel(ctx)

	if err := e.dial(); err != nil {
		return err
	}

	// Start background goroutines
	go e.readLoop()
	go e.writeLoop()
	go e.keepaliveLoop()

	return nil
}

// dial establishes the WebSocket connection.
func (e *ElevenLabsWS) dial() error {
	e.connMu.Lock()
	defer e.connMu.Unlock()

	// Build WebSocket URL with query params
	outputFormat := e.encodingToAPIFormat()
	url := fmt.Sprintf("%s/%s/stream-input?model_id=%s&output_format=%s",
		elevenLabsWSBaseURL, e.config.VoiceID, e.config.ModelID, outputFormat)

	// Set up headers
	headers := http.Header{}
	headers.Set("xi-api-key", e.config.APIKey)

	// Connect with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.DialContext(e.ctx, url, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket dial failed (status %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	e.conn = conn
	e.connected = true

	// Send Begin of Stream (BOS) message
	bos := map[string]interface{}{
		"text": " ", // Space to initialize
		"voice_settings": map[string]interface{}{
			"stability":        e.config.VoiceSettings.Stability,
			"similarity_boost": e.config.VoiceSettings.SimilarityBoost,
		},
		"generation_config": map[string]interface{}{
			"chunk_length_schedule": []int{120, 160, 250, 290}, // Optimized for low latency
		},
	}
	if err := conn.WriteJSON(bos); err != nil {
		conn.Close()
		return fmt.Errorf("send BOS: %w", err)
	}

	e.logger.Info("websocket connected", "voice", e.config.VoiceID, "model", e.config.ModelID)

	if e.OnConnected != nil {
		e.OnConnected()
	}

	return nil
}

// SendText queues text for synthesis (non-blocking).
func (e *ElevenLabsWS) SendText(chunk string) error {
	if chunk == "" {
		return nil
	}

	select {
	case e.sendCh <- chunk:
		return nil
	case <-e.ctx.Done():
		return e.ctx.Err()
	default:
		// Channel full, log warning but don't block
		e.logger.Warn("send channel full, dropping text chunk")
		return nil
	}
}

// Flush signals end of text stream and waits briefly for final audio.
func (e *ElevenLabsWS) Flush() error {
	e.connMu.Lock()
	defer e.connMu.Unlock()

	if !e.connected || e.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Send End of Stream (EOS) message
	eos := map[string]interface{}{
		"text": "",
	}
	if err := e.conn.WriteJSON(eos); err != nil {
		return fmt.Errorf("send EOS: %w", err)
	}

	return nil
}

// readLoop reads audio chunks from the WebSocket.
func (e *ElevenLabsWS) readLoop() {
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.closeCh:
			return
		default:
		}

		e.connMu.Lock()
		conn := e.conn
		e.connMu.Unlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				e.logger.Error("websocket read error", "error", err)
			}
			e.handleDisconnect()
			continue
		}

		// Parse response
		var resp struct {
			Audio              string `json:"audio"`
			IsFinal            bool   `json:"isFinal"`
			NormalizedAlignment interface{} `json:"normalizedAlignment"`
		}
		if err := json.Unmarshal(message, &resp); err != nil {
			e.logger.Warn("failed to parse response", "error", err)
			continue
		}

		// Decode and deliver audio
		if resp.Audio != "" && e.OnAudio != nil {
			audioData, err := base64.StdEncoding.DecodeString(resp.Audio)
			if err != nil {
				e.logger.Warn("failed to decode audio", "error", err)
				continue
			}
			e.OnAudio(audioData)
		}
	}
}

// writeLoop sends text chunks from the channel.
func (e *ElevenLabsWS) writeLoop() {
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.closeCh:
			return
		case text := <-e.sendCh:
			e.connMu.Lock()
			conn := e.conn
			connected := e.connected
			e.connMu.Unlock()

			if !connected || conn == nil {
				continue
			}

			msg := map[string]interface{}{
				"text": text,
			}
			if err := conn.WriteJSON(msg); err != nil {
				e.logger.Error("failed to send text", "error", err)
				e.handleDisconnect()
			}
		}
	}
}

// keepaliveLoop sends periodic pings to maintain connection.
func (e *ElevenLabsWS) keepaliveLoop() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.closeCh:
			return
		case <-ticker.C:
			e.connMu.Lock()
			conn := e.conn
			connected := e.connected
			e.connMu.Unlock()

			if !connected || conn == nil {
				continue
			}

			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				e.logger.Warn("keepalive ping failed", "error", err)
				e.handleDisconnect()
			}
		}
	}
}

// handleDisconnect handles connection loss and triggers reconnection.
func (e *ElevenLabsWS) handleDisconnect() {
	e.connMu.Lock()
	if e.conn != nil {
		e.conn.Close()
		e.conn = nil
	}
	e.connected = false
	wasReconnecting := e.reconnecting
	e.reconnecting = true
	e.connMu.Unlock()

	if e.OnDisconnect != nil {
		e.OnDisconnect()
	}

	// Only start one reconnection goroutine
	if !wasReconnecting {
		go e.reconnectLoop()
	}
}

// reconnectLoop attempts to reconnect with exponential backoff.
func (e *ElevenLabsWS) reconnectLoop() {
	delay := reconnectBaseDelay

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.closeCh:
			return
		default:
		}

		e.logger.Info("attempting to reconnect", "delay", delay)
		time.Sleep(delay)

		if err := e.dial(); err != nil {
			e.logger.Error("reconnect failed", "error", err)
			// Exponential backoff
			delay *= 2
			if delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
			continue
		}

		// Success
		e.connMu.Lock()
		e.reconnecting = false
		e.connMu.Unlock()
		e.logger.Info("reconnected successfully")
		return
	}
}

// IsConnected returns true if the WebSocket is connected.
func (e *ElevenLabsWS) IsConnected() bool {
	e.connMu.Lock()
	defer e.connMu.Unlock()
	return e.connected
}

// Close terminates the WebSocket connection and cleans up resources.
func (e *ElevenLabsWS) Close() error {
	if e.cancel != nil {
		e.cancel()
	}

	close(e.closeCh)

	e.connMu.Lock()
	defer e.connMu.Unlock()

	if e.conn != nil {
		// Send close message
		e.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		e.conn.Close()
		e.conn = nil
	}
	e.connected = false

	return nil
}

// encodingToAPIFormat converts encoding to ElevenLabs API output_format parameter.
func (e *ElevenLabsWS) encodingToAPIFormat() string {
	switch e.config.OutputFormat {
	case EncodingPCM16:
		return "pcm_16000"
	case EncodingPCM22:
		return "pcm_22050"
	case EncodingPCM24:
		return "pcm_24000"
	case EncodingPCM44:
		return "pcm_44100"
	case EncodingULaw:
		return "ulaw_8000"
	default:
		return "pcm_24000"
	}
}

// VoiceID returns the configured voice ID.
func (e *ElevenLabsWS) VoiceID() string {
	return e.config.VoiceID
}

// ModelID returns the configured model ID.
func (e *ElevenLabsWS) ModelID() string {
	return e.config.ModelID
}

