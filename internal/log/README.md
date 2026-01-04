# internal/log

Structured logging for go-reachy production use.

## Usage

```go
import "github.com/teslashibe/go-reachy/internal/log"

func main() {
    // Initialize with level: "debug", "info", "warn", "error"
    log.Init("info")
    
    // Use convenience functions
    log.Info("server started", "port", 8080)
    log.Error("connection failed", "err", err, "host", host)
    
    // Or get the underlying slog.Logger
    logger := log.With("component", "conversation")
    logger.Debug("processing audio", "bytes", len(data))
}
```

## Guidelines

### When to use structured logging (slog)

- **Library packages (`pkg/`)**: Always use slog for errors, warnings, and important events
- **Internal operations**: Connection events, retries, timeouts
- **Production debugging**: Anything you'd need to diagnose issues in production

### When to use fmt.Print

- **CLI output**: User-facing status messages in `cmd/` programs
- **Demo/test programs**: Interactive feedback with emojis
- **Progress indicators**: Real-time status updates

### Log Levels

| Level | Use Case |
|-------|----------|
| Debug | Verbose debugging, request/response details |
| Info | Normal operations, startup, connections |
| Warn | Recoverable issues, deprecation notices |
| Error | Failures that need attention |

## Environment Variables

- `GO_ENV=production`: Use JSON output (for log aggregation)
- Default: Use text output (human-readable)




