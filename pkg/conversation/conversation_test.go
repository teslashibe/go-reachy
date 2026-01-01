package conversation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMockProvider(t *testing.T) {
	t.Run("connect and disconnect", func(t *testing.T) {
		m := NewMock()

		if m.IsConnected() {
			t.Error("should not be connected initially")
		}

		if err := m.Connect(context.Background()); err != nil {
			t.Errorf("connect failed: %v", err)
		}

		if !m.IsConnected() {
			t.Error("should be connected after Connect")
		}

		if err := m.Close(); err != nil {
			t.Errorf("close failed: %v", err)
		}

		if m.IsConnected() {
			t.Error("should not be connected after Close")
		}
	})

	t.Run("send audio when connected", func(t *testing.T) {
		m := NewMock()
		_ = m.Connect(context.Background())

		audio := []byte{1, 2, 3, 4}
		if err := m.SendAudio(audio); err != nil {
			t.Errorf("send audio failed: %v", err)
		}

		if len(m.AudioSent) != 1 {
			t.Errorf("expected 1 audio sent, got %d", len(m.AudioSent))
		}

		if string(m.AudioSent[0]) != string(audio) {
			t.Error("audio data mismatch")
		}
	})

	t.Run("send audio when not connected", func(t *testing.T) {
		m := NewMock()

		if err := m.SendAudio([]byte{1}); !errors.Is(err, ErrNotConnected) {
			t.Errorf("expected ErrNotConnected, got %v", err)
		}
	})

	t.Run("simulate callbacks", func(t *testing.T) {
		m := NewMock()

		var audioCalled bool
		var transcriptRole, transcriptText string
		var toolCallID, toolCallName string

		m.OnAudio(func(audio []byte) {
			audioCalled = true
		})

		m.OnTranscript(func(role, text string, isFinal bool) {
			transcriptRole = role
			transcriptText = text
		})

		m.OnToolCall(func(id, name string, args map[string]any) {
			toolCallID = id
			toolCallName = name
		})

		m.SimulateAudio([]byte{1, 2, 3})
		if !audioCalled {
			t.Error("audio callback not called")
		}

		m.SimulateTranscript("user", "hello", true)
		if transcriptRole != "user" || transcriptText != "hello" {
			t.Errorf("transcript mismatch: %s, %s", transcriptRole, transcriptText)
		}

		m.SimulateToolCall("call-1", "describe_scene", nil)
		if toolCallID != "call-1" || toolCallName != "describe_scene" {
			t.Errorf("tool call mismatch: %s, %s", toolCallID, toolCallName)
		}
	})

	t.Run("configure session", func(t *testing.T) {
		m := NewMock()

		opts := SessionOptions{
			SystemPrompt: "You are Eva",
			Voice:        "custom-voice",
		}

		if err := m.ConfigureSession(opts); err != nil {
			t.Errorf("configure session failed: %v", err)
		}

		if m.SessionOptions == nil {
			t.Error("session options not stored")
		}

		if m.SessionOptions.SystemPrompt != "You are Eva" {
			t.Error("system prompt mismatch")
		}
	})

	t.Run("register tools", func(t *testing.T) {
		m := NewMock()

		tool := Tool{
			Name:        "test_tool",
			Description: "A test tool",
		}

		m.RegisterTool(tool)

		tools := m.GetTools()
		if len(tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(tools))
		}

		if tools[0].Name != "test_tool" {
			t.Error("tool name mismatch")
		}
	})

	t.Run("submit tool result", func(t *testing.T) {
		m := NewMock()
		_ = m.Connect(context.Background())

		if err := m.SubmitToolResult("call-1", "result data"); err != nil {
			t.Errorf("submit tool result failed: %v", err)
		}

		if m.ToolResults["call-1"] != "result data" {
			t.Error("tool result not stored")
		}
	})

	t.Run("cancel response", func(t *testing.T) {
		m := NewMock()
		_ = m.Connect(context.Background())

		if err := m.CancelResponse(); err != nil {
			t.Errorf("cancel response failed: %v", err)
		}

		if !m.CancelCalled {
			t.Error("cancel not recorded")
		}
	})

	t.Run("capabilities", func(t *testing.T) {
		m := NewMock()
		caps := m.Capabilities()

		if !caps.SupportsToolCalls {
			t.Error("should support tool calls")
		}

		if !caps.SupportsCustomVoice {
			t.Error("should support custom voice")
		}

		if caps.InputSampleRate != 16000 {
			t.Errorf("expected 16000, got %d", caps.InputSampleRate)
		}
	})

	t.Run("reset", func(t *testing.T) {
		m := NewMock()
		_ = m.Connect(context.Background())
		_ = m.SendAudio([]byte{1})
		_ = m.SubmitToolResult("call-1", "result")
		_ = m.CancelResponse()

		m.Reset()

		if len(m.AudioSent) != 0 {
			t.Error("audio not reset")
		}

		if len(m.ToolResults) != 0 {
			t.Error("tool results not reset")
		}

		if m.CancelCalled {
			t.Error("cancel called not reset")
		}
	})
}

func TestFunctionalOptions(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		cfg := DefaultConfig()
		WithAPIKey("test-key")(cfg)

		if cfg.APIKey != "test-key" {
			t.Error("API key not set")
		}
	})

	t.Run("with agent ID", func(t *testing.T) {
		cfg := DefaultConfig()
		WithAgentID("agent-123")(cfg)

		if cfg.AgentID != "agent-123" {
			t.Error("agent ID not set")
		}
	})

	t.Run("with model", func(t *testing.T) {
		cfg := DefaultConfig()
		WithModel("gpt-4o")(cfg)

		if cfg.Model != "gpt-4o" {
			t.Error("model not set")
		}
	})

	t.Run("with voice", func(t *testing.T) {
		cfg := DefaultConfig()
		WithVoice("custom-voice")(cfg)

		if cfg.Voice != "custom-voice" {
			t.Error("voice not set")
		}
	})

	t.Run("with temperature", func(t *testing.T) {
		cfg := DefaultConfig()
		WithTemperature(0.5)(cfg)

		if cfg.Temperature != 0.5 {
			t.Error("temperature not set")
		}
	})

	t.Run("with timeout", func(t *testing.T) {
		cfg := DefaultConfig()
		WithTimeout(60 * time.Second)(cfg)

		if cfg.Timeout != 60*time.Second {
			t.Error("timeout not set")
		}
	})

	t.Run("with sample rates", func(t *testing.T) {
		cfg := DefaultConfig()
		WithInputSampleRate(24000)(cfg)
		WithOutputSampleRate(24000)(cfg)

		if cfg.InputSampleRate != 24000 {
			t.Error("input sample rate not set")
		}

		if cfg.OutputSampleRate != 24000 {
			t.Error("output sample rate not set")
		}
	})

	t.Run("with tools", func(t *testing.T) {
		cfg := DefaultConfig()
		tools := []Tool{
			{Name: "tool1"},
			{Name: "tool2"},
		}
		WithTools(tools...)(cfg)

		if len(cfg.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(cfg.Tools))
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Temperature != 0.8 {
		t.Errorf("expected temperature 0.8, got %f", cfg.Temperature)
	}

	if cfg.MaxResponseTokens != 4096 {
		t.Errorf("expected 4096 tokens, got %d", cfg.MaxResponseTokens)
	}

	if cfg.InputSampleRate != 16000 {
		t.Errorf("expected 16000 Hz, got %d", cfg.InputSampleRate)
	}

	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", cfg.Timeout)
	}

	if cfg.TurnDetection == nil {
		t.Error("turn detection should have defaults")
	}

	if cfg.TurnDetection.Type != "server_vad" {
		t.Error("turn detection type should be server_vad")
	}
}

func TestDefaultSessionOptions(t *testing.T) {
	opts := DefaultSessionOptions()

	if opts.Temperature != 0.8 {
		t.Errorf("expected temperature 0.8, got %f", opts.Temperature)
	}

	if opts.MaxResponseTokens != 4096 {
		t.Errorf("expected 4096 tokens, got %d", opts.MaxResponseTokens)
	}

	if opts.TurnDetection == nil {
		t.Error("turn detection should have defaults")
	}
}

func TestAPIError(t *testing.T) {
	t.Run("error message with code", func(t *testing.T) {
		err := NewAPIError(400, "invalid_request", "bad request")
		msg := err.Error()

		if msg != "conversation: API error [invalid_request]: bad request" {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("error message with status", func(t *testing.T) {
		err := &APIError{StatusCode: 500, Message: "internal error"}
		msg := err.Error()

		if msg != "conversation: API error (HTTP 500): internal error" {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("retryable errors", func(t *testing.T) {
		err429 := NewAPIError(429, "", "rate limited")
		if !err429.IsRetryable() {
			t.Error("429 should be retryable")
		}

		err500 := NewAPIError(500, "", "server error")
		if !err500.IsRetryable() {
			t.Error("500 should be retryable")
		}

		err400 := NewAPIError(400, "", "bad request")
		if err400.IsRetryable() {
			t.Error("400 should not be retryable")
		}
	})
}

func TestConnectionError(t *testing.T) {
	t.Run("error with cause", func(t *testing.T) {
		cause := errors.New("network error")
		err := NewConnectionError("dial failed", cause, true)

		msg := err.Error()
		if msg != "conversation: connection error: dial failed: network error" {
			t.Errorf("unexpected error message: %s", msg)
		}

		if err.Unwrap() != cause {
			t.Error("unwrap should return cause")
		}
	})

	t.Run("retryable flag", func(t *testing.T) {
		retryable := NewConnectionError("temp failure", nil, true)
		if !retryable.IsRetryable() {
			t.Error("should be retryable")
		}

		notRetryable := NewConnectionError("auth failure", nil, false)
		if notRetryable.IsRetryable() {
			t.Error("should not be retryable")
		}
	})
}

func TestErrorHelpers(t *testing.T) {
	t.Run("IsNotConnected", func(t *testing.T) {
		if !IsNotConnected(ErrNotConnected) {
			t.Error("should match ErrNotConnected")
		}

		if !IsNotConnected(ErrConnectionClosed) {
			t.Error("should match ErrConnectionClosed")
		}

		if IsNotConnected(ErrMissingAPIKey) {
			t.Error("should not match ErrMissingAPIKey")
		}
	})

	t.Run("IsRetryable", func(t *testing.T) {
		if !IsRetryable(ErrRateLimited) {
			t.Error("ErrRateLimited should be retryable")
		}

		if !IsRetryable(ErrTimeout) {
			t.Error("ErrTimeout should be retryable")
		}

		apiErr := NewAPIError(429, "", "")
		if !IsRetryable(apiErr) {
			t.Error("429 API error should be retryable")
		}

		connErr := NewConnectionError("", nil, true)
		if !IsRetryable(connErr) {
			t.Error("retryable connection error should be retryable")
		}
	})

	t.Run("IsRateLimited", func(t *testing.T) {
		if !IsRateLimited(ErrRateLimited) {
			t.Error("ErrRateLimited should be rate limited")
		}

		apiErr := NewAPIError(429, "", "")
		if !IsRateLimited(apiErr) {
			t.Error("429 API error should be rate limited")
		}
	})
}

func TestConnectionState(t *testing.T) {
	states := []struct {
		state    ConnectionState
		expected string
	}{
		{StateDisconnected, "disconnected"},
		{StateConnecting, "connecting"},
		{StateConnected, "connected"},
		{StateReconnecting, "reconnecting"},
		{ConnectionState(99), "unknown"},
	}

	for _, tc := range states {
		if tc.state.String() != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.state.String())
		}
	}
}

func TestConcurrentMockAccess(t *testing.T) {
	m := NewMock()
	_ = m.Connect(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.SendAudio([]byte{1, 2, 3})
			_ = m.IsConnected()
			m.SimulateAudio([]byte{4, 5, 6})
		}()
	}

	wg.Wait()

	if len(m.AudioSent) != 100 {
		t.Errorf("expected 100 audio sent, got %d", len(m.AudioSent))
	}
}


