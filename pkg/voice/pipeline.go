package voice

import (
	"context"
	"errors"
)

// Common errors returned by pipelines.
var (
	ErrNotConnected   = errors.New("voice: pipeline not connected")
	ErrAlreadyStarted = errors.New("voice: pipeline already started")
	ErrMissingAPIKey  = errors.New("voice: missing API key")
)

// Pipeline is the interface for the ElevenLabs voice conversation pipeline.
type Pipeline interface {
	// Lifecycle

	// Start establishes the connection and begins processing.
	// Call this after registering tools and setting up callbacks.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the pipeline.
	Stop() error

	// IsConnected returns true if the pipeline is connected and ready.
	IsConnected() bool

	// Audio I/O

	// SendAudio sends PCM16 audio data to the pipeline.
	// Audio should be 16kHz mono PCM16.
	SendAudio(pcm16 []byte) error

	// OnAudioOut sets the callback for receiving audio output.
	// Audio is PCM16 at 16kHz.
	OnAudioOut(fn func(pcm16 []byte))

	// Events

	// OnSpeechStart is called when the user starts speaking.
	OnSpeechStart(fn func())

	// OnSpeechEnd is called when the user stops speaking.
	OnSpeechEnd(fn func())

	// OnTranscript is called with the user's transcribed speech.
	// isFinal indicates whether this is the final transcript.
	OnTranscript(fn func(text string, isFinal bool))

	// OnResponse is called with the AI's text response.
	// isFinal indicates whether this is the final response.
	OnResponse(fn func(text string, isFinal bool))

	// OnError is called when an error occurs.
	OnError(fn func(err error))

	// Tools

	// RegisterTool adds a tool that the AI can invoke.
	// Must be called before Start().
	RegisterTool(tool Tool)

	// OnToolCall sets the callback for tool invocations.
	// The callback receives the call ID, tool name, and parsed arguments.
	// Call SubmitToolResult with the call ID to return the result.
	OnToolCall(fn func(call ToolCall))

	// SubmitToolResult returns a tool call result to the AI.
	SubmitToolResult(callID string, result string) error

	// Control

	// Interrupt stops the current AI response (for barge-in).
	Interrupt() error

	// Metrics & Config

	// Metrics returns current latency metrics.
	Metrics() Metrics

	// Config returns the current configuration.
	Config() Config

	// UpdateConfig applies new configuration settings.
	// Some settings may require a reconnect to take effect.
	UpdateConfig(cfg Config) error

	// App-side timing markers (called by Eva, not the pipeline)

	// MarkCaptureStart records when WebRTC delivered audio to Eva.
	MarkCaptureStart()

	// MarkCaptureEnd records when audio is buffered and ready to send.
	MarkCaptureEnd()

	// MarkPlaybackStart records when audio was sent to GStreamer.
	MarkPlaybackStart()

	// MarkPlaybackEnd records when audio playback completed (estimated).
	MarkPlaybackEnd()
}

// PipelineFactory is a function that creates a Pipeline.
type PipelineFactory func(cfg Config) (Pipeline, error)

// factory holds the registered ElevenLabs pipeline factory.
var factory PipelineFactory

// Register sets the pipeline factory.
// This is called by the bundled ElevenLabs implementation in init().
func Register(f PipelineFactory) {
	factory = f
}

// New creates a new Pipeline with the given configuration.
// Returns an error if the config is invalid or no factory is registered.
func New(cfg Config) (Pipeline, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if factory == nil {
		return nil, errors.New("voice: no pipeline implementation registered")
	}

	return factory(cfg)
}

// Callbacks groups all pipeline callbacks for convenience.
// This can be used to set up all callbacks at once.
type Callbacks struct {
	OnAudioOut    func(pcm16 []byte)
	OnSpeechStart func()
	OnSpeechEnd   func()
	OnTranscript  func(text string, isFinal bool)
	OnResponse    func(text string, isFinal bool)
	OnToolCall    func(call ToolCall)
	OnError       func(err error)
}

// Apply sets all callbacks on a pipeline.
func (c *Callbacks) Apply(p Pipeline) {
	if c.OnAudioOut != nil {
		p.OnAudioOut(c.OnAudioOut)
	}
	if c.OnSpeechStart != nil {
		p.OnSpeechStart(c.OnSpeechStart)
	}
	if c.OnSpeechEnd != nil {
		p.OnSpeechEnd(c.OnSpeechEnd)
	}
	if c.OnTranscript != nil {
		p.OnTranscript(c.OnTranscript)
	}
	if c.OnResponse != nil {
		p.OnResponse(c.OnResponse)
	}
	if c.OnToolCall != nil {
		p.OnToolCall(c.OnToolCall)
	}
	if c.OnError != nil {
		p.OnError(c.OnError)
	}
}
