# pkg/hub

WebSocket broadcast hub for real-time client communication.

## Overview

Thread-safe pub/sub hub for broadcasting messages to multiple WebSocket clients. Used by the web dashboard for real-time updates.

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/hub"

// Create hub
h := hub.New("camera")

// Run in goroutine
go h.Run()

// Broadcast to all clients
h.BroadcastJSON(map[string]any{"status": "ok"})
h.BroadcastBinary(jpegData)

// Check client count
fmt.Printf("Clients: %d\n", h.ClientCount())
```

## Features

- **Thread-safe** - Safe for concurrent use
- **Buffered channels** - Non-blocking broadcasts
- **Slow client handling** - Drops slow clients automatically
- **JSON/Binary support** - Both message types

## Message Types

| Method | Description |
|--------|-------------|
| `Broadcast(Message)` | Raw message |
| `BroadcastJSON(v)` | JSON-encoded |
| `BroadcastBinary([]byte)` | Binary data |


