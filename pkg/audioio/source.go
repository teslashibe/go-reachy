package audioio

import (
	"context"
	"io"
)

// AudioChunk represents a chunk of audio data.
type AudioChunk struct {
	// Samples contains PCM16 audio samples (little-endian).
	Samples []int16

	// SampleRate is the sample rate of this chunk.
	SampleRate int

	// Channels is the number of channels in this chunk.
	Channels int
}

// Bytes returns the raw bytes of the audio chunk.
func (c *AudioChunk) Bytes() []byte {
	buf := make([]byte, len(c.Samples)*2)
	for i, s := range c.Samples {
		buf[i*2] = byte(s)
		buf[i*2+1] = byte(s >> 8)
	}
	return buf
}

// FromBytes populates the chunk from raw PCM16 bytes.
func (c *AudioChunk) FromBytes(data []byte, sampleRate, channels int) {
	c.SampleRate = sampleRate
	c.Channels = channels
	c.Samples = make([]int16, len(data)/2)
	for i := range c.Samples {
		c.Samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
}

// Duration returns the duration of this audio chunk.
func (c *AudioChunk) Duration() float64 {
	if c.SampleRate == 0 || c.Channels == 0 {
		return 0
	}
	return float64(len(c.Samples)) / float64(c.SampleRate*c.Channels)
}

// Source captures audio from a microphone or other input device.
type Source interface {
	// Start begins audio capture.
	// After calling Start, audio chunks will be available via Read or Stream.
	Start(ctx context.Context) error

	// Stop halts audio capture.
	// It is safe to call Stop multiple times.
	Stop() error

	// Read reads the next audio chunk, blocking if necessary.
	// Returns io.EOF when the source is stopped.
	Read(ctx context.Context) (AudioChunk, error)

	// Stream returns a channel that receives audio chunks.
	// The channel is closed when the source is stopped.
	Stream() <-chan AudioChunk

	// Config returns the current audio configuration.
	Config() Config

	// Name returns the backend name (e.g., "alsa", "coreaudio", "mock").
	Name() string

	// Close releases all resources.
	// After Close, the source cannot be restarted.
	io.Closer
}

// SourceStats contains statistics about the audio source.
type SourceStats struct {
	// ChunksRead is the total number of chunks read.
	ChunksRead int64 `json:"chunks_read"`

	// SamplesRead is the total number of samples read.
	SamplesRead int64 `json:"samples_read"`

	// Overruns is the number of buffer overruns (dropped audio).
	Overruns int64 `json:"overruns"`

	// Running indicates if the source is currently capturing.
	Running bool `json:"running"`

	// Backend is the name of the audio backend.
	Backend string `json:"backend"`
}

// SourceWithStats extends Source with statistics.
type SourceWithStats interface {
	Source
	Stats() SourceStats
}

