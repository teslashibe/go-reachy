# tts

Text-to-Speech providers for Eva.

## Overview

This package provides multiple TTS backends for converting text to speech.

## Providers

### OpenAI TTS

Uses OpenAI's TTS API for high-quality voice synthesis.

```go
provider := tts.NewOpenAI(apiKey, "nova")
audio, err := provider.Synthesize(ctx, "Hello, world!")
```

### ElevenLabs TTS

Uses ElevenLabs for ultra-realistic voice synthesis.

```go
provider := tts.NewElevenLabs(apiKey, voiceID)
audio, err := provider.Synthesize(ctx, "Hello, world!")
```

### ElevenLabs WebSocket Streaming

Low-latency streaming TTS via WebSocket.

```go
provider := tts.NewElevenLabsWS(apiKey, voiceID)
err := provider.Connect(ctx)

provider.OnAudio(func(chunk []byte) {
    player.Play(chunk)
})

provider.StreamText("Hello, world!")
```

### Chain Provider

Chains multiple providers with fallback.

```go
chain := tts.NewChain(primary, fallback)
audio, err := chain.Synthesize(ctx, text)
```

## Interface

All providers implement:

```go
type Provider interface {
    Synthesize(ctx context.Context, text string) ([]byte, error)
    Voice() string
    Close() error
}
```

## Configuration

| Env Variable | Description |
|--------------|-------------|
| `OPENAI_API_KEY` | OpenAI API key |
| `ELEVENLABS_API_KEY` | ElevenLabs API key |

## Voice Options

### OpenAI
- alloy, echo, fable, onyx, nova, shimmer

### ElevenLabs
- Use voice ID from ElevenLabs dashboard
- Common: lily, rachel, drew, etc.

## Audio Output

All providers output:
- 16kHz or 24kHz sample rate
- 16-bit PCM
- Mono channel
