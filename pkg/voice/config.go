package voice

import (
	"errors"
	"time"
)

// Config holds all tunable parameters for the ElevenLabs voice pipeline.
// Parameters are organized by stage for clarity.
type Config struct {
	// API Keys
	ElevenLabsKey     string // Required: ElevenLabs API key
	ElevenLabsVoiceID string // Required: ElevenLabs voice ID

	// Audio settings
	InputSampleRate  int           // Input audio sample rate (default: 16000)
	OutputSampleRate int           // Output audio sample rate (default: 16000)
	BufferDuration   time.Duration // Audio buffer before sending (default: 100ms)
	ChunkDuration    time.Duration // Duration of each audio chunk sent (default: 50ms)

	// VAD (Voice Activity Detection) settings
	VADMode            string        // VAD mode: "server_vad" (default)
	VADThreshold       float64       // Activation threshold 0.0-1.0 (default: 0.5)
	VADPrefixPadding   time.Duration // Audio to include before speech start (default: 300ms)
	VADSilenceDuration time.Duration // Silence duration to detect end of speech (default: 500ms)

	// LLM (Language Model) settings
	LLMModel       string  // Model name (default: gpt-5-mini)
	LLMTemperature float64 // Response randomness 0.0-2.0 (default: 0.8)
	LLMMaxTokens   int     // Maximum response tokens (default: 4096)
	SystemPrompt   string  // System instructions for the AI

	// TTS (Text-to-Speech) settings
	TTSModel      string  // TTS model (default: eleven_flash_v2)
	TTSSpeed      float64 // Speech speed multiplier (default: 1.0)
	TTSStability  float64 // Voice stability 0.0-1.0 (default: 0.5)
	TTSSimilarity float64 // Similarity boost 0.0-1.0 (default: 0.75)

	// STT (Speech-to-Text) settings
	STTModel string // STT model (default: scribe_v2_realtime)

	// Debug settings
	Debug          bool // Enable debug logging
	ProfileLatency bool // Log detailed latency breakdown
}

// TTS model constants - ordered by latency (fastest first)
const (
	// Flash models - fastest (~75ms latency)
	TTSFlash   = "eleven_flash_v2"   // Fast, English only (~75ms) - USE FOR AGENTS
	TTSFlashML = "eleven_flash_v2_5" // Fast, 32 languages (~75ms) - Multilingual only

	// Turbo models - balanced quality/speed (~250-300ms)
	TTSTurbo   = "eleven_turbo_v2"   // Balanced, English only (~250-300ms)
	TTSTurboML = "eleven_turbo_v2_5" // Balanced, 32 languages (~250-300ms)

	// Quality models - best quality (higher latency)
	TTSMultilingual = "eleven_multilingual_v2" // Best quality, 29 languages (~400ms+)
	TTSV3           = "eleven_v3"              // Most expressive, 70+ languages (alpha)
)

// STT model constants - ordered by latency (fastest first)
const (
	STTRealtime = "scribe_v2_realtime" // Fastest (~150ms latency), 90 languages
	STTV1       = "scribe_v1"          // More accurate, 99 languages
)

// LLM model constants - ordered by TTFA benchmark results (fastest first)
const (
	LLMGpt5Mini       = "gpt-5-mini"       // Fastest (1.25s TTFA)
	LLMGpt41Mini      = "gpt-4.1-mini"     // Fast (1.26s TTFA)
	LLMGemini20Flash  = "gemini-2.0-flash" // Most consistent (1.28s TTFA)
	LLMClaude35Sonnet = "claude-3.5-sonnet" // Good quality (1.28s TTFA)
	LLMGpt4oMini      = "gpt-4o-mini"       // Reliable (1.28s TTFA)
)

// DefaultConfig returns a Config with optimal defaults for lowest latency.
// Uses gpt-5-mini + eleven_flash_v2 + scribe_v2_realtime.
func DefaultConfig() Config {
	return Config{
		// Audio
		InputSampleRate:  16000,
		OutputSampleRate: 16000,
		BufferDuration:   100 * time.Millisecond,
		ChunkDuration:    50 * time.Millisecond, // Smaller chunks for lower latency

		// VAD
		VADMode:            "server_vad",
		VADThreshold:       0.5,
		VADPrefixPadding:   300 * time.Millisecond,
		VADSilenceDuration: 500 * time.Millisecond,

		// LLM - fastest from benchmark
		LLMModel:       LLMGpt5Mini,
		LLMTemperature: 0.8,
		LLMMaxTokens:   4096,

		// TTS - fastest for English agents
		TTSModel:      TTSFlash, // eleven_flash_v2 (~75ms)
		TTSSpeed:      1.0,
		TTSStability:  0.5,
		TTSSimilarity: 0.75,

		// STT - fastest
		STTModel: STTRealtime, // scribe_v2_realtime (~150ms)
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.ElevenLabsKey == "" {
		return errors.New("voice: ElevenLabs API key required")
	}
	if c.ElevenLabsVoiceID == "" {
		return errors.New("voice: ElevenLabs voice ID required")
	}

	if c.VADThreshold < 0 || c.VADThreshold > 1 {
		return errors.New("voice: VAD threshold must be between 0 and 1")
	}

	if c.LLMTemperature < 0 || c.LLMTemperature > 2 {
		return errors.New("voice: LLM temperature must be between 0 and 2")
	}

	return nil
}

// WithSystemPrompt returns a copy with the system prompt set.
func (c Config) WithSystemPrompt(prompt string) Config {
	c.SystemPrompt = prompt
	return c
}

// WithLLM returns a copy with the LLM model set.
func (c Config) WithLLM(model string) Config {
	c.LLMModel = model
	return c
}

// WithTTS returns a copy with the TTS model set.
func (c Config) WithTTS(model string) Config {
	c.TTSModel = model
	return c
}

// WithSTT returns a copy with the STT model set.
func (c Config) WithSTT(model string) Config {
	c.STTModel = model
	return c
}

// WithVAD returns a copy with VAD settings.
func (c Config) WithVAD(silenceDuration time.Duration) Config {
	c.VADSilenceDuration = silenceDuration
	return c
}

// WithChunkDuration returns a copy with the chunk duration set.
func (c Config) WithChunkDuration(d time.Duration) Config {
	c.ChunkDuration = d
	return c
}

// WithDebug returns a copy with debug enabled.
func (c Config) WithDebug(debug bool) Config {
	c.Debug = debug
	return c
}
