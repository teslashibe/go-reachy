// Package inference provides a unified interface for LLM and vision inference.
//
// The package abstracts chat completions and vision analysis behind a single
// Provider interface, enabling seamless switching between providers like
// OpenAI, Ollama, vLLM, Together, and others that implement the OpenAI-compatible API.
//
// Example usage:
//
//	client, _ := inference.NewClient(
//	    inference.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
//	    inference.WithModel("gpt-4o-mini"),
//	    inference.WithVisionModel("gpt-4o"),
//	)
//	defer client.Close()
//
//	// Chat
//	resp, _ := client.Chat(ctx, &inference.ChatRequest{
//	    Messages: []inference.Message{
//	        {Role: inference.RoleUser, Content: "Hello!"},
//	    },
//	})
//
//	// Vision
//	visionResp, _ := client.Vision(ctx, &inference.VisionRequest{
//	    Image:  frame,
//	    Prompt: "What do you see?",
//	})
package inference

import (
	"context"
	"image"
)

// Provider is the unified inference interface for chat and vision.
// All implementations must satisfy this interface.
type Provider interface {
	// Chat generates a response from a sequence of messages.
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Stream generates a streaming response for real-time output.
	Stream(ctx context.Context, req *ChatRequest) (Stream, error)

	// Vision analyzes an image with a text prompt.
	Vision(ctx context.Context, req *VisionRequest) (*VisionResponse, error)

	// Embed generates vector embeddings for text.
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)

	// Capabilities returns what features this provider supports.
	Capabilities() Capabilities

	// Health checks provider connectivity and API key validity.
	Health(ctx context.Context) error

	// Close releases any resources held by the provider.
	Close() error
}

// Stream is a streaming response for real-time output.
type Stream interface {
	// Recv returns the next chunk. Returns nil when stream is complete.
	Recv() (*StreamChunk, error)

	// Close stops the stream and releases resources.
	Close() error
}

// StreamChunk is a piece of a streaming response.
type StreamChunk struct {
	// Delta is the incremental text content.
	Delta string

	// FinishReason indicates why generation stopped (stop, length, tool_calls).
	FinishReason string

	// Done is true when the stream is complete.
	Done bool
}

// Capabilities describes what features a provider supports.
type Capabilities struct {
	Chat       bool // Supports chat completions
	Vision     bool // Supports image input
	Streaming  bool // Supports streaming responses
	Tools      bool // Supports function/tool calling
	Embeddings bool // Supports text embeddings
}

// ChatRequest for chat completions.
type ChatRequest struct {
	// Messages is the conversation history.
	Messages []Message

	// Model overrides the default model.
	Model string

	// MaxTokens limits the response length.
	MaxTokens int

	// Temperature controls randomness (0.0-2.0).
	Temperature float64

	// TopP controls nucleus sampling.
	TopP float64

	// Stop sequences that halt generation.
	Stop []string

	// Tools available for the model to call.
	Tools []Tool

	// ToolChoice controls tool use: "auto", "none", "required".
	ToolChoice string
}

// ChatResponse from chat completion.
type ChatResponse struct {
	// Message is the assistant's response.
	Message Message

	// FinishReason indicates why generation stopped.
	FinishReason string

	// Usage tracks token consumption.
	Usage Usage

	// Model used for generation.
	Model string

	// LatencyMs is the response time in milliseconds.
	LatencyMs int64
}

// VisionRequest for image analysis.
type VisionRequest struct {
	// Image to analyze (single image).
	Image image.Image

	// Images for multi-image analysis.
	Images []image.Image

	// Prompt describing what to analyze or ask about the image.
	Prompt string

	// Model overrides the default vision model.
	Model string

	// MaxTokens limits the response length.
	MaxTokens int

	// Temperature controls randomness.
	Temperature float64
}

// VisionResponse from image analysis.
type VisionResponse struct {
	// Content is the natural language response.
	Content string

	// Usage tracks token consumption.
	Usage Usage

	// Model used for analysis.
	Model string

	// LatencyMs is the response time in milliseconds.
	LatencyMs int64
}

// EmbedRequest for text embeddings.
type EmbedRequest struct {
	// Input texts to embed.
	Input []string

	// Model overrides the default embedding model.
	Model string
}

// EmbedResponse with vector embeddings.
type EmbedResponse struct {
	// Embeddings are the vector representations.
	Embeddings [][]float64

	// Usage tracks token consumption.
	Usage Usage

	// LatencyMs is the response time in milliseconds.
	LatencyMs int64
}

// Usage tracks token consumption for billing and limits.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
