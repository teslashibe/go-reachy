package audioio

import (
	"context"
	"io"
)

// Sink plays audio to a speaker or other output device.
type Sink interface {
	// Start begins audio playback.
	// After calling Start, audio can be written via Write.
	Start(ctx context.Context) error

	// Stop halts audio playback.
	// It is safe to call Stop multiple times.
	Stop() error

	// Write sends an audio chunk to the output device.
	// This may block if the output buffer is full.
	Write(ctx context.Context, chunk AudioChunk) error

	// Flush waits for all buffered audio to be played.
	Flush(ctx context.Context) error

	// Clear discards all buffered audio immediately.
	// Use this to interrupt playback (e.g., when user speaks).
	Clear() error

	// Config returns the current audio configuration.
	Config() Config

	// Name returns the backend name (e.g., "alsa", "coreaudio", "mock").
	Name() string

	// Close releases all resources.
	// After Close, the sink cannot be restarted.
	io.Closer
}

// SinkStats contains statistics about the audio sink.
type SinkStats struct {
	// ChunksWritten is the total number of chunks written.
	ChunksWritten int64 `json:"chunks_written"`

	// SamplesWritten is the total number of samples written.
	SamplesWritten int64 `json:"samples_written"`

	// Underruns is the number of buffer underruns (audio gaps).
	Underruns int64 `json:"underruns"`

	// Running indicates if the sink is currently playing.
	Running bool `json:"running"`

	// Backend is the name of the audio backend.
	Backend string `json:"backend"`

	// BufferedSamples is the number of samples currently buffered.
	BufferedSamples int64 `json:"buffered_samples"`
}

// SinkWithStats extends Sink with statistics.
type SinkWithStats interface {
	Sink
	Stats() SinkStats
}

