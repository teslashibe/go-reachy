// Package tts provides a unified interface for text-to-speech providers.
//
// The package supports multiple TTS backends including ElevenLabs (custom voice cloning),
// OpenAI (built-in voices), and local providers (Piper, Coqui). All providers implement
// the Provider interface, enabling seamless switching without changing caller code.
//
// Example usage:
//
//	provider, _ := tts.NewElevenLabs(
//	    tts.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
//	    tts.WithVoice("your-voice-id"),
//	)
//	defer provider.Close()
//
//	result, _ := provider.Synthesize(ctx, "Hello world")
//	// result.Audio contains PCM/MP3 audio bytes
package tts

import (
	"context"
	"time"
)

// Provider defines the TTS provider interface.
// All implementations must satisfy this interface for seamless provider switching.
type Provider interface {
	// Synthesize converts text to audio, returning the complete audio buffer.
	// Use this for short text where latency to first byte is less critical.
	Synthesize(ctx context.Context, text string) (*AudioResult, error)

	// Stream converts text to audio with streaming output for lowest latency.
	// Audio chunks are returned as they become available.
	Stream(ctx context.Context, text string) (AudioStream, error)

	// Health checks provider connectivity and API key validity.
	Health(ctx context.Context) error

	// Close releases any resources held by the provider.
	Close() error
}

// AudioStream represents a streaming audio response.
// Callers should read until Read returns nil, then call Close.
type AudioStream interface {
	// Read returns the next audio chunk.
	// Returns nil when the stream is complete (not an error).
	Read() ([]byte, error)

	// Close stops the stream and releases resources.
	Close() error

	// Format returns the audio format metadata.
	Format() AudioFormat
}

// AudioResult represents a complete audio synthesis result.
type AudioResult struct {
	// Audio contains the raw audio data in the specified format.
	Audio []byte

	// Format describes the audio encoding and sample rate.
	Format AudioFormat

	// Duration is the estimated audio playback duration.
	Duration time.Duration

	// CharCount is the number of characters synthesized.
	CharCount int

	// LatencyMs is the time to first byte in milliseconds.
	LatencyMs int64
}

// AudioFormat describes the audio encoding parameters.
type AudioFormat struct {
	// Encoding specifies the audio codec (e.g., pcm_24000, mp3_44100_128).
	Encoding Encoding

	// SampleRate in Hz (e.g., 24000, 44100, 22050).
	SampleRate int

	// Channels is 1 for mono, 2 for stereo.
	Channels int

	// BitDepth for PCM formats (e.g., 16 for PCM16).
	BitDepth int
}

// Encoding represents audio encoding types.
// These match ElevenLabs output format options.
type Encoding string

const (
	// PCM formats (raw audio, lowest latency)
	EncodingPCM16 Encoding = "pcm_16000" // 16kHz mono PCM16
	EncodingPCM22 Encoding = "pcm_22050" // 22.05kHz mono PCM16
	EncodingPCM24 Encoding = "pcm_24000" // 24kHz mono PCM16 (matches OpenAI Realtime)
	EncodingPCM44 Encoding = "pcm_44100" // 44.1kHz mono PCM16

	// Compressed formats
	EncodingMP3  Encoding = "mp3_44100_128" // MP3 128kbps
	EncodingOpus Encoding = "opus"          // Opus codec
	EncodingULaw Encoding = "ulaw_8000"     // μ-law 8kHz (telephony)
)

// VoiceSettings controls voice characteristics for providers that support it.
// These settings affect the expressiveness and consistency of the generated speech.
type VoiceSettings struct {
	// Stability controls voice consistency (0.0-1.0).
	// Lower values = more expressive/variable, higher = more consistent.
	Stability float64

	// SimilarityBoost controls how closely the voice matches the original (0.0-1.0).
	// Higher values = closer to original voice sample.
	SimilarityBoost float64

	// Style controls style exaggeration (0.0-1.0).
	// Only supported by ElevenLabs v2 models.
	Style float64

	// SpeakerBoost enhances speaker clarity.
	// Recommended for noisy environments.
	SpeakerBoost bool
}

// DefaultVoiceSettings returns sensible defaults for voice synthesis.
func DefaultVoiceSettings() VoiceSettings {
	return VoiceSettings{
		Stability:       0.5,
		SimilarityBoost: 0.75,
		Style:           0.0,
		SpeakerBoost:    true,
	}
}

// SampleRateFromEncoding extracts the sample rate from an encoding type.
func SampleRateFromEncoding(enc Encoding) int {
	switch enc {
	case EncodingPCM16:
		return 16000
	case EncodingPCM22:
		return 22050
	case EncodingPCM24:
		return 24000
	case EncodingPCM44, EncodingMP3:
		return 44100
	case EncodingULaw:
		return 8000
	default:
		return 24000 // Default to 24kHz
	}
}

