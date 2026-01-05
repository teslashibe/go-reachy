package voice

import (
	"errors"
	"time"
)

// Provider identifies the voice pipeline provider.
type Provider string

const (
	// ProviderOpenAI uses OpenAI's Realtime API (GPT-4o + built-in TTS).
	ProviderOpenAI Provider = "openai"

	// ProviderElevenLabs uses ElevenLabs Conversational AI (custom voice + choice of LLM).
	ProviderElevenLabs Provider = "elevenlabs"

	// ProviderGemini uses Google's Gemini Live API (lowest latency).
	ProviderGemini Provider = "gemini"
)

// Config holds all tunable parameters for voice pipelines.
// Parameters are organized by stage for clarity.
type Config struct {
	// Provider selection
	Provider Provider

	// API Keys (provider-specific)
	OpenAIKey      string
	ElevenLabsKey  string
	ElevenLabsVoiceID string // Required for ElevenLabs
	GoogleAPIKey   string

	// Audio settings
	InputSampleRate  int           // Input audio sample rate (default: 24000 for OpenAI, 16000 for others)
	OutputSampleRate int           // Output audio sample rate (default: 24000)
	BufferDuration   time.Duration // Audio buffer before sending (default: 100ms)

	// VAD (Voice Activity Detection) settings
	VADThreshold       float64       // Activation threshold 0.0-1.0 (default: 0.5)
	VADPrefixPadding   time.Duration // Audio to include before speech start (default: 300ms)
	VADSilenceDuration time.Duration // Silence duration to detect end of speech (default: 500ms)

	// ASR (Automatic Speech Recognition) settings
	ASRModel    string // Model for transcription (default: provider-specific)
	ASRLanguage string // Language hint (default: "en")

	// LLM (Language Model) settings
	LLMModel       string  // Model name (default: provider-specific)
	LLMTemperature float64 // Response randomness 0.0-2.0 (default: 0.8)
	LLMMaxTokens   int     // Maximum response tokens (default: 4096)
	SystemPrompt   string  // System instructions for the AI

	// TTS (Text-to-Speech) settings
	TTSVoice      string  // Voice ID or name
	TTSSpeed      float64 // Speech speed multiplier (default: 1.0)
	TTSStability  float64 // ElevenLabs: voice stability 0.0-1.0 (default: 0.5)
	TTSSimilarity float64 // ElevenLabs: similarity boost 0.0-1.0 (default: 0.75)

	// Streaming settings
	StreamingEnabled bool // Enable streaming responses (default: true)

	// Debug settings
	Debug          bool // Enable debug logging
	ProfileLatency bool // Log detailed latency breakdown
}

// DefaultConfig returns a Config with sensible defaults for OpenAI.
func DefaultConfig() Config {
	return Config{
		Provider: ProviderOpenAI,

		// Audio
		InputSampleRate:  24000,
		OutputSampleRate: 24000,
		BufferDuration:   100 * time.Millisecond,

		// VAD
		VADThreshold:       0.5,
		VADPrefixPadding:   300 * time.Millisecond,
		VADSilenceDuration: 500 * time.Millisecond,

		// ASR
		ASRLanguage: "en",

		// LLM
		LLMTemperature: 0.8,
		LLMMaxTokens:   4096,

		// TTS
		TTSSpeed:      1.0,
		TTSStability:  0.5,
		TTSSimilarity: 0.75,

		// Streaming
		StreamingEnabled: true,
	}
}

// DefaultElevenLabsConfig returns a Config with defaults for ElevenLabs.
func DefaultElevenLabsConfig() Config {
	cfg := DefaultConfig()
	cfg.Provider = ProviderElevenLabs
	cfg.InputSampleRate = 16000  // ElevenLabs uses 16kHz
	cfg.OutputSampleRate = 16000
	cfg.LLMModel = "gemini-2.0-flash" // Default LLM for ElevenLabs
	return cfg
}

// DefaultGeminiConfig returns a Config with defaults for Gemini Live.
func DefaultGeminiConfig() Config {
	cfg := DefaultConfig()
	cfg.Provider = ProviderGemini
	cfg.InputSampleRate = 16000  // Gemini uses 16kHz input
	cfg.OutputSampleRate = 24000 // Gemini outputs 24kHz
	cfg.LLMModel = "gemini-2.0-flash-exp"
	return cfg
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	switch c.Provider {
	case ProviderOpenAI:
		if c.OpenAIKey == "" {
			return errors.New("voice: OpenAI API key required")
		}
	case ProviderElevenLabs:
		if c.ElevenLabsKey == "" {
			return errors.New("voice: ElevenLabs API key required")
		}
		if c.ElevenLabsVoiceID == "" {
			return errors.New("voice: ElevenLabs voice ID required")
		}
	case ProviderGemini:
		if c.GoogleAPIKey == "" {
			return errors.New("voice: Google API key required")
		}
	default:
		return errors.New("voice: unknown provider: " + string(c.Provider))
	}

	if c.VADThreshold < 0 || c.VADThreshold > 1 {
		return errors.New("voice: VAD threshold must be between 0 and 1")
	}

	if c.LLMTemperature < 0 || c.LLMTemperature > 2 {
		return errors.New("voice: LLM temperature must be between 0 and 2")
	}

	return nil
}

// WithProvider returns a copy with the provider set.
func (c Config) WithProvider(p Provider) Config {
	c.Provider = p
	return c
}

// WithSystemPrompt returns a copy with the system prompt set.
func (c Config) WithSystemPrompt(prompt string) Config {
	c.SystemPrompt = prompt
	return c
}

// WithVAD returns a copy with VAD settings.
func (c Config) WithVAD(threshold float64, silenceDuration time.Duration) Config {
	c.VADThreshold = threshold
	c.VADSilenceDuration = silenceDuration
	return c
}

// WithDebug returns a copy with debug enabled.
func (c Config) WithDebug(debug bool) Config {
	c.Debug = debug
	return c
}

