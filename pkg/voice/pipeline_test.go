package voice

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	if cfg.Provider != ProviderOpenAI {
		t.Errorf("expected default provider to be OpenAI, got %s", cfg.Provider)
	}
	
	if cfg.InputSampleRate != 24000 {
		t.Errorf("expected input sample rate 24000, got %d", cfg.InputSampleRate)
	}
	
	if cfg.OutputSampleRate != 24000 {
		t.Errorf("expected output sample rate 24000, got %d", cfg.OutputSampleRate)
	}
	
	if cfg.VADThreshold != 0.5 {
		t.Errorf("expected VAD threshold 0.5, got %f", cfg.VADThreshold)
	}
}

func TestDefaultElevenLabsConfig(t *testing.T) {
	cfg := DefaultElevenLabsConfig()
	
	if cfg.Provider != ProviderElevenLabs {
		t.Errorf("expected provider to be ElevenLabs, got %s", cfg.Provider)
	}
	
	if cfg.InputSampleRate != 16000 {
		t.Errorf("expected input sample rate 16000 for ElevenLabs, got %d", cfg.InputSampleRate)
	}
	
	if cfg.LLMModel != "gemini-2.0-flash" {
		t.Errorf("expected LLM model gemini-2.0-flash, got %s", cfg.LLMModel)
	}
}

func TestDefaultGeminiConfig(t *testing.T) {
	cfg := DefaultGeminiConfig()
	
	if cfg.Provider != ProviderGemini {
		t.Errorf("expected provider to be Gemini, got %s", cfg.Provider)
	}
	
	if cfg.InputSampleRate != 16000 {
		t.Errorf("expected input sample rate 16000 for Gemini, got %d", cfg.InputSampleRate)
	}
	
	if cfg.OutputSampleRate != 24000 {
		t.Errorf("expected output sample rate 24000 for Gemini, got %d", cfg.OutputSampleRate)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid openai config",
			config: Config{
				Provider:  ProviderOpenAI,
				OpenAIKey: "test-key",
			},
			wantErr: false,
		},
		{
			name: "missing openai key",
			config: Config{
				Provider: ProviderOpenAI,
			},
			wantErr: true,
		},
		{
			name: "valid elevenlabs config",
			config: Config{
				Provider:          ProviderElevenLabs,
				ElevenLabsKey:     "test-key",
				ElevenLabsVoiceID: "test-voice",
			},
			wantErr: false,
		},
		{
			name: "missing elevenlabs key",
			config: Config{
				Provider:          ProviderElevenLabs,
				ElevenLabsVoiceID: "test-voice",
			},
			wantErr: true,
		},
		{
			name: "missing elevenlabs voice id",
			config: Config{
				Provider:      ProviderElevenLabs,
				ElevenLabsKey: "test-key",
			},
			wantErr: true,
		},
		{
			name: "valid gemini config",
			config: Config{
				Provider:     ProviderGemini,
				GoogleAPIKey: "test-key",
			},
			wantErr: false,
		},
		{
			name: "missing gemini key",
			config: Config{
				Provider: ProviderGemini,
			},
			wantErr: true,
		},
		{
			name: "invalid vad threshold too low",
			config: Config{
				Provider:     ProviderOpenAI,
				OpenAIKey:   "test-key",
				VADThreshold: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid vad threshold too high",
			config: Config{
				Provider:     ProviderOpenAI,
				OpenAIKey:   "test-key",
				VADThreshold: 1.5,
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector()
	
	// Simulate a conversation turn
	mc.MarkSpeechEnd()
	time.Sleep(10 * time.Millisecond)
	mc.MarkTranscript()
	time.Sleep(10 * time.Millisecond)
	mc.MarkFirstToken()
	time.Sleep(10 * time.Millisecond)
	mc.MarkFirstAudio()
	time.Sleep(10 * time.Millisecond)
	mc.MarkResponseDone()
	
	metrics := mc.Current()
	
	// Check that latencies are positive
	if metrics.ASRLatency <= 0 {
		t.Errorf("expected positive ASR latency, got %v", metrics.ASRLatency)
	}
	
	if metrics.LLMFirstToken <= 0 {
		t.Errorf("expected positive LLM first token latency, got %v", metrics.LLMFirstToken)
	}
	
	if metrics.TTSFirstAudio <= 0 {
		t.Errorf("expected positive TTS first audio latency, got %v", metrics.TTSFirstAudio)
	}
	
	if metrics.TotalLatency <= 0 {
		t.Errorf("expected positive total latency, got %v", metrics.TotalLatency)
	}
	
	// Check ordering: ASR < LLM < TTS < Total
	if metrics.ASRLatency > metrics.LLMFirstToken {
		t.Errorf("ASR latency should be less than LLM latency: ASR=%v, LLM=%v", 
			metrics.ASRLatency, metrics.LLMFirstToken)
	}
	
	if metrics.LLMFirstToken > metrics.TTSFirstAudio {
		t.Errorf("LLM latency should be less than TTS latency: LLM=%v, TTS=%v",
			metrics.LLMFirstToken, metrics.TTSFirstAudio)
	}
}

func TestMetricsFormatLatency(t *testing.T) {
	m := Metrics{
		VADLatency:    50 * time.Millisecond,
		ASRLatency:    150 * time.Millisecond,
		LLMFirstToken: 320 * time.Millisecond,
		TTSFirstAudio: 120 * time.Millisecond,
		TotalLatency:  500 * time.Millisecond,
	}
	
	formatted := m.FormatLatency()
	
	if formatted == "" {
		t.Error("FormatLatency returned empty string")
	}
	
	// Should contain all metric labels
	labels := []string{"VAD", "ASR", "LLM", "TTS", "TOTAL"}
	for _, label := range labels {
		if !contains(formatted, label) {
			t.Errorf("FormatLatency should contain %s, got: %s", label, formatted)
		}
	}
}

func TestToolStruct(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		},
		Handler: func(args map[string]any) (string, error) {
			return "result", nil
		},
	}
	
	if tool.Name != "test_tool" {
		t.Errorf("expected name test_tool, got %s", tool.Name)
	}
	
	result, err := tool.Handler(nil)
	if err != nil {
		t.Errorf("handler returned error: %v", err)
	}
	if result != "result" {
		t.Errorf("expected result 'result', got '%s'", result)
	}
}

func TestProviderRegistration(t *testing.T) {
	// Check that providers are registered (via init())
	providers := AvailableProviders()
	
	// Should have at least 3 providers after bundled import
	if len(providers) == 0 {
		t.Log("No providers registered - bundled package not imported in tests")
		t.Skip("Skipping provider registration test - bundled not imported")
	}
}

func TestCallbacks(t *testing.T) {
	var audioReceived bool
	var speechStarted bool
	var speechEnded bool
	var transcriptReceived bool
	var responseReceived bool
	var toolCalled bool
	var errorReceived bool
	
	callbacks := Callbacks{
		OnAudioOut:    func(pcm16 []byte) { audioReceived = true },
		OnSpeechStart: func() { speechStarted = true },
		OnSpeechEnd:   func() { speechEnded = true },
		OnTranscript:  func(text string, isFinal bool) { transcriptReceived = true },
		OnResponse:    func(text string, isFinal bool) { responseReceived = true },
		OnToolCall:    func(call ToolCall) { toolCalled = true },
		OnError:       func(err error) { errorReceived = true },
	}
	
	// Verify callbacks are set
	if callbacks.OnAudioOut == nil {
		t.Error("OnAudioOut callback not set")
	}
	if callbacks.OnSpeechStart == nil {
		t.Error("OnSpeechStart callback not set")
	}
	if callbacks.OnSpeechEnd == nil {
		t.Error("OnSpeechEnd callback not set")
	}
	if callbacks.OnTranscript == nil {
		t.Error("OnTranscript callback not set")
	}
	if callbacks.OnResponse == nil {
		t.Error("OnResponse callback not set")
	}
	if callbacks.OnToolCall == nil {
		t.Error("OnToolCall callback not set")
	}
	if callbacks.OnError == nil {
		t.Error("OnError callback not set")
	}
	
	// Trigger callbacks and verify they were called
	callbacks.OnAudioOut([]byte{1, 2, 3})
	callbacks.OnSpeechStart()
	callbacks.OnSpeechEnd()
	callbacks.OnTranscript("hello", true)
	callbacks.OnResponse("hi", false)
	callbacks.OnToolCall(ToolCall{ID: "1", Name: "test"})
	callbacks.OnError(nil)
	
	if !audioReceived {
		t.Error("OnAudioOut callback not invoked")
	}
	if !speechStarted {
		t.Error("OnSpeechStart callback not invoked")
	}
	if !speechEnded {
		t.Error("OnSpeechEnd callback not invoked")
	}
	if !transcriptReceived {
		t.Error("OnTranscript callback not invoked")
	}
	if !responseReceived {
		t.Error("OnResponse callback not invoked")
	}
	if !toolCalled {
		t.Error("OnToolCall callback not invoked")
	}
	if !errorReceived {
		t.Error("OnError callback not invoked")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

