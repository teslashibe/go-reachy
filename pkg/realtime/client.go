// Package realtime provides a client for OpenAI's Realtime API
// for low-latency speech-to-speech conversations with tool use
package realtime

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	RealtimeURL = "wss://api.openai.com/v1/realtime"
	Model       = "gpt-4o-realtime-preview-2024-12-17"
)

// Tool represents a function that Eva can use
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Handler     func(args map[string]interface{}) (string, error)
}

// Client manages the WebSocket connection to OpenAI Realtime API
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

	// Callbacks
	OnTranscript      func(text string, isFinal bool)
	OnAudioDelta      func(audioBase64 string)
	OnAudioDone       func()
	OnFunctionCall    func(name string, args map[string]interface{}) string
	OnError           func(err error)
	OnSessionCreated  func()
	OnSpeechStarted   func() // User started speaking
	OnSpeechStopped   func() // User stopped speaking

	// Internal state
	closed bool
}

// NewClient creates a new Realtime API client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:   apiKey,
		tools:    []Tool{},
		toolsMap: make(map[string]Tool),
	}
}

// RegisterTool adds a tool that Eva can use during conversation
func (c *Client) RegisterTool(tool Tool) {
	c.tools = append(c.tools, tool)
	c.toolsMap[tool.Name] = tool
}

// Connect establishes WebSocket connection to OpenAI Realtime API
func (c *Client) Connect() error {
	url := fmt.Sprintf("%s?model=%s", RealtimeURL, Model)

	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + c.apiKey}
	header["OpenAI-Beta"] = []string{"realtime=v1"}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	var err error
	c.ws, _, err = dialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("failed to connect to Realtime API: %w", err)
	}

	// Set up ping/pong handlers to keep connection alive
	c.ws.SetPingHandler(func(appData string) error {
		// Respond to ping with pong
		c.wsMu.Lock()
		defer c.wsMu.Unlock()
		return c.ws.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	// Set read deadline - reset on each message
	c.ws.SetReadDeadline(time.Now().Add(120 * time.Second))

	c.connected = true

	// Start message handler
	go c.handleMessages()

	// Start keepalive pinger
	go c.keepAlive()

	return nil
}

// keepAlive sends periodic pings to keep connection alive
func (c *Client) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for !c.closed {
		select {
		case <-ticker.C:
			c.wsMu.Lock()
			if c.ws != nil && !c.closed {
				c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					c.wsMu.Unlock()
					return
				}
			}
			c.wsMu.Unlock()
		}
	}
}

// ConfigureSession sets up the session with voice, instructions, and tools
func (c *Client) ConfigureSession(instructions string, voice string) error {
	if voice == "" {
		voice = "alloy"
	}

	// Build tools array for API
	apiTools := make([]map[string]interface{}, len(c.tools))
	for i, tool := range c.tools {
		apiTools[i] = map[string]interface{}{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": tool.Parameters,
				"required":   []string{}, // Make all optional for flexibility
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

// SendAudio sends PCM16 audio data to the API
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

// CommitAudio commits the audio buffer (triggers processing)
func (c *Client) CommitAudio() error {
	return c.sendJSON(map[string]string{
		"type": "input_audio_buffer.commit",
	})
}

// ClearAudio clears the audio input buffer
func (c *Client) ClearAudio() error {
	return c.sendJSON(map[string]string{
		"type": "input_audio_buffer.clear",
	})
}

// SendText sends a text message (for testing or hybrid input)
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

	// Request a response
	return c.sendJSON(map[string]string{
		"type": "response.create",
	})
}

// CancelResponse interrupts the current response
func (c *Client) CancelResponse() error {
	return c.sendJSON(map[string]string{
		"type": "response.cancel",
	})
}

// Close closes the WebSocket connection
func (c *Client) Close() {
	c.closed = true
	if c.ws != nil {
		c.ws.Close()
	}
}

// handleMessages processes incoming WebSocket messages
func (c *Client) handleMessages() {
	for !c.closed {
		// Reset read deadline on each message
		c.ws.SetReadDeadline(time.Now().Add(120 * time.Second))

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
			// Session configuration confirmed

		case "input_audio_buffer.speech_started":
			// User started speaking - trigger callback for interruption
			if c.OnSpeechStarted != nil {
				c.OnSpeechStarted()
			}

		case "input_audio_buffer.speech_stopped":
			// User stopped speaking
			if c.OnSpeechStopped != nil {
				c.OnSpeechStopped()
			}

		case "input_audio_buffer.committed":
			// Audio buffer was committed

		case "conversation.item.input_audio_transcription.completed":
			// Got transcription of user's speech
			if transcript, ok := msg["transcript"].(string); ok && c.OnTranscript != nil {
				c.OnTranscript(transcript, true)
			}

		case "response.audio.delta":
			// Streaming audio chunk from assistant
			if delta, ok := msg["delta"].(string); ok && c.OnAudioDelta != nil {
				c.OnAudioDelta(delta)
			}

		case "response.audio.done":
			// Audio response complete
			if c.OnAudioDone != nil {
				c.OnAudioDone()
			}

		case "response.audio_transcript.delta":
			// Streaming transcript of assistant's speech
			if delta, ok := msg["delta"].(string); ok && c.OnTranscript != nil {
				c.OnTranscript(delta, false)
			}

		case "response.function_call_arguments.done":
			// Function call completed - execute the tool
			c.handleFunctionCall(msg)

		case "response.done":
			// Full response complete

		case "error":
			if errData, ok := msg["error"].(map[string]interface{}); ok {
				if errMsg, ok := errData["message"].(string); ok && c.OnError != nil {
					c.OnError(fmt.Errorf("API error: %s", errMsg))
				}
			}
		}
	}
}

// handleFunctionCall executes a tool and sends the result back
func (c *Client) handleFunctionCall(msg map[string]interface{}) {
	name, _ := msg["name"].(string)
	callID, _ := msg["call_id"].(string)
	argsStr, _ := msg["arguments"].(string)

	fmt.Printf("🔧 Tool called: %s (args: %s)\n", name, argsStr)

	var args map[string]interface{}
	json.Unmarshal([]byte(argsStr), &args)

	// Find and execute the tool
	var result string
	if tool, ok := c.toolsMap[name]; ok && tool.Handler != nil {
		var err error
		result, err = tool.Handler(args)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
		}
		fmt.Printf("🔧 Tool result: %s\n", result)
	} else if c.OnFunctionCall != nil {
		result = c.OnFunctionCall(name, args)
	} else {
		result = "Function not found"
		fmt.Printf("⚠️  Tool not found: %s\n", name)
	}

	// Send result back to continue conversation
	responseMsg := map[string]interface{}{
		"type": "conversation.item.create",
		"item": map[string]interface{}{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  result,
		},
	}

	c.sendJSON(responseMsg)

	// Request continuation of response
	c.sendJSON(map[string]string{
		"type": "response.create",
	})
}

// sendJSON sends a JSON message over WebSocket
func (c *Client) sendJSON(v interface{}) error {
	c.wsMu.Lock()
	defer c.wsMu.Unlock()

	if c.ws == nil {
		return fmt.Errorf("not connected")
	}

	return c.ws.WriteJSON(v)
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	return c.connected && !c.closed
}

// IsReady returns whether the session is ready for conversation
func (c *Client) IsReady() bool {
	return c.sessionReady
}
