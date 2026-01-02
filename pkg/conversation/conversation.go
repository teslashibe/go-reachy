// Package conversation provides a unified interface for real-time voice conversation
// with AI agents. It supports multiple backends including OpenAI Realtime API and
// ElevenLabs Agents Platform.
//
// The package abstracts the complexity of WebSocket-based audio streaming, speech
// recognition, language model processing, and text-to-speech synthesis into a
// simple Provider interface.
//
// Example usage:
//
//	provider, err := conversation.NewElevenLabs(
//	    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
//	    conversation.WithAgentID(os.Getenv("ELEVENLABS_AGENT_ID")),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.Close()
//
//	provider.OnAudio(func(audio []byte) {
//	    // Play audio to speaker
//	})
//
//	provider.OnToolCall(func(id, name string, args map[string]any) {
//	    // Handle tool call
//	    result := executeToolI(name, args)
//	    provider.SubmitToolResult(id, result)
//	})
//
//	if err := provider.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Stream audio from microphone
//	for audio := range microphoneStream {
//	    provider.SendAudio(audio)
//	}
package conversation

import (
	"context"
	"time"
)

// Provider defines the interface for real-time voice conversation providers.
// Implementations handle the full conversation loop: STT → LLM → TTS.
type Provider interface {
	// Connect establishes the WebSocket connection to the conversation service.
	// Call this after setting up event handlers.
	Connect(ctx context.Context) error

	// Close gracefully shuts down the connection and releases resources.
	Close() error

	// IsConnected returns true if the provider has an active connection.
	IsConnected() bool

	// SendAudio streams audio data to the conversation service.
	// Audio should be PCM16 mono at the provider's expected sample rate.
	SendAudio(audio []byte) error

	// OnAudio sets the callback for receiving audio from the agent.
	// Audio is PCM16 mono at the provider's output sample rate.
	OnAudio(fn func(audio []byte))

	// OnAudioDone sets the callback for when the agent finishes speaking.
	OnAudioDone(fn func())

	// OnTranscript sets the callback for transcript events.
	// role is "user" or "agent", isFinal indicates if transcription is complete.
	OnTranscript(fn func(role, text string, isFinal bool))

	// OnToolCall sets the callback for tool/function calls from the agent.
	// Use SubmitToolResult to return the result.
	OnToolCall(fn func(id, name string, args map[string]any))

	// OnError sets the callback for error events.
	OnError(fn func(err error))

	// OnInterruption sets the callback for when the user interrupts the agent.
	OnInterruption(fn func())

	// ConfigureSession configures the conversation session.
	// Call this after Connect but before sending audio.
	ConfigureSession(opts SessionOptions) error

	// RegisterTool registers a tool that the agent can call.
	// Call this before Connect or use ConfigureSession.
	RegisterTool(tool Tool)

	// CancelResponse interrupts the current agent response.
	CancelResponse() error

	// SubmitToolResult returns the result of a tool call to the agent.
	SubmitToolResult(callID, result string) error

	// Capabilities returns what this provider supports.
	Capabilities() Capabilities
}

// SessionOptions configures a conversation session.
type SessionOptions struct {
	// SystemPrompt is the system instruction for the agent.
	SystemPrompt string

	// Voice is the voice ID or name to use for TTS.
	Voice string

	// Language is the language code (e.g., "en", "es").
	Language string

	// Temperature controls randomness in responses (0.0-1.0).
	Temperature float64

	// MaxResponseTokens limits the response length.
	MaxResponseTokens int

	// TurnDetection configures voice activity detection.
	TurnDetection *TurnDetection

	// Tools is the list of tools available to the agent.
	Tools []Tool
}

// TurnDetection configures voice activity detection for turn-taking.
type TurnDetection struct {
	// Type is the detection type: "server_vad" or "none".
	Type string

	// Threshold is the VAD threshold (0.0-1.0).
	Threshold float64

	// PrefixPaddingMs is silence before speech starts.
	PrefixPaddingMs int

	// SilenceDurationMs is silence duration to end turn.
	SilenceDurationMs int
}

// Tool defines a function that the agent can call.
type Tool struct {
	// Name is the function name.
	Name string `json:"name"`

	// Description explains what the tool does.
	Description string `json:"description"`

	// Parameters is the JSON Schema for the function parameters.
	Parameters map[string]any `json:"parameters"`
}

// Capabilities describes what a provider supports.
type Capabilities struct {
	// SupportsToolCalls indicates if the provider supports function calling.
	SupportsToolCalls bool

	// SupportsInterruption indicates if the provider handles user interruptions.
	SupportsInterruption bool

	// SupportsCustomVoice indicates if custom/cloned voices are available.
	SupportsCustomVoice bool

	// SupportsStreaming indicates if audio is streamed (vs batch).
	SupportsStreaming bool

	// InputSampleRate is the expected audio input sample rate in Hz.
	InputSampleRate int

	// OutputSampleRate is the audio output sample rate in Hz.
	OutputSampleRate int

	// SupportedModels lists available LLM models.
	SupportedModels []string
}

// TranscriptRole identifies who is speaking.
type TranscriptRole string

const (
	RoleUser  TranscriptRole = "user"
	RoleAgent TranscriptRole = "agent"
)

// Event types for internal use.
type EventType string

const (
	EventAudio        EventType = "audio"
	EventAudioDone    EventType = "audio_done"
	EventTranscript   EventType = "transcript"
	EventToolCall     EventType = "tool_call"
	EventError        EventType = "error"
	EventInterruption EventType = "interruption"
	EventConnected    EventType = "connected"
	EventDisconnected EventType = "disconnected"
)

// DefaultSessionOptions returns sensible defaults for a conversation session.
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

// ConnectionState represents the state of the provider connection.
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateReconnecting
)

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

// Metrics tracks conversation statistics.
type Metrics struct {
	// ConnectionTime is when the connection was established.
	ConnectionTime time.Time

	// MessagesSent is the count of messages sent.
	MessagesSent int64

	// MessagesReceived is the count of messages received.
	MessagesReceived int64

	// AudioBytesSent is the total audio bytes sent.
	AudioBytesSent int64

	// AudioBytesReceived is the total audio bytes received.
	AudioBytesReceived int64

	// ToolCallsReceived is the count of tool calls from the agent.
	ToolCallsReceived int64

	// Errors is the count of errors encountered.
	Errors int64
}
