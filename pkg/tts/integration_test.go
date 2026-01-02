//go:build integration

package tts_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teslashibe/go-reachy/pkg/tts"
)

// TestElevenLabsIntegration tests real ElevenLabs API.
// Run with: go test -tags=integration -v ./pkg/tts/...
func TestElevenLabsIntegration(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	voiceID := os.Getenv("ELEVENLABS_VOICE_ID")

	if apiKey == "" {
		t.Skip("ELEVENLABS_API_KEY not set")
	}
	if voiceID == "" {
		t.Skip("ELEVENLABS_VOICE_ID not set")
	}

	provider, err := tts.NewElevenLabs(
		tts.WithAPIKey(apiKey),
		tts.WithVoice(voiceID),
		tts.WithModel(tts.ModelTurboV2_5),
		tts.WithOutputFormat(tts.EncodingPCM24),
	)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Health", func(t *testing.T) {
		if err := provider.Health(ctx); err != nil {
			t.Fatalf("health check failed: %v", err)
		}
		t.Log("✅ Health check passed")
	})

	t.Run("Synthesize", func(t *testing.T) {
		result, err := provider.Synthesize(ctx, "Hello, this is Eva speaking.")
		if err != nil {
			t.Fatalf("synthesize failed: %v", err)
		}

		t.Logf("✅ Synthesized: %d bytes, latency: %dms", len(result.Audio), result.LatencyMs)

		if len(result.Audio) < 1000 {
			t.Error("audio too short, expected at least 1KB")
		}
		if result.Format.SampleRate != 24000 {
			t.Errorf("expected 24000 sample rate, got %d", result.Format.SampleRate)
		}
	})

	t.Run("Stream", func(t *testing.T) {
		stream, err := provider.Stream(ctx, "Testing streaming audio.")
		if err != nil {
			t.Fatalf("stream failed: %v", err)
		}
		defer stream.Close()

		totalBytes := 0
		chunkCount := 0
		for {
			chunk, err := stream.Read()
			if err != nil {
				t.Fatalf("stream read error: %v", err)
			}
			if chunk == nil {
				break
			}
			totalBytes += len(chunk)
			chunkCount++
		}

		t.Logf("✅ Streamed: %d bytes in %d chunks", totalBytes, chunkCount)

		if totalBytes < 1000 {
			t.Error("streamed audio too short")
		}
	})
}

// TestOpenAIIntegration tests real OpenAI TTS API.
// Run with: go test -tags=integration -v ./pkg/tts/...
func TestOpenAIIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")

	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	provider, err := tts.NewOpenAI(
		tts.WithAPIKey(apiKey),
		tts.WithVoice(tts.VoiceShimmer),
		tts.WithModel(tts.ModelTTS1),
	)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Health", func(t *testing.T) {
		if err := provider.Health(ctx); err != nil {
			t.Fatalf("health check failed: %v", err)
		}
		t.Log("✅ Health check passed")
	})

	t.Run("Synthesize", func(t *testing.T) {
		result, err := provider.Synthesize(ctx, "Hello from OpenAI.")
		if err != nil {
			t.Fatalf("synthesize failed: %v", err)
		}

		t.Logf("✅ Synthesized: %d bytes, latency: %dms", len(result.Audio), result.LatencyMs)

		if len(result.Audio) < 1000 {
			t.Error("audio too short, expected at least 1KB")
		}
		// OpenAI returns MP3
		if result.Format.Encoding != tts.EncodingMP3 {
			t.Errorf("expected MP3 encoding, got %s", result.Format.Encoding)
		}
	})
}

// TestChainIntegration tests provider chain with real APIs.
// Run with: go test -tags=integration -v ./pkg/tts/...
func TestChainIntegration(t *testing.T) {
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	elevenLabsVoice := os.Getenv("ELEVENLABS_VOICE_ID")
	openAIKey := os.Getenv("OPENAI_API_KEY")

	if openAIKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	var providers []tts.Provider

	// Add ElevenLabs if configured
	if elevenLabsKey != "" && elevenLabsVoice != "" {
		el, err := tts.NewElevenLabs(
			tts.WithAPIKey(elevenLabsKey),
			tts.WithVoice(elevenLabsVoice),
			tts.WithModel(tts.ModelTurboV2_5),
		)
		if err == nil {
			providers = append(providers, el)
			t.Log("✅ ElevenLabs added to chain")
		}
	}

	// Add OpenAI as fallback
	oai, err := tts.NewOpenAI(
		tts.WithAPIKey(openAIKey),
		tts.WithVoice(tts.VoiceShimmer),
	)
	if err != nil {
		t.Fatalf("failed to create OpenAI provider: %v", err)
	}
	providers = append(providers, oai)
	t.Log("✅ OpenAI added to chain")

	chain, err := tts.NewChain(providers...)
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}
	defer chain.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Chain synthesize", func(t *testing.T) {
		result, err := chain.Synthesize(ctx, "Testing provider chain.")
		if err != nil {
			t.Fatalf("synthesize failed: %v", err)
		}

		t.Logf("✅ Chain synthesized: %d bytes", len(result.Audio))
	})
}

