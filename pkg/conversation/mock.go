package conversation

import (
	"context"
	"sync"
)

// Mock is a mock implementation of Provider for testing.
type Mock struct {
	mu        sync.RWMutex
	connected bool
	tools     []Tool

	// Recorded data
	AudioSent      [][]byte
	ToolResults    map[string]string
	CancelCalled   bool
	SessionOptions *SessionOptions

	// Callbacks
	onAudio        func([]byte)
	onAudioDone    func()
	onTranscript   func(role, text string, isFinal bool)
	onToolCall     func(id, name string, args map[string]any)
	onError        func(error)
	onInterruption func()
}

// NewMock creates a new mock provider for testing.
func NewMock() *Mock {
	return &Mock{
		ToolResults: make(map[string]string),
	}
}

// Connect simulates connecting.
func (m *Mock) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

// Close simulates disconnecting.
func (m *Mock) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

// IsConnected returns the connection state.
func (m *Mock) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// SendAudio records sent audio.
func (m *Mock) SendAudio(audio []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.AudioSent = append(m.AudioSent, audio)
	return nil
}

// ConfigureSession stores session options.
func (m *Mock) ConfigureSession(opts SessionOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SessionOptions = &opts
	return nil
}

// RegisterTool adds a tool.
func (m *Mock) RegisterTool(tool Tool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools = append(m.tools, tool)
}

// GetTools returns registered tools.
func (m *Mock) GetTools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools
}

// CancelResponse records cancellation.
func (m *Mock) CancelResponse() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.CancelCalled = true
	return nil
}

// SubmitToolResult records tool results.
func (m *Mock) SubmitToolResult(callID, result string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.ToolResults[callID] = result
	return nil
}

// Capabilities returns mock capabilities.
func (m *Mock) Capabilities() Capabilities {
	return Capabilities{
		SupportsToolCalls:    true,
		SupportsInterruption: true,
		SupportsCustomVoice:  true,
		SupportsStreaming:    true,
		InputSampleRate:      16000,
		OutputSampleRate:     16000,
	}
}

// OnAudio sets the audio callback.
func (m *Mock) OnAudio(fn func([]byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAudio = fn
}

// OnAudioDone sets the audio done callback.
func (m *Mock) OnAudioDone(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAudioDone = fn
}

// OnTranscript sets the transcript callback.
func (m *Mock) OnTranscript(fn func(role, text string, isFinal bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTranscript = fn
}

// OnToolCall sets the tool call callback.
func (m *Mock) OnToolCall(fn func(id, name string, args map[string]any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onToolCall = fn
}

// OnError sets the error callback.
func (m *Mock) OnError(fn func(error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onError = fn
}

// OnInterruption sets the interruption callback.
func (m *Mock) OnInterruption(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onInterruption = fn
}

// SimulateAudio triggers the audio callback.
func (m *Mock) SimulateAudio(audio []byte) {
	m.mu.RLock()
	fn := m.onAudio
	m.mu.RUnlock()
	if fn != nil {
		fn(audio)
	}
}

// SimulateAudioDone triggers the audio done callback.
func (m *Mock) SimulateAudioDone() {
	m.mu.RLock()
	fn := m.onAudioDone
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// SimulateTranscript triggers the transcript callback.
func (m *Mock) SimulateTranscript(role, text string, isFinal bool) {
	m.mu.RLock()
	fn := m.onTranscript
	m.mu.RUnlock()
	if fn != nil {
		fn(role, text, isFinal)
	}
}

// SimulateToolCall triggers the tool call callback.
func (m *Mock) SimulateToolCall(id, name string, args map[string]any) {
	m.mu.RLock()
	fn := m.onToolCall
	m.mu.RUnlock()
	if fn != nil {
		fn(id, name, args)
	}
}

// SimulateError triggers the error callback.
func (m *Mock) SimulateError(err error) {
	m.mu.RLock()
	fn := m.onError
	m.mu.RUnlock()
	if fn != nil {
		fn(err)
	}
}

// SimulateInterruption triggers the interruption callback.
func (m *Mock) SimulateInterruption() {
	m.mu.RLock()
	fn := m.onInterruption
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// Reset clears all recorded data.
func (m *Mock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AudioSent = nil
	m.ToolResults = make(map[string]string)
	m.CancelCalled = false
}

// Ensure Mock implements Provider
var _ Provider = (*Mock)(nil)




