package conversation

import (
	"log/slog"
	"time"
)

// Config holds configuration for conversation providers.
type Config struct {
	// APIKey is the authentication key for the provider.
	APIKey string

	// AgentID is the agent/assistant identifier (ElevenLabs).
	// If empty and AutoCreateAgent is true, an agent will be created.
	AgentID string

	// Model is the LLM model to use.
	Model string

	// Voice is the voice ID for TTS (OpenAI).
	Voice string

	// VoiceID is the ElevenLabs voice ID for programmatic agent creation.
	VoiceID string

	// LLM is the language model to use (ElevenLabs: "gemini-2.0-flash", "claude-3-5-sonnet", "gpt-4o").
	LLM string

	// AgentName is the name for the agent (for dashboard reference).
	AgentName string

	// FirstMessage is the first message the agent will say (empty = wait for user).
	FirstMessage string

	// AutoCreateAgent enables automatic agent creation if AgentID is not provided.
	AutoCreateAgent bool

	// BaseURL overrides the default API endpoint.
	BaseURL string

	// SystemPrompt is the default system instruction.
	SystemPrompt string

	// Temperature controls response randomness (0.0-1.0).
	Temperature float64

	// MaxResponseTokens limits response length.
	MaxResponseTokens int

	// InputSampleRate is the audio input sample rate in Hz.
	InputSampleRate int

	// OutputSampleRate is the audio output sample rate in Hz.
	OutputSampleRate int

	// Timeout is the connection timeout.
	Timeout time.Duration

	// ReadTimeout is the timeout for reading messages.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for writing messages.
	WriteTimeout time.Duration

	// ReconnectAttempts is the number of reconnection attempts.
	ReconnectAttempts int

	// ReconnectDelay is the delay between reconnection attempts.
	ReconnectDelay time.Duration

	// EnableMetrics enables metrics collection.
	EnableMetrics bool

	// Logger is the structured logger to use.
	Logger *slog.Logger

	// Tools is the list of tools available to the agent.
	Tools []Tool

	// TurnDetection configures voice activity detection.
	TurnDetection *TurnDetection
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Temperature:       0.8,
		MaxResponseTokens: 4096,
		InputSampleRate:   16000,
		OutputSampleRate:  16000,
		Timeout:           30 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      30 * time.Second,
		ReconnectAttempts: 3,
		ReconnectDelay:    time.Second,
		EnableMetrics:     true,
		Logger:            slog.Default(),
		TurnDetection: &TurnDetection{
			Type:              "server_vad",
			Threshold:         0.5,
			PrefixPaddingMs:   300,
			SilenceDurationMs: 500,
		},
	}
}

// Apply applies functional options to the config.
func (c *Config) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(c)
	}
}

// Validate checks the configuration for required fields.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return ErrMissingAPIKey
	}
	return nil
}

// Option is a functional option for configuring providers.
type Option func(*Config)

// WithAPIKey sets the API key.
func WithAPIKey(key string) Option {
	return func(c *Config) {
		c.APIKey = key
	}
}

// WithAgentID sets the agent/assistant ID.
func WithAgentID(id string) Option {
	return func(c *Config) {
		c.AgentID = id
	}
}

// WithModel sets the LLM model.
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithVoice sets the TTS voice.
func WithVoice(voice string) Option {
	return func(c *Config) {
		c.Voice = voice
	}
}

// WithBaseURL sets the API base URL.
func WithBaseURL(url string) Option {
	return func(c *Config) {
		c.BaseURL = url
	}
}

// WithSystemPrompt sets the system instruction.
func WithSystemPrompt(prompt string) Option {
	return func(c *Config) {
		c.SystemPrompt = prompt
	}
}

// WithTemperature sets the response temperature.
func WithTemperature(temp float64) Option {
	return func(c *Config) {
		c.Temperature = temp
	}
}

// WithMaxTokens sets the maximum response tokens.
func WithMaxTokens(tokens int) Option {
	return func(c *Config) {
		c.MaxResponseTokens = tokens
	}
}

// WithInputSampleRate sets the audio input sample rate.
func WithInputSampleRate(rate int) Option {
	return func(c *Config) {
		c.InputSampleRate = rate
	}
}

// WithOutputSampleRate sets the audio output sample rate.
func WithOutputSampleRate(rate int) Option {
	return func(c *Config) {
		c.OutputSampleRate = rate
	}
}

// WithTimeout sets the connection timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithReconnect configures reconnection behavior.
func WithReconnect(attempts int, delay time.Duration) Option {
	return func(c *Config) {
		c.ReconnectAttempts = attempts
		c.ReconnectDelay = delay
	}
}

// WithLogger sets the structured logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithTools sets the available tools.
func WithTools(tools ...Tool) Option {
	return func(c *Config) {
		c.Tools = tools
	}
}

// WithTurnDetection configures voice activity detection.
func WithTurnDetection(td *TurnDetection) Option {
	return func(c *Config) {
		c.TurnDetection = td
	}
}

// WithMetrics enables or disables metrics collection.
func WithMetrics(enabled bool) Option {
	return func(c *Config) {
		c.EnableMetrics = enabled
	}
}

// WithVoiceID sets the ElevenLabs voice ID for programmatic agent creation.
func WithVoiceID(id string) Option {
	return func(c *Config) {
		c.VoiceID = id
	}
}

// WithLLM sets the language model for ElevenLabs agents.
// Supported values: "gemini-2.0-flash", "claude-3-5-sonnet", "gpt-4o"
func WithLLM(model string) Option {
	return func(c *Config) {
		c.LLM = model
	}
}

// WithAgentName sets the agent name for dashboard reference.
func WithAgentName(name string) Option {
	return func(c *Config) {
		c.AgentName = name
	}
}

// WithFirstMessage sets the first message the agent will say.
// If empty, the agent waits for the user to speak first.
func WithFirstMessage(msg string) Option {
	return func(c *Config) {
		c.FirstMessage = msg
	}
}

// WithAutoCreateAgent enables automatic agent creation if AgentID is not provided.
// Requires VoiceID to be set.
func WithAutoCreateAgent(enabled bool) Option {
	return func(c *Config) {
		c.AutoCreateAgent = enabled
	}
}

// Common voice constants for convenience.
const (
	// OpenAI voices
	VoiceAlloy   = "alloy"
	VoiceEcho    = "echo"
	VoiceFable   = "fable"
	VoiceOnyx    = "onyx"
	VoiceNova    = "nova"
	VoiceShimmer = "shimmer"

	// ElevenLabs models
	ModelFlashV2_5 = "eleven_flash_v2_5"
	ModelTurboV2_5 = "eleven_turbo_v2_5"
)
