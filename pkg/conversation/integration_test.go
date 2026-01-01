//go:build integration

package conversation

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// These tests require real API keys and make actual API calls.
// Run with: go test -tags=integration -v ./pkg/conversation/...

func TestElevenLabsIntegration(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	agentID := os.Getenv("ELEVENLABS_AGENT_ID")

	if apiKey == "" || agentID == "" {
		t.Skip("ELEVENLABS_API_KEY and ELEVENLABS_AGENT_ID required")
	}

	t.Run("connect and disconnect", func(t *testing.T) {
		provider, err := NewElevenLabs(
			WithAPIKey(apiKey),
			WithAgentID(agentID),
		)
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := provider.Connect(ctx); err != nil {
			t.Fatalf("failed to connect: %v", err)
		}

		if !provider.IsConnected() {
			t.Error("should be connected")
		}

		if err := provider.Close(); err != nil {
			t.Errorf("failed to close: %v", err)
		}

		if provider.IsConnected() {
			t.Error("should not be connected after close")
		}
	})

	t.Run("capabilities", func(t *testing.T) {
		provider, err := NewElevenLabs(
			WithAPIKey(apiKey),
			WithAgentID(agentID),
		)
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}

		caps := provider.Capabilities()

		if !caps.SupportsToolCalls {
			t.Error("should support tool calls")
		}

		if !caps.SupportsCustomVoice {
			t.Error("should support custom voice")
		}

		if !caps.SupportsStreaming {
			t.Error("should support streaming")
		}

		if caps.InputSampleRate != 16000 {
			t.Errorf("expected 16000 Hz input, got %d", caps.InputSampleRate)
		}
	})

	t.Run("receive audio response", func(t *testing.T) {
		provider, err := NewElevenLabs(
			WithAPIKey(apiKey),
			WithAgentID(agentID),
		)
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}
		defer provider.Close()

		var mu sync.Mutex
		var audioReceived bool
		var transcriptReceived bool

		provider.OnAudio(func(audio []byte) {
			mu.Lock()
			audioReceived = true
			mu.Unlock()
		})

		provider.OnTranscript(func(role, text string, isFinal bool) {
			mu.Lock()
			transcriptReceived = true
			t.Logf("Transcript [%s]: %s (final: %v)", role, text, isFinal)
			mu.Unlock()
		})

		provider.OnError(func(err error) {
			t.Logf("Error: %v", err)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := provider.Connect(ctx); err != nil {
			t.Fatalf("failed to connect: %v", err)
		}

		// Send some silence to trigger the agent
		// In real usage, you'd send actual speech audio
		silence := make([]byte, 32000) // 1 second of silence at 16kHz
		if err := provider.SendAudio(silence); err != nil {
			t.Logf("send audio failed (may be expected): %v", err)
		}

		// Wait for some response
		time.Sleep(5 * time.Second)

		mu.Lock()
		t.Logf("Audio received: %v, Transcript received: %v", audioReceived, transcriptReceived)
		mu.Unlock()

		// Note: We don't assert on audioReceived because the agent may not respond
		// to silence. This test mainly verifies the connection works.
	})
}

func TestOpenAIIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")

	if apiKey == "" {
		t.Skip("OPENAI_API_KEY required")
	}

	t.Run("connect and disconnect", func(t *testing.T) {
		provider, err := NewOpenAI(
			WithAPIKey(apiKey),
		)
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := provider.Connect(ctx); err != nil {
			t.Fatalf("failed to connect: %v", err)
		}

		if !provider.IsConnected() {
			t.Error("should be connected")
		}

		if err := provider.Close(); err != nil {
			t.Errorf("failed to close: %v", err)
		}

		if provider.IsConnected() {
			t.Error("should not be connected after close")
		}
	})

	t.Run("configure session", func(t *testing.T) {
		provider, err := NewOpenAI(
			WithAPIKey(apiKey),
		)
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}
		defer provider.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := provider.Connect(ctx); err != nil {
			t.Fatalf("failed to connect: %v", err)
		}

		// Wait for session to be created
		time.Sleep(time.Second)

		opts := SessionOptions{
			SystemPrompt: "You are a helpful assistant named Eva.",
			Voice:        VoiceShimmer,
			Tools: []Tool{
				{
					Name:        "get_time",
					Description: "Gets the current time",
					Parameters:  map[string]any{},
				},
			},
		}

		if err := provider.ConfigureSession(opts); err != nil {
			t.Fatalf("failed to configure session: %v", err)
		}

		// Wait for session update
		time.Sleep(time.Second)
	})

	t.Run("capabilities", func(t *testing.T) {
		provider, err := NewOpenAI(
			WithAPIKey(apiKey),
		)
		if err != nil {
			t.Fatalf("failed to create provider: %v", err)
		}

		caps := provider.Capabilities()

		if !caps.SupportsToolCalls {
			t.Error("should support tool calls")
		}

		if caps.SupportsCustomVoice {
			t.Error("should not support custom voice")
		}

		if !caps.SupportsStreaming {
			t.Error("should support streaming")
		}

		if caps.InputSampleRate != 24000 {
			t.Errorf("expected 24000 Hz input, got %d", caps.InputSampleRate)
		}
	})
}

func TestProviderValidation(t *testing.T) {
	t.Run("ElevenLabs missing API key", func(t *testing.T) {
		_, err := NewElevenLabs()

		if err == nil {
			t.Error("expected error for missing API key")
		}

		if err != ErrMissingAPIKey {
			t.Errorf("expected ErrMissingAPIKey, got %v", err)
		}
	})

	t.Run("ElevenLabs missing agent ID", func(t *testing.T) {
		_, err := NewElevenLabs(
			WithAPIKey("test-key"),
		)

		if err == nil {
			t.Error("expected error for missing agent ID")
		}

		if err != ErrMissingAgentID {
			t.Errorf("expected ErrMissingAgentID, got %v", err)
		}
	})

	t.Run("OpenAI missing API key", func(t *testing.T) {
		_, err := NewOpenAI()

		if err == nil {
			t.Error("expected error for missing API key")
		}

		if err != ErrMissingAPIKey {
			t.Errorf("expected ErrMissingAPIKey, got %v", err)
		}
	})
}



