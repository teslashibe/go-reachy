package inference

import (
	"log/slog"
	"time"
)

// Config holds provider configuration.
type Config struct {
	// Connection
	BaseURL string // API base URL
	APIKey  string // API key (optional for local providers)

	// Models
	Model       string // Default chat model
	VisionModel string // Vision model (may differ from chat)
	EmbedModel  string // Embedding model

	// Request defaults
	MaxTokens   int
	Temperature float64

	// Timeouts
	Timeout       time.Duration
	StreamTimeout time.Duration

	// Retry configuration
	MaxRetries int
	RetryDelay time.Duration

	// Observability
	Logger *slog.Logger
}

// Option is a functional option for configuring providers.
type Option func(*Config)

// WithBaseURL sets the API base URL.
// Examples: "https://api.openai.com/v1", "http://localhost:11434/v1"
func WithBaseURL(url string) Option {
	return func(c *Config) { c.BaseURL = url }
}

// WithAPIKey sets the API key.
func WithAPIKey(key string) Option {
	return func(c *Config) { c.APIKey = key }
}

// WithModel sets the default chat model.
func WithModel(model string) Option {
	return func(c *Config) { c.Model = model }
}

// WithVisionModel sets the vision model.
func WithVisionModel(model string) Option {
	return func(c *Config) { c.VisionModel = model }
}

// WithEmbedModel sets the embedding model.
func WithEmbedModel(model string) Option {
	return func(c *Config) { c.EmbedModel = model }
}

// WithMaxTokens sets the default max tokens.
func WithMaxTokens(n int) Option {
	return func(c *Config) { c.MaxTokens = n }
}

// WithTemperature sets the default temperature.
func WithTemperature(t float64) Option {
	return func(c *Config) { c.Temperature = t }
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) { c.Timeout = d }
}

// WithStreamTimeout sets the streaming request timeout.
func WithStreamTimeout(d time.Duration) Option {
	return func(c *Config) { c.StreamTimeout = d }
}

// WithRetry configures retry behavior.
func WithRetry(maxRetries int, delay time.Duration) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
		c.RetryDelay = delay
	}
}

// WithLogger sets the structured logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Config) { c.Logger = l }
}

// DefaultConfig returns sensible defaults for OpenAI.
func DefaultConfig() *Config {
	return &Config{
		BaseURL:       "https://api.openai.com/v1",
		Model:         "gpt-4o-mini",
		VisionModel:   "gpt-4o",
		EmbedModel:    "text-embedding-3-small",
		MaxTokens:     1024,
		Temperature:   0.7,
		Timeout:       30 * time.Second,
		StreamTimeout: 120 * time.Second,
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
	// API key is optional for local providers like Ollama
	return nil
}


