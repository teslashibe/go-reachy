//go:build !linux

package audioio

import (
	"fmt"
	"log/slog"
)

// newALSASource returns an error on non-Linux platforms.
func newALSASource(cfg Config, logger *slog.Logger) (Source, error) {
	return nil, fmt.Errorf("ALSA is only available on Linux")
}

// newALSASink returns an error on non-Linux platforms.
func newALSASink(cfg Config, logger *slog.Logger) (Sink, error) {
	return nil, fmt.Errorf("ALSA is only available on Linux")
}

