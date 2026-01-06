package tts

import (
	"log/slog"
	"time"
)

// Config holds TTS provider configuration.
// Use functional options (WithXxx) to set these values.
type Config struct {
	// Provider credentials
	APIKey  string
	BaseURL string

	// Voice configuration
	VoiceID       string
	ModelID       string
	VoiceSettings VoiceSettings

	// Audio output
	OutputFormat Encoding

	// Timeouts
	Timeout       time.Duration
	StreamTimeout time.Duration

	// Retry configuration
	MaxRetries int
	RetryDelay time.Duration

	// Observability
	Logger *slog.Logger
}

// Option is a functional option for configuring TTS providers.
type Option func(*Config)

// WithAPIKey sets the API key for the provider.
func WithAPIKey(key string) Option {
	return func(c *Config) {
		c.APIKey = key
	}
}

// WithBaseURL overrides the default API base URL.
func WithBaseURL(url string) Option {
	return func(c *Config) {
		c.BaseURL = url
	}
}

// WithVoice sets the voice ID.
func WithVoice(voiceID string) Option {
	return func(c *Config) {
		c.VoiceID = voiceID
	}
}

// WithModel sets the model ID.
func WithModel(modelID string) Option {
	return func(c *Config) {
		c.ModelID = modelID
	}
}

// WithOutputFormat sets the audio output format.
func WithOutputFormat(format Encoding) Option {
	return func(c *Config) {
		c.OutputFormat = format
	}
}

// WithVoiceSettings sets voice characteristics.
func WithVoiceSettings(settings VoiceSettings) Option {
	return func(c *Config) {
		c.VoiceSettings = settings
	}
}

// WithTimeout sets the request timeout for non-streaming requests.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithStreamTimeout sets the timeout for streaming requests.
func WithStreamTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.StreamTimeout = timeout
	}
}

// WithRetry configures retry behavior for failed requests.
func WithRetry(maxRetries int, delay time.Duration) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
		c.RetryDelay = delay
	}
}

// WithLogger sets the structured logger for the provider.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		ModelID:       "eleven_turbo_v2_5", // Fastest ElevenLabs model
		OutputFormat:  EncodingPCM24,       // Match robot's 24kHz
		VoiceSettings: DefaultVoiceSettings(),
		Timeout:       30 * time.Second,
		StreamTimeout: 60 * time.Second,
		MaxRetries:    3,
		RetryDelay:    100 * time.Millisecond,
		Logger:        slog.Default(),
	}
}

// Apply applies functional options to the config.
func (c *Config) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(c)
	}
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return ErrNoAPIKey
	}
	return nil
}

// ValidateWithVoice checks that both API key and voice ID are present.
func (c *Config) ValidateWithVoice() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.VoiceID == "" {
		return ErrNoVoiceID
	}
	return nil
}




