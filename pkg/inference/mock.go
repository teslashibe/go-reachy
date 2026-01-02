package inference

import (
	"context"
	"sync"
	"time"
)

// Mock implements Provider for testing.
type Mock struct {
	// ChatFunc is called when Chat is invoked.
	ChatFunc func(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// StreamFunc is called when Stream is invoked.
	StreamFunc func(ctx context.Context, req *ChatRequest) (Stream, error)

	// VisionFunc is called when Vision is invoked.
	VisionFunc func(ctx context.Context, req *VisionRequest) (*VisionResponse, error)

	// EmbedFunc is called when Embed is invoked.
	EmbedFunc func(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)

	// HealthFunc is called when Health is invoked.
	HealthFunc func(ctx context.Context) error

	// CloseFunc is called when Close is invoked.
	CloseFunc func() error

	// CapabilitiesOverride overrides default capabilities.
	CapabilitiesOverride *Capabilities

	mu    sync.Mutex
	calls []MockCall
}

// MockCall records a method invocation.
type MockCall struct {
	Method string
	Time   time.Time
}

// NewMock creates a new mock provider with sensible defaults.
func NewMock() *Mock {
	return &Mock{
		ChatFunc: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			return &ChatResponse{
				Message:      NewAssistantMessage("Mock response"),
				FinishReason: "stop",
				Usage:        Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			}, nil
		},
		VisionFunc: func(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
			return &VisionResponse{
				Content: "I see a mock image",
				Usage:   Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
			}, nil
		},
		EmbedFunc: func(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
			embeddings := make([][]float64, len(req.Input))
			for i := range embeddings {
				embeddings[i] = make([]float64, 256) // Mock 256-dim embeddings
			}
			return &EmbedResponse{
				Embeddings: embeddings,
				Usage:      Usage{PromptTokens: 10, TotalTokens: 10},
			}, nil
		},
		HealthFunc: func(ctx context.Context) error {
			return nil
		},
	}
}

// Chat calls ChatFunc and records the call.
func (m *Mock) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	m.record("Chat")
	if m.ChatFunc != nil {
		return m.ChatFunc(ctx, req)
	}
	return nil, WrapError("mock", ErrProviderUnavailable)
}

// Stream calls StreamFunc and records the call.
func (m *Mock) Stream(ctx context.Context, req *ChatRequest) (Stream, error) {
	m.record("Stream")
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, req)
	}
	// Default: return a mock stream with the chat response
	if m.ChatFunc != nil {
		resp, err := m.ChatFunc(ctx, req)
		if err != nil {
			return nil, err
		}
		return &mockStream{content: resp.Message.Content}, nil
	}
	return nil, WrapError("mock", ErrProviderUnavailable)
}

// Vision calls VisionFunc and records the call.
func (m *Mock) Vision(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
	m.record("Vision")
	if m.VisionFunc != nil {
		return m.VisionFunc(ctx, req)
	}
	return nil, WrapError("mock", ErrVisionNotSupported)
}

// Embed calls EmbedFunc and records the call.
func (m *Mock) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	m.record("Embed")
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, req)
	}
	return nil, WrapError("mock", ErrEmbeddingsNotSupported)
}

// Capabilities returns mock capabilities.
func (m *Mock) Capabilities() Capabilities {
	if m.CapabilitiesOverride != nil {
		return *m.CapabilitiesOverride
	}
	return Capabilities{
		Chat:       m.ChatFunc != nil,
		Vision:     m.VisionFunc != nil,
		Streaming:  m.StreamFunc != nil || m.ChatFunc != nil,
		Tools:      true,
		Embeddings: m.EmbedFunc != nil,
	}
}

// Health calls HealthFunc and records the call.
func (m *Mock) Health(ctx context.Context) error {
	m.record("Health")
	if m.HealthFunc != nil {
		return m.HealthFunc(ctx)
	}
	return nil
}

// Close calls CloseFunc and records the call.
func (m *Mock) Close() error {
	m.record("Close")
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// record adds a call to the tracking list.
func (m *Mock) record(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MockCall{
		Method: method,
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
		ChatFunc: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			return nil, err
		},
		StreamFunc: func(ctx context.Context, req *ChatRequest) (Stream, error) {
			return nil, err
		},
		VisionFunc: func(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
			return nil, err
		},
		EmbedFunc: func(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
			return nil, err
		},
		HealthFunc: func(ctx context.Context) error {
			return err
		},
	}
}

// mockStream is a simple stream that returns content then closes.
type mockStream struct {
	content string
	done    bool
}

func (s *mockStream) Recv() (*StreamChunk, error) {
	if s.done {
		return &StreamChunk{Done: true}, nil
	}
	s.done = true
	return &StreamChunk{
		Delta:        s.content,
		FinishReason: "stop",
		Done:         true,
	}, nil
}

func (s *mockStream) Close() error {
	return nil
}

// Verify Mock implements Provider at compile time.
var _ Provider = (*Mock)(nil)
