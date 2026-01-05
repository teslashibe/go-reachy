package bundled

import (
	"context"
	"fmt"

	"github.com/teslashibe/go-reachy/pkg/conversation"
	"github.com/teslashibe/go-reachy/pkg/voice"
)

// ElevenLabs implements voice.Pipeline using ElevenLabs Conversational AI.
// This provides custom cloned voices with choice of LLM (Gemini, Claude, GPT-4o).
type ElevenLabs struct {
	config   voice.Config
	provider *conversation.ElevenLabs
	
	// Metrics
	metrics *voice.MetricsCollector
	
	// Tools registered before Connect
	tools []voice.Tool
	
	// Callbacks
	onAudioOut    func(pcm16 []byte)
	onSpeechStart func()
	onSpeechEnd   func()
	onTranscript  func(text string, isFinal bool)
	onResponse    func(text string, isFinal bool)
	onToolCall    func(call voice.ToolCall)
	onError       func(err error)
}

// NewElevenLabs creates a new ElevenLabs voice pipeline.
func NewElevenLabs(cfg voice.Config) (*ElevenLabs, error) {
	if cfg.ElevenLabsKey == "" {
		return nil, voice.ErrMissingAPIKey
	}
	if cfg.ElevenLabsVoiceID == "" {
		return nil, fmt.Errorf("voice/elevenlabs: voice ID required")
	}
	
	return &ElevenLabs{
		config:  cfg,
		metrics: voice.NewMetricsCollector(),
		tools:   []voice.Tool{},
	}, nil
}

// Start establishes the connection and begins processing.
func (e *ElevenLabs) Start(ctx context.Context) error {
	// Build conversation options
	opts := []conversation.Option{
		conversation.WithAPIKey(e.config.ElevenLabsKey),
		conversation.WithVoiceID(e.config.ElevenLabsVoiceID),
		conversation.WithAutoCreateAgent(true),
	}
	
	// LLM model
	llm := e.config.LLMModel
	if llm == "" {
		llm = "gemini-2.0-flash"
	}
	opts = append(opts, conversation.WithLLM(llm))
	
	// System prompt
	if e.config.SystemPrompt != "" {
		opts = append(opts, conversation.WithSystemPrompt(e.config.SystemPrompt))
	}
	
	// Create provider
	provider, err := conversation.NewElevenLabs(opts...)
	if err != nil {
		return fmt.Errorf("voice/elevenlabs: failed to create provider: %w", err)
	}
	e.provider = provider
	
	// Register tools
	for _, tool := range e.tools {
		e.provider.RegisterTool(conversation.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	
	// Wire callbacks
	e.provider.OnAudio(func(audio []byte) {
		e.metrics.MarkFirstAudio()
		e.metrics.IncrementAudioOut()
		if e.onAudioOut != nil {
			e.onAudioOut(audio)
		}
	})
	
	e.provider.OnAudioDone(func() {
		e.metrics.MarkResponseDone()
		if e.config.ProfileLatency {
			m := e.metrics.Current()
			fmt.Printf("⏱️  %s\n", m.FormatLatency())
		}
	})
	
	e.provider.OnTranscript(func(role, text string, isFinal bool) {
		if role == "user" {
			if isFinal {
				e.metrics.MarkSpeechEnd()
				e.metrics.MarkTranscript()
				if e.onSpeechEnd != nil {
					e.onSpeechEnd()
				}
			}
			if e.onTranscript != nil {
				e.onTranscript(text, isFinal)
			}
		} else if role == "agent" || role == "assistant" {
			e.metrics.MarkFirstToken()
			if e.onResponse != nil {
				e.onResponse(text, isFinal)
			}
		}
	})
	
	e.provider.OnToolCall(func(id, name string, args map[string]any) {
		if e.onToolCall != nil {
			e.onToolCall(voice.ToolCall{
				ID:        id,
				Name:      name,
				Arguments: args,
			})
		} else {
			// Execute internally if no external handler
			for _, tool := range e.tools {
				if tool.Name == name && tool.Handler != nil {
					result, err := tool.Handler(args)
					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}
					if submitErr := e.provider.SubmitToolResult(id, result); submitErr != nil {
						if e.onError != nil {
							e.onError(submitErr)
						}
					}
					return
				}
			}
			// Tool not found
			if submitErr := e.provider.SubmitToolResult(id, "Function not found"); submitErr != nil {
				if e.onError != nil {
					e.onError(submitErr)
				}
			}
		}
	})
	
	e.provider.OnError(func(err error) {
		if e.onError != nil {
			e.onError(err)
		}
	})
	
	e.provider.OnInterruption(func() {
		// Treat interruption as speech start
		if e.onSpeechStart != nil {
			e.onSpeechStart()
		}
	})
	
	// Connect
	if err := e.provider.Connect(ctx); err != nil {
		return fmt.Errorf("voice/elevenlabs: failed to connect: %w", err)
	}
	
	return nil
}

// Stop gracefully shuts down the pipeline.
func (e *ElevenLabs) Stop() error {
	if e.provider != nil {
		return e.provider.Close()
	}
	return nil
}

// IsConnected returns true if connected and ready.
func (e *ElevenLabs) IsConnected() bool {
	if e.provider == nil {
		return false
	}
	return e.provider.IsConnected()
}

// SendAudio sends PCM16 audio to the pipeline.
// Note: ElevenLabs expects 16kHz audio.
func (e *ElevenLabs) SendAudio(pcm16 []byte) error {
	if e.provider == nil {
		return voice.ErrNotConnected
	}
	e.metrics.IncrementAudioIn()
	return e.provider.SendAudio(pcm16)
}

// OnAudioOut sets the callback for audio output.
func (e *ElevenLabs) OnAudioOut(fn func(pcm16 []byte)) {
	e.onAudioOut = fn
}

// OnSpeechStart sets the callback for speech start.
func (e *ElevenLabs) OnSpeechStart(fn func()) {
	e.onSpeechStart = fn
}

// OnSpeechEnd sets the callback for speech end.
func (e *ElevenLabs) OnSpeechEnd(fn func()) {
	e.onSpeechEnd = fn
}

// OnTranscript sets the callback for transcripts.
func (e *ElevenLabs) OnTranscript(fn func(text string, isFinal bool)) {
	e.onTranscript = fn
}

// OnResponse sets the callback for AI responses.
func (e *ElevenLabs) OnResponse(fn func(text string, isFinal bool)) {
	e.onResponse = fn
}

// OnError sets the error callback.
func (e *ElevenLabs) OnError(fn func(err error)) {
	e.onError = fn
}

// RegisterTool adds a tool the AI can invoke.
func (e *ElevenLabs) RegisterTool(tool voice.Tool) {
	e.tools = append(e.tools, tool)
	// If already connected, register with provider
	if e.provider != nil {
		e.provider.RegisterTool(conversation.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
}

// OnToolCall sets the callback for tool invocations.
func (e *ElevenLabs) OnToolCall(fn func(call voice.ToolCall)) {
	e.onToolCall = fn
}

// SubmitToolResult returns a tool result to the AI.
func (e *ElevenLabs) SubmitToolResult(callID string, result string) error {
	if e.provider == nil {
		return voice.ErrNotConnected
	}
	return e.provider.SubmitToolResult(callID, result)
}

// Interrupt stops the current AI response.
func (e *ElevenLabs) Interrupt() error {
	if e.provider == nil {
		return voice.ErrNotConnected
	}
	return e.provider.CancelResponse()
}

// Metrics returns current latency metrics.
func (e *ElevenLabs) Metrics() voice.Metrics {
	return e.metrics.Current()
}

// Config returns the current configuration.
func (e *ElevenLabs) Config() voice.Config {
	return e.config
}

// UpdateConfig applies new configuration.
// Note: Most settings require reconnection to take effect.
func (e *ElevenLabs) UpdateConfig(cfg voice.Config) error {
	e.config = cfg
	// ElevenLabs doesn't support runtime config updates
	// Would need to reconnect to apply changes
	return nil
}

// Ensure ElevenLabs implements voice.Pipeline at compile time.
var _ voice.Pipeline = (*ElevenLabs)(nil)

// Register ElevenLabs provider in voice package.
func init() {
	voice.Register(voice.ProviderElevenLabs, func(cfg voice.Config) (voice.Pipeline, error) {
		return NewElevenLabs(cfg)
	})
}

