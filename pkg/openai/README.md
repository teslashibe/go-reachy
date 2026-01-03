# openai

OpenAI Realtime API client for low-latency voice conversations.

## Overview

This package provides a WebSocket client for OpenAI's Realtime API, enabling real-time speech-to-speech conversations with GPT-4.

## Features

- Real-time audio streaming (input and output)
- Voice activity detection (VAD)
- Function/tool calling
- Interruption handling
- Session configuration

## Usage

```go
client := openai.NewClient(apiKey)

// Configure callbacks
client.OnAudio(func(audio []byte) {
    player.Play(audio)
})

client.OnTranscript(func(role, text string, final bool) {
    fmt.Printf("[%s] %s\n", role, text)
})

client.OnFunctionCall(func(name string, args map[string]any) string {
    return handleToolCall(name, args)
})

// Register tools
client.RegisterTool(openai.Tool{
    Name:        "look",
    Description: "Look at something",
    Parameters:  schema,
})

// Connect
err := client.Connect(ctx)
if err != nil {
    return err
}
defer client.Close()

// Send audio
client.SendAudio(pcmData)
```

## Configuration

| Option | Description | Default |
|--------|-------------|---------|
| Model | GPT model to use | gpt-4o-realtime |
| Voice | TTS voice | alloy |
| Temperature | Response randomness | 0.8 |
| MaxTokens | Max response length | 4096 |

## Audio Format

- Input: 16kHz, 16-bit PCM, mono
- Output: 24kHz, 16-bit PCM, mono

## Events

| Event | Description |
|-------|-------------|
| `OnAudio` | Audio chunk received |
| `OnTranscript` | Transcript (partial or final) |
| `OnFunctionCall` | AI wants to call a function |
| `OnError` | Error occurred |
| `OnInterruption` | User interrupted AI |

