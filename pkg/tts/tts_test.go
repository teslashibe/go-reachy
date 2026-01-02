package tts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teslashibe/go-reachy/pkg/tts"
)

func TestMockProvider(t *testing.T) {
	mock := tts.NewMock()
	ctx := context.Background()

	t.Run("Synthesize returns audio", func(t *testing.T) {
		result, err := mock.Synthesize(ctx, "Hello world")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Audio) == 0 {
			t.Error("expected audio data")
		}
		if result.CharCount != 11 {
			t.Errorf("expected 11 chars, got %d", result.CharCount)
		}
		if result.Format.SampleRate != 24000 {
			t.Errorf("expected 24000 sample rate, got %d", result.Format.SampleRate)
		}
	})

	t.Run("Stream returns audio stream", func(t *testing.T) {
		stream, err := mock.Stream(ctx, "Test stream")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer stream.Close()

		chunk, err := stream.Read()
		if err != nil {
			t.Fatalf("read error: %v", err)
		}
		if len(chunk) == 0 {
			t.Error("expected audio chunk")
		}
	})

	t.Run("Health returns nil", func(t *testing.T) {
		if err := mock.Health(ctx); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Calls are tracked", func(t *testing.T) {
		calls := mock.Calls()
		if len(calls) != 3 {
			t.Errorf("expected 3 calls, got %d", len(calls))
		}
		if mock.CallCount("Synthesize") != 1 {
			t.Errorf("expected 1 Synthesize call, got %d", mock.CallCount("Synthesize"))
		}
	})

	t.Run("Reset clears calls", func(t *testing.T) {
		mock.Reset()
		if len(mock.Calls()) != 0 {
			t.Error("expected calls to be cleared")
		}
	})
}

func TestMockWithError(t *testing.T) {
	testErr := errors.New("test error")
	mock := tts.WithError(testErr)
	ctx := context.Background()

	t.Run("Synthesize returns error", func(t *testing.T) {
		_, err := mock.Synthesize(ctx, "Hello")
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, testErr) {
			t.Errorf("expected test error, got %v", err)
		}
	})

	t.Run("Stream returns error", func(t *testing.T) {
		_, err := mock.Stream(ctx, "Hello")
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("Health returns error", func(t *testing.T) {
		err := mock.Health(ctx)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestMockWithLatency(t *testing.T) {
	mock := tts.NewMock()
	mock = tts.WithLatency(mock, 50*time.Millisecond)
	ctx := context.Background()

	t.Run("Synthesize has latency", func(t *testing.T) {
		start := time.Now()
		_, err := mock.Synthesize(ctx, "Hello")
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed < 50*time.Millisecond {
			t.Errorf("expected at least 50ms latency, got %v", elapsed)
		}
	})

	t.Run("Context cancellation works", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, err := mock.Synthesize(ctx, "Hello")
		if err == nil {
			t.Error("expected context deadline error")
		}
	})
}

func TestDefaultVoiceSettings(t *testing.T) {
	settings := tts.DefaultVoiceSettings()

	if settings.Stability != 0.5 {
		t.Errorf("expected stability 0.5, got %f", settings.Stability)
	}
	if settings.SimilarityBoost != 0.75 {
		t.Errorf("expected similarity 0.75, got %f", settings.SimilarityBoost)
	}
	if settings.Style != 0.0 {
		t.Errorf("expected style 0.0, got %f", settings.Style)
	}
	if !settings.SpeakerBoost {
		t.Error("expected speaker boost true")
	}
}

func TestFunctionalOptions(t *testing.T) {
	cfg := tts.DefaultConfig()
	cfg.Apply(
		tts.WithVoice("test-voice"),
		tts.WithModel("test-model"),
		tts.WithTimeout(5*time.Second),
		tts.WithOutputFormat(tts.EncodingMP3),
	)

	if cfg.VoiceID != "test-voice" {
		t.Errorf("expected voice test-voice, got %s", cfg.VoiceID)
	}
	if cfg.ModelID != "test-model" {
		t.Errorf("expected model test-model, got %s", cfg.ModelID)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", cfg.Timeout)
	}
	if cfg.OutputFormat != tts.EncodingMP3 {
		t.Errorf("expected MP3 format, got %s", cfg.OutputFormat)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("Validate requires API key", func(t *testing.T) {
		cfg := tts.DefaultConfig()
		if err := cfg.Validate(); err != tts.ErrNoAPIKey {
			t.Errorf("expected ErrNoAPIKey, got %v", err)
		}
	})

	t.Run("Validate passes with API key", func(t *testing.T) {
		cfg := tts.DefaultConfig()
		cfg.APIKey = "test-key"
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ValidateWithVoice requires voice", func(t *testing.T) {
		cfg := tts.DefaultConfig()
		cfg.APIKey = "test-key"
		if err := cfg.ValidateWithVoice(); err != tts.ErrNoVoiceID {
			t.Errorf("expected ErrNoVoiceID, got %v", err)
		}
	})

	t.Run("ValidateWithVoice passes with both", func(t *testing.T) {
		cfg := tts.DefaultConfig()
		cfg.APIKey = "test-key"
		cfg.VoiceID = "test-voice"
		if err := cfg.ValidateWithVoice(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestAPIError(t *testing.T) {
	t.Run("IsRateLimited", func(t *testing.T) {
		err := &tts.APIError{StatusCode: 429, Message: "rate limited"}
		if !err.IsRateLimited() {
			t.Error("expected IsRateLimited true")
		}
		if err.IsUnauthorized() {
			t.Error("expected IsUnauthorized false")
		}
	})

	t.Run("IsUnauthorized", func(t *testing.T) {
		err := &tts.APIError{StatusCode: 401, Message: "unauthorized"}
		if !err.IsUnauthorized() {
			t.Error("expected IsUnauthorized true")
		}
	})

	t.Run("IsServerError", func(t *testing.T) {
		for _, code := range []int{500, 502, 503, 504} {
			err := &tts.APIError{StatusCode: code}
			if !err.IsServerError() {
				t.Errorf("expected IsServerError true for %d", code)
			}
			if !err.IsRetryable() {
				t.Errorf("expected IsRetryable true for %d", code)
			}
		}
	})

	t.Run("Error message format", func(t *testing.T) {
		err := &tts.APIError{
			StatusCode: 400,
			Message:    "bad request",
			Code:       "invalid_input",
			Provider:   "elevenlabs",
		}
		msg := err.Error()
		if msg != "tts [elevenlabs]: API error 400 (invalid_input): bad request" {
			t.Errorf("unexpected error message: %s", msg)
		}
	})
}

func TestSampleRateFromEncoding(t *testing.T) {
	tests := []struct {
		encoding   tts.Encoding
		sampleRate int
	}{
		{tts.EncodingPCM16, 16000},
		{tts.EncodingPCM22, 22050},
		{tts.EncodingPCM24, 24000},
		{tts.EncodingPCM44, 44100},
		{tts.EncodingMP3, 44100},
		{tts.EncodingULaw, 8000},
	}

	for _, tt := range tests {
		t.Run(string(tt.encoding), func(t *testing.T) {
			rate := tts.SampleRateFromEncoding(tt.encoding)
			if rate != tt.sampleRate {
				t.Errorf("expected %d, got %d", tt.sampleRate, rate)
			}
		})
	}
}

func TestChain(t *testing.T) {
	ctx := context.Background()

	t.Run("NewChain requires providers", func(t *testing.T) {
		_, err := tts.NewChain()
		if err != tts.ErrProviderUnavailable {
			t.Errorf("expected ErrProviderUnavailable, got %v", err)
		}
	})

	t.Run("First provider succeeds", func(t *testing.T) {
		mock1 := tts.NewMock()
		mock2 := tts.NewMock()

		chain, err := tts.NewChain(mock1, mock2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer chain.Close()

		_, err = chain.Synthesize(ctx, "Hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only first provider should be called
		if mock1.CallCount("Synthesize") != 1 {
			t.Error("expected first provider to be called")
		}
		if mock2.CallCount("Synthesize") != 0 {
			t.Error("expected second provider not to be called")
		}
	})

	t.Run("Fallback on failure", func(t *testing.T) {
		failMock := tts.WithError(errors.New("provider 1 failed"))
		successMock := tts.NewMock()

		chain, err := tts.NewChain(failMock, successMock)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer chain.Close()

		result, err := chain.Synthesize(ctx, "Hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected result from fallback provider")
		}
	})

	t.Run("All providers fail", func(t *testing.T) {
		fail1 := tts.WithError(errors.New("fail 1"))
		fail2 := tts.WithError(errors.New("fail 2"))

		chain, err := tts.NewChain(fail1, fail2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer chain.Close()

		_, err = chain.Synthesize(ctx, "Hello")
		if err == nil {
			t.Error("expected error when all providers fail")
		}
	})

	t.Run("Health checks all providers", func(t *testing.T) {
		mock1 := tts.NewMock()
		mock2 := tts.NewMock()

		chain, err := tts.NewChain(mock1, mock2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = chain.Health(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestProviderError(t *testing.T) {
	inner := errors.New("connection failed")
	err := tts.WrapError("elevenlabs", inner)

	if err == nil {
		t.Fatal("expected error")
	}

	if err.Error() != "tts [elevenlabs]: connection failed" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// Unwrap should return inner error
	var pe *tts.ProviderError
	if !errors.As(err, &pe) {
		t.Error("expected ProviderError")
	}
	if pe.Provider != "elevenlabs" {
		t.Errorf("expected provider elevenlabs, got %s", pe.Provider)
	}
}
