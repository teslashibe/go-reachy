package conversation

import (
	"context"
	"sync"
)

// Mock is a mock implementation of Provider for testing.
type Mock struct {
	mu sync.RWMutex

	// State
	connected bool
	tools     []Tool

	// Callbacks
	onAudio        func(audio []byte)
	onAudioDone    func()
	onTranscript   func(role, text string, isFinal bool)
	onToolCall     func(id, name string, args map[string]any)
	onError        func(err error)
	onInterruption func()

	// Configurable behavior
	ConnectFunc           func(ctx context.Context) error
	CloseFunc             func() error
	SendAudioFunc         func(audio []byte) error
	ConfigureSessionFunc  func(opts SessionOptions) error
	CancelResponseFunc    func() error
	SubmitToolResultFunc  func(callID, result string) error

	// Captured calls for assertions
	AudioSent        [][]byte
	SessionOptions   *SessionOptions
	ToolResults      map[string]string
	CancelCalled     bool
}

// NewMock creates a new Mock provider.
func NewMock() *Mock {
	return &Mock{
		ToolResults: make(map[string]string),
	}
}

// Connect implements Provider.
func (m *Mock) Connect(ctx context.Context) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

// Close implements Provider.
func (m *Mock) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

// IsConnected implements Provider.
func (m *Mock) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// SendAudio implements Provider.
func (m *Mock) SendAudio(audio []byte) error {
	if m.SendAudioFunc != nil {
		return m.SendAudioFunc(audio)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.AudioSent = append(m.AudioSent, audio)
	return nil
}

// OnAudio implements Provider.
func (m *Mock) OnAudio(fn func(audio []byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAudio = fn
}

// OnAudioDone implements Provider.
func (m *Mock) OnAudioDone(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAudioDone = fn
}

// OnTranscript implements Provider.
func (m *Mock) OnTranscript(fn func(role, text string, isFinal bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTranscript = fn
}

// OnToolCall implements Provider.
func (m *Mock) OnToolCall(fn func(id, name string, args map[string]any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onToolCall = fn
}

// OnError implements Provider.
func (m *Mock) OnError(fn func(err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onError = fn
}

// OnInterruption implements Provider.
func (m *Mock) OnInterruption(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onInterruption = fn
}

// ConfigureSession implements Provider.
func (m *Mock) ConfigureSession(opts SessionOptions) error {
	if m.ConfigureSessionFunc != nil {
		return m.ConfigureSessionFunc(opts)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SessionOptions = &opts
	return nil
}

// RegisterTool implements Provider.
func (m *Mock) RegisterTool(tool Tool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools = append(m.tools, tool)
}

// CancelResponse implements Provider.
func (m *Mock) CancelResponse() error {
	if m.CancelResponseFunc != nil {
		return m.CancelResponseFunc()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.CancelCalled = true
	return nil
}

// SubmitToolResult implements Provider.
func (m *Mock) SubmitToolResult(callID, result string) error {
	if m.SubmitToolResultFunc != nil {
		return m.SubmitToolResultFunc(callID, result)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.ToolResults[callID] = result
	return nil
}

// Capabilities implements Provider.
func (m *Mock) Capabilities() Capabilities {
	return Capabilities{
		SupportsToolCalls:    true,
		SupportsInterruption: true,
		SupportsCustomVoice:  true,
		SupportsStreaming:    true,
		InputSampleRate:      16000,
		OutputSampleRate:     16000,
		SupportedModels:      []string{"mock-model"},
	}
}

// Test helpers

// SimulateAudio triggers the OnAudio callback with the given audio.
func (m *Mock) SimulateAudio(audio []byte) {
	m.mu.RLock()
	fn := m.onAudio
	m.mu.RUnlock()
	if fn != nil {
		fn(audio)
	}
}

// SimulateAudioDone triggers the OnAudioDone callback.
func (m *Mock) SimulateAudioDone() {
	m.mu.RLock()
	fn := m.onAudioDone
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// SimulateTranscript triggers the OnTranscript callback.
func (m *Mock) SimulateTranscript(role, text string, isFinal bool) {
	m.mu.RLock()
	fn := m.onTranscript
	m.mu.RUnlock()
	if fn != nil {
		fn(role, text, isFinal)
	}
}

// SimulateToolCall triggers the OnToolCall callback.
func (m *Mock) SimulateToolCall(id, name string, args map[string]any) {
	m.mu.RLock()
	fn := m.onToolCall
	m.mu.RUnlock()
	if fn != nil {
		fn(id, name, args)
	}
}

// SimulateError triggers the OnError callback.
func (m *Mock) SimulateError(err error) {
	m.mu.RLock()
	fn := m.onError
	m.mu.RUnlock()
	if fn != nil {
		fn(err)
	}
}

// SimulateInterruption triggers the OnInterruption callback.
func (m *Mock) SimulateInterruption() {
	m.mu.RLock()
	fn := m.onInterruption
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// Reset clears all captured data.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AudioSent = nil
	m.SessionOptions = nil
	m.ToolResults = make(map[string]string)
	m.CancelCalled = false
	m.tools = nil
}

// GetTools returns the registered tools.
func (m *Mock) GetTools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Tool{}, m.tools...)
}

// Ensure Mock implements Provider.
var _ Provider = (*Mock)(nil)


