// Package main provides a standalone test for ElevenLabs WebSocket communication.
// Run with: go run ./cmd/test-elevenlabs/websocket_test.go
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

// Message structures matching ElevenLabs format
type audioEvent struct {
	EventID     int    `json:"event_id"`
	AudioBase64 string `json:"audio_base_64"`
}

type pingEvent struct {
	EventID int `json:"event_id"`
	PingMS  int `json:"ping_ms"`
}

type userTranscriptionEvent struct {
	UserTranscript string `json:"user_transcript"`
}

type agentResponseEvent struct {
	AgentResponse string `json:"agent_response"`
}

type incomingMessage struct {
	Type    string `json:"type"`
	Audio   string `json:"audio,omitempty"`
	Text    string `json:"text,omitempty"`
	IsFinal bool   `json:"is_final,omitempty"`
	Message string `json:"message,omitempty"`

	// Nested event structures
	AudioEvent             *audioEvent             `json:"audio_event,omitempty"`
	PingEvent              *pingEvent              `json:"ping_event,omitempty"`
	UserTranscriptionEvent *userTranscriptionEvent `json:"user_transcription_event,omitempty"`
	AgentResponseEvent     *agentResponseEvent     `json:"agent_response_event,omitempty"`
}

func main() {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		log.Fatal("ELEVENLABS_API_KEY required")
	}

	// Use an existing agent or create one
	agentID := os.Getenv("ELEVENLABS_AGENT_ID")
	if agentID == "" {
		log.Println("⚠️  No ELEVENLABS_AGENT_ID set, will create a temporary agent...")
		var err error
		agentID, err = createTestAgent(apiKey)
		if err != nil {
			log.Fatalf("Failed to create agent: %v", err)
		}
		log.Printf("✅ Created agent: %s\n", agentID)
		defer deleteAgent(apiKey, agentID)
	}

	log.Printf("🔌 Connecting to ElevenLabs with agent: %s\n", agentID)

	// Build WebSocket URL
	wsURL, _ := url.Parse("wss://api.elevenlabs.io/v1/convai/conversation")
	q := wsURL.Query()
	q.Set("agent_id", agentID)
	wsURL.RawQuery = q.Encode()

	// Connect with auth header
	header := http.Header{}
	header.Set("xi-api-key", apiKey)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err != nil {
		if resp != nil {
			log.Fatalf("WebSocket dial failed: %v (status: %d)", err, resp.StatusCode)
		}
		log.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	log.Println("✅ Connected to ElevenLabs WebSocket")

	// Handle Ctrl+C
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		log.Println("\n👋 Shutting down...")
		cancel()
	}()

	// Read messages in goroutine
	msgCh := make(chan []byte, 100)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("❌ Read error: %v", err)
				return
			}
			msgCh <- data
		}
	}()

	// Send audio in goroutine (16kHz mono PCM silence)
	go func() {
		// Wait for init
		time.Sleep(500 * time.Millisecond)

		log.Println("🎙️ Starting to send audio (silence)...")

		// Send 50ms chunks of silence (16kHz = 800 samples per 50ms, 2 bytes per sample = 1600 bytes)
		silenceChunk := make([]byte, 1600)
		encoded := base64.StdEncoding.EncodeToString(silenceChunk)

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		count := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				msg := map[string]string{
					"user_audio_chunk": encoded,
				}
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("❌ Write error: %v", err)
					return
				}
				count++
				if count%100 == 0 {
					log.Printf("🎙️ Sent %d audio chunks", count)
				}
			}
		}
	}()

	// Process messages
	audioChunks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-msgCh:
			// Log raw message
			preview := string(data)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			log.Printf("📨 RAW [%d bytes]: %s", len(data), preview)

			// Parse message
			var msg incomingMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("❌ Parse error: %v", err)
				continue
			}

			// Log parsed fields
			log.Printf("🔍 PARSED: type=%q has_audio_event=%v has_ping=%v has_user_transcript=%v has_agent_response=%v",
				msg.Type,
				msg.AudioEvent != nil,
				msg.PingEvent != nil,
				msg.UserTranscriptionEvent != nil,
				msg.AgentResponseEvent != nil,
			)

			// Handle by type
			switch msg.Type {
			case "conversation_initiation_metadata":
				log.Println("📞 Conversation initiated!")

			case "ping":
				if msg.PingEvent != nil {
					log.Printf("🏓 Ping event_id=%d ping_ms=%d", msg.PingEvent.EventID, msg.PingEvent.PingMS)
					// Send pong
					pong := map[string]interface{}{
						"type":     "pong",
						"event_id": msg.PingEvent.EventID,
					}
					pongData, _ := json.Marshal(pong)
					conn.WriteMessage(websocket.TextMessage, pongData)
				}

			case "audio":
				audioChunks++
				if msg.AudioEvent != nil {
					decoded, _ := base64.StdEncoding.DecodeString(msg.AudioEvent.AudioBase64)
					log.Printf("🔊 AUDIO chunk #%d: event_id=%d decoded_bytes=%d",
						audioChunks, msg.AudioEvent.EventID, len(decoded))
				} else if msg.Audio != "" {
					decoded, _ := base64.StdEncoding.DecodeString(msg.Audio)
					log.Printf("🔊 AUDIO chunk #%d (flat format): decoded_bytes=%d",
						audioChunks, len(decoded))
				} else {
					log.Printf("⚠️  AUDIO message but no audio data found!")
				}

			case "user_transcript":
				text := msg.Text
				if msg.UserTranscriptionEvent != nil {
					text = msg.UserTranscriptionEvent.UserTranscript
				}
				log.Printf("🎤 USER: %q (final=%v)", text, msg.IsFinal)

			case "agent_response":
				text := msg.Text
				if msg.AgentResponseEvent != nil {
					text = msg.AgentResponseEvent.AgentResponse
				}
				log.Printf("🤖 AGENT: %q (final=%v)", text, msg.IsFinal)

			case "interruption":
				log.Println("⚡ User interrupted!")

			case "error":
				log.Printf("❌ ERROR: %s", msg.Message)

			default:
				log.Printf("❓ Unknown type: %q", msg.Type)
			}
		}
	}
}

func createTestAgent(apiKey string) (string, error) {
	voiceID := os.Getenv("ELEVENLABS_VOICE_ID")
	if voiceID == "" {
		voiceID = "EXAVITQu4vr4xnSDxMaL" // Sarah voice
	}

	payload := map[string]interface{}{
		"name": "websocket-test-agent",
		"conversation_config": map[string]interface{}{
			"agent": map[string]interface{}{
				"prompt": map[string]interface{}{
					"prompt": "You are a helpful assistant. Keep responses brief.",
				},
				"first_message": "Hello! I'm a test agent.",
				"language":      "en",
			},
			"tts": map[string]interface{}{
				"voice_id": voiceID,
			},
		},
	}

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://api.elevenlabs.io/v1/convai/agents/create", bytes.NewReader(data))
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return "", fmt.Errorf("API error %d: %v", resp.StatusCode, errBody)
	}

	var result struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.AgentID, nil
}

func deleteAgent(apiKey, agentID string) {
	req, _ := http.NewRequest("DELETE", "https://api.elevenlabs.io/v1/convai/agents/"+agentID, nil)
	req.Header.Set("xi-api-key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("⚠️  Failed to delete agent: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("🗑️  Deleted test agent: %s", agentID)
}


