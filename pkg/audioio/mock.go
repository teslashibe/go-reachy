package audioio

import (
	"context"
	"io"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// MockSource is a mock audio source for testing.
// It generates synthetic audio (silence or sine wave).
type MockSource struct {
	cfg    Config
	logger *slog.Logger

	mu       sync.Mutex
	running  bool
	closed   bool
	streamCh chan AudioChunk
	stopCh   chan struct{}

	// Stats
	chunksRead  atomic.Int64
	samplesRead atomic.Int64

	// Synthetic audio generation
	phase      float64
	frequency  float64 // Hz, 0 = silence
	amplitude  float64 // 0.0 to 1.0
}

// MockSourceOption configures a MockSource.
type MockSourceOption func(*MockSource)

// WithSineWave configures the mock to generate a sine wave.
func WithSineWave(frequency, amplitude float64) MockSourceOption {
	return func(m *MockSource) {
		m.frequency = frequency
		m.amplitude = amplitude
	}
}

// NewMockSource creates a new mock audio source.
func NewMockSource(cfg Config, logger *slog.Logger, opts ...MockSourceOption) *MockSource {
	if logger == nil {
		logger = slog.Default()
	}

	m := &MockSource{
		cfg:       cfg,
		logger:    logger,
		streamCh:  make(chan AudioChunk, 10),
		stopCh:    make(chan struct{}),
		frequency: 0,    // Silence by default
		amplitude: 0.5,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start begins generating audio.
func (m *MockSource) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return io.ErrClosedPipe
	}
	if m.running {
		return nil
	}

	m.running = true
	m.stopCh = make(chan struct{})
	m.streamCh = make(chan AudioChunk, 10)

	go m.generateLoop(ctx)

	m.logger.Info("mock audio source started",
		"sample_rate", m.cfg.SampleRate,
		"frequency", m.frequency,
	)

	return nil
}

func (m *MockSource) generateLoop(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.BufferDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.Stop()
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			chunk := m.generateChunk()
			select {
			case m.streamCh <- chunk:
				m.chunksRead.Add(1)
				m.samplesRead.Add(int64(len(chunk.Samples)))
			default:
				// Buffer full, drop chunk (overrun)
				m.logger.Debug("mock source: buffer full, dropping chunk")
			}
		}
	}
}

func (m *MockSource) generateChunk() AudioChunk {
	bufferSize := m.cfg.BufferSize()
	samples := make([]int16, bufferSize*m.cfg.Channels)

	if m.frequency > 0 {
		// Generate sine wave
		for i := 0; i < bufferSize; i++ {
			sample := m.amplitude * math.Sin(2*math.Pi*m.frequency*m.phase/float64(m.cfg.SampleRate))
			sampleInt := int16(sample * 32767)

			for ch := 0; ch < m.cfg.Channels; ch++ {
				samples[i*m.cfg.Channels+ch] = sampleInt
			}

			m.phase++
			if m.phase >= float64(m.cfg.SampleRate) {
				m.phase = 0
			}
		}
	}
	// else: samples are already zero (silence)

	return AudioChunk{
		Samples:    samples,
		SampleRate: m.cfg.SampleRate,
		Channels:   m.cfg.Channels,
	}
}

// Stop halts audio generation.
func (m *MockSource) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.running = false
	close(m.stopCh)
	close(m.streamCh)

	m.logger.Info("mock audio source stopped")

	return nil
}

// Read reads the next audio chunk.
func (m *MockSource) Read(ctx context.Context) (AudioChunk, error) {
	select {
	case <-ctx.Done():
		return AudioChunk{}, ctx.Err()
	case chunk, ok := <-m.streamCh:
		if !ok {
			return AudioChunk{}, io.EOF
		}
		return chunk, nil
	}
}

// Stream returns the audio chunk channel.
func (m *MockSource) Stream() <-chan AudioChunk {
	return m.streamCh
}

// Config returns the audio configuration.
func (m *MockSource) Config() Config {
	return m.cfg
}

// Name returns "mock".
func (m *MockSource) Name() string {
	return "mock"
}

// Close releases resources.
func (m *MockSource) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()

	m.Stop()
	return nil
}

// Stats returns source statistics.
func (m *MockSource) Stats() SourceStats {
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()

	return SourceStats{
		ChunksRead:  m.chunksRead.Load(),
		SamplesRead: m.samplesRead.Load(),
		Overruns:    0,
		Running:     running,
		Backend:     "mock",
	}
}

// Ensure MockSource implements SourceWithStats.
var _ SourceWithStats = (*MockSource)(nil)

// MockSink is a mock audio sink for testing.
// It discards audio data but tracks statistics.
type MockSink struct {
	cfg    Config
	logger *slog.Logger

	mu      sync.Mutex
	running bool
	closed  bool

	// Stats
	chunksWritten  atomic.Int64
	samplesWritten atomic.Int64

	// Buffer simulation
	buffer []AudioChunk
}

// NewMockSink creates a new mock audio sink.
func NewMockSink(cfg Config, logger *slog.Logger) *MockSink {
	if logger == nil {
		logger = slog.Default()
	}

	return &MockSink{
		cfg:    cfg,
		logger: logger,
		buffer: make([]AudioChunk, 0, 100),
	}
}

// Start begins accepting audio.
func (m *MockSink) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return io.ErrClosedPipe
	}

	m.running = true
	m.logger.Info("mock audio sink started")

	return nil
}

// Stop halts audio acceptance.
func (m *MockSink) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.running = false
	m.logger.Info("mock audio sink stopped")

	return nil
}

// Write accepts an audio chunk.
func (m *MockSink) Write(ctx context.Context, chunk AudioChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return io.ErrClosedPipe
	}
	if !m.running {
		return io.ErrClosedPipe
	}

	// Simulate buffering
	m.buffer = append(m.buffer, chunk)

	m.chunksWritten.Add(1)
	m.samplesWritten.Add(int64(len(chunk.Samples)))

	return nil
}

// Flush simulates waiting for playback.
func (m *MockSink) Flush(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate playback time
	totalSamples := 0
	for _, chunk := range m.buffer {
		totalSamples += len(chunk.Samples)
	}

	if totalSamples > 0 && m.cfg.SampleRate > 0 {
		duration := time.Duration(float64(totalSamples) / float64(m.cfg.SampleRate) * float64(time.Second))
		// Don't actually wait the full duration in mock mode, just a token amount
		waitTime := duration / 100
		if waitTime > 10*time.Millisecond {
			waitTime = 10 * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}

	m.buffer = m.buffer[:0]
	return nil
}

// Clear discards buffered audio.
func (m *MockSink) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.buffer = m.buffer[:0]
	m.logger.Debug("mock audio sink cleared")

	return nil
}

// Config returns the audio configuration.
func (m *MockSink) Config() Config {
	return m.cfg
}

// Name returns "mock".
func (m *MockSink) Name() string {
	return "mock"
}

// Close releases resources.
func (m *MockSink) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()

	m.Stop()
	return nil
}

// Stats returns sink statistics.
func (m *MockSink) Stats() SinkStats {
	m.mu.Lock()
	running := m.running
	buffered := int64(0)
	for _, chunk := range m.buffer {
		buffered += int64(len(chunk.Samples))
	}
	m.mu.Unlock()

	return SinkStats{
		ChunksWritten:   m.chunksWritten.Load(),
		SamplesWritten:  m.samplesWritten.Load(),
		Underruns:       0,
		Running:         running,
		Backend:         "mock",
		BufferedSamples: buffered,
	}
}

// Ensure MockSink implements SinkWithStats.
var _ SinkWithStats = (*MockSink)(nil)

