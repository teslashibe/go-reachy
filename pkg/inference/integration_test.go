//go:build integration

package inference

import (
	"context"
	"os"
	"testing"
	"time"
)

// Integration tests for real API calls.
// Run with: go test -tags=integration -v ./pkg/inference/...

func TestOpenAIIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client, err := NewClient(
		WithAPIKey(apiKey),
		WithModel("gpt-4o-mini"),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test health
	t.Run("Health", func(t *testing.T) {
		err := client.Health(ctx)
		if err != nil {
			t.Errorf("Health check failed: %v", err)
		}
	})

	// Test chat
	t.Run("Chat", func(t *testing.T) {
		resp, err := client.Chat(ctx, &ChatRequest{
			Messages: []Message{
				NewSystemMessage("You are a helpful assistant. Be very brief."),
				NewUserMessage("What is 2+2?"),
			},
			MaxTokens: 50,
		})
		if err != nil {
			t.Fatalf("Chat failed: %v", err)
		}

		if resp.Message.Content == "" {
			t.Error("Expected non-empty response")
		}
		t.Logf("Response: %s", resp.Message.Content)
		t.Logf("Tokens: %d prompt, %d completion", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	})

	// Test streaming
	t.Run("Stream", func(t *testing.T) {
		stream, err := client.Stream(ctx, &ChatRequest{
			Messages: []Message{
				NewUserMessage("Count from 1 to 5, one number per line."),
			},
			MaxTokens: 100,
		})
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		defer stream.Close()

		var chunks int
		var content string
		for {
			chunk, err := stream.Recv()
			if err != nil {
				t.Fatalf("Stream recv error: %v", err)
			}
			if chunk.Done {
				break
			}
			chunks++
			content += chunk.Delta
		}

		t.Logf("Received %d chunks, total content: %s", chunks, content)
		if chunks == 0 {
			t.Error("Expected at least one chunk")
		}
	})

	// Test tools
	t.Run("Tools", func(t *testing.T) {
		resp, err := client.Chat(ctx, &ChatRequest{
			Messages: []Message{
				NewUserMessage("What's the weather like in Paris?"),
			},
			Tools: []Tool{
				NewTool("get_weather", "Get current weather for a city", map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type":        "string",
							"description": "The city name",
						},
					},
					"required": []string{"city"},
				}),
			},
			MaxTokens: 100,
		})
		if err != nil {
			t.Fatalf("Tool call failed: %v", err)
		}

		if len(resp.Message.ToolCalls) == 0 {
			// Model might answer directly sometimes
			t.Log("Model did not call tool (may have answered directly)")
		} else {
			tc := resp.Message.ToolCalls[0]
			t.Logf("Tool call: %s(%s)", tc.Name, tc.Arguments)
		}
	})
}

func TestOllamaIntegration(t *testing.T) {
	// Skip if Ollama isn't running locally
	client, err := NewClient(
		WithBaseURL("http://localhost:11434/v1"),
		WithModel("llama3.2"),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Quick health check to see if Ollama is running
	err = client.Health(ctx)
	if err != nil {
		t.Skip("Ollama not running: " + err.Error())
	}

	t.Run("Chat", func(t *testing.T) {
		resp, err := client.Chat(ctx, &ChatRequest{
			Messages: []Message{
				NewUserMessage("Say 'hello' and nothing else."),
			},
			MaxTokens: 10,
		})
		if err != nil {
			t.Fatalf("Chat failed: %v", err)
		}

		t.Logf("Ollama response: %s", resp.Message.Content)
	})
}

func TestChainIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	// Create chain with failing mock first, then real OpenAI
	failing := WithError(ErrProviderUnavailable)

	real, _ := NewClient(
		WithAPIKey(apiKey),
		WithModel("gpt-4o-mini"),
	)
	defer real.Close()

	chain, err := NewChain(failing, real)
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	defer chain.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := chain.Chat(ctx, &ChatRequest{
		Messages: []Message{
			NewUserMessage("Say 'fallback works' and nothing else."),
		},
		MaxTokens: 20,
	})
	if err != nil {
		t.Fatalf("Chain chat failed: %v", err)
	}

	t.Logf("Chain response (via fallback): %s", resp.Message.Content)
}


