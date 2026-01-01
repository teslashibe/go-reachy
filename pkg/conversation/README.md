# pkg/conversation

Unified interface for real-time voice conversation with AI agents.

## Overview

The `conversation` package provides a `Provider` interface that abstracts the complexity of WebSocket-based audio streaming, speech recognition, language model processing, and text-to-speech synthesis.

## Supported Providers

| Provider | Implementation | Custom Voice | LLM Choice | Tool Calling |
|----------|----------------|--------------|------------|--------------|
| ElevenLabs Agents ⭐ | `elevenlabs.go` | ✅ Custom cloned | ✅ Gemini/Claude/GPT-4o | ✅ |
| OpenAI Realtime | `openai.go` | ❌ Fixed voices | ❌ GPT-4o only | ✅ |

## Installation

```go
import "github.com/teslashibe/go-reachy/pkg/conversation"
```

## Quick Start

### ElevenLabs - Programmatic Agent (Recommended) ⭐

**No dashboard required!** All configuration in code:

```go
provider, err := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithVoiceID(os.Getenv("ELEVENLABS_VOICE_ID")),
    conversation.WithLLM("gemini-2.0-flash"),  // or "claude-3-5-sonnet", "gpt-4o"
    conversation.WithSystemPrompt("You are Eva, a friendly robot assistant."),
    conversation.WithAutoCreateAgent(true),
)
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Register tools before connecting
provider.RegisterTool(conversation.Tool{
    Name:        "describe_scene",
    Description: "Describes what Eva sees through her camera",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "focus": map[string]any{"type": "string"},
        },
    },
})

// Set up callbacks
provider.OnAudio(func(audio []byte) {
    speaker.Write(audio) // PCM16 @ 16kHz
})

provider.OnTranscript(func(role, text string, isFinal bool) {
    fmt.Printf("[%s] %s\n", role, text)
})

provider.OnToolCall(func(id, name string, args map[string]any) {
    result := executeToolI(name, args)
    provider.SubmitToolResult(id, result)
})

// Connect - agent is auto-created
ctx := context.Background()
if err := provider.Connect(ctx); err != nil {
    log.Fatal(err)
}

fmt.Printf("Agent ID: %s\n", provider.AgentID())

// Stream microphone audio
for audio := range microphone {
    provider.SendAudio(audio)
}
```

### ElevenLabs - Dashboard Agent (Legacy)

If you prefer to configure the agent in the ElevenLabs dashboard:

```go
provider, err := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithAgentID(os.Getenv("ELEVENLABS_AGENT_ID")), // From dashboard
)
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
conversation.WithSystemPrompt(prompt)  // System instruction
conversation.WithTemperature(0.8)      // Response randomness
conversation.WithTimeout(30*time.Second)
conversation.WithLogger(slog.Default())

// ElevenLabs - Programmatic (recommended)
conversation.WithVoiceID(id)           // Voice ID from ElevenLabs
conversation.WithLLM(model)            // "gemini-2.0-flash", "claude-3-5-sonnet", "gpt-4o"
conversation.WithAgentName(name)       // Name for dashboard reference
conversation.WithFirstMessage(msg)     // First message (empty = wait for user)
conversation.WithAutoCreateAgent(true) // Enable programmatic agent creation

// ElevenLabs - Dashboard (legacy)
conversation.WithAgentID(id)           // Agent ID from ElevenLabs dashboard

// OpenAI
conversation.WithVoice(voice)          // Voice: shimmer, alloy, echo, etc.
conversation.WithModel(model)          // Model (usually gpt-4o-realtime)
```

## Tool Calling

```go
// Register tools before connecting
provider.RegisterTool(conversation.Tool{
    Name:        "get_weather",
    Description: "Gets the current weather",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "location": map[string]any{
                "type":        "string",
                "description": "City name",
            },
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
# ElevenLabs - Programmatic (recommended)
ELEVENLABS_API_KEY=your-api-key
ELEVENLABS_VOICE_ID=your-voice-id    # Get from ElevenLabs dashboard or API

# ElevenLabs - Dashboard (legacy)
ELEVENLABS_API_KEY=your-api-key
ELEVENLABS_AGENT_ID=your-agent-id    # From ElevenLabs dashboard

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

## Comparison

| Feature | ElevenLabs ⭐ | OpenAI |
|---------|--------------|--------|
| Voice | Custom cloned | Fixed (shimmer, etc.) |
| LLM Choice | Gemini, Claude, GPT-4o | GPT-4o only |
| Programmatic Config | ✅ Full | ✅ Full |
| Latency | ~200-400ms | ~300-500ms |
| Tool Calling | ✅ | ✅ |
| Interruption | ✅ | ✅ |
| Sample Rate | 16kHz | 24kHz |

## LLM Options (ElevenLabs)

| Model | Description |
|-------|-------------|
| `gemini-2.0-flash` | Fast, cost-effective (default) |
| `claude-3-5-sonnet` | Best reasoning |
| `gpt-4o` | OpenAI's flagship |

## Migration from Dashboard to Programmatic

**Before (dashboard required):**
```go
provider, _ := conversation.NewElevenLabs(
    conversation.WithAPIKey(apiKey),
    conversation.WithAgentID(agentID), // Had to create in dashboard
)
```

**After (fully programmatic):**
```go
provider, _ := conversation.NewElevenLabs(
    conversation.WithAPIKey(apiKey),
    conversation.WithVoiceID(voiceID),
    conversation.WithLLM("gemini-2.0-flash"),
    conversation.WithSystemPrompt(instructions),
    conversation.WithAutoCreateAgent(true),
)

// Register tools
for _, tool := range myTools {
    provider.RegisterTool(tool)
}
```
