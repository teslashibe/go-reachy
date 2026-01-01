# pkg/debug

Simple debug logging utility with a global enable flag.

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/debug"

// Enable debug mode (usually from CLI flag)
debug.Enabled = true

// Log messages (only printed if Enabled)
debug.Log("Value: %d\n", 42)
debug.Logln("Message with newline")
```

## Functions

| Function | Description |
|----------|-------------|
| `Log(format, args...)` | Printf-style, only if enabled |
| `Logln(msg)` | Println-style, only if enabled |

## CLI Integration

```go
debugFlag := flag.Bool("debug", false, "Enable debug logging")
flag.Parse()
debug.Enabled = *debugFlag
```


