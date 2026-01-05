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
	// Gemini Live API WebSocket endpoint
	geminiLiveURL = "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent"
	
	// Default model for Gemini Live
	geminiDefaultModel = "models/gemini-2.0-flash-exp"
)

// Gemini implements voice.Pipeline using Google's Gemini Live API.
// This provides the lowest latency speech-to-speech experience with
// Gemini 2.0 Flash handling VAD, ASR, LLM, and TTS in a single stream.
type Gemini struct {
	config voice.Config
	
	// WebSocket connection
	ws     *websocket.Conn
	wsMu   sync.Mutex
	
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
	// Build tools for Gemini format
	var toolDeclarations []map[string]any
	for _, tool := range g.tools {
		toolDeclarations = append(toolDeclarations, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.Parameters,
		})
	}
	
	// Voice selection (Gemini voices: Puck, Charon, Kore, Fenrir, Aoede)
	voiceName := g.config.TTSVoice
	if voiceName == "" {
		voiceName = "Puck" // Default voice
	}
	
	setup := map[string]any{
		"setup": map[string]any{
			"model": model,
			"generation_config": map[string]any{
				"response_modalities": []string{"AUDIO"},
				"speech_config": map[string]any{
					"voice_config": map[string]any{
						"prebuilt_voice_config": map[string]any{
							"voice_name": voiceName,
						},
					},
				},
			},
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
	
	g.metrics.IncrementAudioIn()
	
	// Encode audio as base64
	encoded := base64.StdEncoding.EncodeToString(pcm16)
	
	msg := map[string]any{
		"realtime_input": map[string]any{
			"media_chunks": []map[string]any{
				{
					"data":      encoded,
					"mime_type": "audio/pcm",
				},
			},
		},
	}
	
	return g.sendJSON(msg)
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
	// Handle setup complete
	if _, ok := msg["setupComplete"]; ok {
		if g.config.Debug {
			debug.Logln("🌟 Gemini Live session ready")
		}
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
		g.metrics.MarkSpeechEnd()
		if g.onSpeechStart != nil {
			g.onSpeechStart()
		}
		return
	}
	
	if g.config.Debug {
		debug.Log("🌟 Gemini message: %v\n", msg)
	}
}

// handleServerContent processes audio/text from Gemini.
func (g *Gemini) handleServerContent(content map[string]any) {
	// Check if this is a turn complete message
	if turnComplete, ok := content["turnComplete"].(bool); ok && turnComplete {
		g.metrics.MarkResponseDone()
		if g.config.ProfileLatency {
			m := g.metrics.Current()
			fmt.Printf("⏱️  %s\n", m.FormatLatency())
		}
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
									g.metrics.MarkFirstAudio()
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
			}
		}
	}
	
	// Handle input transcription (what user said)
	if inputTranscript, ok := content["inputTranscription"].(map[string]any); ok {
		if text, ok := inputTranscript["text"].(string); ok {
			// Mark speech end when we get transcript
			g.metrics.MarkSpeechEnd()
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

// Ensure Gemini implements voice.Pipeline at compile time.
var _ voice.Pipeline = (*Gemini)(nil)

// Register Gemini provider in voice package.
func init() {
	voice.Register(voice.ProviderGemini, func(cfg voice.Config) (voice.Pipeline, error) {
		return NewGemini(cfg)
	})
}

