# pkg/inference

Unified inference package for LLM chat and vision. Works with any OpenAI-compatible API.

## Features

- **Unified Interface**: Single `Provider` interface for chat, vision, streaming, and embeddings
- **OpenAI-Compatible**: Works with OpenAI, Ollama, vLLM, Together, Groq, LMStudio, and more
- **Provider Chain**: Automatic fallback between providers
- **Streaming**: SSE streaming for real-time responses
- **Tool Calling**: Full function/tool support
- **Idiomatic Go**: Functional options, structured logging, context support

## Quick Start

```go
import "github.com/teslashibe/go-reachy/pkg/inference"

// Create a client
client, _ := inference.NewClient(
    inference.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
    inference.WithModel("gpt-4o-mini"),
)
defer client.Close()

// Chat
resp, _ := client.Chat(ctx, &inference.ChatRequest{
    Messages: []inference.Message{
        inference.NewUserMessage("Hello!"),
    },
})
fmt.Println(resp.Message.Content)

// Vision
visionResp, _ := client.Vision(ctx, &inference.VisionRequest{
    Image:  myImage,
    Prompt: "What do you see?",
})
fmt.Println(visionResp.Content)
```

## Supported Providers

The `Client` works with any OpenAI-compatible API:

| Provider | Base URL | Notes |
|----------|----------|-------|
| OpenAI | `https://api.openai.com/v1` | Default |
| Ollama | `http://localhost:11434/v1` | No API key needed |
| vLLM | `http://localhost:8000/v1` | Local GPU server |
| Together | `https://api.together.xyz/v1` | Cloud GPUs |
| Groq | `https://api.groq.com/openai/v1` | Ultra-fast |
| LMStudio | `http://localhost:1234/v1` | Desktop app |

### Using Ollama

```go
client, _ := inference.NewClient(
    inference.WithBaseURL("http://localhost:11434/v1"),
    inference.WithModel("llama3.2"),
    inference.WithVisionModel("llava"),
)
```

## Streaming

```go
stream, _ := client.Stream(ctx, &inference.ChatRequest{
    Messages: []inference.Message{
        inference.NewUserMessage("Tell me a story"),
    },
})
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err != nil || chunk.Done {
        break
    }
    fmt.Print(chunk.Delta) // Print as it streams
}
```

## Tool Calling

```go
resp, _ := client.Chat(ctx, &inference.ChatRequest{
    Messages: []inference.Message{
        inference.NewUserMessage("What's the weather in Paris?"),
    },
    Tools: []inference.Tool{
        inference.NewTool("get_weather", "Get weather for a city", map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "city": map[string]interface{}{
                    "type": "string",
                },
            },
        }),
    },
})

if len(resp.Message.ToolCalls) > 0 {
    tc := resp.Message.ToolCalls[0]
    fmt.Printf("Call %s with %s\n", tc.Name, tc.Arguments)
}
```

## Provider Chain (Fallback)

```go
// Try Ollama first, fall back to OpenAI
ollama, _ := inference.NewClient(
    inference.WithBaseURL("http://localhost:11434/v1"),
    inference.WithModel("llama3.2"),
)

openai, _ := inference.NewClient(
    inference.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
)

chain, _ := inference.NewChain(ollama, openai)
defer chain.Close()

// If Ollama fails, automatically tries OpenAI
resp, _ := chain.Chat(ctx, &inference.ChatRequest{...})
```

## Configuration Options

```go
inference.NewClient(
    inference.WithBaseURL("https://api.openai.com/v1"),
    inference.WithAPIKey("sk-..."),
    inference.WithModel("gpt-4o-mini"),       // Chat model
    inference.WithVisionModel("gpt-4o"),       // Vision model
    inference.WithEmbedModel("text-embedding-3-small"),
    inference.WithMaxTokens(1024),
    inference.WithTemperature(0.7),
    inference.WithTimeout(30 * time.Second),
    inference.WithRetry(3, 100*time.Millisecond),
    inference.WithLogger(slog.Default()),
)
```

## Error Handling

```go
resp, err := client.Chat(ctx, req)
if err != nil {
    var apiErr *inference.APIError
    if errors.As(err, &apiErr) {
        if apiErr.IsRateLimited() {
            // Wait and retry
        }
        if apiErr.IsUnauthorized() {
            // Check API key
        }
    }
}
```

## Testing

Use the `Mock` provider for testing:

```go
mock := inference.NewMock()
mock.ChatFunc = func(ctx context.Context, req *inference.ChatRequest) (*inference.ChatResponse, error) {
    return &inference.ChatResponse{
        Message: inference.NewAssistantMessage("Test response"),
    }, nil
}

// Verify calls
mock.Chat(ctx, req)
assert.Equal(t, 1, mock.CallCount("Chat"))
```

## Environment Variables

```bash
# OpenAI (default)
OPENAI_API_KEY=sk-...

# Custom provider
INFERENCE_BASE_URL=http://localhost:11434/v1
INFERENCE_MODEL=llama3.2
INFERENCE_VISION_MODEL=llava
```

## Running Tests

```bash
# Unit tests
go test ./pkg/inference/...

# Integration tests (requires API keys)
OPENAI_API_KEY=sk-... go test -tags=integration ./pkg/inference/...
```

## Provider Interface

```go
type Provider interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req *ChatRequest) (Stream, error)
    Vision(ctx context.Context, req *VisionRequest) (*VisionResponse, error)
    Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)
    Capabilities() Capabilities
    Health(ctx context.Context) error
    Close() error
}
```

## License

MIT


