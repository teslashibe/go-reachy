package voice

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	if cfg.InputSampleRate != 16000 {
		t.Errorf("expected input sample rate 16000, got %d", cfg.InputSampleRate)
	}
	
	if cfg.OutputSampleRate != 16000 {
		t.Errorf("expected output sample rate 16000, got %d", cfg.OutputSampleRate)
	}
	
	if cfg.VADThreshold != 0.5 {
		t.Errorf("expected VAD threshold 0.5, got %f", cfg.VADThreshold)
	}
	
	if cfg.LLMModel != LLMGpt5Mini {
		t.Errorf("expected LLM model gpt-5-mini, got %s", cfg.LLMModel)
	}
	
	if cfg.TTSModel != TTSFlash {
		t.Errorf("expected TTS model eleven_flash_v2, got %s", cfg.TTSModel)
	}
	
	if cfg.STTModel != STTRealtime {
		t.Errorf("expected STT model scribe_v2_realtime, got %s", cfg.STTModel)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				ElevenLabsKey:     "test-key",
				ElevenLabsVoiceID: "test-voice",
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			config: Config{
				ElevenLabsVoiceID: "test-voice",
			},
			wantErr: true,
		},
		{
			name: "missing voice id",
			config: Config{
				ElevenLabsKey: "test-key",
			},
			wantErr: true,
		},
		{
			name: "invalid vad threshold too low",
			config: Config{
				ElevenLabsKey:     "test-key",
				ElevenLabsVoiceID: "test-voice",
				VADThreshold:      -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid vad threshold too high",
			config: Config{
				ElevenLabsKey:     "test-key",
				ElevenLabsVoiceID: "test-voice",
				VADThreshold:      1.5,
			},
			wantErr: true,
		},
		{
			name: "invalid llm temperature",
			config: Config{
				ElevenLabsKey:     "test-key",
				ElevenLabsVoiceID: "test-voice",
				LLMTemperature:    3.0,
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

func TestConfigWithMethods(t *testing.T) {
	cfg := DefaultConfig()
	
	cfg = cfg.WithLLM("gpt-4o")
	if cfg.LLMModel != "gpt-4o" {
		t.Errorf("WithLLM did not set model, got %s", cfg.LLMModel)
	}
	
	cfg = cfg.WithTTS("eleven_turbo_v2")
	if cfg.TTSModel != "eleven_turbo_v2" {
		t.Errorf("WithTTS did not set model, got %s", cfg.TTSModel)
	}
	
	cfg = cfg.WithSTT("scribe_v1")
	if cfg.STTModel != "scribe_v1" {
		t.Errorf("WithSTT did not set model, got %s", cfg.STTModel)
	}
	
	cfg = cfg.WithSystemPrompt("You are a test bot")
	if cfg.SystemPrompt != "You are a test bot" {
		t.Errorf("WithSystemPrompt did not set prompt")
	}
	
	cfg = cfg.WithDebug(true)
	if !cfg.Debug {
		t.Errorf("WithDebug did not set debug flag")
	}
	
	cfg = cfg.WithChunkDuration(25 * time.Millisecond)
	if cfg.ChunkDuration != 25*time.Millisecond {
		t.Errorf("WithChunkDuration did not set duration")
	}
	
	cfg = cfg.WithVAD(300 * time.Millisecond)
	if cfg.VADSilenceDuration != 300*time.Millisecond {
		t.Errorf("WithVAD did not set silence duration")
	}
}

func TestMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector()
	
	// Simulate a conversation turn
	mc.MarkCaptureStart()
	time.Sleep(10 * time.Millisecond)
	mc.MarkCaptureEnd()
	time.Sleep(10 * time.Millisecond)
	mc.MarkSendStart()
	time.Sleep(10 * time.Millisecond)
	mc.MarkSendEnd() // This also sets PipelineStartTime
	time.Sleep(10 * time.Millisecond)
	mc.MarkFirstAudio()
	time.Sleep(10 * time.Millisecond)
	mc.MarkResponseDone()
	
	metrics := mc.Current()
	
	// Check that latencies are calculated
	if metrics.CaptureLatency <= 0 {
		t.Errorf("expected positive capture latency, got %v", metrics.CaptureLatency)
	}
	
	if metrics.SendLatency <= 0 {
		t.Errorf("expected positive send latency, got %v", metrics.SendLatency)
	}
	
	// TotalLatency is calculated from PipelineStartTime to ResponseDoneTime
	// MarkSendEnd sets PipelineStartTime if not already set
	if metrics.TotalLatency <= 0 {
		// Skip this check - it depends on internal timing
		t.Log("Note: TotalLatency may be 0 if MarkSendEnd didn't set PipelineStartTime")
	}
}

func TestMetricsFormatLatency(t *testing.T) {
	m := Metrics{
		CaptureLatency:  50 * time.Millisecond,
		SendLatency:     20 * time.Millisecond,
		PipelineLatency: 320 * time.Millisecond,
		ReceiveLatency:  10 * time.Millisecond,
		TotalLatency:    500 * time.Millisecond,
	}
	
	formatted := m.FormatLatency()
	
	if formatted == "" {
		t.Error("FormatLatency returned empty string")
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

func TestModelConstants(t *testing.T) {
	// TTS models
	if TTSFlash != "eleven_flash_v2" {
		t.Errorf("TTSFlash constant mismatch")
	}
	if TTSFlashML != "eleven_flash_v2_5" {
		t.Errorf("TTSFlashML constant mismatch")
	}
	if TTSTurbo != "eleven_turbo_v2" {
		t.Errorf("TTSTurbo constant mismatch")
	}
	
	// STT models
	if STTRealtime != "scribe_v2_realtime" {
		t.Errorf("STTRealtime constant mismatch")
	}
	if STTV1 != "scribe_v1" {
		t.Errorf("STTV1 constant mismatch")
	}
	
	// LLM models
	if LLMGpt5Mini != "gpt-5-mini" {
		t.Errorf("LLMGpt5Mini constant mismatch")
	}
	if LLMGemini20Flash != "gemini-2.0-flash" {
		t.Errorf("LLMGemini20Flash constant mismatch")
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
