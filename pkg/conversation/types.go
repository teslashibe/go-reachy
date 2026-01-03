package conversation

import (
	"context"
	"time"
)

// ConnectionState represents the WebSocket connection state.
type ConnectionState int

const (
	// StateDisconnected indicates no active connection.
	StateDisconnected ConnectionState = iota
	// StateConnecting indicates connection is being established.
	StateConnecting
	// StateConnected indicates an active connection.
	StateConnected
	// StateReconnecting indicates reconnection is in progress.
	StateReconnecting
)

// String returns a human-readable connection state.
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// Tool represents an AI tool/function that can be called by the agent.
type Tool struct {
	// Name is the unique identifier for the tool.
	Name string

	// Description explains what the tool does (shown to the AI).
	Description string

	// Parameters defines the JSON Schema for tool arguments.
	// Use map[string]any for flexibility with different parameter types.
	Parameters map[string]any

	// Handler is the function called when the AI invokes this tool.
	// It receives the parsed arguments and returns a result string or error.
	Handler func(args map[string]any) (string, error)
}

// TurnDetection configures voice activity detection (VAD) for turn-taking.
type TurnDetection struct {
	// Type specifies the VAD mode: "server_vad", "client_vad", "none".
	Type string

	// Threshold is the VAD sensitivity (0.0-1.0, higher = less sensitive).
	Threshold float64

	// PrefixPaddingMs is the audio to include before speech detection (ms).
	PrefixPaddingMs int

	// SilenceDurationMs is how long silence indicates end of turn (ms).
	SilenceDurationMs int
}

// SessionOptions configures a conversation session.
type SessionOptions struct {
	// Tools available to the agent during this session.
	Tools []Tool

	// SystemPrompt is the system instruction for the agent.
	SystemPrompt string

	// Voice is the TTS voice to use.
	Voice string

	// Temperature controls response randomness (0.0-1.0).
	Temperature float64

	// MaxTokens limits response length (deprecated, use MaxResponseTokens).
	MaxTokens int

	// MaxResponseTokens limits response length.
	MaxResponseTokens int

	// TurnDetection configures VAD settings.
	TurnDetection *TurnDetection
}

// DefaultSessionOptions returns SessionOptions with sensible defaults.
func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		Temperature:       0.8,
		MaxResponseTokens: 4096,
		TurnDetection: &TurnDetection{
			Type:              "server_vad",
			Threshold:         0.5,
			PrefixPaddingMs:   300,
			SilenceDurationMs: 500,
		},
	}
}

// Capabilities describes what features a provider supports.
type Capabilities struct {
	// SupportsToolCalls indicates the provider can call tools.
	SupportsToolCalls bool

	// SupportsInterruption indicates audio can be interrupted mid-stream.
	SupportsInterruption bool

	// SupportsCustomVoice indicates custom voice selection is available.
	SupportsCustomVoice bool

	// SupportsStreaming indicates audio is streamed (not batched).
	SupportsStreaming bool

	// InputSampleRate is the expected audio input sample rate (Hz).
	InputSampleRate int

	// OutputSampleRate is the audio output sample rate (Hz).
	OutputSampleRate int

	// SupportedModels lists available LLM models.
	SupportedModels []string
}

// Metrics tracks connection and usage statistics.
type Metrics struct {
	// ConnectionTime is when the connection was established.
	ConnectionTime time.Time

	// MessagesSent is the total messages sent.
	MessagesSent int64

	// MessagesReceived is the total messages received.
	MessagesReceived int64

	// AudioBytesSent is the total audio bytes sent.
	AudioBytesSent int64

	// AudioBytesReceived is the total audio bytes received.
	AudioBytesReceived int64

	// ToolCallsExecuted is the total tool calls processed.
	ToolCallsExecuted int64

	// Errors is the total errors encountered.
	Errors int64
}

// Provider is the interface for conversation providers (OpenAI, ElevenLabs, etc.).
// Implementations handle the WebSocket connection and message processing.
type Provider interface {
	// Connect establishes the WebSocket connection.
	Connect(ctx context.Context) error

	// Close gracefully closes the connection.
	Close() error

	// IsConnected returns true if connected.
	IsConnected() bool

	// SendAudio sends audio data to the conversation.
	SendAudio(audio []byte) error

	// ConfigureSession sets up the conversation session.
	ConfigureSession(opts SessionOptions) error

	// RegisterTool adds a tool to the session.
	RegisterTool(tool Tool)

	// CancelResponse interrupts the current response.
	CancelResponse() error

	// SubmitToolResult returns a tool call result to the provider.
	SubmitToolResult(callID, result string) error

	// Capabilities returns provider capabilities.
	Capabilities() Capabilities

	// Callbacks

	// OnAudio is called when audio is received.
	OnAudio(fn func(audio []byte))

	// OnAudioDone is called when audio streaming completes.
	OnAudioDone(fn func())

	// OnTranscript is called when a transcript is received.
	OnTranscript(fn func(role, text string, isFinal bool))

	// OnToolCall is called when the agent invokes a tool.
	OnToolCall(fn func(id, name string, args map[string]any))

	// OnError is called when an error occurs.
	OnError(fn func(err error))

	// OnInterruption is called when the user interrupts.
	OnInterruption(fn func())
}

