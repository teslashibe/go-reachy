# debug

Debug logging utilities.

## Overview

This package provides conditional debug logging that can be enabled/disabled globally.

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/debug"

// Enable debug mode
debug.Enable()

// Log debug messages (only printed when enabled)
debug.Log("Processing frame %d\n", frameNum)
debug.Logln("Starting detection")

// Check if debug is enabled
if debug.Enabled() {
    // Expensive debug operation
}
```

## Functions

| Function | Description |
|----------|-------------|
| `Enable()` | Turn on debug logging |
| `Disable()` | Turn off debug logging |
| `Enabled() bool` | Check if debug is on |
| `Log(format, args...)` | Printf-style debug log |
| `Logln(msg)` | Println-style debug log |

## CLI Integration

Typically enabled via command-line flag:

```go
if *debugFlag {
    debug.Enable()
}
```



