// Package audioio provides cross-platform audio capture and playback.
//
// This package supports multiple backends:
//   - ALSA (Linux/Robot) - Production use on Raspberry Pi
//   - CoreAudio (macOS) - Development on Mac
//   - Mock - CI/Testing without hardware
//
// The backend is selected automatically based on build tags and platform,
// or can be explicitly specified via configuration.
package audioio

import (
	"fmt"
	"time"
)

// Backend represents the audio backend type.
type Backend string

const (
	// BackendAuto automatically selects the best available backend.
	BackendAuto Backend = "auto"
	// BackendALSA uses Linux ALSA for audio I/O.
	BackendALSA Backend = "alsa"
	// BackendCoreAudio uses macOS CoreAudio for audio I/O.
	BackendCoreAudio Backend = "coreaudio"
	// BackendPortAudio uses PortAudio for cross-platform audio I/O.
	BackendPortAudio Backend = "portaudio"
	// BackendMock uses a mock implementation for testing.
	BackendMock Backend = "mock"
)

// Config holds audio configuration.
type Config struct {
	// Backend specifies which audio backend to use.
	// Default: "auto" (selects best available for platform)
	Backend Backend `yaml:"backend" json:"backend"`

	// SampleRate is the audio sample rate in Hz.
	// Default: 24000 (required by OpenAI Realtime)
	SampleRate int `yaml:"sample_rate" json:"sample_rate"`

	// Channels is the number of audio channels.
	// Default: 1 (mono)
	Channels int `yaml:"channels" json:"channels"`

	// BufferDuration is the size of audio buffers.
	// Default: 20ms (480 samples at 24kHz)
	BufferDuration time.Duration `yaml:"buffer_duration" json:"buffer_duration"`

	// Device is the platform-specific device identifier.
	// Examples:
	//   - ALSA: "hw:0,0", "default", "plughw:1,0"
	//   - CoreAudio: device UID or empty for default
	//   - Mock: ignored
	Device string `yaml:"device" json:"device"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Backend:        BackendAuto,
		SampleRate:     24000, // OpenAI Realtime requirement
		Channels:       1,     // Mono
		BufferDuration: 20 * time.Millisecond,
		Device:         "", // Use system default
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.SampleRate <= 0 {
		return fmt.Errorf("sample_rate must be positive, got %d", c.SampleRate)
	}
	if c.Channels <= 0 {
		return fmt.Errorf("channels must be positive, got %d", c.Channels)
	}
	if c.BufferDuration <= 0 {
		return fmt.Errorf("buffer_duration must be positive, got %v", c.BufferDuration)
	}
	return nil
}

// BufferSize returns the number of samples per buffer.
func (c *Config) BufferSize() int {
	return int(float64(c.SampleRate) * c.BufferDuration.Seconds())
}

// BufferBytes returns the size of a buffer in bytes (assuming int16 samples).
func (c *Config) BufferBytes() int {
	return c.BufferSize() * c.Channels * 2 // 2 bytes per int16 sample
}

