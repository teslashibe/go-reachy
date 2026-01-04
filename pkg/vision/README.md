# vision

Vision and web search capabilities for Eva.

## Overview

This package provides visual understanding and web search functionality using external APIs.

## Components

### GeminiVision

Uses Google's Gemini API for visual understanding.

```go
vision := vision.NewGeminiVision(apiKey)

// Analyze an image
description, err := vision.Describe(jpegData, "What do you see?")
```

### WebSearch

Performs web searches using Google Custom Search.

```go
results, err := vision.WebSearch(apiKey, searchEngineID, "Reachy Mini robot")
```

## Provider Interface

```go
type Provider interface {
    Describe(image []byte, prompt string) (string, error)
}
```

## Configuration

| Env Variable | Description |
|--------------|-------------|
| `GOOGLE_API_KEY` | Google API key for Gemini and Search |
| `SEARCH_ENGINE_ID` | Google Custom Search engine ID |

## AI Tool Integration

Vision capabilities are exposed to Eva as tools:

- `look` - Describe what Eva sees
- `search_web` - Search the internet for information



