# pkg/audio

Client for audio Direction of Arrival (DOA) data from go-eva daemon.

## Overview

Connects to go-eva on the robot to receive real-time DOA angles for audio-based head tracking.

## Usage

```go
client := audio.NewClient("192.168.68.77")

// Health check
if err := client.Health(); err != nil {
    log.Fatal(err)
}

// Stream DOA updates
client.StreamDOA(ctx, func(result *audio.DOAResult) {
    if result.Speaking {
        fmt.Printf("Speaker at %.1f°\n", result.Angle * 180 / math.Pi)
    }
})
```

## DOAResult

| Field | Type | Description |
|-------|------|-------------|
| `Angle` | `float64` | Radians: 0=front, +π/2=left, -π/2=right |
| `Speaking` | `bool` | Voice activity detected |
| `Confidence` | `float64` | 0-1 confidence score |

## Requirements

- go-eva daemon on robot port 9000
- XVF3800 DSP chip for 4-mic DOA


Client for audio Direction of Arrival (DOA) data from the go-eva daemon.

## Overview

The `audio` package provides a WebSocket client that connects to go-eva running on the Reachy Mini robot to receive real-time Direction of Arrival (DOA) angles. This enables audio-based head tracking to look toward speakers.

## Features

- **WebSocket streaming** - Real-time DOA updates via WebSocket
- **HTTP fallback** - Polling mode if WebSocket unavailable
- **Auto-reconnection** - Exponential backoff reconnection
- **Voice activity detection** - Includes speaking flag

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/audio"

// Create client
client := audio.NewClient("192.168.68.77")

// Check if go-eva is running
if err := client.Health(); err != nil {
    log.Fatal("go-eva not running:", err)
}

// Stream DOA updates
ctx := context.Background()
client.StreamDOA(ctx, func(result *audio.DOAResult) {
    if result.Speaking {
        fmt.Printf("Speaker at %.1f° (confidence: %.1f%%)\n", 
            result.Angle * 180 / math.Pi,
            result.Confidence * 100)
    }
})

// Later: close connection
client.Close()
```

## DOAResult

```go
type DOAResult struct {
    Angle      float64   // Radians: 0=front, +π/2=left, -π/2=right
    Speaking   bool      // Voice activity detected
    Confidence float64   // 0-1 confidence score
    Timestamp  time.Time // When reading was taken
    RawAngle   float64   // Original XVF3800 angle
}
```

## API Endpoints (go-eva)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/audio/doa` | GET | Current DOA reading (HTTP polling) |
| `/api/audio/doa/stream` | WS | WebSocket streaming |

## Dependencies

Requires go-eva daemon running on the robot:
- Port 9000 for API
- XVF3800 DSP chip for 4-mic DOA

## See Also

- [go-eva](../../go-eva/) - The audio daemon
- [pkg/tracking](../tracking/) - Uses DOA for head tracking

