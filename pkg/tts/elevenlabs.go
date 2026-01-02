package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	elevenLabsBaseURL  = "https://api.elevenlabs.io/v1"
	providerElevenLabs = "elevenlabs"
)

// ElevenLabs model IDs
const (
	// ModelTurboV2_5 is the fastest English model (~200ms latency).
	ModelTurboV2_5 = "eleven_turbo_v2_5"

	// ModelFlashV2_5 is the fastest multilingual model (~150ms latency).
	ModelFlashV2_5 = "eleven_flash_v2_5"

	// ModelMultilingualV2 is the highest quality multilingual model (~300ms latency).
	ModelMultilingualV2 = "eleven_multilingual_v2"

	// ModelMonolingualV1 is the legacy English model.
	ModelMonolingualV1 = "eleven_monolingual_v1"
)

// ElevenLabs implements Provider for ElevenLabs TTS.
type ElevenLabs struct {
	config  *Config
	client  *http.Client
	logger  *slog.Logger
	baseURL string
}

// NewElevenLabs creates a new ElevenLabs TTS provider.
func NewElevenLabs(opts ...Option) (*ElevenLabs, error) {
	cfg := DefaultConfig()
	cfg.Apply(opts...)

	if err := cfg.ValidateWithVoice(); err != nil {
		return nil, err
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = elevenLabsBaseURL
	}

	return &ElevenLabs{
		config:  cfg,
		client:  &http.Client{Timeout: cfg.Timeout},
		logger:  cfg.Logger.With("component", "tts.elevenlabs"),
		baseURL: baseURL,
	}, nil
}

// Synthesize converts text to audio, returning the complete audio buffer.
func (e *ElevenLabs) Synthesize(ctx context.Context, text string) (*AudioResult, error) {
	start := time.Now()

	url := fmt.Sprintf("%s/text-to-speech/%s", e.baseURL, e.config.VoiceID)

	payload := e.buildPayload(text)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerElevenLabs, fmt.Errorf("marshal payload: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerElevenLabs, fmt.Errorf("create request: %w", err))
	}

	e.setHeaders(req)

	resp, err := e.doWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		return nil, e.parseError(resp)
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapError(providerElevenLabs, fmt.Errorf("read response: %w", err))
	}

	e.logger.Debug("synthesized audio",
		"chars", len(text),
		"bytes", len(audio),
		"latency_ms", latency,
		"model", e.config.ModelID,
	)

	return &AudioResult{
		Audio:     audio,
		Format:    e.outputFormat(),
		CharCount: len(text),
		LatencyMs: latency,
		Duration:  e.estimateDuration(len(audio)),
	}, nil
}

// Stream converts text to audio with streaming output for lowest latency.
func (e *ElevenLabs) Stream(ctx context.Context, text string) (AudioStream, error) {
	url := fmt.Sprintf("%s/text-to-speech/%s/stream", e.baseURL, e.config.VoiceID)

	payload := e.buildPayload(text)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerElevenLabs, fmt.Errorf("marshal payload: %w", err))
	}

	// Use stream timeout for streaming requests
	client := &http.Client{Timeout: e.config.StreamTimeout}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerElevenLabs, fmt.Errorf("create request: %w", err))
	}

	e.setHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, WrapError(providerElevenLabs, fmt.Errorf("stream request: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, e.parseError(resp)
	}

	return &httpStream{
		body:   resp.Body,
		format: e.outputFormat(),
	}, nil
}

// Health checks API connectivity and API key validity.
func (e *ElevenLabs) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/user", e.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return WrapError(providerElevenLabs, err)
	}

	req.Header.Set("xi-api-key", e.config.APIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return WrapError(providerElevenLabs, fmt.Errorf("health check: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return e.parseError(resp)
	}

	return nil
}

// Close releases resources held by the provider.
func (e *ElevenLabs) Close() error {
	e.client.CloseIdleConnections()
	return nil
}

// VoiceID returns the configured voice ID.
func (e *ElevenLabs) VoiceID() string {
	return e.config.VoiceID
}

// ModelID returns the configured model ID.
func (e *ElevenLabs) ModelID() string {
	return e.config.ModelID
}

// buildPayload constructs the API request payload.
func (e *ElevenLabs) buildPayload(text string) map[string]interface{} {
	return map[string]interface{}{
		"text":     text,
		"model_id": e.config.ModelID,
		"voice_settings": map[string]interface{}{
			"stability":         e.config.VoiceSettings.Stability,
			"similarity_boost":  e.config.VoiceSettings.SimilarityBoost,
			"style":             e.config.VoiceSettings.Style,
			"use_speaker_boost": e.config.VoiceSettings.SpeakerBoost,
		},
	}
}

// setHeaders sets required HTTP headers.
func (e *ElevenLabs) setHeaders(req *http.Request) {
	req.Header.Set("xi-api-key", e.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", e.formatToMIME())
}

// doWithRetry performs the request with retry logic.
func (e *ElevenLabs) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(e.config.RetryDelay * time.Duration(attempt)):
			}

			// Clone the request for retry (body needs to be re-readable)
			body, _ := io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = WrapError(providerElevenLabs, err)
			continue
		}

		// Check if retryable
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = e.parseError(resp)
			e.logger.Warn("retrying request",
				"attempt", attempt+1,
				"status", resp.StatusCode,
			)
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}

// parseError reads and parses an error response.
func (e *ElevenLabs) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse JSON error
	var errResp struct {
		Detail struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"detail"`
	}

	message := string(body)
	if json.Unmarshal(body, &errResp) == nil && errResp.Detail.Message != "" {
		message = errResp.Detail.Message
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Provider:   providerElevenLabs,
	}
}

// outputFormat returns the audio format configuration.
func (e *ElevenLabs) outputFormat() AudioFormat {
	return AudioFormat{
		Encoding:   e.config.OutputFormat,
		SampleRate: SampleRateFromEncoding(e.config.OutputFormat),
		Channels:   1,
		BitDepth:   16,
	}
}

// formatToMIME converts the encoding to MIME type.
func (e *ElevenLabs) formatToMIME() string {
	switch e.config.OutputFormat {
	case EncodingMP3:
		return "audio/mpeg"
	case EncodingPCM16, EncodingPCM22, EncodingPCM24, EncodingPCM44:
		return "audio/pcm"
	case EncodingOpus:
		return "audio/opus"
	case EncodingULaw:
		return "audio/basic"
	default:
		return "audio/mpeg"
	}
}

// estimateDuration estimates audio duration from byte count.
func (e *ElevenLabs) estimateDuration(bytes int) time.Duration {
	sampleRate := SampleRateFromEncoding(e.config.OutputFormat)
	// PCM16 = 2 bytes per sample
	samples := bytes / 2
	seconds := float64(samples) / float64(sampleRate)
	return time.Duration(seconds * float64(time.Second))
}

// httpStream wraps an HTTP response body as AudioStream.
type httpStream struct {
	body   io.ReadCloser
	format AudioFormat
	buf    [4096]byte
}

// Read returns the next audio chunk.
func (s *httpStream) Read() ([]byte, error) {
	n, err := s.body.Read(s.buf[:])
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	chunk := make([]byte, n)
	copy(chunk, s.buf[:n])
	return chunk, nil
}

// Close stops the stream.
func (s *httpStream) Close() error {
	return s.body.Close()
}

// Format returns the audio format.
func (s *httpStream) Format() AudioFormat {
	return s.format
}

// Verify ElevenLabs implements Provider at compile time.
var _ Provider = (*ElevenLabs)(nil)

