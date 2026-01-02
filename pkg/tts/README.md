# pkg/tts - Text-to-Speech Provider Abstraction

Production-grade, idiomatic Go package for text-to-speech synthesis with support for multiple providers.

## Features

- **Provider Interface** - Swap TTS providers without changing caller code
- **ElevenLabs Support** - Custom voice cloning with streaming
- **OpenAI TTS** - Built-in voices as fallback
- **Provider Chain** - Automatic fallback when a provider fails
- **Streaming** - Low-latency audio streaming for real-time playback
- **Functional Options** - Clean, idiomatic configuration
- **Full Test Coverage** - Unit tests + integration tests

## Quick Start

```go
package main

import (
    "context"
    "os"
    
    "github.com/teslashibe/go-reachy/pkg/tts"
)

func main() {
    // Create ElevenLabs provider with custom voice
    provider, err := tts.NewElevenLabs(
        tts.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
        tts.WithVoice("your-voice-id"),
        tts.WithModel(tts.ModelTurboV2_5),
        tts.WithOutputFormat(tts.EncodingPCM24),
    )
    if err != nil {
        panic(err)
    }
    defer provider.Close()
    
    // Synthesize text to audio
    ctx := context.Background()
    result, err := provider.Synthesize(ctx, "Hello, I'm Eva!")
    if err != nil {
        panic(err)
    }
    
    // result.Audio contains PCM16 audio at 24kHz
    // result.Format describes the audio encoding
    playAudio(result.Audio, result.Format)
}
```

## Providers

### ElevenLabs (Custom Voice)

Best for custom voice cloning with high-quality output.

```go
provider, err := tts.NewElevenLabs(
    tts.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    tts.WithVoice("voice-id-from-voice-lab"),
    tts.WithModel(tts.ModelTurboV2_5),      // Fastest
    tts.WithOutputFormat(tts.EncodingPCM24), // 24kHz PCM
    tts.WithVoiceSettings(tts.VoiceSettings{
        Stability:       0.5,
        SimilarityBoost: 0.75,
        SpeakerBoost:    true,
    }),
)
```

**Models:**

| Model | Latency | Languages | Use Case |
|-------|---------|-----------|----------|
| `ModelTurboV2_5` | ~200ms | English | Fastest, recommended |
| `ModelFlashV2_5` | ~150ms | Multi | Fastest multilingual |
| `ModelMultilingualV2` | ~300ms | 29 languages | Best quality |

### OpenAI TTS (Built-in Voices)

Good fallback with consistent quality.

```go
provider, err := tts.NewOpenAI(
    tts.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
    tts.WithVoice(tts.VoiceShimmer),
    tts.WithModel(tts.ModelTTS1),
)
```

**Voices:** `VoiceAlloy`, `VoiceEcho`, `VoiceFable`, `VoiceOnyx`, `VoiceNova`, `VoiceShimmer`

### Provider Chain (Fallback)

Automatically try multiple providers in order.

```go
// Try ElevenLabs first, fall back to OpenAI
chain, err := tts.NewChain(elevenLabsProvider, openAIProvider)
if err != nil {
    panic(err)
}
defer chain.Close()

// Uses first successful provider
result, err := chain.Synthesize(ctx, "Hello world")
```

## Streaming Audio

For lowest latency, use streaming:

```go
stream, err := provider.Stream(ctx, "Long text to synthesize...")
if err != nil {
    panic(err)
}
defer stream.Close()

for {
    chunk, err := stream.Read()
    if err != nil {
        panic(err)
    }
    if chunk == nil {
        break // Stream complete
    }
    playChunk(chunk)
}
```

## Configuration Options

| Option | Description |
|--------|-------------|
| `WithAPIKey(key)` | API key for the provider |
| `WithVoice(id)` | Voice ID |
| `WithModel(id)` | Model ID |
| `WithOutputFormat(enc)` | Audio format (PCM16, MP3, etc.) |
| `WithVoiceSettings(s)` | Voice characteristics (ElevenLabs) |
| `WithTimeout(d)` | Request timeout |
| `WithStreamTimeout(d)` | Streaming timeout |
| `WithRetry(n, delay)` | Retry configuration |
| `WithLogger(logger)` | Structured logger (slog) |

## Audio Formats

| Encoding | Sample Rate | Description |
|----------|-------------|-------------|
| `EncodingPCM24` | 24kHz | Matches OpenAI Realtime API |
| `EncodingPCM44` | 44.1kHz | CD quality |
| `EncodingPCM22` | 22.05kHz | Voice-optimized |
| `EncodingPCM16` | 16kHz | Low bandwidth |
| `EncodingMP3` | 44.1kHz | Compressed |
| `EncodingOpus` | Variable | Efficient streaming |

## Testing

### Unit Tests

```bash
go test ./pkg/tts/...
```

### Integration Tests

```bash
# Set API keys
export ELEVENLABS_API_KEY="your-key"
export ELEVENLABS_VOICE_ID="your-voice-id"
export OPENAI_API_KEY="your-key"

# Run integration tests
go test -tags=integration -v ./pkg/tts/...
```

### Mock Provider

For testing code that uses TTS:

```go
mock := tts.NewMock()

// Custom behavior
mock.SynthesizeFunc = func(ctx context.Context, text string) (*tts.AudioResult, error) {
    return &tts.AudioResult{Audio: []byte("fake audio")}, nil
}

// Track calls
result, _ := mock.Synthesize(ctx, "Hello")
calls := mock.Calls()
assert.Equal(t, 1, mock.CallCount("Synthesize"))
```

## Error Handling

```go
result, err := provider.Synthesize(ctx, text)
if err != nil {
    var apiErr *tts.APIError
    if errors.As(err, &apiErr) {
        if apiErr.IsRateLimited() {
            // Retry with backoff
        }
        if apiErr.IsUnauthorized() {
            // Check API key
        }
    }
    return err
}
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `ELEVENLABS_API_KEY` | For ElevenLabs | API key from elevenlabs.io |
| `ELEVENLABS_VOICE_ID` | For ElevenLabs | Voice ID from Voice Lab |
| `OPENAI_API_KEY` | For OpenAI | OpenAI API key |

## Future Providers (Planned)

- **Piper** - Ultra-fast local TTS (ONNX)
- **Coqui** - Open source voice cloning
- **Bark** - Expressive, emotional TTS
- **Ollama** - Local LLM-based TTS

## Architecture

```
pkg/tts/
├── tts.go              # Provider interface
├── config.go           # Configuration + options
├── errors.go           # Error types
├── elevenlabs.go       # ElevenLabs implementation
├── openai.go           # OpenAI implementation  
├── chain.go            # Provider chain
├── mock.go             # Mock for testing
├── tts_test.go         # Unit tests
├── integration_test.go # Integration tests
└── README.md           # This file
```

## References

- [ElevenLabs API Docs](https://elevenlabs.io/docs)
- [OpenAI TTS API](https://platform.openai.com/docs/guides/text-to-speech)
- [Go Functional Options Pattern](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis)




