# pkg/conversation

Unified interface for real-time voice conversation with AI agents.

## Overview

The `conversation` package provides a `Provider` interface that abstracts the complexity of WebSocket-based audio streaming, speech recognition, language model processing, and text-to-speech synthesis.

## Supported Providers

| Provider | Implementation | Custom Voice | Tool Calling |
|----------|----------------|--------------|--------------|
| OpenAI Realtime | `openai.go` | ❌ Fixed voices | ✅ |
| ElevenLabs Agents | `elevenlabs.go` | ✅ Custom cloned | ✅ |

## Installation

```go
import "github.com/teslashibe/go-reachy/pkg/conversation"
```

## Quick Start

### ElevenLabs (Custom Voice)

```go
provider, err := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithAgentID(os.Getenv("ELEVENLABS_AGENT_ID")),
)
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Set up callbacks
provider.OnAudio(func(audio []byte) {
    // Play audio to speaker (PCM16 @ 16kHz)
    speaker.Write(audio)
})

provider.OnTranscript(func(role, text string, isFinal bool) {
    fmt.Printf("[%s] %s\n", role, text)
})

provider.OnToolCall(func(id, name string, args map[string]any) {
    result := executeToolI(name, args)
    provider.SubmitToolResult(id, result)
})

// Connect
ctx := context.Background()
if err := provider.Connect(ctx); err != nil {
    log.Fatal(err)
}

// Stream microphone audio
for audio := range microphone {
    provider.SendAudio(audio)
}
```

### OpenAI Realtime

```go
provider, err := conversation.NewOpenAI(
    conversation.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
    conversation.WithVoice(conversation.VoiceShimmer),
)
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Connect
if err := provider.Connect(ctx); err != nil {
    log.Fatal(err)
}

// Configure session
opts := conversation.SessionOptions{
    SystemPrompt: "You are Eva, a helpful robot assistant.",
    Voice:        conversation.VoiceShimmer,
    Tools: []conversation.Tool{
        {
            Name:        "describe_scene",
            Description: "Describes what Eva sees",
            Parameters:  map[string]any{"focus": map[string]any{"type": "string"}},
        },
    },
}
provider.ConfigureSession(opts)

// Stream audio...
```

## Provider Interface

```go
type Provider interface {
    // Connection lifecycle
    Connect(ctx context.Context) error
    Close() error
    IsConnected() bool

    // Audio streaming
    SendAudio(audio []byte) error

    // Event callbacks
    OnAudio(fn func(audio []byte))
    OnAudioDone(fn func())
    OnTranscript(fn func(role, text string, isFinal bool))
    OnToolCall(fn func(id, name string, args map[string]any))
    OnError(fn func(err error))
    OnInterruption(fn func())

    // Configuration
    ConfigureSession(opts SessionOptions) error
    RegisterTool(tool Tool)

    // Control
    CancelResponse() error
    SubmitToolResult(callID, result string) error

    // Info
    Capabilities() Capabilities
}
```

## Audio Format

| Provider | Input | Output |
|----------|-------|--------|
| OpenAI | PCM16 mono @ 24kHz | PCM16 mono @ 24kHz |
| ElevenLabs | PCM16 mono @ 16kHz | PCM16 mono @ 16kHz |

## Configuration Options

```go
// Common options
conversation.WithAPIKey(key)           // API key (required)
conversation.WithAgentID(id)           // Agent ID (ElevenLabs only)
conversation.WithModel(model)          // LLM model
conversation.WithVoice(voice)          // Voice ID
conversation.WithSystemPrompt(prompt)  // System instruction
conversation.WithTemperature(0.8)      // Response randomness
conversation.WithTimeout(30*time.Second)
conversation.WithLogger(slog.Default())
conversation.WithTools(tools...)
```

## Tool Calling

```go
// Register tools before connecting
provider.RegisterTool(conversation.Tool{
    Name:        "get_weather",
    Description: "Gets the current weather",
    Parameters: map[string]any{
        "location": map[string]any{
            "type":        "string",
            "description": "City name",
        },
    },
})

// Handle tool calls
provider.OnToolCall(func(id, name string, args map[string]any) {
    var result string
    switch name {
    case "get_weather":
        location := args["location"].(string)
        result = getWeather(location)
    default:
        result = "Unknown tool"
    }
    provider.SubmitToolResult(id, result)
})
```

## Error Handling

```go
// Check error types
if conversation.IsNotConnected(err) {
    // Reconnect
}

if conversation.IsRateLimited(err) {
    // Wait and retry
}

if conversation.IsRetryable(err) {
    // Safe to retry
}

// Handle API errors
var apiErr *conversation.APIError
if errors.As(err, &apiErr) {
    fmt.Printf("API error [%s]: %s\n", apiErr.Code, apiErr.Message)
}
```

## Testing

Use the `Mock` provider for testing:

```go
func TestMyHandler(t *testing.T) {
    mock := conversation.NewMock()
    
    // Set up your handler with mock
    handler := NewHandler(mock)
    
    // Connect
    mock.Connect(context.Background())
    
    // Simulate events
    mock.SimulateTranscript("user", "hello", true)
    mock.SimulateToolCall("call-1", "get_time", nil)
    
    // Assert
    if len(mock.ToolResults) != 1 {
        t.Error("expected tool result")
    }
}
```

## Environment Variables

```bash
# ElevenLabs
ELEVENLABS_API_KEY=your-api-key
ELEVENLABS_AGENT_ID=your-agent-id

# OpenAI
OPENAI_API_KEY=your-api-key
```

## Running Tests

```bash
# Unit tests
go test ./pkg/conversation/...

# Integration tests (requires API keys)
go test -tags=integration -v ./pkg/conversation/...
```

## ElevenLabs Agent Setup

1. Go to [ElevenLabs Agents Platform](https://elevenlabs.io/agents)
2. Create a new Agent
3. Configure:
   - **Voice**: Select or create a custom voice
   - **System Prompt**: Set Eva's personality
   - **LLM**: Choose GPT-4o, Claude, or custom
   - **Tools**: Add function definitions
4. Copy the Agent ID to your environment

## Comparison

| Feature | OpenAI Realtime | ElevenLabs Agents |
|---------|-----------------|-------------------|
| Voice | Fixed (shimmer, etc.) | Custom cloned |
| Latency | ~300-500ms | ~200-400ms |
| LLM | GPT-4o only | GPT-4o, Claude, custom |
| Tool Calling | ✅ | ✅ |
| Interruption | ✅ | ✅ |
| Sample Rate | 24kHz | 16kHz |


