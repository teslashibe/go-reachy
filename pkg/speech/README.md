# speech

Speech handling utilities.

## Overview

This package provides speech-related utilities for handling audio input and output coordination.

## Components

### Handler

Coordinates speech input/output, handling:
- Barge-in (user interrupting AI)
- Audio queue management
- Silence detection

## Usage

```go
handler := speech.NewHandler(config)

// Handle incoming audio
handler.ProcessAudio(audioData)

// Check for barge-in
if handler.IsBargeIn() {
    player.Stop()
}
```

## Features

- Barge-in detection
- Audio buffering
- State machine for conversation flow

