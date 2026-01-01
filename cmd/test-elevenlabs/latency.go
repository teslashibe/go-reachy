package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/teslashibe/go-reachy/pkg/conversation"
)

// runLatencyTest measures real-time latency by sending audio and measuring response time
func runLatencyTest(apiKey, voiceID string) error {
	fmt.Println("\n🔬 Running Latency Benchmark...")
	fmt.Println("=" + string(make([]byte, 50)))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create provider
	provider, err := conversation.NewElevenLabs(
		conversation.WithAPIKey(apiKey),
		conversation.WithVoiceID(voiceID),
		conversation.WithLLM("gemini-2.0-flash"),
		conversation.WithSystemPrompt("You are a test agent. When you hear audio, respond with a single word: 'pong'. Keep responses very short."),
		conversation.WithAgentName("latency-test"),
		conversation.WithAutoCreateAgent(true),
		conversation.WithTimeout(30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}

	agentID := ""
	defer func() {
		provider.Close()
		if agentID != "" {
			deleteAgent(apiKey, agentID)
		}
	}()

	// Latency tracking
	var (
		mu                  sync.Mutex
		audioSendTime       time.Time
		firstAudioTime      time.Time
		firstTranscriptTime time.Time
		gotResponse         bool
		responseDone        = make(chan struct{})
	)

	provider.OnAudio(func(audio []byte) {
		mu.Lock()
		defer mu.Unlock()
		if firstAudioTime.IsZero() && !audioSendTime.IsZero() {
			firstAudioTime = time.Now()
			fmt.Printf("⚡ First audio response: %v\n", firstAudioTime.Sub(audioSendTime))
		}
	})

	provider.OnTranscript(func(role, text string, isFinal bool) {
		mu.Lock()
		defer mu.Unlock()
		if role == "agent" && !gotResponse && !audioSendTime.IsZero() {
			if firstTranscriptTime.IsZero() {
				firstTranscriptTime = time.Now()
				fmt.Printf("⚡ First transcript: %v (text: %q)\n", firstTranscriptTime.Sub(audioSendTime), text)
			}
			if isFinal {
				gotResponse = true
				select {
				case <-responseDone:
				default:
					close(responseDone)
				}
			}
		}
	})

	provider.OnError(func(err error) {
		fmt.Printf("⚠️  Error: %v\n", err)
	})

	// Connect
	fmt.Print("🔌 Connecting... ")
	if err := provider.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	fmt.Println("✅")

	agentID = provider.AgentID()
	fmt.Printf("📋 Agent: %s\n\n", agentID)

	// Generate synthetic audio (16kHz PCM16 sine wave - simulates speech)
	// This creates a 1-second audio clip that sounds like a tone
	sampleRate := 16000
	duration := 1.0 // seconds
	frequency := 440.0 // Hz (A4 note)
	
	samples := int(float64(sampleRate) * duration)
	audio := make([]byte, samples*2) // 16-bit = 2 bytes per sample
	
	for i := 0; i < samples; i++ {
		// Generate sine wave
		t := float64(i) / float64(sampleRate)
		sample := int16(32767 * 0.5 * math.Sin(2*math.Pi*frequency*t))
		
		// Little-endian encoding
		audio[i*2] = byte(sample)
		audio[i*2+1] = byte(sample >> 8)
	}

	fmt.Println("📤 Sending 1 second of test audio (440Hz tone)...")
	
	// Send audio in 50ms chunks (matching our low-latency setting)
	chunkSize := sampleRate / 20 * 2 // 50ms of audio in bytes
	
	mu.Lock()
	audioSendTime = time.Now()
	mu.Unlock()

	for i := 0; i < len(audio); i += chunkSize {
		end := i + chunkSize
		if end > len(audio) {
			end = len(audio)
		}
		
		if err := provider.SendAudio(audio[i:end]); err != nil {
			fmt.Printf("⚠️  Send error: %v\n", err)
		}
		
		// Simulate real-time sending
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println("📤 Audio sent, waiting for response...")

	// Wait for response or timeout
	select {
	case <-responseDone:
		fmt.Println("✅ Got response!")
	case <-time.After(10 * time.Second):
		fmt.Println("⏰ Timeout waiting for response")
	case <-ctx.Done():
		fmt.Println("❌ Context cancelled")
	}

	// Print summary
	mu.Lock()
	fmt.Println("\n" + "=" + string(make([]byte, 50)))
	fmt.Println("📊 LATENCY SUMMARY")
	fmt.Println("=" + string(make([]byte, 50)))
	
	if !firstTranscriptTime.IsZero() {
		fmt.Printf("   Time to first transcript: %v\n", firstTranscriptTime.Sub(audioSendTime))
	}
	if !firstAudioTime.IsZero() {
		fmt.Printf("   Time to first audio:      %v\n", firstAudioTime.Sub(audioSendTime))
	}
	if firstAudioTime.IsZero() && firstTranscriptTime.IsZero() {
		fmt.Println("   ⚠️  No response received (agent may not recognize synthetic audio)")
		fmt.Println("   💡 Real latency test requires actual speech audio from Eva")
	}
	mu.Unlock()

	fmt.Println("\n📝 Note: This is synthetic audio (sine wave).")
	fmt.Println("   Real speech from Eva will have different characteristics.")
	fmt.Println("   Expected real-world latency: 400-700ms to first response")

	return nil
}

