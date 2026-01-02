package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientChat(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("Expected /chat/completions, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Expected Bearer test-key, got %s", auth)
		}

		// Return mock response
		resp := chatCompletionResponse{
			ID:    "test-id",
			Model: "gpt-4o-mini",
			Choices: []struct {
				Message struct {
					Role      string        `json:"role"`
					Content   string        `json:"content"`
					ToolCalls []apiToolCall `json:"tool_calls"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Role      string        `json:"role"`
						Content   string        `json:"content"`
						ToolCalls []apiToolCall `json:"tool_calls"`
					}{
						Role:    "assistant",
						Content: "Hello! How can I help?",
					},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client, err := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
		WithModel("gpt-4o-mini"),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test chat
	ctx := context.Background()
	resp, err := client.Chat(ctx, &ChatRequest{
		Messages: []Message{
			NewUserMessage("Hello"),
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Message.Content != "Hello! How can I help?" {
		t.Errorf("Unexpected content: %s", resp.Message.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %s", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Expected 15 tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestClientChatWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request to verify tools are sent
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		tools, ok := reqBody["tools"].([]interface{})
		if !ok || len(tools) == 0 {
			t.Error("Expected tools in request")
		}

		// Return response with tool call
		resp := map[string]interface{}{
			"id":    "test-id",
			"model": "gpt-4o-mini",
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call-123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_weather",
									"arguments": `{"city": "London"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     20,
				"completion_tokens": 10,
				"total_tokens":      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("test-key"),
	)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Chat(ctx, &ChatRequest{
		Messages: []Message{
			NewUserMessage("What's the weather in London?"),
		},
		Tools: []Tool{
			NewTool("get_weather", "Get weather for a city", map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{
						"type": "string",
					},
				},
			}),
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}

	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call-123" {
		t.Errorf("Expected tool call ID 'call-123', got %s", tc.ID)
	}
	if tc.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %s", tc.Name)
	}
}

func TestClientHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("Expected /models, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{},
		})
	}))
	defer server.Close()

	client, _ := NewClient(WithBaseURL(server.URL))
	defer client.Close()

	err := client.Health(context.Background())
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Invalid API key",
				"code":    "invalid_api_key",
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(
		WithBaseURL(server.URL),
		WithAPIKey("bad-key"),
	)
	defer client.Close()

	ctx := context.Background()
	_, err := client.Chat(ctx, &ChatRequest{
		Messages: []Message{NewUserMessage("test")},
	})

	if err == nil {
		t.Fatal("Expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", apiErr.StatusCode)
	}
	if !apiErr.IsUnauthorized() {
		t.Error("Expected IsUnauthorized() to be true")
	}
}

func TestClientCapabilities(t *testing.T) {
	client, _ := NewClient()
	defer client.Close()

	caps := client.Capabilities()
	if !caps.Chat {
		t.Error("Expected Chat capability")
	}
	if !caps.Vision {
		t.Error("Expected Vision capability")
	}
	if !caps.Streaming {
		t.Error("Expected Streaming capability")
	}
	if !caps.Tools {
		t.Error("Expected Tools capability")
	}
	if !caps.Embeddings {
		t.Error("Expected Embeddings capability")
	}
}

func TestClientNoAPIKey(t *testing.T) {
	// For local providers like Ollama, no API key is required
	client, err := NewClient(
		WithBaseURL("http://localhost:11434/v1"),
		// No API key
	)
	if err != nil {
		t.Fatalf("Should allow creation without API key: %v", err)
	}
	client.Close()
}
