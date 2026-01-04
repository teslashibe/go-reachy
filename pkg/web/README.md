# web

Web dashboard server for Eva monitoring.

## Overview

This package provides an HTTP server for the Eva web dashboard, showing real-time status, logs, and controls.

## Usage

```go
server := web.NewServer(":8080")

// Set state updater for real-time updates
server.SetStateUpdater(stateUpdater)

// Start server
go server.Start()
```

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Dashboard HTML |
| `/api/status` | GET | Robot status JSON |
| `/api/logs` | GET | Recent logs |
| `/ws` | WS | Real-time updates |

## Dashboard Features

- Connection status
- Face tracking visualization
- Audio levels
- Conversation transcript
- Tool call history
- Debug logs

## StateUpdater Interface

```go
type StateUpdater interface {
    UpdateFacePosition(position, yaw float64)
    AddLog(logType, message string)
}
```



