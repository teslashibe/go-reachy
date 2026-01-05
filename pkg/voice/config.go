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
	OpenAIKey         string
	ElevenLabsKey     string
	ElevenLabsVoiceID string // Required for ElevenLabs
	GoogleAPIKey      string

	// Audio settings
	InputSampleRate  int           // Input audio sample rate (default: 24000 for OpenAI, 16000 for others)
	OutputSampleRate int           // Output audio sample rate (default: 24000)
	BufferDuration   time.Duration // Audio buffer before sending (default: 100ms)

	// VAD (Voice Activity Detection) settings
	VADMode            string        // VAD mode: "server_vad", "semantic_vad" (OpenAI), "automatic" (Gemini)
	VADThreshold       float64       // Activation threshold 0.0-1.0 (default: 0.5)
	VADPrefixPadding   time.Duration // Audio to include before speech start (default: 300ms)
	VADSilenceDuration time.Duration // Silence duration to detect end of speech (default: 500ms)
	VADEagerness       string        // Semantic VAD eagerness: "low", "medium", "high" (OpenAI only)

	// Gemini-specific VAD settings
	VADStartSensitivity string // Start of speech sensitivity: "LOW", "MEDIUM", "HIGH"
	VADEndSensitivity   string // End of speech sensitivity: "LOW", "MEDIUM", "HIGH"

	// Audio chunk settings (for optimization)
	ChunkDuration time.Duration // Duration of each audio chunk sent (default: 100ms, try 10-50ms for lower latency)

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
	TTSModel      string  // TTS model (ElevenLabs: eleven_flash_v2_5, eleven_turbo_v2_5, eleven_multilingual_v2)
	TTSSpeed      float64 // Speech speed multiplier (default: 1.0)
	TTSStability  float64 // ElevenLabs: voice stability 0.0-1.0 (default: 0.5)
	TTSSimilarity float64 // ElevenLabs: similarity boost 0.0-1.0 (default: 0.75)

	// STT (Speech-to-Text) settings
	STTModel string // STT model (ElevenLabs: scribe_v2_realtime, scribe_v1)

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
		ChunkDuration:    100 * time.Millisecond, // Default 100ms chunks

		// VAD - Use semantic_vad for better turn detection
		VADMode:            "semantic_vad",
		VADEagerness:       "medium",
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

// ElevenLabs model constants
const (
	// TTS models (text-to-speech) - in order of speed
	// Flash models - fastest (~75ms latency)
	ElevenLabsTTSFlash   = "eleven_flash_v2_5" // Fastest, 32 languages (~75ms)
	ElevenLabsTTSFlashV2 = "eleven_flash_v2"   // Fast, English only (~75ms)

	// Turbo models - balanced quality/speed (~250-300ms)
	ElevenLabsTTSTurbo   = "eleven_turbo_v2_5" // Balanced, 32 languages (~250-300ms)
	ElevenLabsTTSTurboV2 = "eleven_turbo_v2"   // Balanced, English only (~250-300ms)

	// Quality models - best quality (higher latency)
	ElevenLabsTTSMultilingual = "eleven_multilingual_v2" // Best quality, 29 languages (~400ms+)
	ElevenLabsTTSV3           = "eleven_v3"              // Most expressive, 70+ languages (alpha)

	// Specialty models
	ElevenLabsTTSMultilingualSTS = "eleven_multilingual_sts_v2" // Speech-to-speech, 29 languages
	ElevenLabsTTSEnglishSTS      = "eleven_english_sts_v2"      // Speech-to-speech, English only
	ElevenLabsTTSMultilingualTTV = "eleven_multilingual_ttv_v2" // Text-to-voice design
	ElevenLabsTTSV3TTV           = "eleven_ttv_v3"              // Text-to-voice design v3

	// STT models (speech-to-text) - in order of speed
	ElevenLabsSTTRealtime = "scribe_v2_realtime" // Fastest (~150ms latency), 90 languages
	ElevenLabsSTTV1       = "scribe_v1"          // More accurate, 99 languages
)

// DefaultElevenLabsConfig returns a Config with defaults for ElevenLabs.
// Uses the fastest TTS and STT models by default for minimal latency.
func DefaultElevenLabsConfig() Config {
	cfg := DefaultConfig()
	cfg.Provider = ProviderElevenLabs
	cfg.InputSampleRate = 16000 // ElevenLabs uses 16kHz
	cfg.OutputSampleRate = 16000
	cfg.LLMModel = "gemini-2.0-flash"    // Default LLM for ElevenLabs
	cfg.TTSModel = ElevenLabsTTSFlash    // Fastest TTS (~75ms)
	cfg.STTModel = ElevenLabsSTTRealtime // Fastest STT (~150ms)
	return cfg
}

// DefaultGeminiConfig returns a Config with defaults for Gemini Live.
func DefaultGeminiConfig() Config {
	cfg := DefaultConfig()
	cfg.Provider = ProviderGemini
	cfg.InputSampleRate = 16000                                    // Gemini uses 16kHz input
	cfg.OutputSampleRate = 24000                                   // Gemini outputs 24kHz
	cfg.LLMModel = "gemini-2.5-flash-native-audio-preview-12-2025" // Latest native audio model
	// Gemini VAD settings - much faster than OpenAI defaults
	cfg.VADMode = "automatic"
	cfg.VADSilenceDuration = 100 * time.Millisecond // Gemini supports 100ms!
	cfg.VADStartSensitivity = "HIGH"
	cfg.VADEndSensitivity = "HIGH"
	cfg.VADPrefixPadding = 20 * time.Millisecond
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
