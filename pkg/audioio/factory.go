package audioio

import (
	"fmt"
	"log/slog"
	"runtime"
)

// NewSource creates a new audio source with the given configuration.
// If cfg.Backend is BackendAuto, the best available backend is selected.
func NewSource(cfg Config, logger *slog.Logger) (Source, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	backend := cfg.Backend
	if backend == BackendAuto {
		backend = detectBestBackend()
	}

	logger.Info("creating audio source",
		"backend", backend,
		"sample_rate", cfg.SampleRate,
		"channels", cfg.Channels,
		"buffer_ms", cfg.BufferDuration.Milliseconds(),
	)

	switch backend {
	case BackendMock:
		return NewMockSource(cfg, logger), nil
	case BackendALSA:
		return newALSASource(cfg, logger)
	case BackendCoreAudio, BackendPortAudio:
		return newDarwinSource(cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// NewSink creates a new audio sink with the given configuration.
// If cfg.Backend is BackendAuto, the best available backend is selected.
func NewSink(cfg Config, logger *slog.Logger) (Sink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	backend := cfg.Backend
	if backend == BackendAuto {
		backend = detectBestBackend()
	}

	logger.Info("creating audio sink",
		"backend", backend,
		"sample_rate", cfg.SampleRate,
		"channels", cfg.Channels,
		"buffer_ms", cfg.BufferDuration.Milliseconds(),
	)

	switch backend {
	case BackendMock:
		return NewMockSink(cfg, logger), nil
	case BackendALSA:
		return newALSASink(cfg, logger)
	case BackendCoreAudio, BackendPortAudio:
		return newDarwinSink(cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}
}

// detectBestBackend returns the best available backend for the current platform.
func detectBestBackend() Backend {
	switch runtime.GOOS {
	case "linux":
		return BackendALSA
	case "darwin":
		return BackendCoreAudio
	default:
		return BackendMock
	}
}

// AvailableBackends returns the list of backends available on this platform.
func AvailableBackends() []Backend {
	backends := []Backend{BackendMock}

	switch runtime.GOOS {
	case "linux":
		backends = append(backends, BackendALSA)
	case "darwin":
		backends = append(backends, BackendCoreAudio)
	}

	return backends
}

