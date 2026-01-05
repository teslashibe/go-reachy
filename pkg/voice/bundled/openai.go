// Package bundled provides all-in-one voice pipeline implementations.
package bundled

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/voice"
)

const (
	openAIRealtimeURL = "wss://api.openai.com/v1/realtime"
	openAIModel       = "gpt-4o-realtime-preview-2024-12-17"
)

// openAIPendingToolCall represents a tool call waiting to be executed.
type openAIPendingToolCall struct {
	Name   string
	CallID string
	Args   map[string]any
}

// OpenAI implements voice.Pipeline using OpenAI's Realtime API.
// This provides GPT-4o with built-in VAD, ASR, and TTS in a single WebSocket.
type OpenAI struct {
	config voice.Config
	
	// WebSocket connection
	ws     *websocket.Conn
	wsMu   sync.Mutex
	
	// Tools
	tools    []voice.Tool
	toolsMap map[string]voice.Tool
	
	// Session state
	mu           sync.RWMutex
	connected    bool
	sessionReady bool
	closed       bool
	ctx          context.Context
	cancel       context.CancelFunc
	
	// Parallel tool execution
	pendingTools   []openAIPendingToolCall
	pendingToolsMu sync.Mutex
	toolBatchTimer *time.Timer
	
	// Metrics
	metrics *voice.MetricsCollector
	
	// Callbacks
	onAudioOut    func(pcm16 []byte)
	onSpeechStart func()
	onSpeechEnd   func()
	onTranscript  func(text string, isFinal bool)
	onResponse    func(text string, isFinal bool)
	onToolCall    func(call voice.ToolCall)
	onError       func(err error)
}

// NewOpenAI creates a new OpenAI Realtime pipeline.
func NewOpenAI(cfg voice.Config) (*OpenAI, error) {
	if cfg.OpenAIKey == "" {
		return nil, voice.ErrMissingAPIKey
	}
	
	return &OpenAI{
		config:   cfg,
		tools:    []voice.Tool{},
		toolsMap: make(map[string]voice.Tool),
		metrics:  voice.NewMetricsCollector(),
	}, nil
}

// Start establishes the WebSocket connection and begins processing.
func (o *OpenAI) Start(ctx context.Context) error {
	o.mu.Lock()
	if o.connected {
		o.mu.Unlock()
		return voice.ErrAlreadyStarted
	}
	o.mu.Unlock()
	
	o.ctx, o.cancel = context.WithCancel(ctx)
	
	url := fmt.Sprintf("%s?model=%s", openAIRealtimeURL, openAIModel)
	
	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + o.config.OpenAIKey}
	header["OpenAI-Beta"] = []string{"realtime=v1"}
	
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	var err error
	var resp *http.Response
	o.ws, resp, err = dialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("voice/openai: failed to connect: %w", err)
	}
	
	if resp != nil && o.config.Debug {
		debug.Logln("🎤 OpenAI Response Headers:")
		for key, values := range resp.Header {
			debug.Log("🎤   %s: %v\n", key, values)
		}
	}
	
	o.mu.Lock()
	o.connected = true
	o.closed = false
	o.mu.Unlock()
	
	// Configure session
	if err := o.configureSession(); err != nil {
		o.Stop()
		return fmt.Errorf("voice/openai: failed to configure session: %w", err)
	}
	
	go o.handleMessages()
	
	return nil
}

// Stop gracefully shuts down the pipeline.
func (o *OpenAI) Stop() error {
	o.mu.Lock()
	o.closed = true
	o.connected = false
	o.mu.Unlock()
	
	if o.cancel != nil {
		o.cancel()
	}
	
	// Clean up pending tool batch timer
	o.pendingToolsMu.Lock()
	if o.toolBatchTimer != nil {
		o.toolBatchTimer.Stop()
		o.toolBatchTimer = nil
	}
	o.pendingTools = nil
	o.pendingToolsMu.Unlock()
	
	if o.ws != nil {
		return o.ws.Close()
	}
	return nil
}

// IsConnected returns true if connected and ready.
func (o *OpenAI) IsConnected() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.connected && o.sessionReady && !o.closed
}

// SendAudio sends PCM16 audio to the pipeline.
func (o *OpenAI) SendAudio(pcm16 []byte) error {
	o.mu.RLock()
	if !o.connected || o.closed {
		o.mu.RUnlock()
		return voice.ErrNotConnected
	}
	o.mu.RUnlock()

	// Stage 2: Send timing - start
	o.metrics.MarkSendStart()

	o.metrics.IncrementAudioIn()

	encoded := base64.StdEncoding.EncodeToString(pcm16)
	msg := map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": encoded,
	}

	err := o.sendJSON(msg)

	// Stage 2: Send timing - end (also marks pipeline start)
	o.metrics.MarkSendEnd()

	return err
}

// OnAudioOut sets the callback for audio output.
func (o *OpenAI) OnAudioOut(fn func(pcm16 []byte)) {
	o.onAudioOut = fn
}

// OnSpeechStart sets the callback for speech start.
func (o *OpenAI) OnSpeechStart(fn func()) {
	o.onSpeechStart = fn
}

// OnSpeechEnd sets the callback for speech end.
func (o *OpenAI) OnSpeechEnd(fn func()) {
	o.onSpeechEnd = fn
}

// OnTranscript sets the callback for transcripts.
func (o *OpenAI) OnTranscript(fn func(text string, isFinal bool)) {
	o.onTranscript = fn
}

// OnResponse sets the callback for AI responses.
func (o *OpenAI) OnResponse(fn func(text string, isFinal bool)) {
	o.onResponse = fn
}

// OnError sets the error callback.
func (o *OpenAI) OnError(fn func(err error)) {
	o.onError = fn
}

// RegisterTool adds a tool the AI can invoke.
func (o *OpenAI) RegisterTool(tool voice.Tool) {
	o.tools = append(o.tools, tool)
	o.toolsMap[tool.Name] = tool
}

// OnToolCall sets the callback for tool invocations.
func (o *OpenAI) OnToolCall(fn func(call voice.ToolCall)) {
	o.onToolCall = fn
}

// SubmitToolResult returns a tool result to the AI.
func (o *OpenAI) SubmitToolResult(callID string, result string) error {
	msg := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  result,
		},
	}
	
	if err := o.sendJSON(msg); err != nil {
		return err
	}
	
	// Request response after tool result
	return o.sendJSON(map[string]string{
		"type": "response.create",
	})
}

// Interrupt stops the current AI response.
func (o *OpenAI) Interrupt() error {
	return o.sendJSON(map[string]string{
		"type": "response.cancel",
	})
}

// Metrics returns current latency metrics.
func (o *OpenAI) Metrics() voice.Metrics {
	return o.metrics.Current()
}

// Config returns the current configuration.
func (o *OpenAI) Config() voice.Config {
	return o.config
}

// UpdateConfig applies new configuration.
// Note: Some settings require reconnection to take effect.
func (o *OpenAI) UpdateConfig(cfg voice.Config) error {
	o.config = cfg
	
	// If connected, reconfigure session with new settings
	if o.IsConnected() {
		return o.configureSession()
	}
	return nil
}

// configureSession sets up the OpenAI session with current config.
func (o *OpenAI) configureSession() error {
	voice := o.config.TTSVoice
	if voice == "" {
		voice = "alloy"
	}
	
	apiTools := make([]map[string]any, len(o.tools))
	for i, tool := range o.tools {
		apiTools[i] = map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": tool.Parameters,
				"required":   []string{},
			},
		}
	}
	
	// Convert durations to milliseconds for API
	prefixPaddingMs := int(o.config.VADPrefixPadding.Milliseconds())
	if prefixPaddingMs == 0 {
		prefixPaddingMs = 300
	}
	silenceDurationMs := int(o.config.VADSilenceDuration.Milliseconds())
	if silenceDurationMs == 0 {
		silenceDurationMs = 500
	}
	
	threshold := o.config.VADThreshold
	if threshold == 0 {
		threshold = 0.5
	}
	
	msg := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":          []string{"text", "audio"},
			"instructions":        o.config.SystemPrompt,
			"voice":               voice,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"input_audio_transcription": map[string]any{
				"model": "whisper-1",
			},
			"turn_detection": map[string]any{
				"type":                "server_vad",
				"threshold":           threshold,
				"prefix_padding_ms":   prefixPaddingMs,
				"silence_duration_ms": silenceDurationMs,
			},
			"tools":       apiTools,
			"tool_choice": "auto",
		},
	}
	
	return o.sendJSON(msg)
}

// handleMessages processes incoming WebSocket messages.
func (o *OpenAI) handleMessages() {
	for {
		o.mu.RLock()
		closed := o.closed
		o.mu.RUnlock()
		
		if closed {
			return
		}
		
		_, message, err := o.ws.ReadMessage()
		if err != nil {
			o.mu.RLock()
			closed := o.closed
			o.mu.RUnlock()
			
			if !closed && o.onError != nil {
				o.onError(err)
			}
			return
		}
		
		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		
		msgType, _ := msg["type"].(string)
		
		switch msgType {
		case "session.created":
			o.mu.Lock()
			o.sessionReady = true
			o.mu.Unlock()
			if o.config.Debug {
				debug.Logln("🎤 OpenAI session created")
			}
			
		case "session.updated":
			if o.config.Debug {
				debug.Logln("🎤 Session configured")
			}
			
		case "input_audio_buffer.speech_started":
			if o.config.Debug {
				debug.Logln("🎤 VAD: Speech started")
			}
			if o.onSpeechStart != nil {
				o.onSpeechStart()
			}
			
		case "input_audio_buffer.speech_stopped":
			// Stage 4: Receive timing - start
			o.metrics.MarkReceiveStart()
			if o.config.Debug {
				debug.Logln("🎤 VAD: Speech stopped")
			}
			// Stage 3: Pipeline timing - VAD detected speech end
			o.metrics.MarkPipelineStart()
			if o.onSpeechEnd != nil {
				o.onSpeechEnd()
			}
			
		case "conversation.item.input_audio_transcription.completed":
			o.metrics.MarkTranscript()
			if transcript, ok := msg["transcript"].(string); ok && o.onTranscript != nil {
				o.onTranscript(transcript, true)
			}
			
		case "response.audio.delta":
			// Stage 3: Pipeline timing - first audio received
			o.metrics.MarkPipelineEnd()
			o.metrics.IncrementAudioOut()
			if delta, ok := msg["delta"].(string); ok && o.onAudioOut != nil {
				audioData, err := base64.StdEncoding.DecodeString(delta)
				if err == nil {
					o.onAudioOut(audioData)
				}
			}
			
		case "response.audio.done":
			// Stage 4: Receive timing - end
			o.metrics.MarkReceiveEnd()
			o.metrics.MarkResponseDone()
			if o.config.ProfileLatency {
				m := o.metrics.Current()
				fmt.Printf("⏱️  %s\n", m.FormatLatency())
			}
			// Reset metrics for next turn
			o.metrics.Reset()
			
		case "response.audio_transcript.delta":
			o.metrics.MarkFirstToken()
			if delta, ok := msg["delta"].(string); ok && o.onResponse != nil {
				o.onResponse(delta, false)
			}
			
		case "response.audio_transcript.done":
			if o.onResponse != nil {
				// Final response, extract full transcript if available
				if transcript, ok := msg["transcript"].(string); ok {
					o.onResponse(transcript, true)
				}
			}
			
		case "response.function_call_arguments.done":
			o.handleFunctionCall(msg)
			
		case "error":
			if errData, ok := msg["error"].(map[string]any); ok {
				if errMsg, ok := errData["message"].(string); ok {
					if o.onError != nil {
						o.onError(fmt.Errorf("OpenAI API error: %s", errMsg))
					}
				}
			}
			
		default:
			if o.config.Debug && msgType != "" && 
				msgType != "response.audio.delta" && 
				msgType != "response.audio_transcript.delta" {
				debug.Log("🎤 Message: %s\n", msgType)
			}
		}
	}
}

// Tool batch execution window
const openAIToolBatchWindow = 50 * time.Millisecond

// handleFunctionCall queues a tool call for parallel execution.
func (o *OpenAI) handleFunctionCall(msg map[string]any) {
	name, _ := msg["name"].(string)
	callID, _ := msg["call_id"].(string)
	argsStr, _ := msg["arguments"].(string)
	
	if o.config.Debug {
		fmt.Printf("🔧 Tool queued: %s\n", name)
	}
	
	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		args = make(map[string]any)
	}
	
	// If external callback is set, use that instead of internal handling
	if o.onToolCall != nil {
		o.onToolCall(voice.ToolCall{
			ID:        callID,
			Name:      name,
			Arguments: args,
		})
		return
	}
	
	// Internal tool handling with batching
	o.pendingToolsMu.Lock()
	o.pendingTools = append(o.pendingTools, openAIPendingToolCall{
		Name:   name,
		CallID: callID,
		Args:   args,
	})
	
	if o.toolBatchTimer != nil {
		o.toolBatchTimer.Stop()
	}
	o.toolBatchTimer = time.AfterFunc(openAIToolBatchWindow, o.executeToolBatch)
	o.pendingToolsMu.Unlock()
}

// executeToolBatch executes all pending tools in parallel.
func (o *OpenAI) executeToolBatch() {
	o.pendingToolsMu.Lock()
	tools := o.pendingTools
	o.pendingTools = nil
	o.pendingToolsMu.Unlock()
	
	if len(tools) == 0 {
		return
	}
	
	startTime := time.Now()
	if o.config.Debug {
		fmt.Printf("🔧 Executing %d tools in parallel...\n", len(tools))
	}
	
	var wg sync.WaitGroup
	results := make([]struct {
		CallID string
		Result string
	}, len(tools))
	
	for i, tool := range tools {
		wg.Add(1)
		go func(idx int, t openAIPendingToolCall) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = struct {
						CallID string
						Result string
					}{t.CallID, fmt.Sprintf("Error: tool panicked: %v", r)}
				}
			}()
			
			var result string
			if handler, ok := o.toolsMap[t.Name]; ok && handler.Handler != nil {
				var err error
				result, err = handler.Handler(t.Args)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
			} else {
				result = "Function not found"
			}
			
			results[idx] = struct {
				CallID string
				Result string
			}{t.CallID, result}
		}(i, tool)
	}
	
	wg.Wait()
	
	if o.config.Debug {
		elapsed := time.Since(startTime)
		fmt.Printf("🔧 All %d tools completed in %dms\n", len(tools), elapsed.Milliseconds())
	}
	
	// Check connection before sending results
	if !o.IsConnected() {
		return
	}
	
	// Send all results back
	for _, r := range results {
		msg := map[string]any{
			"type": "conversation.item.create",
			"item": map[string]any{
				"type":    "function_call_output",
				"call_id": r.CallID,
				"output":  r.Result,
			},
		}
		if err := o.sendJSON(msg); err != nil {
			if o.onError != nil {
				o.onError(fmt.Errorf("failed to send tool result: %w", err))
			}
			return
		}
	}
	
	// Request response after all tool results
	if err := o.sendJSON(map[string]string{"type": "response.create"}); err != nil {
		if o.onError != nil {
			o.onError(fmt.Errorf("failed to request response: %w", err))
		}
	}
}

// sendJSON sends a JSON message over WebSocket.
func (o *OpenAI) sendJSON(v any) error {
	o.wsMu.Lock()
	defer o.wsMu.Unlock()
	
	if o.ws == nil {
		return voice.ErrNotConnected
	}
	
	return o.ws.WriteJSON(v)
}

// App-side timing markers

// MarkCaptureStart records when WebRTC delivered audio to Eva.
func (o *OpenAI) MarkCaptureStart() {
	o.metrics.MarkCaptureStart()
}

// MarkCaptureEnd records when audio is buffered and ready to send.
func (o *OpenAI) MarkCaptureEnd() {
	o.metrics.MarkCaptureEnd()
}

// MarkPlaybackStart records when audio was sent to GStreamer.
func (o *OpenAI) MarkPlaybackStart() {
	o.metrics.MarkPlaybackStart()
}

// MarkPlaybackEnd records when audio playback completed.
func (o *OpenAI) MarkPlaybackEnd() {
	o.metrics.MarkPlaybackEnd()
}

// Ensure OpenAI implements voice.Pipeline at compile time.
var _ voice.Pipeline = (*OpenAI)(nil)

// Register OpenAI provider in voice package.
func init() {
	voice.Register(voice.ProviderOpenAI, func(cfg voice.Config) (voice.Pipeline, error) {
		return NewOpenAI(cfg)
	})
}

