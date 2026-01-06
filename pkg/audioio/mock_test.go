package audioio

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestMockSource_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BufferDuration = 10 * time.Millisecond

	src := NewMockSource(cfg, nil)
	defer src.Close()

	ctx := context.Background()

	// Start should succeed
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Starting again should be a no-op
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	// Stop should succeed
	if err := src.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stopping again should be a no-op
	if err := src.Stop(); err != nil {
		t.Fatalf("Second Stop failed: %v", err)
	}
}

func TestMockSource_Read(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BufferDuration = 10 * time.Millisecond

	src := NewMockSource(cfg, nil)
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Read a chunk
	chunk, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	expectedSamples := cfg.BufferSize() * cfg.Channels
	if len(chunk.Samples) != expectedSamples {
		t.Errorf("Expected %d samples, got %d", expectedSamples, len(chunk.Samples))
	}

	if chunk.SampleRate != cfg.SampleRate {
		t.Errorf("Expected sample rate %d, got %d", cfg.SampleRate, chunk.SampleRate)
	}

	if chunk.Channels != cfg.Channels {
		t.Errorf("Expected %d channels, got %d", cfg.Channels, chunk.Channels)
	}
}

func TestMockSource_Stream(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BufferDuration = 10 * time.Millisecond

	src := NewMockSource(cfg, nil)
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	stream := src.Stream()
	chunkCount := 0

	for {
		select {
		case <-ctx.Done():
			goto done
		case _, ok := <-stream:
			if !ok {
				goto done
			}
			chunkCount++
		}
	}

done:
	if chunkCount < 3 {
		t.Errorf("Expected at least 3 chunks in 100ms, got %d", chunkCount)
	}
}

func TestMockSource_SineWave(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BufferDuration = 10 * time.Millisecond

	// Create source with 440Hz sine wave
	src := NewMockSource(cfg, nil, WithSineWave(440, 0.5))
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	chunk, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify samples are not all zero (sine wave should have non-zero values)
	hasNonZero := false
	for _, s := range chunk.Samples {
		if s != 0 {
			hasNonZero = true
			break
		}
	}

	if !hasNonZero {
		t.Error("Expected non-zero samples from sine wave generator")
	}
}

func TestMockSource_Close(t *testing.T) {
	cfg := DefaultConfig()
	src := NewMockSource(cfg, nil)

	ctx := context.Background()
	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close should succeed
	if err := src.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Start after close should fail
	if err := src.Start(ctx); err != io.ErrClosedPipe {
		t.Errorf("Expected ErrClosedPipe after close, got: %v", err)
	}

	// Closing again should be a no-op
	if err := src.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}

func TestMockSource_Stats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BufferDuration = 10 * time.Millisecond

	src := NewMockSource(cfg, nil)
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := src.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Read some chunks
	for i := 0; i < 3; i++ {
		_, err := src.Read(ctx)
		if err != nil {
			break
		}
	}

	stats := src.Stats()

	if stats.ChunksRead < 3 {
		t.Errorf("Expected at least 3 chunks read, got %d", stats.ChunksRead)
	}

	if stats.Backend != "mock" {
		t.Errorf("Expected backend 'mock', got '%s'", stats.Backend)
	}
}

func TestMockSink_WriteFlushClear(t *testing.T) {
	cfg := DefaultConfig()
	sink := NewMockSink(cfg, nil)
	defer sink.Close()

	ctx := context.Background()

	if err := sink.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Write a chunk
	chunk := AudioChunk{
		Samples:    make([]int16, 480),
		SampleRate: 24000,
		Channels:   1,
	}

	if err := sink.Write(ctx, chunk); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	stats := sink.Stats()
	if stats.ChunksWritten != 1 {
		t.Errorf("Expected 1 chunk written, got %d", stats.ChunksWritten)
	}

	// Flush should succeed
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Write more and clear
	if err := sink.Write(ctx, chunk); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := sink.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Stats should still show 2 chunks written
	stats = sink.Stats()
	if stats.ChunksWritten != 2 {
		t.Errorf("Expected 2 chunks written, got %d", stats.ChunksWritten)
	}
}

func TestMockSink_NotRunning(t *testing.T) {
	cfg := DefaultConfig()
	sink := NewMockSink(cfg, nil)
	defer sink.Close()

	ctx := context.Background()

	// Write without starting should fail
	chunk := AudioChunk{
		Samples:    make([]int16, 480),
		SampleRate: 24000,
		Channels:   1,
	}

	err := sink.Write(ctx, chunk)
	if err == nil {
		t.Error("Expected error when writing to non-running sink")
	}
}

func TestAudioChunk_Bytes(t *testing.T) {
	chunk := AudioChunk{
		Samples:    []int16{0x0102, 0x0304, -1},
		SampleRate: 24000,
		Channels:   1,
	}

	bytes := chunk.Bytes()
	if len(bytes) != 6 {
		t.Errorf("Expected 6 bytes, got %d", len(bytes))
	}

	// Check little-endian encoding
	if bytes[0] != 0x02 || bytes[1] != 0x01 {
		t.Errorf("First sample not encoded correctly: %v", bytes[0:2])
	}
}

func TestAudioChunk_FromBytes(t *testing.T) {
	data := []byte{0x02, 0x01, 0x04, 0x03, 0xFF, 0xFF}

	var chunk AudioChunk
	chunk.FromBytes(data, 24000, 1)

	if len(chunk.Samples) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(chunk.Samples))
	}

	if chunk.Samples[0] != 0x0102 {
		t.Errorf("First sample incorrect: got %d, expected %d", chunk.Samples[0], 0x0102)
	}

	if chunk.Samples[2] != -1 {
		t.Errorf("Third sample incorrect: got %d, expected -1", chunk.Samples[2])
	}
}

func TestAudioChunk_Duration(t *testing.T) {
	chunk := AudioChunk{
		Samples:    make([]int16, 480), // 20ms at 24kHz mono
		SampleRate: 24000,
		Channels:   1,
	}

	duration := chunk.Duration()
	expected := 0.02 // 20ms

	if duration < expected-0.001 || duration > expected+0.001 {
		t.Errorf("Expected duration ~%f, got %f", expected, duration)
	}
}

