# pkg/audioio

Cross-platform audio capture and playback for Go.

## Overview

This package provides a unified interface for audio I/O across different platforms:

| Platform | Backend | Status |
|----------|---------|--------|
| Linux (Raspberry Pi) | ALSA | Scaffold |
| macOS | CoreAudio | Scaffold |
| All | Mock | Complete |

## Usage

```go
import (
    "context"
    "log/slog"
    
    "github.com/teslashibe/go-reachy/pkg/audioio"
)

func main() {
    cfg := audioio.DefaultConfig()
    cfg.Backend = audioio.BackendAuto // Selects best for platform
    
    logger := slog.Default()
    
    // Create audio source (microphone)
    source, err := audioio.NewSource(cfg, logger)
    if err != nil {
        panic(err)
    }
    defer source.Close()
    
    ctx := context.Background()
    
    // Start capturing
    if err := source.Start(ctx); err != nil {
        panic(err)
    }
    
    // Read audio chunks
    for chunk := range source.Stream() {
        // Process chunk.Samples ([]int16 PCM16)
        // chunk.SampleRate = 24000
        // chunk.Channels = 1
    }
}
```

## Configuration

```go
cfg := audioio.Config{
    Backend:        audioio.BackendAuto,     // "auto", "alsa", "coreaudio", "mock"
    SampleRate:     24000,                   // OpenAI Realtime requires 24kHz
    Channels:       1,                       // Mono
    BufferDuration: 20 * time.Millisecond,   // 20ms chunks (480 samples)
    Device:         "",                      // Platform-specific device ID
}
```

## Interfaces

### Source (Microphone)

```go
type Source interface {
    Start(ctx context.Context) error
    Stop() error
    Read(ctx context.Context) (AudioChunk, error)
    Stream() <-chan AudioChunk
    Config() Config
    Name() string
    Close() error
}
```

### Sink (Speaker)

```go
type Sink interface {
    Start(ctx context.Context) error
    Stop() error
    Write(ctx context.Context, chunk AudioChunk) error
    Flush(ctx context.Context) error
    Clear() error
    Config() Config
    Name() string
    Close() error
}
```

## Utilities

### Resampling

```go
// Resample from 48kHz to 24kHz
samples24k := audioio.Resample(samples48k, 48000, 24000)
```

### Byte Conversion

```go
// PCM16 bytes <-> int16 samples
samples := audioio.BytesToSamples(rawBytes)
rawBytes := audioio.SamplesToBytes(samples)
```

### Channel Conversion

```go
stereo := audioio.MonoToStereo(mono)
mono := audioio.StereoToMono(stereo)
```

## Testing

Use the mock backend for unit tests:

```go
cfg := audioio.DefaultConfig()
cfg.Backend = audioio.BackendMock

source := audioio.NewMockSource(cfg, nil, audioio.WithSineWave(440, 0.5))
```

## TODO

- [ ] Complete ALSA implementation with real audio capture
- [ ] Complete CoreAudio implementation for Mac development
- [ ] Add PortAudio backend for maximum portability
- [ ] Add VAD (Voice Activity Detection) integration

