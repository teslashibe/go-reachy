//go:build darwin

package audioio

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// DarwinSource captures audio using macOS CoreAudio.
// This is used for development on Mac.
type DarwinSource struct {
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
	overruns    atomic.Int64
}

// newDarwinSource creates a new CoreAudio source.
func newDarwinSource(cfg Config, logger *slog.Logger) (*DarwinSource, error) {
	s := &DarwinSource{
		cfg:      cfg,
		logger:   logger,
		streamCh: make(chan AudioChunk, 10),
		stopCh:   make(chan struct{}),
	}

	logger.Info("Darwin/CoreAudio source created",
		"sample_rate", cfg.SampleRate,
		"channels", cfg.Channels,
	)

	return s, nil
}

// Start begins audio capture.
func (s *DarwinSource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}
	if s.running {
		return nil
	}

	// TODO: Initialize CoreAudio
	// Options:
	// - github.com/gordonklaus/portaudio (requires PortAudio installed)
	// - Direct CoreAudio via CGO
	// - github.com/gen2brain/malgo (miniaudio bindings)

	s.running = true
	s.stopCh = make(chan struct{})
	s.streamCh = make(chan AudioChunk, 10)

	go s.captureLoop(ctx)

	s.logger.Info("Darwin audio source started")

	return nil
}

func (s *DarwinSource) captureLoop(ctx context.Context) {
	// TODO: Actual CoreAudio capture
	// For now, simulate with silence (allows testing the pipeline)
	ticker := time.NewTicker(s.cfg.BufferDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.Stop()
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			chunk := s.readFromDevice()
			select {
			case s.streamCh <- chunk:
				s.chunksRead.Add(1)
				s.samplesRead.Add(int64(len(chunk.Samples)))
			default:
				s.overruns.Add(1)
				s.logger.Debug("Darwin source: buffer full, dropping chunk")
			}
		}
	}
}

func (s *DarwinSource) readFromDevice() AudioChunk {
	// TODO: Read from CoreAudio
	// Placeholder: return silence
	bufferSize := s.cfg.BufferSize()
	samples := make([]int16, bufferSize*s.cfg.Channels)

	return AudioChunk{
		Samples:    samples,
		SampleRate: s.cfg.SampleRate,
		Channels:   s.cfg.Channels,
	}
}

// Stop halts audio capture.
func (s *DarwinSource) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	close(s.stopCh)
	close(s.streamCh)

	s.logger.Info("Darwin audio source stopped")

	return nil
}

// Read reads the next audio chunk.
func (s *DarwinSource) Read(ctx context.Context) (AudioChunk, error) {
	select {
	case <-ctx.Done():
		return AudioChunk{}, ctx.Err()
	case chunk, ok := <-s.streamCh:
		if !ok {
			return AudioChunk{}, io.EOF
		}
		return chunk, nil
	}
}

// Stream returns the audio chunk channel.
func (s *DarwinSource) Stream() <-chan AudioChunk {
	return s.streamCh
}

// Config returns the audio configuration.
func (s *DarwinSource) Config() Config {
	return s.cfg
}

// Name returns "coreaudio".
func (s *DarwinSource) Name() string {
	return "coreaudio"
}

// Close releases resources.
func (s *DarwinSource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.Stop()
	return nil
}

// Stats returns source statistics.
func (s *DarwinSource) Stats() SourceStats {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	return SourceStats{
		ChunksRead:  s.chunksRead.Load(),
		SamplesRead: s.samplesRead.Load(),
		Overruns:    s.overruns.Load(),
		Running:     running,
		Backend:     "coreaudio",
	}
}

var _ SourceWithStats = (*DarwinSource)(nil)

// DarwinSink plays audio using macOS CoreAudio.
type DarwinSink struct {
	cfg    Config
	logger *slog.Logger

	mu      sync.Mutex
	running bool
	closed  bool

	// Stats
	chunksWritten  atomic.Int64
	samplesWritten atomic.Int64
	underruns      atomic.Int64
}

// newDarwinSink creates a new CoreAudio sink.
func newDarwinSink(cfg Config, logger *slog.Logger) (*DarwinSink, error) {
	s := &DarwinSink{
		cfg:    cfg,
		logger: logger,
	}

	logger.Info("Darwin/CoreAudio sink created",
		"sample_rate", cfg.SampleRate,
		"channels", cfg.Channels,
	)

	return s, nil
}

// Start begins audio playback.
func (s *DarwinSink) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}

	s.running = true
	s.logger.Info("Darwin audio sink started")

	return nil
}

// Stop halts audio playback.
func (s *DarwinSink) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.running = false
	s.logger.Info("Darwin audio sink stopped")

	return nil
}

// Write sends audio to the output device.
func (s *DarwinSink) Write(ctx context.Context, chunk AudioChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}
	if !s.running {
		return fmt.Errorf("sink not running")
	}

	// TODO: Write to CoreAudio
	s.chunksWritten.Add(1)
	s.samplesWritten.Add(int64(len(chunk.Samples)))

	return nil
}

// Flush waits for buffered audio to play.
func (s *DarwinSink) Flush(ctx context.Context) error {
	return nil
}

// Clear discards buffered audio.
func (s *DarwinSink) Clear() error {
	s.logger.Debug("Darwin sink cleared")
	return nil
}

// Config returns the audio configuration.
func (s *DarwinSink) Config() Config {
	return s.cfg
}

// Name returns "coreaudio".
func (s *DarwinSink) Name() string {
	return "coreaudio"
}

// Close releases resources.
func (s *DarwinSink) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.Stop()
	return nil
}

// Stats returns sink statistics.
func (s *DarwinSink) Stats() SinkStats {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	return SinkStats{
		ChunksWritten:   s.chunksWritten.Load(),
		SamplesWritten:  s.samplesWritten.Load(),
		Underruns:       s.underruns.Load(),
		Running:         running,
		Backend:         "coreaudio",
		BufferedSamples: 0,
	}
}

var _ SinkWithStats = (*DarwinSink)(nil)

