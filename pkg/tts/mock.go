package tts

import (
	"context"
	"sync"
	"time"
)

// Mock implements Provider for testing.
// All methods can be customized via function fields.
type Mock struct {
	// SynthesizeFunc is called when Synthesize is invoked.
	// If nil, returns silent audio of appropriate length.
	SynthesizeFunc func(ctx context.Context, text string) (*AudioResult, error)

	// StreamFunc is called when Stream is invoked.
	// If nil, returns an error.
	StreamFunc func(ctx context.Context, text string) (AudioStream, error)

	// HealthFunc is called when Health is invoked.
	// If nil, returns nil (healthy).
	HealthFunc func(ctx context.Context) error

	// CloseFunc is called when Close is invoked.
	// If nil, returns nil.
	CloseFunc func() error

	// Tracking
	mu    sync.Mutex
	calls []MockCall
}

// MockCall records a method invocation for verification.
type MockCall struct {
	Method string
	Text   string
	Time   time.Time
}

// NewMock creates a new mock provider with sensible defaults.
func NewMock() *Mock {
	return &Mock{
		SynthesizeFunc: func(ctx context.Context, text string) (*AudioResult, error) {
			// Generate silent audio (~20ms per character at 24kHz PCM16)
			// This gives roughly natural speech pacing
			bytesPerChar := 960 // ~20ms at 24kHz * 2 bytes per sample
			silence := make([]byte, len(text)*bytesPerChar)

			return &AudioResult{
				Audio: silence,
				Format: AudioFormat{
					Encoding:   EncodingPCM24,
					SampleRate: 24000,
					Channels:   1,
					BitDepth:   16,
				},
				CharCount: len(text),
				LatencyMs: 10,
				Duration:  time.Duration(len(text)) * 20 * time.Millisecond,
			}, nil
		},
		HealthFunc: func(ctx context.Context) error {
			return nil
		},
	}
}

// Synthesize calls SynthesizeFunc and records the call.
func (m *Mock) Synthesize(ctx context.Context, text string) (*AudioResult, error) {
	m.recordCall("Synthesize", text)
	if m.SynthesizeFunc != nil {
		return m.SynthesizeFunc(ctx, text)
	}
	return nil, WrapError("mock", ErrProviderUnavailable)
}

// Stream calls StreamFunc and records the call.
func (m *Mock) Stream(ctx context.Context, text string) (AudioStream, error) {
	m.recordCall("Stream", text)
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, text)
	}
	// Default: convert Synthesize result to stream
	if m.SynthesizeFunc != nil {
		result, err := m.SynthesizeFunc(ctx, text)
		if err != nil {
			return nil, err
		}
		return &bufferStream{data: result.Audio, format: result.Format}, nil
	}
	return nil, WrapError("mock", ErrProviderUnavailable)
}

// Health calls HealthFunc and records the call.
func (m *Mock) Health(ctx context.Context) error {
	m.recordCall("Health", "")
	if m.HealthFunc != nil {
		return m.HealthFunc(ctx)
	}
	return nil
}

// Close calls CloseFunc and records the call.
func (m *Mock) Close() error {
	m.recordCall("Close", "")
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// recordCall adds a call to the tracking list.
func (m *Mock) recordCall(method, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MockCall{
		Method: method,
		Text:   text,
		Time:   time.Now(),
	})
}

// Calls returns all recorded method calls.
func (m *Mock) Calls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockCall, len(m.calls))
	copy(result, m.calls)
	return result
}

// CallCount returns the number of times a method was called.
func (m *Mock) CallCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.calls {
		if c.Method == method {
			count++
		}
	}
	return count
}

// LastCall returns the most recent call, or nil if none.
func (m *Mock) LastCall() *MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	call := m.calls[len(m.calls)-1]
	return &call
}

// Reset clears all recorded calls.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
}

// WithError returns a mock that always returns the given error.
func WithError(err error) *Mock {
	return &Mock{
		SynthesizeFunc: func(ctx context.Context, text string) (*AudioResult, error) {
			return nil, err
		},
		StreamFunc: func(ctx context.Context, text string) (AudioStream, error) {
			return nil, err
		},
		HealthFunc: func(ctx context.Context) error {
			return err
		},
	}
}

// WithLatency wraps a mock to add artificial latency.
func WithLatency(m *Mock, delay time.Duration) *Mock {
	originalSynthesize := m.SynthesizeFunc
	m.SynthesizeFunc = func(ctx context.Context, text string) (*AudioResult, error) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if originalSynthesize != nil {
			return originalSynthesize(ctx, text)
		}
		return nil, WrapError("mock", ErrProviderUnavailable)
	}
	return m
}

// Verify Mock implements Provider at compile time.
var _ Provider = (*Mock)(nil)
