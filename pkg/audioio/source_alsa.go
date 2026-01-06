//go:build linux

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

// ALSASource captures audio using Linux ALSA.
// This is the production implementation for the Raspberry Pi.
type ALSASource struct {
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

	// ALSA handle (placeholder - would use C bindings or pure Go ALSA)
	device string
}

// newALSASource creates a new ALSA audio source.
func newALSASource(cfg Config, logger *slog.Logger) (*ALSASource, error) {
	device := cfg.Device
	if device == "" {
		device = "default"
	}

	s := &ALSASource{
		cfg:      cfg,
		logger:   logger,
		device:   device,
		streamCh: make(chan AudioChunk, 10),
		stopCh:   make(chan struct{}),
	}

	logger.Info("ALSA source created",
		"device", device,
		"sample_rate", cfg.SampleRate,
		"channels", cfg.Channels,
	)

	return s, nil
}

// Start begins audio capture.
func (s *ALSASource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}
	if s.running {
		return nil
	}

	// TODO: Open ALSA device and configure
	// This would use either:
	// - github.com/jfreymuth/pulse (PulseAudio)
	// - Direct ALSA via CGO
	// - github.com/gordonklaus/portaudio

	s.running = true
	s.stopCh = make(chan struct{})
	s.streamCh = make(chan AudioChunk, 10)

	go s.captureLoop(ctx)

	s.logger.Info("ALSA audio source started",
		"device", s.device,
	)

	return nil
}

func (s *ALSASource) captureLoop(ctx context.Context) {
	// TODO: Actual ALSA capture loop
	// For now, generate silence as a placeholder
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
				s.logger.Debug("ALSA source: buffer full, dropping chunk")
			}
		}
	}
}

func (s *ALSASource) readFromDevice() AudioChunk {
	// TODO: Read from actual ALSA device
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
func (s *ALSASource) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	close(s.stopCh)
	close(s.streamCh)

	s.logger.Info("ALSA audio source stopped")

	return nil
}

// Read reads the next audio chunk.
func (s *ALSASource) Read(ctx context.Context) (AudioChunk, error) {
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
func (s *ALSASource) Stream() <-chan AudioChunk {
	return s.streamCh
}

// Config returns the audio configuration.
func (s *ALSASource) Config() Config {
	return s.cfg
}

// Name returns "alsa".
func (s *ALSASource) Name() string {
	return "alsa"
}

// Close releases resources.
func (s *ALSASource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.Stop()
	// TODO: Close ALSA device handle
	return nil
}

// Stats returns source statistics.
func (s *ALSASource) Stats() SourceStats {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	return SourceStats{
		ChunksRead:  s.chunksRead.Load(),
		SamplesRead: s.samplesRead.Load(),
		Overruns:    s.overruns.Load(),
		Running:     running,
		Backend:     "alsa",
	}
}

var _ SourceWithStats = (*ALSASource)(nil)

// ALSASink plays audio using Linux ALSA.
type ALSASink struct {
	cfg    Config
	logger *slog.Logger

	mu      sync.Mutex
	running bool
	closed  bool

	// Stats
	chunksWritten  atomic.Int64
	samplesWritten atomic.Int64
	underruns      atomic.Int64

	device string
}

// newALSASink creates a new ALSA audio sink.
func newALSASink(cfg Config, logger *slog.Logger) (*ALSASink, error) {
	device := cfg.Device
	if device == "" {
		device = "default"
	}

	s := &ALSASink{
		cfg:    cfg,
		logger: logger,
		device: device,
	}

	logger.Info("ALSA sink created",
		"device", device,
		"sample_rate", cfg.SampleRate,
		"channels", cfg.Channels,
	)

	return s, nil
}

// Start begins audio playback.
func (s *ALSASink) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}

	// TODO: Open ALSA device for playback
	s.running = true
	s.logger.Info("ALSA audio sink started", "device", s.device)

	return nil
}

// Stop halts audio playback.
func (s *ALSASink) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.running = false
	s.logger.Info("ALSA audio sink stopped")

	return nil
}

// Write sends audio to the output device.
func (s *ALSASink) Write(ctx context.Context, chunk AudioChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return io.ErrClosedPipe
	}
	if !s.running {
		return fmt.Errorf("sink not running")
	}

	// TODO: Write to ALSA device
	s.chunksWritten.Add(1)
	s.samplesWritten.Add(int64(len(chunk.Samples)))

	return nil
}

// Flush waits for buffered audio to play.
func (s *ALSASink) Flush(ctx context.Context) error {
	// TODO: Wait for ALSA buffer to drain
	return nil
}

// Clear discards buffered audio.
func (s *ALSASink) Clear() error {
	// TODO: Clear ALSA buffer
	s.logger.Debug("ALSA sink cleared")
	return nil
}

// Config returns the audio configuration.
func (s *ALSASink) Config() Config {
	return s.cfg
}

// Name returns "alsa".
func (s *ALSASink) Name() string {
	return "alsa"
}

// Close releases resources.
func (s *ALSASink) Close() error {
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
func (s *ALSASink) Stats() SinkStats {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	return SinkStats{
		ChunksWritten:   s.chunksWritten.Load(),
		SamplesWritten:  s.samplesWritten.Load(),
		Underruns:       s.underruns.Load(),
		Running:         running,
		Backend:         "alsa",
		BufferedSamples: 0, // TODO: Get from ALSA
	}
}

var _ SinkWithStats = (*ALSASink)(nil)

