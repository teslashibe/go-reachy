package conversation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	elevenLabsBaseURL = "wss://api.elevenlabs.io/v1/convai/conversation"
)

// ElevenLabs implements Provider for the ElevenLabs Agents Platform.
type ElevenLabs struct {
	config    *Config
	logger    *slog.Logger
	apiClient *apiClient

	mu        sync.RWMutex
	conn      *websocket.Conn
	state     ConnectionState
	tools     []Tool
	metrics   Metrics
	cancelCtx context.CancelFunc

	// agentID is the resolved agent ID (either from config or auto-created)
	agentID string

	// Callbacks
	onAudio        func(audio []byte)
	onAudioDone    func()
	onTranscript   func(role, text string, isFinal bool)
	onToolCall     func(id, name string, args map[string]any)
	onError        func(err error)
	onInterruption func()

	// Atomic counters for metrics
	messagesSent     atomic.Int64
	messagesReceived atomic.Int64
}

// NewElevenLabs creates a new ElevenLabs conversation provider.
//
// There are two modes of operation:
//
// 1. Dashboard-configured agent (legacy):
//
//	provider, _ := NewElevenLabs(
//	    WithAPIKey(apiKey),
//	    WithAgentID(agentID),  // From ElevenLabs dashboard
//	)
//
// 2. Programmatic agent (recommended):
//
//	provider, _ := NewElevenLabs(
//	    WithAPIKey(apiKey),
//	    WithVoiceID(voiceID),
//	    WithLLM("gemini-2.0-flash"),
//	    WithSystemPrompt(instructions),
//	    WithAutoCreateAgent(true),
//	)
func NewElevenLabs(opts ...Option) (*ElevenLabs, error) {
	cfg := DefaultConfig()
	cfg.Apply(opts...)

	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	// Validate configuration based on mode
	if cfg.AgentID == "" && !cfg.AutoCreateAgent {
		return nil, ErrMissingAgentID
	}

	if cfg.AutoCreateAgent && cfg.VoiceID == "" {
		return nil, ErrMissingVoiceID
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Set default agent name if not provided
	if cfg.AgentName == "" {
		cfg.AgentName = "eva-agent"
	}

	// Set default LLM if not provided
	if cfg.LLM == "" {
		cfg.LLM = "gemini-2.0-flash"
	}

	return &ElevenLabs{
		config:    cfg,
		logger:    cfg.Logger.With("component", "conversation.elevenlabs"),
		apiClient: newAPIClient(cfg.APIKey),
		agentID:   cfg.AgentID, // May be empty if auto-create
		state:     StateDisconnected,
	}, nil
}

// Connect establishes the WebSocket connection to ElevenLabs.
// If AutoCreateAgent is enabled and no AgentID is set, an agent will be created first.
func (e *ElevenLabs) Connect(ctx context.Context) error {
	e.mu.Lock()
	if e.state == StateConnected {
		e.mu.Unlock()
		return ErrAlreadyConnected
	}
	e.state = StateConnecting
	e.mu.Unlock()

	// Auto-create agent if needed
	if e.agentID == "" && e.config.AutoCreateAgent {
		e.logger.Info("creating agent programmatically",
			"voice_id", e.config.VoiceID,
			"llm", e.config.LLM,
		)

		agentCfg := buildAgentConfig(e.config, e.tools)
		resp, err := e.apiClient.CreateAgent(ctx, agentCfg)
		if err != nil {
			e.mu.Lock()
			e.state = StateDisconnected
			e.mu.Unlock()
			return fmt.Errorf("conversation.elevenlabs: create agent failed: %w", err)
		}

		e.agentID = resp.AgentID
		e.logger.Info("agent created successfully",
			"agent_id", e.agentID,
		)
	}

	// Ensure we have an agent ID at this point
	if e.agentID == "" {
		e.mu.Lock()
		e.state = StateDisconnected
		e.mu.Unlock()
		return ErrMissingAgentID
	}

	// Build WebSocket URL
	wsURL, err := url.Parse(elevenLabsBaseURL)
	if err != nil {
		return fmt.Errorf("conversation.elevenlabs: invalid URL: %w", err)
	}

	q := wsURL.Query()
	q.Set("agent_id", e.agentID)
	wsURL.RawQuery = q.Encode()

	// Set up headers
	headers := http.Header{}
	headers.Set("xi-api-key", e.config.APIKey)

	// Connect with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: e.config.Timeout,
	}

	e.logger.Info("connecting to ElevenLabs Agents Platform",
		"agent_id", e.agentID,
	)

	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), headers)
	if err != nil {
		e.mu.Lock()
		e.state = StateDisconnected
		e.mu.Unlock()
		if resp != nil {
			return NewConnectionError(
				fmt.Sprintf("dial failed with status %d", resp.StatusCode),
				err,
				resp.StatusCode >= 500,
			)
		}
		return NewConnectionError("dial failed", err, true)
	}

	// Create cancellation context for message handler
	msgCtx, cancel := context.WithCancel(context.Background())

	e.mu.Lock()
	e.conn = conn
	e.state = StateConnected
	e.cancelCtx = cancel
	e.metrics.ConnectionTime = time.Now()
	e.mu.Unlock()

	// Start message handler
	go e.handleMessages(msgCtx)

	e.logger.Info("connected to ElevenLabs Agents Platform")

	return nil
}

// AgentID returns the current agent ID (may be auto-created).
func (e *ElevenLabs) AgentID() string {
	return e.agentID
}

// Close gracefully closes the connection.
func (e *ElevenLabs) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateDisconnected {
		return nil
	}

	// Cancel message handler
	if e.cancelCtx != nil {
		e.cancelCtx()
	}

	// Close WebSocket
	if e.conn != nil {
		// Send close message
		deadline := time.Now().Add(time.Second)
		_ = e.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			deadline,
		)
		e.conn.Close()
		e.conn = nil
	}

	e.state = StateDisconnected
	e.logger.Info("disconnected from ElevenLabs Agents Platform")

	return nil
}

// IsConnected returns true if connected.
func (e *ElevenLabs) IsConnected() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state == StateConnected
}

// SendAudio sends audio to the conversation.
func (e *ElevenLabs) SendAudio(audio []byte) error {
	e.mu.RLock()
	conn := e.conn
	state := e.state
	e.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	// Encode audio as base64 - ElevenLabs expects "user_audio_chunk" format
	encoded := base64.StdEncoding.EncodeToString(audio)

	// ElevenLabs uses a flat format, not type-based
	msg := map[string]string{
		"user_audio_chunk": encoded,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("conversation.elevenlabs: marshal failed: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return NewConnectionError("send audio failed", err, true)
	}

	e.messagesSent.Add(1)
	return nil
}

// OnAudio sets the audio callback.
func (e *ElevenLabs) OnAudio(fn func(audio []byte)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onAudio = fn
}

// OnAudioDone sets the audio done callback.
func (e *ElevenLabs) OnAudioDone(fn func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onAudioDone = fn
}

// OnTranscript sets the transcript callback.
func (e *ElevenLabs) OnTranscript(fn func(role, text string, isFinal bool)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onTranscript = fn
}

// OnToolCall sets the tool call callback.
func (e *ElevenLabs) OnToolCall(fn func(id, name string, args map[string]any)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onToolCall = fn
}

// OnError sets the error callback.
func (e *ElevenLabs) OnError(fn func(err error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onError = fn
}

// OnInterruption sets the interruption callback.
func (e *ElevenLabs) OnInterruption(fn func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onInterruption = fn
}

// ConfigureSession configures the conversation session.
func (e *ElevenLabs) ConfigureSession(opts SessionOptions) error {
	// ElevenLabs configures most options in the dashboard
	// We can send a session update if needed
	e.mu.Lock()
	defer e.mu.Unlock()

	// Store tools for reference
	e.tools = opts.Tools

	return nil
}

// RegisterTool registers a tool.
func (e *ElevenLabs) RegisterTool(tool Tool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tools = append(e.tools, tool)
}

// CancelResponse cancels the current response.
func (e *ElevenLabs) CancelResponse() error {
	e.mu.RLock()
	conn := e.conn
	state := e.state
	e.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	msg := elevenLabsMessage{
		Type: "interrupt",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("conversation.elevenlabs: marshal failed: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return NewConnectionError("cancel failed", err, true)
	}

	e.messagesSent.Add(1)
	return nil
}

// SubmitToolResult submits the result of a tool call.
func (e *ElevenLabs) SubmitToolResult(callID, result string) error {
	e.mu.RLock()
	conn := e.conn
	state := e.state
	e.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return ErrNotConnected
	}

	// ElevenLabs uses "client_tool_result" format
	msg := map[string]interface{}{
		"type":         "client_tool_result",
		"tool_call_id": callID,
		"result":       result,
		"is_error":     false,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("conversation.elevenlabs: marshal failed: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return NewConnectionError("submit tool result failed", err, true)
	}

	e.messagesSent.Add(1)
	e.logger.Debug("submitted tool result",
		"call_id", callID,
		"result_len", len(result),
	)

	return nil
}

// Capabilities returns provider capabilities.
func (e *ElevenLabs) Capabilities() Capabilities {
	return Capabilities{
		SupportsToolCalls:    true,
		SupportsInterruption: true,
		SupportsCustomVoice:  true,
		SupportsStreaming:    true,
		InputSampleRate:      16000,
		OutputSampleRate:     16000,
		SupportedModels:      []string{"gpt-4o", "claude-3-5-sonnet", "gemini-2.0-flash"},
	}
}

// handleMessages processes incoming WebSocket messages.
func (e *ElevenLabs) handleMessages(ctx context.Context) {
	defer func() {
		e.mu.Lock()
		if e.state == StateConnected {
			e.state = StateDisconnected
		}
		e.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.mu.RLock()
		conn := e.conn
		e.mu.RUnlock()

		if conn == nil {
			return
		}

		// Set read deadline
		_ = conn.SetReadDeadline(time.Now().Add(e.config.ReadTimeout))

		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				e.logger.Info("connection closed normally")
				return
			}
			e.logger.Error("read error", "error", err)
			e.emitError(NewConnectionError("read failed", err, true))
			return
		}

		e.messagesReceived.Add(1)

		var msg elevenLabsIncoming
		if err := json.Unmarshal(data, &msg); err != nil {
			e.logger.Warn("failed to parse message", "error", err)
			continue
		}

		e.handleMessage(msg)
	}
}

// handleMessage processes a single message.
func (e *ElevenLabs) handleMessage(msg elevenLabsIncoming) {
	switch msg.Type {
	case "audio":
		// Handle both nested (audio_event) and flat (audio) formats
		var audioData string
		if msg.AudioEvent != nil && msg.AudioEvent.AudioBase64 != "" {
			audioData = msg.AudioEvent.AudioBase64
		} else if msg.Audio != "" {
			audioData = msg.Audio
		}
		if audioData != "" {
			audio, err := base64.StdEncoding.DecodeString(audioData)
			if err != nil {
				e.logger.Warn("failed to decode audio", "error", err)
				return
			}
			e.emitAudio(audio)
		}

	case "audio_done", "agent_response_done":
		e.emitAudioDone()

	case "user_transcript":
		e.emitTranscript("user", msg.Text, msg.IsFinal)

	case "agent_response":
		e.emitTranscript("agent", msg.Text, msg.IsFinal)

	case "tool_call", "client_tool_call":
		// Handle both nested and flat tool call formats
		toolName := msg.ToolName
		toolCallID := msg.ToolCallID
		params := msg.Parameters
		if msg.ClientToolCall != nil {
			toolName = msg.ClientToolCall.ToolName
			toolCallID = msg.ClientToolCall.ToolCallID
			params = msg.ClientToolCall.Parameters
		}
		e.emitToolCall(toolCallID, toolName, params)

	case "interruption":
		e.emitInterruption()

	case "error":
		e.emitError(NewAPIError(0, msg.Code, msg.Message))

	case "ping":
		// Respond to ping with pong including event_id
		eventID := 0
		if msg.PingEvent != nil {
			eventID = msg.PingEvent.EventID
		}
		e.sendPong(eventID)

	default:
		e.logger.Debug("unhandled message type", "type", msg.Type)
	}
}

// sendPong responds to a ping message with the event_id.
func (e *ElevenLabs) sendPong(eventID int) {
	e.mu.RLock()
	conn := e.conn
	e.mu.RUnlock()

	if conn == nil {
		return
	}

	msg := map[string]interface{}{
		"type":     "pong",
		"event_id": eventID,
	}
	data, _ := json.Marshal(msg)
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

// Emit helpers

func (e *ElevenLabs) emitAudio(audio []byte) {
	e.mu.RLock()
	fn := e.onAudio
	e.mu.RUnlock()
	if fn != nil {
		fn(audio)
	}
}

func (e *ElevenLabs) emitAudioDone() {
	e.mu.RLock()
	fn := e.onAudioDone
	e.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (e *ElevenLabs) emitTranscript(role, text string, isFinal bool) {
	e.mu.RLock()
	fn := e.onTranscript
	e.mu.RUnlock()
	if fn != nil {
		fn(role, text, isFinal)
	}
}

func (e *ElevenLabs) emitToolCall(id, name string, args map[string]any) {
	e.mu.RLock()
	fn := e.onToolCall
	e.mu.RUnlock()
	if fn != nil {
		fn(id, name, args)
	}
}

func (e *ElevenLabs) emitInterruption() {
	e.mu.RLock()
	fn := e.onInterruption
	e.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (e *ElevenLabs) emitError(err error) {
	e.mu.RLock()
	fn := e.onError
	e.mu.RUnlock()
	if fn != nil {
		fn(err)
	}
}

// Message types for ElevenLabs API

type elevenLabsMessage struct {
	Type       string `json:"type"`
	Audio      string `json:"audio,omitempty"`
	Text       string `json:"text,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Result     string `json:"result,omitempty"`
}

type elevenLabsIncoming struct {
	Type       string         `json:"type"`
	Audio      string         `json:"audio,omitempty"`
	Text       string         `json:"text,omitempty"`
	IsFinal    bool           `json:"is_final,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Code       string         `json:"code,omitempty"`
	Message    string         `json:"message,omitempty"`

	// Nested event structures (ElevenLabs format)
	AudioEvent     *audioEvent     `json:"audio_event,omitempty"`
	PingEvent      *pingEvent      `json:"ping_event,omitempty"`
	ClientToolCall *clientToolCall `json:"client_tool_call,omitempty"`
}

type audioEvent struct {
	EventID     int    `json:"event_id"`
	AudioBase64 string `json:"audio_base_64"`
}

type pingEvent struct {
	EventID int `json:"event_id"`
	PingMs  int `json:"ping_ms,omitempty"`
}

type clientToolCall struct {
	ToolName   string         `json:"tool_name"`
	ToolCallID string         `json:"tool_call_id"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// Ensure ElevenLabs implements Provider.
var _ Provider = (*ElevenLabs)(nil)
