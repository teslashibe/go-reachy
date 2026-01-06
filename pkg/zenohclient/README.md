# pkg/zenohclient

High-level Zenoh client for Reachy Mini robot communication.

## Overview

This package wraps [zenoh-go](https://github.com/teslashibe/zenoh-go) to provide:

- Session management with automatic reconnection
- Audio streaming over Zenoh topics
- Pre-defined topics for Reachy Mini protocol
- Thread-safe pub/sub operations

## Usage

### Basic Client

```go
import (
    "context"
    "log/slog"
    
    "github.com/teslashibe/go-reachy/pkg/zenohclient"
)

func main() {
    cfg := zenohclient.DefaultConfig()
    cfg.Endpoint = "tcp/192.168.68.83:7447" // Robot IP
    
    logger := slog.Default()
    
    client, err := zenohclient.New(cfg, logger)
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    ctx := context.Background()
    
    // Connect with retry
    if err := client.ConnectWithRetry(ctx); err != nil {
        panic(err)
    }
    
    // Publish motor command
    cmd := []byte(`{"head_pose": [0, 0, -15]}`)
    if err := client.Publish(client.Topics().Command(), cmd); err != nil {
        panic(err)
    }
    
    // Subscribe to joint positions
    sub, err := client.Subscribe(client.Topics().JointPositions(), func(data []byte) {
        // Process joint position data
    })
    if err != nil {
        panic(err)
    }
    defer sub.Close()
}
```

### Audio Streaming

```go
// Publish microphone audio to robot
audioPub, err := zenohclient.NewAudioPublisher(
    client,
    client.Topics().AudioMic(),
    logger,
)
if err != nil {
    panic(err)
}
defer audioPub.Close()

// Publish audio chunks from microphone
for chunk := range audioSource.Stream() {
    if err := audioPub.PublishRaw(chunk.Samples, 24000, 1); err != nil {
        log.Printf("publish error: %v", err)
    }
}

// Subscribe to speaker audio from robot
audioSub, err := zenohclient.NewAudioSubscriber(
    client,
    client.Topics().AudioSpeaker(),
    50, // buffer size
    logger,
)
if err != nil {
    panic(err)
}
defer audioSub.Close()

// Play received audio
for chunk := range audioSub.Stream() {
    if err := audioSink.Write(ctx, audioio.AudioChunk{
        Samples:    chunk.Samples,
        SampleRate: chunk.SampleRate,
        Channels:   chunk.Channels,
    }); err != nil {
        log.Printf("playback error: %v", err)
    }
}
```

## Configuration

```go
cfg := zenohclient.Config{
    Endpoint:             "tcp/localhost:7447",
    Mode:                 "client",      // "client" or "peer"
    Prefix:               "reachy_mini", // Topic prefix
    ReconnectInterval:    2 * time.Second,
    MaxReconnectAttempts: 0,             // 0 = unlimited
}
```

## Topics

| Topic | Direction | Format | Description |
|-------|-----------|--------|-------------|
| `{prefix}/command` | Pub | JSON | Motor commands |
| `{prefix}/joint_positions` | Sub | JSON | Joint feedback |
| `{prefix}/head_pose` | Sub | JSON | Head pose matrix |
| `{prefix}/daemon_status` | Sub | JSON | Robot status |
| `{prefix}/task` | Pub | JSON | Task requests |
| `{prefix}/task_progress` | Sub | JSON | Task progress |
| `{prefix}/audio/mic` | Pub | Binary | Microphone PCM16 |
| `{prefix}/audio/speaker` | Sub | Binary | Speaker PCM16 |
| `{prefix}/audio/doa` | Pub | JSON | Direction of arrival |

## Audio Wire Format

Audio chunks use a simple binary format for low overhead:

```
[4 bytes] sample_rate (uint32, little-endian)
[4 bytes] channels (uint32, little-endian)
[4 bytes] num_samples (uint32, little-endian)
[N bytes] samples (int16[], little-endian, N = num_samples * 2)
```

## Prerequisites

Requires [zenoh-c 1.0.0](https://github.com/eclipse-zenoh/zenoh-c/releases/tag/1.0.0) to be installed for CGO mode.

For mock mode (testing), no external dependencies are required:

```bash
go build -tags mock
```

## TODO

- [ ] Add DOA (Direction of Arrival) publisher
- [ ] Add motor command helpers with validation
- [ ] Add connection health monitoring
- [ ] Add metrics/tracing integration

