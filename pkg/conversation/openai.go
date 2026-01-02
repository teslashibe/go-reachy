package conversation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	openAIRealtimeURL = "wss://api.openai.com/v1/realtime"
	openAIModel       = "gpt-4o-realtime-preview-2024-12-17"
)

// OpenAI implements Provider for the OpenAI Realtime API.
type OpenAI struct {
	config *Config
	logger *slog.Logger

	mu           sync.RWMutex
	conn         *websocket.Conn
	state        ConnectionState
	sessionReady bool
	tools        []Tool
	toolsMap     map[string]Tool
	cancelCtx    context.CancelFunc

	// Callbacks
	onAudio        func(audio []byte)
	onAudioDone    func()
	onTranscript   func(role, text string, isFinal bool)
	onToolCall     func(id, name string, args map[string]any)
	onError        func(err error)
	onInterruption func()

	// Metrics
	messagesSent     atomic.Int64
	messagesReceived atomic.Int64
}

// NewOpenAI creates a new OpenAI Realtime conversation provider.
func NewOpenAI(opts ...Option) (*OpenAI, error) {
	cfg := DefaultConfig()
	cfg.Model = openAIModel
	cfg.InputSampleRate = 24000 // OpenAI uses 24kHz
	cfg.OutputSampleRate = 24000
	cfg.Apply(opts...)

	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Voice == "" {
		cfg.Voice = VoiceShimmer
	}

	return &OpenAI{
		config:   cfg,
		logger:   cfg.Logger.With("component", "conversation.openai"),
		state:    StateDisconnected,
		toolsMap: make(map[string]Tool),
	}, nil
}

// Connect establishes the WebSocket connection to OpenAI.
func (o *OpenAI) Connect(ctx context.Context) error {
	o.mu.Lock()
	if o.state == StateConnected {
		o.mu.Unlock()
		return ErrAlreadyConnected
	}
	o.state = StateConnecting
	o.mu.Unlock()

	url := fmt.Sprintf("%s?model=%s", openAIRealtimeURL, o.config.Model)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+o.config.APIKey)
	headers.Set("OpenAI-Beta", "realtime=v1")

	dialer := websocket.Dialer{
		HandshakeTimeout: o.config.Timeout,
	}

	o.logger.Info("connecting to OpenAI Realtime API",
		"model", o.config.Model,
	)

	conn, resp, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		o.mu.Lock()
		o.state = StateDisconnected
		o.mu.Unlock()
		if resp != nil {
			return NewConnectionError(
				fmt.Sprintf("dial failed with status %d", resp.StatusCode),
				err,
				resp.StatusCode >= 500,
			)
		}
		return NewConnectionError("dial failed", err, true)
	}

	// Create cancellation context
	msgCtx, cancel := context.WithCancel(context.Background())

	o.mu.Lock()
	o.conn = conn
	o.state = StateConnected
	o.cancelCtx = cancel
	o.mu.Unlock()

	// Start message handler
	go o.handleMessages(msgCtx)

	o.logger.Info("connected to OpenAI Realtime API")

	return nil
}

// Close gracefully closes the connection.
func (o *OpenAI) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == StateDisconnected {
		return nil
	}

	if o.cancelCtx != nil {
		o.cancelCtx()
	}

	if o.conn != nil {
		deadline := time.Now().Add(time.Second)
		_ = o.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			deadline,
		)
		o.conn.Close()
		o.conn = nil
	}

	o.state = StateDisconnected
	o.sessionReady = false
	o.logger.Info("disconnected from OpenAI Realtime API")

	return nil
}

// IsConnected returns true if connected.
func (o *OpenAI) IsConnected() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.state == StateConnected
}

// SendAudio sends audio to the conversation.
func (o *OpenAI) SendAudio(audio []byte) error {
	o.mu.RLock()
	conn := o.conn
	state := o.state
	o.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	encoded := base64.StdEncoding.EncodeToString(audio)

	msg := map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": encoded,
	}

	o.mu.Lock()
	err := conn.WriteJSON(msg)
	o.mu.Unlock()

	if err != nil {
		return NewConnectionError("send audio failed", err, true)
	}

	o.messagesSent.Add(1)
	return nil
}

// OnAudio sets the audio callback.
func (o *OpenAI) OnAudio(fn func(audio []byte)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onAudio = fn
}

// OnAudioDone sets the audio done callback.
func (o *OpenAI) OnAudioDone(fn func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onAudioDone = fn
}

// OnTranscript sets the transcript callback.
func (o *OpenAI) OnTranscript(fn func(role, text string, isFinal bool)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onTranscript = fn
}

// OnToolCall sets the tool call callback.
func (o *OpenAI) OnToolCall(fn func(id, name string, args map[string]any)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onToolCall = fn
}

// OnError sets the error callback.
func (o *OpenAI) OnError(fn func(err error)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onError = fn
}

// OnInterruption sets the interruption callback.
func (o *OpenAI) OnInterruption(fn func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onInterruption = fn
}

// ConfigureSession configures the conversation session.
func (o *OpenAI) ConfigureSession(opts SessionOptions) error {
	o.mu.RLock()
	conn := o.conn
	state := o.state
	o.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	voice := opts.Voice
	if voice == "" {
		voice = o.config.Voice
	}

	// Build tools array
	apiTools := make([]map[string]any, len(opts.Tools))
	for i, tool := range opts.Tools {
		apiTools[i] = map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": tool.Parameters,
				"required":   []string{},
			},
		}
	}

	// Add registered tools
	o.mu.RLock()
	for _, tool := range o.tools {
		apiTools = append(apiTools, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": tool.Parameters,
				"required":   []string{},
			},
		})
	}
	o.mu.RUnlock()

	turnDetection := map[string]any{
		"type":                "server_vad",
		"threshold":           0.5,
		"prefix_padding_ms":   300,
		"silence_duration_ms": 500,
	}
	if opts.TurnDetection != nil {
		turnDetection["type"] = opts.TurnDetection.Type
		if opts.TurnDetection.Threshold > 0 {
			turnDetection["threshold"] = opts.TurnDetection.Threshold
		}
		if opts.TurnDetection.PrefixPaddingMs > 0 {
			turnDetection["prefix_padding_ms"] = opts.TurnDetection.PrefixPaddingMs
		}
		if opts.TurnDetection.SilenceDurationMs > 0 {
			turnDetection["silence_duration_ms"] = opts.TurnDetection.SilenceDurationMs
		}
	}

	msg := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":          []string{"text", "audio"},
			"instructions":        opts.SystemPrompt,
			"voice":               voice,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"input_audio_transcription": map[string]any{
				"model": "whisper-1",
			},
			"turn_detection": turnDetection,
			"tools":          apiTools,
			"tool_choice":    "auto",
		},
	}

	o.mu.Lock()
	err := conn.WriteJSON(msg)
	o.mu.Unlock()

	if err != nil {
		return NewConnectionError("configure session failed", err, true)
	}

	o.messagesSent.Add(1)
	return nil
}

// RegisterTool registers a tool.
func (o *OpenAI) RegisterTool(tool Tool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tools = append(o.tools, tool)
	o.toolsMap[tool.Name] = tool
}

// CancelResponse cancels the current response.
func (o *OpenAI) CancelResponse() error {
	o.mu.RLock()
	conn := o.conn
	state := o.state
	o.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	msg := map[string]string{"type": "response.cancel"}

	o.mu.Lock()
	err := conn.WriteJSON(msg)
	o.mu.Unlock()

	if err != nil {
		return NewConnectionError("cancel response failed", err, true)
	}

	o.messagesSent.Add(1)
	return nil
}

// SubmitToolResult submits the result of a tool call.
func (o *OpenAI) SubmitToolResult(callID, result string) error {
	o.mu.RLock()
	conn := o.conn
	state := o.state
	o.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	// Send tool result
	resultMsg := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  result,
		},
	}

	o.mu.Lock()
	err := conn.WriteJSON(resultMsg)
	o.mu.Unlock()

	if err != nil {
		return NewConnectionError("submit tool result failed", err, true)
	}

	// Request continuation
	continueMsg := map[string]string{"type": "response.create"}

	o.mu.Lock()
	err = conn.WriteJSON(continueMsg)
	o.mu.Unlock()

	if err != nil {
		return NewConnectionError("continue after tool result failed", err, true)
	}

	o.messagesSent.Add(2)
	o.logger.Debug("submitted tool result",
		"call_id", callID,
		"result_len", len(result),
	)

	return nil
}

// Capabilities returns provider capabilities.
func (o *OpenAI) Capabilities() Capabilities {
	return Capabilities{
		SupportsToolCalls:    true,
		SupportsInterruption: true,
		SupportsCustomVoice:  false, // OpenAI only has fixed voices
		SupportsStreaming:    true,
		InputSampleRate:      24000,
		OutputSampleRate:     24000,
		SupportedModels:      []string{"gpt-4o-realtime-preview"},
	}
}

// handleMessages processes incoming WebSocket messages.
func (o *OpenAI) handleMessages(ctx context.Context) {
	defer func() {
		o.mu.Lock()
		if o.state == StateConnected {
			o.state = StateDisconnected
		}
		o.sessionReady = false
		o.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		o.mu.RLock()
		conn := o.conn
		o.mu.RUnlock()

		if conn == nil {
			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(o.config.ReadTimeout))

		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				o.logger.Info("connection closed normally")
				return
			}
			o.logger.Error("read error", "error", err)
			o.emitError(NewConnectionError("read failed", err, true))
			return
		}

		o.messagesReceived.Add(1)

		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			o.logger.Warn("failed to parse message", "error", err)
			continue
		}

		o.handleMessage(msg)
	}
}

// handleMessage processes a single message.
func (o *OpenAI) handleMessage(msg map[string]any) {
	msgType, _ := msg["type"].(string)

	switch msgType {
	case "session.created":
		o.mu.Lock()
		o.sessionReady = true
		o.mu.Unlock()
		o.logger.Info("session created")

	case "session.updated":
		o.logger.Debug("session updated")

	case "input_audio_buffer.speech_started":
		o.logger.Debug("speech started")
		o.emitInterruption()

	case "input_audio_buffer.speech_stopped":
		o.logger.Debug("speech stopped")

	case "conversation.item.input_audio_transcription.completed":
		if transcript, ok := msg["transcript"].(string); ok {
			o.emitTranscript("user", transcript, true)
		}

	case "response.audio.delta":
		if delta, ok := msg["delta"].(string); ok {
			audio, err := base64.StdEncoding.DecodeString(delta)
			if err == nil {
				o.emitAudio(audio)
			}
		}

	case "response.audio.done":
		o.emitAudioDone()

	case "response.audio_transcript.delta":
		if delta, ok := msg["delta"].(string); ok {
			o.emitTranscript("agent", delta, false)
		}

	case "response.function_call_arguments.done":
		o.handleFunctionCall(msg)

	case "error":
		if errData, ok := msg["error"].(map[string]any); ok {
			errMsg, _ := errData["message"].(string)
			errCode, _ := errData["code"].(string)
			o.emitError(NewAPIError(0, errCode, errMsg))
		}

	default:
		// Ignore other message types
	}
}

// handleFunctionCall processes a function call from the API.
func (o *OpenAI) handleFunctionCall(msg map[string]any) {
	name, _ := msg["name"].(string)
	callID, _ := msg["call_id"].(string)
	argsStr, _ := msg["arguments"].(string)

	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		args = make(map[string]any)
	}

	o.logger.Info("tool call received",
		"name", name,
		"call_id", callID,
	)

	o.emitToolCall(callID, name, args)
}

// Emit helpers

func (o *OpenAI) emitAudio(audio []byte) {
	o.mu.RLock()
	fn := o.onAudio
	o.mu.RUnlock()
	if fn != nil {
		fn(audio)
	}
}

func (o *OpenAI) emitAudioDone() {
	o.mu.RLock()
	fn := o.onAudioDone
	o.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (o *OpenAI) emitTranscript(role, text string, isFinal bool) {
	o.mu.RLock()
	fn := o.onTranscript
	o.mu.RUnlock()
	if fn != nil {
		fn(role, text, isFinal)
	}
}

func (o *OpenAI) emitToolCall(id, name string, args map[string]any) {
	o.mu.RLock()
	fn := o.onToolCall
	o.mu.RUnlock()
	if fn != nil {
		fn(id, name, args)
	}
}

func (o *OpenAI) emitInterruption() {
	o.mu.RLock()
	fn := o.onInterruption
	o.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (o *OpenAI) emitError(err error) {
	o.mu.RLock()
	fn := o.onError
	o.mu.RUnlock()
	if fn != nil {
		fn(err)
	}
}

// Ensure OpenAI implements Provider.
var _ Provider = (*OpenAI)(nil)

