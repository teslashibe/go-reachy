package inference

import (
	"context"
	"errors"
	"testing"
)

func TestChainFallback(t *testing.T) {
	ctx := context.Background()

	// First provider fails
	failing := WithError(errors.New("provider 1 failed"))

	// Second provider succeeds
	working := NewMock()
	working.ChatFunc = func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{
			Message:      NewAssistantMessage("From working provider"),
			FinishReason: "stop",
		}, nil
	}

	chain, err := NewChain(failing, working)
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	defer chain.Close()

	resp, err := chain.Chat(ctx, &ChatRequest{
		Messages: []Message{NewUserMessage("test")},
	})
	if err != nil {
		t.Fatalf("Chain chat failed: %v", err)
	}

	if resp.Message.Content != "From working provider" {
		t.Errorf("Unexpected response: %s", resp.Message.Content)
	}
}

func TestChainAllFail(t *testing.T) {
	ctx := context.Background()

	p1 := WithError(errors.New("provider 1 failed"))
	p2 := WithError(errors.New("provider 2 failed"))

	chain, _ := NewChain(p1, p2)
	defer chain.Close()

	_, err := chain.Chat(ctx, &ChatRequest{
		Messages: []Message{NewUserMessage("test")},
	})

	if err == nil {
		t.Fatal("Expected error when all providers fail")
	}

	chainErr, ok := err.(*ChainError)
	if !ok {
		t.Fatalf("Expected ChainError, got %T", err)
	}

	if len(chainErr.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(chainErr.Errors))
	}
}

func TestChainVision(t *testing.T) {
	ctx := context.Background()

	// Provider without vision
	noVision := NewMock()
	noVision.VisionFunc = nil
	noVision.CapabilitiesOverride = &Capabilities{Chat: true, Vision: false}

	// Provider with vision
	hasVision := NewMock()
	hasVision.VisionFunc = func(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
		return &VisionResponse{Content: "I see a cat"}, nil
	}

	chain, _ := NewChain(noVision, hasVision)
	defer chain.Close()

	resp, err := chain.Vision(ctx, &VisionRequest{Prompt: "What do you see?"})
	if err != nil {
		t.Fatalf("Chain vision failed: %v", err)
	}

	if resp.Content != "I see a cat" {
		t.Errorf("Unexpected response: %s", resp.Content)
	}
}

func TestChainCapabilities(t *testing.T) {
	// One provider with chat only
	chatOnly := NewMock()
	chatOnly.VisionFunc = nil
	chatOnly.CapabilitiesOverride = &Capabilities{Chat: true}

	// One provider with vision
	visionOnly := NewMock()
	visionOnly.ChatFunc = nil
	visionOnly.CapabilitiesOverride = &Capabilities{Vision: true}

	chain, _ := NewChain(chatOnly, visionOnly)
	defer chain.Close()

	caps := chain.Capabilities()
	if !caps.Chat {
		t.Error("Expected Chat capability from chain")
	}
	if !caps.Vision {
		t.Error("Expected Vision capability from chain")
	}
}

func TestChainHealth(t *testing.T) {
	ctx := context.Background()

	// One healthy, one unhealthy
	healthy := NewMock()
	unhealthy := WithError(errors.New("unhealthy"))

	chain, _ := NewChain(healthy, unhealthy)
	defer chain.Close()

	// Should pass because at least one is healthy
	err := chain.Health(ctx)
	if err != nil {
		t.Errorf("Health check should pass with at least one healthy provider: %v", err)
	}
}

func TestChainHealthAllUnhealthy(t *testing.T) {
	ctx := context.Background()

	p1 := WithError(errors.New("unhealthy 1"))
	p2 := WithError(errors.New("unhealthy 2"))

	chain, _ := NewChain(p1, p2)
	defer chain.Close()

	err := chain.Health(ctx)
	if err == nil {
		t.Error("Health check should fail when all providers are unhealthy")
	}
}

func TestChainEmpty(t *testing.T) {
	_, err := NewChain()
	if err == nil {
		t.Error("Expected error for empty chain")
	}
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("Expected ErrProviderUnavailable, got %v", err)
	}
}

func TestChainProviders(t *testing.T) {
	p1 := NewMock()
	p2 := NewMock()

	chain, _ := NewChain(p1, p2)
	defer chain.Close()

	providers := chain.Providers()
	if len(providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(providers))
	}
}

func TestChainStream(t *testing.T) {
	ctx := context.Background()

	// First provider fails streaming
	failing := NewMock()
	failing.StreamFunc = func(ctx context.Context, req *ChatRequest) (Stream, error) {
		return nil, errors.New("streaming failed")
	}
	failing.CapabilitiesOverride = &Capabilities{Streaming: true}

	// Second provider streams successfully
	working := NewMock()
	working.CapabilitiesOverride = &Capabilities{Chat: true, Streaming: true}

	chain, _ := NewChain(failing, working)
	defer chain.Close()

	stream, err := chain.Stream(ctx, &ChatRequest{
		Messages: []Message{NewUserMessage("test")},
	})
	if err != nil {
		t.Fatalf("Chain stream failed: %v", err)
	}
	defer stream.Close()

	chunk, err := stream.Recv()
	if err != nil {
		t.Fatalf("Stream recv failed: %v", err)
	}
	if chunk.Delta == "" && !chunk.Done {
		t.Error("Expected content or done")
	}
}

