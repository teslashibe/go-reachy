# hub

WebSocket hub for real-time client communication.

## Overview

This package provides a WebSocket hub for broadcasting messages to connected clients (e.g., web dashboard).

## Usage

```go
hub := hub.New()
go hub.Run()

// Register client
hub.Register(client)

// Broadcast message
hub.Broadcast(message)
```

## Features

- Client connection management
- Message broadcasting
- Automatic client cleanup
- Thread-safe operations

## Message Types

```go
type Message struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
}
```

## Integration

Used by the web dashboard for:
- Real-time log streaming
- Status updates
- Face tracking visualization



