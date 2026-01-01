# pkg/speech

Speech processing handler (legacy/placeholder).

## Status

⚠️ **This package is a placeholder.** The actual speech processing is now handled by:

- `pkg/conversation` - Real-time voice conversation providers
- `pkg/realtime` - OpenAI Realtime API client (legacy)
- `pkg/tts` - Text-to-speech providers

## Original Intent

This package was intended to handle speech recognition and synthesis:

```go
handler := speech.NewHandler(apiKey, robot)
go handler.Run(ctx)
response, _ := handler.ProcessCommand("look left")
```

## Migration

Use `pkg/conversation` instead:

```go
provider, _ := conversation.NewElevenLabs(...)
provider.OnTranscript(func(role, text string, isFinal bool) {
    // Handle transcripts
})
```

