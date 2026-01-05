package bundled

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/voice"
)

const (
	// Gemini Live API WebSocket endpoint
	geminiLiveURL = "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent"

	// Default model for Gemini Live
	// gemini-2.5-flash-native-audio-preview is optimized for low-latency audio
	geminiDefaultModel = "models/gemini-2.5-flash-native-audio-preview-12-2025"
)

// Gemini implements voice.Pipeline using Google's Gemini Live API.
// This provides the lowest latency speech-to-speech experience with
// Gemini 2.0 Flash handling VAD, ASR, LLM, and TTS in a single stream.
type Gemini struct {
	config voice.Config

	// WebSocket connection
	ws   *websocket.Conn
	wsMu sync.Mutex

	// Tools
	tools    []voice.Tool
	toolsMap map[string]voice.Tool

	// Session state
	mu        sync.RWMutex
	connected bool
	closed    bool
	ctx       context.Context
	cancel    context.CancelFunc

	// Metrics
	metrics *voice.MetricsCollector

	// Latency tracking
	lastAudioSentTime  time.Time
	firstAudioReceived bool
	latencyMu          sync.Mutex

	// Callbacks
	onAudioOut    func(pcm16 []byte)
	onSpeechStart func()
	onSpeechEnd   func()
	onTranscript  func(text string, isFinal bool)
	onResponse    func(text string, isFinal bool)
	onToolCall    func(call voice.ToolCall)
	onError       func(err error)
}

// NewGemini creates a new Gemini Live pipeline.
func NewGemini(cfg voice.Config) (*Gemini, error) {
	if cfg.GoogleAPIKey == "" {
		return nil, voice.ErrMissingAPIKey
	}

	return &Gemini{
		config:   cfg,
		tools:    []voice.Tool{},
		toolsMap: make(map[string]voice.Tool),
		metrics:  voice.NewMetricsCollector(),
	}, nil
}

// Start establishes the WebSocket connection and begins processing.
func (g *Gemini) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.connected {
		g.mu.Unlock()
		return voice.ErrAlreadyStarted
	}
	g.mu.Unlock()

	g.ctx, g.cancel = context.WithCancel(ctx)

	// Build WebSocket URL with API key
	model := g.config.LLMModel
	if model == "" {
		model = geminiDefaultModel
	}

	url := fmt.Sprintf("%s?key=%s", geminiLiveURL, g.config.GoogleAPIKey)

	header := make(http.Header)
	header.Set("Content-Type", "application/json")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	var err error
	g.ws, _, err = dialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("voice/gemini: failed to connect: %w", err)
	}

	g.mu.Lock()
	g.connected = true
	g.closed = false
	g.mu.Unlock()

	// Send setup message
	if err := g.sendSetup(model); err != nil {
		g.Stop()
		return fmt.Errorf("voice/gemini: failed to configure session: %w", err)
	}

	go g.handleMessages()

	if g.config.Debug {
		debug.Logln("🌟 Gemini Live connected")
	}

	return nil
}

// sendSetup sends the initial configuration to Gemini Live.
func (g *Gemini) sendSetup(model string) error {
	// Build tools for Gemini format (convert to proper JSON Schema)
	var toolDeclarations []map[string]any
	for _, tool := range g.tools {
		// Eva tools have Parameters as map[string]any where keys are param names
		// Gemini expects JSON Schema: { type: "object", properties: {...}, required: [...] }
		params := convertToJSONSchema(tool.Parameters)
		toolDeclarations = append(toolDeclarations, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  params,
		})
	}

	// Voice selection (Gemini voices: Puck, Charon, Kore, Fenrir, Aoede)
	voiceName := g.config.TTSVoice
	if voiceName == "" {
		voiceName = "Puck" // Default voice
	}

	// Native audio models may not support voice_config, so we conditionally include it
	generationConfig := map[string]any{
		"response_modalities": []string{"AUDIO"},
	}

	// Only add speech_config for non-native-audio models
	if !strings.Contains(model, "native-audio") {
		generationConfig["speech_config"] = map[string]any{
			"voice_config": map[string]any{
				"prebuilt_voice_config": map[string]any{
					"voice_name": voiceName,
				},
			},
		}
	}

	// Build VAD configuration for realtime input
	// See: https://ai.google.dev/gemini-api/docs/live-guide
	startSensitivity := g.config.VADStartSensitivity
	if startSensitivity == "" {
		startSensitivity = "START_SENSITIVITY_HIGH" // Faster speech detection
	} else {
		startSensitivity = "START_SENSITIVITY_" + startSensitivity
	}
	
	endSensitivity := g.config.VADEndSensitivity
	if endSensitivity == "" {
		endSensitivity = "END_SENSITIVITY_HIGH" // Faster end detection
	} else {
		endSensitivity = "END_SENSITIVITY_" + endSensitivity
	}
	
	prefixPaddingMs := int(g.config.VADPrefixPadding.Milliseconds())
	if prefixPaddingMs == 0 {
		prefixPaddingMs = 20 // Gemini default: 20ms
	}
	
	silenceDurationMs := int(g.config.VADSilenceDuration.Milliseconds())
	if silenceDurationMs == 0 {
		silenceDurationMs = 100 // Gemini default: 100ms (much faster than OpenAI!)
	}

	realtimeInputConfig := map[string]any{
		"automatic_activity_detection": map[string]any{
			"disabled":                    false,
			"start_of_speech_sensitivity": startSensitivity,
			"end_of_speech_sensitivity":   endSensitivity,
			"prefix_padding_ms":           prefixPaddingMs,
			"silence_duration_ms":         silenceDurationMs,
		},
	}

	setup := map[string]any{
		"setup": map[string]any{
			"model":                model,
			"generation_config":    generationConfig,
			"realtime_input_config": realtimeInputConfig,
			"system_instruction": map[string]any{
				"parts": []map[string]any{
					{"text": g.config.SystemPrompt},
				},
			},
		},
	}

	// Add tools if any
	if len(toolDeclarations) > 0 {
		setup["setup"].(map[string]any)["tools"] = []map[string]any{
			{
				"function_declarations": toolDeclarations,
			},
		}
	}

	if g.config.Debug {
		fmt.Printf("🌟 Gemini VAD: start=%s, end=%s, prefix=%dms, silence=%dms\n",
			startSensitivity, endSensitivity, prefixPaddingMs, silenceDurationMs)
	}

	return g.sendJSON(setup)
}

// Stop gracefully shuts down the pipeline.
func (g *Gemini) Stop() error {
	g.mu.Lock()
	g.closed = true
	g.connected = false
	g.mu.Unlock()

	if g.cancel != nil {
		g.cancel()
	}

	if g.ws != nil {
		return g.ws.Close()
	}
	return nil
}

// IsConnected returns true if connected and ready.
func (g *Gemini) IsConnected() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.connected && !g.closed
}

// SendAudio sends PCM16 audio to the pipeline.
// Gemini expects 16kHz mono PCM16 audio.
func (g *Gemini) SendAudio(pcm16 []byte) error {
	g.mu.RLock()
	if !g.connected || g.closed {
		g.mu.RUnlock()
		return voice.ErrNotConnected
	}
	g.mu.RUnlock()

	// Stage 2: Send timing - start
	g.metrics.MarkSendStart()

	g.metrics.IncrementAudioIn()
	count := g.metrics.AudioInChunks()

	// Track last audio sent time for latency measurement
	g.latencyMu.Lock()
	g.lastAudioSentTime = time.Now()
	g.latencyMu.Unlock()

	// Log every 50th chunk to show audio is flowing
	if count == 1 || count%50 == 0 {
		debug.Log("🌟 Gemini audio chunk #%d (%d bytes)\n", count, len(pcm16))
	}

	// Encode audio as base64
	encoded := base64.StdEncoding.EncodeToString(pcm16)

	// Gemini Live expects audio/pcm with sample rate specified
	msg := map[string]any{
		"realtime_input": map[string]any{
			"media_chunks": []map[string]any{
				{
					"data":      encoded,
					"mime_type": "audio/pcm;rate=16000",
				},
			},
		},
	}

	err := g.sendJSON(msg)

	// Stage 2: Send timing - end (also marks pipeline start)
	g.metrics.MarkSendEnd()

	return err
}

// OnAudioOut sets the callback for audio output.
func (g *Gemini) OnAudioOut(fn func(pcm16 []byte)) {
	g.onAudioOut = fn
}

// OnSpeechStart sets the callback for speech start.
func (g *Gemini) OnSpeechStart(fn func()) {
	g.onSpeechStart = fn
}

// OnSpeechEnd sets the callback for speech end.
func (g *Gemini) OnSpeechEnd(fn func()) {
	g.onSpeechEnd = fn
}

// OnTranscript sets the callback for transcripts.
func (g *Gemini) OnTranscript(fn func(text string, isFinal bool)) {
	g.onTranscript = fn
}

// OnResponse sets the callback for AI responses.
func (g *Gemini) OnResponse(fn func(text string, isFinal bool)) {
	g.onResponse = fn
}

// OnError sets the error callback.
func (g *Gemini) OnError(fn func(err error)) {
	g.onError = fn
}

// RegisterTool adds a tool the AI can invoke.
func (g *Gemini) RegisterTool(tool voice.Tool) {
	g.tools = append(g.tools, tool)
	g.toolsMap[tool.Name] = tool
}

// OnToolCall sets the callback for tool invocations.
func (g *Gemini) OnToolCall(fn func(call voice.ToolCall)) {
	g.onToolCall = fn
}

// SubmitToolResult returns a tool result to the AI.
func (g *Gemini) SubmitToolResult(callID string, result string) error {
	msg := map[string]any{
		"tool_response": map[string]any{
			"function_responses": []map[string]any{
				{
					"id":       callID,
					"response": map[string]any{"result": result},
				},
			},
		},
	}

	return g.sendJSON(msg)
}

// Interrupt stops the current AI response.
func (g *Gemini) Interrupt() error {
	// Gemini Live handles interruption automatically via VAD
	// Sending audio during AI response will interrupt it
	return nil
}

// SignalTurnComplete signals to Gemini that the user has finished speaking.
// Use this to manually trigger a response when VAD doesn't detect end of speech.
func (g *Gemini) SignalTurnComplete() error {
	msg := map[string]any{
		"client_content": map[string]any{
			"turns": []map[string]any{
				{
					"role":  "user",
					"parts": []map[string]any{},
				},
			},
			"turn_complete": true,
		},
	}
	return g.sendJSON(msg)
}

// Metrics returns current latency metrics.
func (g *Gemini) Metrics() voice.Metrics {
	return g.metrics.Current()
}

// Config returns the current configuration.
func (g *Gemini) Config() voice.Config {
	return g.config
}

// UpdateConfig applies new configuration.
// Note: Gemini Live doesn't support runtime config updates.
func (g *Gemini) UpdateConfig(cfg voice.Config) error {
	g.config = cfg
	return nil
}

// handleMessages processes incoming WebSocket messages.
func (g *Gemini) handleMessages() {
	for {
		g.mu.RLock()
		closed := g.closed
		g.mu.RUnlock()

		if closed {
			return
		}

		_, message, err := g.ws.ReadMessage()
		if err != nil {
			g.mu.RLock()
			closed := g.closed
			g.mu.RUnlock()

			if !closed && g.onError != nil {
				g.onError(err)
			}
			return
		}

		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err != nil {
			if g.config.Debug {
				debug.Log("🌟 Gemini: failed to parse message: %v\n", err)
			}
			continue
		}

		g.handleMessage(msg)
	}
}

// handleMessage processes a single Gemini Live message.
func (g *Gemini) handleMessage(msg map[string]any) {
	// Log all incoming messages for debugging
	debug.Log("🌟 Gemini RAW: %v\n", msg)

	// Handle setup complete
	if _, ok := msg["setupComplete"]; ok {
		debug.Logln("🌟 Gemini Live session ready")
		return
	}

	// Handle server content (audio/text responses)
	if serverContent, ok := msg["serverContent"].(map[string]any); ok {
		g.handleServerContent(serverContent)
		return
	}

	// Handle tool calls
	if toolCall, ok := msg["toolCall"].(map[string]any); ok {
		g.handleToolCall(toolCall)
		return
	}

	// Handle tool call cancellation
	if _, ok := msg["toolCallCancellation"]; ok {
		if g.config.Debug {
			debug.Logln("🌟 Gemini: tool call cancelled")
		}
		return
	}

	// Handle interruption (user started speaking during AI response)
	if _, ok := msg["interrupted"]; ok {
		g.metrics.MarkCaptureStart() // User started speaking (interruption)
		if g.onSpeechStart != nil {
			g.onSpeechStart()
		}
		return
	}

	// Always log unknown messages for debugging
	debug.Log("🌟 Gemini unknown message: %v\n", msg)
}

// handleServerContent processes audio/text from Gemini.
func (g *Gemini) handleServerContent(content map[string]any) {
	// Stage 4: Receive timing - start (first data from WebSocket)
	g.metrics.MarkReceiveStart()

	// Check if this is a turn complete message
	if turnComplete, ok := content["turnComplete"].(bool); ok && turnComplete {
		// Stage 4: Receive timing - end
		g.metrics.MarkReceiveEnd()
		g.metrics.MarkResponseDone()
		if g.config.ProfileLatency {
			m := g.metrics.Current()
			fmt.Printf("⏱️  %s\n", m.FormatLatency())
		}
		// Reset latency tracking for next turn
		g.latencyMu.Lock()
		g.firstAudioReceived = false
		g.latencyMu.Unlock()
		// Reset metrics for next turn
		g.metrics.Reset()
		return
	}

	// Check if interrupted
	if interrupted, ok := content["interrupted"].(bool); ok && interrupted {
		if g.onSpeechStart != nil {
			g.onSpeechStart()
		}
		return
	}

	// Handle model turn (AI response)
	if modelTurn, ok := content["modelTurn"].(map[string]any); ok {
		if parts, ok := modelTurn["parts"].([]any); ok {
			for _, part := range parts {
				partMap, ok := part.(map[string]any)
				if !ok {
					continue
				}

				// Handle inline audio data
				if inlineData, ok := partMap["inlineData"].(map[string]any); ok {
					if mimeType, ok := inlineData["mimeType"].(string); ok {
						if mimeType == "audio/pcm" || mimeType == "audio/pcm;rate=24000" {
							if data, ok := inlineData["data"].(string); ok {
								audioData, err := base64.StdEncoding.DecodeString(data)
								if err == nil && len(audioData) > 0 {
									// Measure latency from last audio sent to first audio received
									g.latencyMu.Lock()
									if !g.firstAudioReceived && !g.lastAudioSentTime.IsZero() {
										latency := time.Since(g.lastAudioSentTime)
										g.firstAudioReceived = true
										g.latencyMu.Unlock()

										if g.config.ProfileLatency {
											fmt.Printf("⏱️  GEMINI LATENCY: %dms (last audio → first response)\n", latency.Milliseconds())
										}

										// Stage 3: Pipeline timing - mark first audio received
										g.metrics.MarkPipelineEnd()

										// Trigger speech end callback
										if g.onSpeechEnd != nil {
											g.onSpeechEnd()
										}
									} else {
										g.latencyMu.Unlock()
									}

									g.metrics.IncrementAudioOut()
									if g.onAudioOut != nil {
										g.onAudioOut(audioData)
									}
								}
							}
						}
					}
				}

				// Handle text response
				if text, ok := partMap["text"].(string); ok {
					g.metrics.MarkFirstToken()
					if g.onResponse != nil {
						g.onResponse(text, false)
					}
				}

				// Handle executable code (Gemini's code execution mode for tools)
				if execCode, ok := partMap["executableCode"].(map[string]any); ok {
					g.handleExecutableCode(execCode)
				}
			}
		}
	}

	// Handle input transcription (what user said)
	if inputTranscript, ok := content["inputTranscription"].(map[string]any); ok {
		if text, ok := inputTranscript["text"].(string); ok {
			// Transcript received - VAD has detected end of speech
			g.metrics.MarkVADSpeechEnded() // Key timestamp for pipeline latency
			g.metrics.MarkTranscript()
			if g.onSpeechEnd != nil {
				g.onSpeechEnd()
			}
			if g.onTranscript != nil {
				g.onTranscript(text, true)
			}
		}
	}

	// Handle output transcription (what AI said as text)
	if outputTranscript, ok := content["outputTranscription"].(map[string]any); ok {
		if text, ok := outputTranscript["text"].(string); ok {
			if g.onResponse != nil {
				g.onResponse(text, true)
			}
		}
	}
}

// handleExecutableCode parses Python-style code from Gemini and executes the corresponding tool.
// Format: "default_api.function_name(param='value')" or "function_name(param='value')"
func (g *Gemini) handleExecutableCode(execCode map[string]any) {
	code, ok := execCode["code"].(string)
	if !ok || code == "" {
		return
	}

	if g.config.Debug {
		fmt.Printf("🔧 Gemini executableCode: %s\n", code)
	}

	// Parse the Python function call
	// Examples:
	// - default_api.play_emotion(emotion='curious1')
	// - play_emotion(emotion='happy')
	name, args := parsePythonCall(code)
	if name == "" {
		if g.config.Debug {
			debug.Log("⚠️  Failed to parse executableCode: %s\n", code)
		}
		return
	}

	// Generate a unique ID for this call
	callID := fmt.Sprintf("exec_%d", time.Now().UnixNano())

	if g.onToolCall != nil {
		g.onToolCall(voice.ToolCall{
			ID:        callID,
			Name:      name,
			Arguments: args,
		})
	} else {
		// Execute internally if no external handler
		if handler, ok := g.toolsMap[name]; ok && handler.Handler != nil {
			result, err := handler.Handler(args)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}
			// For code execution, we need to send the result back differently
			if submitErr := g.sendCodeExecutionResult(callID, result); submitErr != nil {
				if g.onError != nil {
					g.onError(submitErr)
				}
			}
		} else {
			if g.config.Debug {
				debug.Log("⚠️  Tool not found: %s\n", name)
			}
		}
	}
}

// parsePythonCall extracts function name and arguments from a Python-style call.
// Input: "default_api.play_emotion(emotion='curious1')"
// Output: "play_emotion", {"emotion": "curious1"}
func parsePythonCall(code string) (string, map[string]any) {
	code = strings.TrimSpace(code)

	// Find the opening parenthesis
	parenIdx := strings.Index(code, "(")
	if parenIdx == -1 {
		return "", nil
	}

	// Extract function name (may have "default_api." prefix)
	funcPart := code[:parenIdx]
	if dotIdx := strings.LastIndex(funcPart, "."); dotIdx != -1 {
		funcPart = funcPart[dotIdx+1:]
	}

	// Extract arguments part (between parentheses)
	closeIdx := strings.LastIndex(code, ")")
	if closeIdx == -1 || closeIdx <= parenIdx {
		return funcPart, make(map[string]any)
	}

	argsStr := code[parenIdx+1 : closeIdx]
	args := make(map[string]any)

	if argsStr == "" {
		return funcPart, args
	}

	// Parse keyword arguments: key='value', key="value", key=123
	// Simple parser - doesn't handle nested structures
	for _, arg := range strings.Split(argsStr, ",") {
		arg = strings.TrimSpace(arg)
		eqIdx := strings.Index(arg, "=")
		if eqIdx == -1 {
			continue
		}

		key := strings.TrimSpace(arg[:eqIdx])
		val := strings.TrimSpace(arg[eqIdx+1:])

		// Remove quotes from string values
		if (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) ||
			(strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) {
			val = val[1 : len(val)-1]
		}

		args[key] = val
	}

	return funcPart, args
}

// sendCodeExecutionResult sends the result of code execution back to Gemini.
func (g *Gemini) sendCodeExecutionResult(id string, result string) error {
	msg := map[string]any{
		"tool_response": map[string]any{
			"function_responses": []map[string]any{
				{
					"id":       id,
					"response": map[string]any{"result": result},
				},
			},
		},
	}
	return g.sendJSON(msg)
}

// handleToolCall processes a function call from Gemini.
func (g *Gemini) handleToolCall(toolCall map[string]any) {
	functionCalls, ok := toolCall["functionCalls"].([]any)
	if !ok {
		return
	}

	for _, fc := range functionCalls {
		fcMap, ok := fc.(map[string]any)
		if !ok {
			continue
		}

		name, _ := fcMap["name"].(string)
		id, _ := fcMap["id"].(string)
		args, _ := fcMap["args"].(map[string]any)

		if g.config.Debug {
			fmt.Printf("🔧 Gemini tool call: %s\n", name)
		}

		if g.onToolCall != nil {
			g.onToolCall(voice.ToolCall{
				ID:        id,
				Name:      name,
				Arguments: args,
			})
		} else {
			// Execute internally if no external handler
			if handler, ok := g.toolsMap[name]; ok && handler.Handler != nil {
				result, err := handler.Handler(args)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				if submitErr := g.SubmitToolResult(id, result); submitErr != nil {
					if g.onError != nil {
						g.onError(submitErr)
					}
				}
			} else {
				if submitErr := g.SubmitToolResult(id, "Function not found"); submitErr != nil {
					if g.onError != nil {
						g.onError(submitErr)
					}
				}
			}
		}
	}
}

// sendJSON sends a JSON message over WebSocket.
func (g *Gemini) sendJSON(v any) error {
	g.wsMu.Lock()
	defer g.wsMu.Unlock()

	if g.ws == nil {
		return voice.ErrNotConnected
	}

	return g.ws.WriteJSON(v)
}

// convertToJSONSchema converts Eva's simplified tool parameters to proper JSON Schema.
// Eva tools use: map[string]any{"paramName": {"type": "string", "description": "..."}}
// Gemini expects: {"type": "object", "properties": {...}, "required": [...]}
func convertToJSONSchema(params map[string]any) map[string]any {
	if len(params) == 0 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	// Check if it's already in JSON Schema format (has "type": "object")
	if typeVal, ok := params["type"].(string); ok && typeVal == "object" {
		return params
	}

	// Convert from Eva's format to JSON Schema
	properties := make(map[string]any)
	required := make([]string, 0)

	for paramName, paramDef := range params {
		if paramMap, ok := paramDef.(map[string]any); ok {
			// Copy the parameter definition as-is (it already has type, description, etc.)
			properties[paramName] = paramMap
			// All parameters are considered required by default
			required = append(required, paramName)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// App-side timing markers

// MarkCaptureStart records when WebRTC delivered audio to Eva.
func (g *Gemini) MarkCaptureStart() {
	g.metrics.MarkCaptureStart()
}

// MarkCaptureEnd records when audio is buffered and ready to send.
func (g *Gemini) MarkCaptureEnd() {
	g.metrics.MarkCaptureEnd()
}

// MarkPlaybackStart records when audio was sent to GStreamer.
func (g *Gemini) MarkPlaybackStart() {
	g.metrics.MarkPlaybackStart()
}

// MarkPlaybackEnd records when audio playback completed.
func (g *Gemini) MarkPlaybackEnd() {
	g.metrics.MarkPlaybackEnd()
}

// Ensure Gemini implements voice.Pipeline at compile time.
var _ voice.Pipeline = (*Gemini)(nil)

// Register Gemini provider in voice package.
func init() {
	voice.Register(voice.ProviderGemini, func(cfg voice.Config) (voice.Pipeline, error) {
		return NewGemini(cfg)
	})
}
