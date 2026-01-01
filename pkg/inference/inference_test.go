package inference

import (
	"context"
	"errors"
	"testing"
)

func TestMockProvider(t *testing.T) {
	ctx := context.Background()
	mock := NewMock()

	// Test Chat
	resp, err := mock.Chat(ctx, &ChatRequest{
		Messages: []Message{NewUserMessage("Hello")},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Message.Content == "" {
		t.Error("Expected content in response")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %s", resp.FinishReason)
	}

	// Test Vision
	visionResp, err := mock.Vision(ctx, &VisionRequest{
		Prompt: "What do you see?",
	})
	if err != nil {
		t.Fatalf("Vision failed: %v", err)
	}
	if visionResp.Content == "" {
		t.Error("Expected content in vision response")
	}

	// Test call tracking
	if mock.CallCount("Chat") != 1 {
		t.Errorf("Expected 1 Chat call, got %d", mock.CallCount("Chat"))
	}
	if mock.CallCount("Vision") != 1 {
		t.Errorf("Expected 1 Vision call, got %d", mock.CallCount("Vision"))
	}

	// Test all calls
	calls := mock.Calls()
	if len(calls) != 2 {
		t.Errorf("Expected 2 calls, got %d", len(calls))
	}

	// Test reset
	mock.Reset()
	if len(mock.Calls()) != 0 {
		t.Error("Expected 0 calls after reset")
	}
}

func TestMockWithError(t *testing.T) {
	ctx := context.Background()
	testErr := errors.New("test error")
	mock := WithError(testErr)

	_, err := mock.Chat(ctx, &ChatRequest{})
	if !errors.Is(err, testErr) {
		t.Errorf("Expected test error, got: %v", err)
	}

	_, err = mock.Vision(ctx, &VisionRequest{})
	if !errors.Is(err, testErr) {
		t.Errorf("Expected test error, got: %v", err)
	}
}

func TestFunctionalOptions(t *testing.T) {
	cfg := DefaultConfig()

	// Apply options
	cfg.Apply(
		WithBaseURL("http://localhost:11434/v1"),
		WithAPIKey("test-key"),
		WithModel("llama3"),
		WithVisionModel("llava"),
		WithMaxTokens(512),
		WithTemperature(0.5),
	)

	if cfg.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("Expected Ollama URL, got %s", cfg.BaseURL)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("Expected test-key, got %s", cfg.APIKey)
	}
	if cfg.Model != "llama3" {
		t.Errorf("Expected llama3, got %s", cfg.Model)
	}
	if cfg.VisionModel != "llava" {
		t.Errorf("Expected llava, got %s", cfg.VisionModel)
	}
	if cfg.MaxTokens != 512 {
		t.Errorf("Expected 512, got %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Expected 0.5, got %f", cfg.Temperature)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("Expected OpenAI URL, got %s", cfg.BaseURL)
	}
	if cfg.Model != "gpt-4o-mini" {
		t.Errorf("Expected gpt-4o-mini, got %s", cfg.Model)
	}
	if cfg.VisionModel != "gpt-4o" {
		t.Errorf("Expected gpt-4o, got %s", cfg.VisionModel)
	}
	if cfg.MaxTokens != 1024 {
		t.Errorf("Expected 1024, got %d", cfg.MaxTokens)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("Expected 3 retries, got %d", cfg.MaxRetries)
	}
}

func TestAPIError(t *testing.T) {
	// Test rate limit
	err := &APIError{StatusCode: 429, Message: "rate limited", Provider: "test"}
	if !err.IsRateLimited() {
		t.Error("Expected IsRateLimited() to be true")
	}
	if !err.IsRetryable() {
		t.Error("Expected IsRetryable() to be true for 429")
	}

	// Test unauthorized
	err = &APIError{StatusCode: 401, Message: "unauthorized", Provider: "test"}
	if !err.IsUnauthorized() {
		t.Error("Expected IsUnauthorized() to be true")
	}
	if err.IsRetryable() {
		t.Error("Expected IsRetryable() to be false for 401")
	}

	// Test server error
	err = &APIError{StatusCode: 500, Message: "server error", Provider: "test"}
	if !err.IsServerError() {
		t.Error("Expected IsServerError() to be true")
	}
	if !err.IsRetryable() {
		t.Error("Expected IsRetryable() to be true for 500")
	}

	// Test error string with code
	err = &APIError{StatusCode: 400, Message: "bad request", Code: "invalid_api_key", Provider: "test"}
	errStr := err.Error()
	if errStr == "" {
		t.Error("Expected non-empty error string")
	}
}

func TestChainError(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	chainErr := &ChainError{Errors: []error{err1, err2}}
	if chainErr.Unwrap() != err2 {
		t.Error("Unwrap should return last error")
	}

	errStr := chainErr.Error()
	if errStr == "" {
		t.Error("Expected non-empty error string")
	}
}

func TestMessageHelpers(t *testing.T) {
	// Test NewSystemMessage
	sys := NewSystemMessage("You are helpful")
	if sys.Role != RoleSystem || sys.Content != "You are helpful" {
		t.Error("NewSystemMessage failed")
	}

	// Test NewUserMessage
	user := NewUserMessage("Hello")
	if user.Role != RoleUser || user.Content != "Hello" {
		t.Error("NewUserMessage failed")
	}

	// Test NewAssistantMessage
	asst := NewAssistantMessage("Hi there")
	if asst.Role != RoleAssistant || asst.Content != "Hi there" {
		t.Error("NewAssistantMessage failed")
	}

	// Test NewToolMessage
	tool := NewToolMessage("call-123", "result")
	if tool.Role != RoleTool || tool.ToolCallID != "call-123" || tool.Content != "result" {
		t.Error("NewToolMessage failed")
	}
}

func TestNewTool(t *testing.T) {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
		},
	}

	tool := NewTool("search", "Search the web", params)
	if tool.Type != "function" {
		t.Errorf("Expected type 'function', got %s", tool.Type)
	}
	if tool.Function.Name != "search" {
		t.Errorf("Expected name 'search', got %s", tool.Function.Name)
	}
	if tool.Function.Description != "Search the web" {
		t.Error("Description mismatch")
	}
}

func TestCapabilities(t *testing.T) {
	mock := NewMock()
	caps := mock.Capabilities()

	if !caps.Chat {
		t.Error("Expected Chat capability")
	}
	if !caps.Vision {
		t.Error("Expected Vision capability")
	}
	if !caps.Streaming {
		t.Error("Expected Streaming capability")
	}
}

func TestMockLastCall(t *testing.T) {
	mock := NewMock()

	// No calls yet
	if mock.LastCall() != nil {
		t.Error("Expected nil LastCall before any calls")
	}

	// Make a call
	ctx := context.Background()
	mock.Chat(ctx, &ChatRequest{})

	last := mock.LastCall()
	if last == nil {
		t.Fatal("Expected non-nil LastCall after call")
	}
	if last.Method != "Chat" {
		t.Errorf("Expected method 'Chat', got %s", last.Method)
	}
}



