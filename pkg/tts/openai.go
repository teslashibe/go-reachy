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
	openAITTSURL   = "https://api.openai.com/v1/audio/speech"
	providerOpenAI = "openai"
)

// OpenAI voice options
const (
	VoiceAlloy   = "alloy"   // Neutral voice
	VoiceEcho    = "echo"    // Male voice
	VoiceFable   = "fable"   // British accent
	VoiceOnyx    = "onyx"    // Deep male voice
	VoiceNova    = "nova"    // Female voice
	VoiceShimmer = "shimmer" // Soft female voice
)

// OpenAI model options
const (
	ModelTTS1   = "tts-1"    // Standard quality, faster
	ModelTTS1HD = "tts-1-hd" // Higher quality, slower
)

// OpenAI implements Provider for OpenAI TTS.
type OpenAI struct {
	config  *Config
	client  *http.Client
	logger  *slog.Logger
	baseURL string
}

// NewOpenAI creates a new OpenAI TTS provider.
func NewOpenAI(opts ...Option) (*OpenAI, error) {
	cfg := DefaultConfig()
	cfg.ModelID = ModelTTS1
	cfg.VoiceID = VoiceShimmer
	cfg.OutputFormat = EncodingMP3
	cfg.Apply(opts...)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Default voice if not set
	if cfg.VoiceID == "" {
		cfg.VoiceID = VoiceShimmer
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = openAITTSURL
	}

	return &OpenAI{
		config:  cfg,
		client:  &http.Client{Timeout: cfg.Timeout},
		logger:  cfg.Logger.With("component", "tts.openai"),
		baseURL: baseURL,
	}, nil
}

// Synthesize converts text to audio, returning the complete audio buffer.
func (o *OpenAI) Synthesize(ctx context.Context, text string) (*AudioResult, error) {
	start := time.Now()

	payload := map[string]interface{}{
		"model": o.config.ModelID,
		"voice": o.config.VoiceID,
		"input": text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerOpenAI, fmt.Errorf("marshal payload: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerOpenAI, fmt.Errorf("create request: %w", err))
	}

	req.Header.Set("Authorization", "Bearer "+o.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.doWithRetry(ctx, req, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		return nil, o.parseError(resp)
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapError(providerOpenAI, fmt.Errorf("read response: %w", err))
	}

	o.logger.Debug("synthesized audio",
		"chars", len(text),
		"bytes", len(audio),
		"latency_ms", latency,
		"voice", o.config.VoiceID,
	)

	return &AudioResult{
		Audio:     audio,
		Format:    o.outputFormat(),
		CharCount: len(text),
		LatencyMs: latency,
	}, nil
}

// Stream converts text to audio with streaming output.
// Note: OpenAI TTS doesn't support true streaming, so this falls back to Synthesize.
func (o *OpenAI) Stream(ctx context.Context, text string) (AudioStream, error) {
	result, err := o.Synthesize(ctx, text)
	if err != nil {
		return nil, err
	}
	return &bufferStream{data: result.Audio, format: result.Format}, nil
}

// Health checks API connectivity.
func (o *OpenAI) Health(ctx context.Context) error {
	// Use models endpoint as health check
	url := "https://api.openai.com/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return WrapError(providerOpenAI, err)
	}

	req.Header.Set("Authorization", "Bearer "+o.config.APIKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return WrapError(providerOpenAI, fmt.Errorf("health check: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return o.parseError(resp)
	}

	return nil
}

// Close releases resources.
func (o *OpenAI) Close() error {
	o.client.CloseIdleConnections()
	return nil
}

// VoiceID returns the configured voice.
func (o *OpenAI) VoiceID() string {
	return o.config.VoiceID
}

// doWithRetry performs the request with retry logic.
func (o *OpenAI) doWithRetry(ctx context.Context, req *http.Request, body []byte) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= o.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(o.config.RetryDelay * time.Duration(attempt)):
			}

			// Reset body for retry
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := o.client.Do(req)
		if err != nil {
			lastErr = WrapError(providerOpenAI, err)
			continue
		}

		// Check if retryable
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = o.parseError(resp)
			o.logger.Warn("retrying request",
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
func (o *OpenAI) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse JSON error
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	message := string(body)
	code := ""
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		message = errResp.Error.Message
		code = errResp.Error.Code
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Code:       code,
		Provider:   providerOpenAI,
	}
}

// outputFormat returns the audio format configuration.
func (o *OpenAI) outputFormat() AudioFormat {
	// OpenAI TTS returns MP3 at 44.1kHz
	return AudioFormat{
		Encoding:   EncodingMP3,
		SampleRate: 44100,
		Channels:   1,
	}
}

// bufferStream wraps a byte slice as AudioStream.
type bufferStream struct {
	data   []byte
	offset int
	format AudioFormat
}

// Read returns the next audio chunk.
func (s *bufferStream) Read() ([]byte, error) {
	if s.offset >= len(s.data) {
		return nil, nil
	}
	// Return entire buffer at once (OpenAI doesn't stream)
	chunk := s.data[s.offset:]
	s.offset = len(s.data)
	return chunk, nil
}

// Close releases resources.
func (s *bufferStream) Close() error {
	return nil
}

// Format returns the audio format.
func (s *bufferStream) Format() AudioFormat {
	return s.format
}

// Verify OpenAI implements Provider at compile time.
var _ Provider = (*OpenAI)(nil)
