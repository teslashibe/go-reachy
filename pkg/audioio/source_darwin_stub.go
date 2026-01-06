//go:build !darwin

package audioio

import (
	"fmt"
	"log/slog"
)

// newDarwinSource returns an error on non-macOS platforms.
func newDarwinSource(cfg Config, logger *slog.Logger) (Source, error) {
	return nil, fmt.Errorf("CoreAudio is only available on macOS")
}

// newDarwinSink returns an error on non-macOS platforms.
func newDarwinSink(cfg Config, logger *slog.Logger) (Sink, error) {
	return nil, fmt.Errorf("CoreAudio is only available on macOS")
}

