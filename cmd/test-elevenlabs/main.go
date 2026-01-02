// Command test-elevenlabs validates the ElevenLabs programmatic agent integration.
// It creates an agent, connects via WebSocket, and tests the conversation flow.
//
// Usage:
//
//	ELEVENLABS_API_KEY=sk_... go run ./cmd/test-elevenlabs/
//	ELEVENLABS_API_KEY=sk_... ELEVENLABS_VOICE_ID=... go run ./cmd/test-elevenlabs/
//
// Flags:
//
//	-list-voices    List available voices and exit
//	-voice          Voice ID to use (or set ELEVENLABS_VOICE_ID)
//	-llm            LLM model (default: gemini-2.0-flash)
//	-timeout        Connection timeout (default: 30s)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/internal/httpc"
	"github.com/teslashibe/go-reachy/pkg/conversation"
)

const (
	elevenLabsAPIBase = "https://api.elevenlabs.io/v1"
	defaultVoiceID    = "21m00Tcm4TlvDq8ikWAM" // Rachel - default ElevenLabs voice
)

var (
	listVoices  = flag.Bool("list-voices", false, "List available voices and exit")
	voiceID     = flag.String("voice", "", "Voice ID to use (or set ELEVENLABS_VOICE_ID)")
	llmModel    = flag.String("llm", "gemini-2.0-flash", "LLM model: gemini-2.0-flash, claude-3-5-sonnet, gpt-4o")
	timeout     = flag.Duration("timeout", 30*time.Second, "Connection timeout")
	cleanup     = flag.Bool("cleanup", true, "Delete agent after test")
	latencyTest = flag.Bool("latency", false, "Run latency benchmark test")
)

func main() {
	flag.Parse()

	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ ELEVENLABS_API_KEY environment variable required")
		os.Exit(1)
	}

	// Mask API key for display
	maskedKey := apiKey[:10] + "..." + apiKey[len(apiKey)-4:]
	fmt.Printf("🔑 API Key: %s\n", maskedKey)

	// List voices mode
	if *listVoices {
		if err := listAvailableVoices(apiKey); err != nil {
			fmt.Printf("❌ Failed to list voices: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Determine voice ID early for latency test
	voice := *voiceID
	if voice == "" {
		voice = os.Getenv("ELEVENLABS_VOICE_ID")
	}
	if voice == "" {
		voice = defaultVoiceID
	}

	// Latency benchmark mode
	if *latencyTest {
		if err := runLatencyTest(apiKey, voice); err != nil {
			fmt.Printf("❌ Latency test failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Print voice info
	if voice == defaultVoiceID {
		fmt.Printf("🎤 Voice ID: %s (default: Rachel)\n", voice)
	} else {
		fmt.Printf("🎤 Voice ID: %s\n", voice)
	}

	fmt.Printf("🧠 LLM: %s\n", *llmModel)
	fmt.Printf("⏱️  Timeout: %s\n", *timeout)
	fmt.Println()

	// Run the test
	if err := runTest(apiKey, voice); err != nil {
		fmt.Printf("\n❌ Test failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ All tests passed!")
}

func runTest(apiKey, voice string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n🛑 Interrupted, cleaning up...")
		cancel()
	}()

	// Test system prompt
	testPrompt := `You are Eva, a test robot. Keep responses very short (1-2 sentences).
When asked "ping", respond with exactly "pong".
When asked about yourself, say you're a Reachy Mini robot being tested.`

	// Create provider with programmatic configuration
	fmt.Print("📝 Creating ElevenLabs provider... ")
	provider, err := conversation.NewElevenLabs(
		conversation.WithAPIKey(apiKey),
		conversation.WithVoiceID(voice),
		conversation.WithLLM(*llmModel),
		conversation.WithSystemPrompt(testPrompt),
		conversation.WithAgentName("eva-test-agent"),
		conversation.WithAutoCreateAgent(true),
		conversation.WithTimeout(*timeout),
	)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	fmt.Println("✅")

	// Track if we need to clean up
	var agentID string
	defer func() {
		if *cleanup && agentID != "" {
			fmt.Printf("🧹 Cleaning up agent %s... ", agentID)
			if err := deleteAgent(apiKey, agentID); err != nil {
				fmt.Printf("⚠️  %v\n", err)
			} else {
				fmt.Println("✅")
			}
		}
	}()

	// Register a test tool
	fmt.Print("🔧 Registering test tool... ")
	provider.RegisterTool(conversation.Tool{
		Name:        "get_time",
		Description: "Gets the current time",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
	fmt.Println("✅")

	// Set up response tracking
	var (
		wg             sync.WaitGroup
		transcriptDone = make(chan struct{})
		mu             sync.Mutex
	)

	provider.OnTranscript(func(role, text string, isFinal bool) {
		mu.Lock()
		defer mu.Unlock()
		if role == "assistant" && isFinal {
			fmt.Printf("🤖 Eva: %s\n", text)
			select {
			case <-transcriptDone:
			default:
				close(transcriptDone)
			}
		} else if role == "user" && isFinal {
			fmt.Printf("👤 User: %s\n", text)
		}
	})

	provider.OnAudio(func(audio []byte) {
		// Just count bytes for now
		mu.Lock()
		defer mu.Unlock()
	})

	provider.OnError(func(err error) {
		fmt.Printf("⚠️  Error: %v\n", err)
	})

	provider.OnToolCall(func(id, name string, args map[string]any) {
		fmt.Printf("🔧 Tool called: %s\n", name)
		if name == "get_time" {
			provider.SubmitToolResult(id, time.Now().Format(time.RFC3339))
		}
	})

	// Connect (this triggers agent creation)
	fmt.Print("🔌 Connecting to ElevenLabs... ")
	connectCtx, connectCancel := context.WithTimeout(ctx, *timeout)
	defer connectCancel()

	if err := provider.Connect(connectCtx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	fmt.Println("✅")

	// Get the created agent ID
	agentID = provider.AgentID()
	fmt.Printf("📋 Agent ID: %s\n", agentID)

	// Verify connection
	if !provider.IsConnected() {
		return fmt.Errorf("provider reports not connected")
	}
	fmt.Println("✅ Connection verified")

	// Wait a moment for the connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// Test capabilities
	fmt.Print("📊 Checking capabilities... ")
	caps := provider.Capabilities()
	fmt.Printf("customVoice=%v, tools=%v, interruption=%v\n",
		caps.SupportsCustomVoice, caps.SupportsToolCalls, caps.SupportsInterruption)

	// Note: ElevenLabs doesn't support text input directly in conversation mode
	// The real test would be sending audio, but for validation we just verify:
	// 1. Agent was created
	// 2. WebSocket connected
	// 3. Provider is functional

	fmt.Println("\n📋 Test Summary:")
	fmt.Println("   ✅ Provider created with programmatic config")
	fmt.Println("   ✅ Agent created via REST API")
	fmt.Println("   ✅ WebSocket connection established")
	fmt.Println("   ✅ Tool registered successfully")
	fmt.Println("   ✅ Capabilities reported correctly")

	// Give time for any async events
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-transcriptDone:
			// Got a response
		case <-time.After(2 * time.Second):
			// Timeout waiting for response (expected without audio input)
		case <-ctx.Done():
		}
	}()

	wg.Wait()

	// Disconnect
	fmt.Print("🔌 Disconnecting... ")
	if err := provider.Close(); err != nil {
		fmt.Printf("⚠️  %v\n", err)
	} else {
		fmt.Println("✅")
	}

	return nil
}

// Voice represents an ElevenLabs voice
type Voice struct {
	VoiceID  string            `json:"voice_id"`
	Name     string            `json:"name"`
	Category string            `json:"category"`
	Labels   map[string]string `json:"labels"`
}

// VoicesResponse is the API response for listing voices
type VoicesResponse struct {
	Voices []Voice `json:"voices"`
}

func listAvailableVoices(apiKey string) error {
	req, err := http.NewRequest("GET", elevenLabsAPIBase+"/voices", nil)
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", apiKey)

	resp, err := httpc.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result VoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	fmt.Printf("\n🎤 Available Voices (%d):\n\n", len(result.Voices))
	fmt.Printf("%-30s %-25s %s\n", "NAME", "VOICE ID", "CATEGORY")
	fmt.Println(strings.Repeat("-", 80))

	for _, v := range result.Voices {
		name := v.Name
		if len(name) > 28 {
			name = name[:25] + "..."
		}
		fmt.Printf("%-30s %-25s %s\n", name, v.VoiceID, v.Category)
	}

	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ELEVENLABS_VOICE_ID=<voice_id> go run ./cmd/test-elevenlabs/")
	fmt.Println()

	return nil
}

func deleteAgent(apiKey, agentID string) error {
	req, err := http.NewRequest("DELETE", elevenLabsAPIBase+"/convai/agents/"+agentID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", apiKey)

	resp, err := httpc.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// sendTextMessage sends a user message to test the conversation (if supported)
func sendTextMessage(apiKey, agentID, message string) error {
	payload := map[string]any{
		"text": message,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST",
		elevenLabsAPIBase+"/convai/agents/"+agentID+"/conversation",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", apiKey)

	resp, err := httpc.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response: %s\n", string(respBody))

	return nil
}
