// Package openai provides a client for OpenAI's Realtime API
// for low-latency speech-to-speech conversations with tool use.
package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teslashibe/go-reachy/pkg/debug"
)

const (
	RealtimeURL = "wss://api.openai.com/v1/realtime"
	Model       = "gpt-4o-realtime-preview-2024-12-17"
)

// Tool represents a function that Eva can use.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Handler     func(args map[string]interface{}) (string, error)
}

// pendingToolCall represents a tool call waiting to be executed
type pendingToolCall struct {
	Name   string
	CallID string
	Args   map[string]interface{}
}

// Client manages the WebSocket connection to OpenAI Realtime API.
type Client struct {
	apiKey string
	ws     *websocket.Conn
	wsMu   sync.Mutex

	// Tools Eva can use
	tools    []Tool
	toolsMap map[string]Tool

	// Session state
	sessionID    string
	connected    bool
	sessionReady bool

	// LATENCY OPTIMIZATION #112: Parallel tool execution
	pendingTools   []pendingToolCall
	pendingToolsMu sync.Mutex
	toolBatchTimer *time.Timer

	// Callbacks
	OnTranscript       func(text string, isFinal bool)
	OnTranscriptDone   func() // Called when response.audio_transcript.done is received
	OnAudioDelta       func(audioBase64 string)
	OnAudioDone        func()
	OnFunctionCall     func(name string, args map[string]interface{}) string
	OnError            func(err error)
	OnSessionCreated   func()
	OnSpeechStarted    func() // User started speaking
	OnSpeechStopped    func() // User stopped speaking

	// Internal state
	closed bool
}

// NewClient creates a new Realtime API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:   apiKey,
		tools:    []Tool{},
		toolsMap: make(map[string]Tool),
	}
}

// RegisterTool adds a tool that Eva can use during conversation.
func (c *Client) RegisterTool(tool Tool) {
	c.tools = append(c.tools, tool)
	c.toolsMap[tool.Name] = tool
}

// Connect establishes WebSocket connection to OpenAI Realtime API.
func (c *Client) Connect() error {
	url := fmt.Sprintf("%s?model=%s", RealtimeURL, Model)

	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + c.apiKey}
	header["OpenAI-Beta"] = []string{"realtime=v1"}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	var err error
	var resp *http.Response
	c.ws, resp, err = dialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("failed to connect to Realtime API: %w", err)
	}

	if resp != nil {
		debug.Logln("🎤 OpenAI Response Headers:")
		for key, values := range resp.Header {
			debug.Log("🎤   %s: %v\n", key, values)
		}
	}

	c.connected = true

	go c.handleMessages()

	return nil
}

// ConfigureSession sets up the session with voice, instructions, and tools.
func (c *Client) ConfigureSession(instructions string, voice string) error {
	if voice == "" {
		voice = "alloy"
	}

	apiTools := make([]map[string]interface{}, len(c.tools))
	for i, tool := range c.tools {
		apiTools[i] = map[string]interface{}{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": tool.Parameters,
				"required":   []string{},
			},
		}
	}

	msg := map[string]interface{}{
		"type": "session.update",
		"session": map[string]interface{}{
			"modalities":          []string{"text", "audio"},
			"instructions":        instructions,
			"voice":               voice,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"input_audio_transcription": map[string]interface{}{
				"model": "whisper-1",
			},
			"turn_detection": map[string]interface{}{
				"type":                "server_vad",
				"threshold":           0.5,
				"prefix_padding_ms":   300,
				"silence_duration_ms": 500,
			},
			"tools":       apiTools,
			"tool_choice": "auto",
		},
	}

	return c.sendJSON(msg)
}

// SendAudio sends PCM16 audio data to the API.
func (c *Client) SendAudio(pcm16Data []byte) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	encoded := base64.StdEncoding.EncodeToString(pcm16Data)

	msg := map[string]interface{}{
		"type":  "input_audio_buffer.append",
		"audio": encoded,
	}

	return c.sendJSON(msg)
}

// CommitAudio commits the audio buffer (triggers processing).
func (c *Client) CommitAudio() error {
	return c.sendJSON(map[string]string{
		"type": "input_audio_buffer.commit",
	})
}

// ClearAudio clears the audio input buffer.
func (c *Client) ClearAudio() error {
	return c.sendJSON(map[string]string{
		"type": "input_audio_buffer.clear",
	})
}

// SendText sends a text message (for testing or hybrid input).
func (c *Client) SendText(text string) error {
	msg := map[string]interface{}{
		"type": "conversation.item.create",
		"item": map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []map[string]interface{}{
				{
					"type": "input_text",
					"text": text,
				},
			},
		},
	}

	if err := c.sendJSON(msg); err != nil {
		return err
	}

	return c.sendJSON(map[string]string{
		"type": "response.create",
	})
}

// CancelResponse interrupts the current response.
func (c *Client) CancelResponse() error {
	return c.sendJSON(map[string]string{
		"type": "response.cancel",
	})
}

// Close closes the WebSocket connection.
func (c *Client) Close() {
	c.closed = true

	// LATENCY OPTIMIZATION #112: Clean up pending tool batch timer
	c.pendingToolsMu.Lock()
	if c.toolBatchTimer != nil {
		c.toolBatchTimer.Stop()
		c.toolBatchTimer = nil
	}
	c.pendingTools = nil
	c.pendingToolsMu.Unlock()

	if c.ws != nil {
		c.ws.Close()
	}
}

// handleMessages processes incoming WebSocket messages.
func (c *Client) handleMessages() {
	for !c.closed {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if !c.closed && c.OnError != nil {
				c.OnError(err)
			}
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "session.created":
			c.sessionReady = true
			if c.OnSessionCreated != nil {
				c.OnSessionCreated()
			}

		case "session.updated":
			debug.Logln("🎤 Session updated/configured")

		case "input_audio_buffer.speech_started":
			debug.Logln("🎤 VAD: Speech started!")
			if c.OnSpeechStarted != nil {
				c.OnSpeechStarted()
			}

		case "input_audio_buffer.speech_stopped":
			debug.Logln("🎤 VAD: Speech stopped")
			if c.OnSpeechStopped != nil {
				c.OnSpeechStopped()
			}

		case "input_audio_buffer.committed":
			debug.Logln("🎤 Audio buffer committed")

		case "conversation.item.input_audio_transcription.completed":
			if transcript, ok := msg["transcript"].(string); ok && c.OnTranscript != nil {
				c.OnTranscript(transcript, true)
			}

		case "conversation.item.input_audio_transcription.failed":
			if errData, ok := msg["error"].(map[string]interface{}); ok {
				errMsg, _ := errData["message"].(string)
				errCode, _ := errData["code"].(string)
				errType, _ := errData["type"].(string)
				fmt.Printf("⚠️  Transcription failed: %s (code: %s, type: %s)\n", errMsg, errCode, errType)
			} else {
				fmt.Printf("⚠️  Transcription failed: %v\n", msg)
			}

		case "response.audio.delta":
			if delta, ok := msg["delta"].(string); ok && c.OnAudioDelta != nil {
				c.OnAudioDelta(delta)
			}

		case "response.audio.done":
			if c.OnAudioDone != nil {
				c.OnAudioDone()
			}

		case "response.audio_transcript.delta":
			if delta, ok := msg["delta"].(string); ok && c.OnTranscript != nil {
				c.OnTranscript(delta, false)
			}

		case "response.audio_transcript.done":
			if c.OnTranscriptDone != nil {
				c.OnTranscriptDone()
			}

		case "response.function_call_arguments.done":
			c.handleFunctionCall(msg)

		case "response.done":
			// Full response complete

		case "error":
			if errData, ok := msg["error"].(map[string]interface{}); ok {
				if errMsg, ok := errData["message"].(string); ok {
					fmt.Printf("⚠️  OpenAI error: %s\n", errMsg)
					if c.OnError != nil {
						c.OnError(fmt.Errorf("API error: %s", errMsg))
					}
				}
			} else {
				fmt.Printf("⚠️  OpenAI error: %v\n", msg)
			}

		default:
			if msgType != "" && msgType != "response.audio.delta" && msgType != "response.audio_transcript.delta" {
				debug.Log("🎤 Message: %s\n", msgType)
			}
		}
	}
}

// LATENCY OPTIMIZATION #112: Batch tool execution window
const toolBatchWindow = 50 * time.Millisecond

// handleFunctionCall queues a tool call for parallel execution.
// Tools are batched within a short window and executed concurrently.
func (c *Client) handleFunctionCall(msg map[string]interface{}) {
	name, _ := msg["name"].(string)
	callID, _ := msg["call_id"].(string)
	argsStr, _ := msg["arguments"].(string)

	fmt.Printf("🔧 Tool queued: %s (args: %s)\n", name, argsStr)

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		fmt.Printf("⚠️  Failed to parse tool args for %s: %v\n", name, err)
		args = make(map[string]interface{}) // Use empty args rather than nil
	}

	c.pendingToolsMu.Lock()
	c.pendingTools = append(c.pendingTools, pendingToolCall{
		Name:   name,
		CallID: callID,
		Args:   args,
	})

	// Start or reset batch timer
	if c.toolBatchTimer != nil {
		c.toolBatchTimer.Stop()
	}
	c.toolBatchTimer = time.AfterFunc(toolBatchWindow, c.executeToolBatch)
	c.pendingToolsMu.Unlock()
}

// executeToolBatch executes all pending tools in parallel.
func (c *Client) executeToolBatch() {
	c.pendingToolsMu.Lock()
	tools := c.pendingTools
	c.pendingTools = nil
	c.pendingToolsMu.Unlock()

	if len(tools) == 0 {
		return
	}

	startTime := time.Now()
	fmt.Printf("🔧 Executing %d tools in parallel...\n", len(tools))

	// Execute all tools concurrently
	var wg sync.WaitGroup
	results := make([]struct {
		CallID string
		Result string
	}, len(tools))

	for i, tool := range tools {
		wg.Add(1)
		go func(idx int, t pendingToolCall) {
			defer wg.Done()

			// LATENCY OPTIMIZATION #112: Recover from panics in tool handlers
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("⚠️  Tool %s panicked: %v\n", t.Name, r)
					results[idx] = struct {
						CallID string
						Result string
					}{t.CallID, fmt.Sprintf("Error: tool panicked: %v", r)}
				}
			}()

			var result string
			if handler, ok := c.toolsMap[t.Name]; ok && handler.Handler != nil {
				var err error
				result, err = handler.Handler(t.Args)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
			} else if c.OnFunctionCall != nil {
				result = c.OnFunctionCall(t.Name, t.Args)
			} else {
				result = "Function not found"
			}

			results[idx] = struct {
				CallID string
				Result string
			}{t.CallID, result}
			fmt.Printf("🔧 Tool %s completed: %s\n", t.Name, result)
		}(i, tool)
	}

	wg.Wait()
	elapsed := time.Since(startTime)
	fmt.Printf("🔧 All %d tools completed in %dms (parallel execution)\n", len(tools), elapsed.Milliseconds())

	// Check connection before sending results
	if c.closed || !c.IsConnected() {
		fmt.Println("⚠️  Connection closed, discarding tool results")
		return
	}

	// Send all results back
	for _, r := range results {
		responseMsg := map[string]interface{}{
			"type": "conversation.item.create",
			"item": map[string]interface{}{
				"type":    "function_call_output",
				"call_id": r.CallID,
				"output":  r.Result,
			},
		}
		if err := c.sendJSON(responseMsg); err != nil {
			fmt.Printf("⚠️  Failed to send tool result: %v\n", err)
			return
		}
	}

	// Request response after all tool results are sent
	if err := c.sendJSON(map[string]string{
		"type": "response.create",
	}); err != nil {
		fmt.Printf("⚠️  Failed to request response: %v\n", err)
	}
}

// sendJSON sends a JSON message over WebSocket.
func (c *Client) sendJSON(v interface{}) error {
	c.wsMu.Lock()
	defer c.wsMu.Unlock()

	if c.ws == nil {
		return fmt.Errorf("not connected")
	}

	return c.ws.WriteJSON(v)
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	return c.connected && !c.closed
}

// IsReady returns whether the session is ready for conversation.
func (c *Client) IsReady() bool {
	return c.sessionReady
}




