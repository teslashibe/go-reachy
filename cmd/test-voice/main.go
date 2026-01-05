// Command test-voice provides standalone integration tests for voice pipelines.
// Run independently of Eva to quickly measure and tune latency.
//
// Usage:
//
//	go run ./cmd/test-voice --provider openai --loops 3
//	go run ./cmd/test-voice --provider gemini --loops 5
//	go run ./cmd/test-voice --provider elevenlabs --loops 3
//	go run ./cmd/test-voice --provider gemini --mic  # Use microphone input
//
// Environment variables required:
//
//	OPENAI_API_KEY      - For OpenAI Realtime
//	GOOGLE_API_KEY      - For Gemini Live
//	ELEVENLABS_API_KEY  - For ElevenLabs
//	ELEVENLABS_VOICE_ID - For ElevenLabs (e.g., cloned voice ID)
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/voice"
	_ "github.com/teslashibe/go-reachy/pkg/voice/bundled" // Register all providers
)

func main() {
	// Parse flags
	provider := flag.String("provider", "openai", "Voice provider: openai, gemini, elevenlabs")
	loops := flag.Int("loops", 3, "Number of test loops to run")
	duration := flag.Duration("duration", 2*time.Second, "Duration of test audio per loop")
	prompt := flag.String("prompt", "You are a helpful assistant. When you receive any audio input, immediately respond with a short greeting. Keep responses very brief.", "System prompt for the AI")
	debug := flag.Bool("debug", false, "Enable debug output")
	interactive := flag.Bool("mic", false, "Use microphone input (interactive mode)")
	flag.Parse()

	if *interactive {
		fmt.Println("🎤 Interactive mode not yet implemented.")
		fmt.Println("   For now, use the synthetic audio test.")
		fmt.Println("   Tip: The test sends speech-like audio patterns to trigger VAD.")
		os.Exit(0)
	}

	fmt.Println("🎤 Voice Pipeline Integration Test")
	fmt.Println("===================================")
	fmt.Printf("Provider: %s\n", *provider)
	fmt.Printf("Loops: %d\n", *loops)
	fmt.Printf("Audio duration: %s\n", *duration)
	fmt.Println()

	// Build config
	cfg := voice.Config{
		Provider:        voice.Provider(*provider),
		OpenAIKey:       os.Getenv("OPENAI_API_KEY"),
		GoogleAPIKey:    os.Getenv("GOOGLE_API_KEY"),
		ElevenLabsKey:   os.Getenv("ELEVENLABS_API_KEY"),
		ElevenLabsVoiceID: os.Getenv("ELEVENLABS_VOICE_ID"),
		SystemPrompt:    *prompt,
		ProfileLatency:  true,
		Debug:           *debug,
		InputSampleRate:  16000,
		OutputSampleRate: 24000,
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		fmt.Printf("❌ Config error: %v\n", err)
		fmt.Println("\nRequired environment variables:")
		fmt.Println("  OPENAI_API_KEY      - For OpenAI Realtime")
		fmt.Println("  GOOGLE_API_KEY      - For Gemini Live")
		fmt.Println("  ELEVENLABS_API_KEY  - For ElevenLabs")
		fmt.Println("  ELEVENLABS_VOICE_ID - For ElevenLabs")
		os.Exit(1)
	}

	// Create pipeline
	pipeline, err := voice.New(cfg.Provider, cfg)
	if err != nil {
		fmt.Printf("❌ Failed to create pipeline: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n🛑 Interrupted")
		cancel()
	}()

	// Run test
	tester := NewPipelineTester(pipeline, cfg)
	results := tester.Run(ctx, *loops, *duration)

	// Print results
	printResults(cfg.Provider, results)

	// Cleanup
	pipeline.Stop()
}

// TestResult holds metrics for a single test run.
type TestResult struct {
	Loop           int
	AudioSent      int           // Bytes of audio sent
	AudioReceived  int           // Bytes of audio received
	PipelineLatency time.Duration // Time from last audio sent to first audio received
	TotalLatency   time.Duration // Total time from start to response complete
	Metrics        voice.Metrics // Full metrics snapshot
	Error          error
}

// PipelineTester runs latency tests on a voice pipeline.
type PipelineTester struct {
	pipeline voice.Pipeline
	config   voice.Config
}

// NewPipelineTester creates a new tester.
func NewPipelineTester(p voice.Pipeline, cfg voice.Config) *PipelineTester {
	return &PipelineTester{
		pipeline: p,
		config:   cfg,
	}
}

// Run executes the test loops.
func (t *PipelineTester) Run(ctx context.Context, loops int, audioDuration time.Duration) []TestResult {
	results := make([]TestResult, 0, loops)

	// Start pipeline
	fmt.Println("🔌 Connecting to pipeline...")
	if err := t.pipeline.Start(ctx); err != nil {
		fmt.Printf("❌ Failed to start pipeline: %v\n", err)
		return results
	}

	// Wait for connection
	for i := 0; i < 50; i++ {
		if t.pipeline.IsConnected() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !t.pipeline.IsConnected() {
		fmt.Println("❌ Pipeline failed to connect")
		return results
	}
	fmt.Println("✅ Connected!")
	fmt.Println()

	// Run test loops
	for i := 1; i <= loops; i++ {
		select {
		case <-ctx.Done():
			return results
		default:
		}

		fmt.Printf("📝 Test %d/%d\n", i, loops)
		result := t.runSingleTest(ctx, i, audioDuration)
		results = append(results, result)

		if result.Error != nil {
			fmt.Printf("   ❌ Error: %v\n", result.Error)
		} else {
			fmt.Printf("   📊 Pipeline: %s | Total: %s\n",
				formatDuration(result.PipelineLatency),
				formatDuration(result.TotalLatency))
		}

		// Wait between loops
		if i < loops {
			time.Sleep(2 * time.Second)
		}
	}

	return results
}

// runSingleTest executes one test loop.
func (t *PipelineTester) runSingleTest(ctx context.Context, loop int, audioDuration time.Duration) TestResult {
	result := TestResult{Loop: loop}

	var mu sync.Mutex
	var firstAudioTime time.Time
	var responseComplete bool
	var audioReceived int

	// Setup callbacks
	t.pipeline.OnAudioOut(func(pcm16 []byte) {
		mu.Lock()
		defer mu.Unlock()
		if firstAudioTime.IsZero() {
			firstAudioTime = time.Now()
		}
		audioReceived += len(pcm16)
	})

	t.pipeline.OnSpeechEnd(func() {
		if t.config.Debug {
			fmt.Println("   🎤 Speech end detected")
		}
	})

	t.pipeline.OnTranscript(func(text string, isFinal bool) {
		if isFinal && t.config.Debug {
			fmt.Printf("   📝 Transcript: %s\n", text)
		}
	})

	t.pipeline.OnResponse(func(text string, isFinal bool) {
		if isFinal {
			mu.Lock()
			responseComplete = true
			mu.Unlock()
			if t.config.Debug {
				fmt.Printf("   💬 Response: %s\n", truncate(text, 50))
			}
		}
	})

	t.pipeline.OnError(func(err error) {
		mu.Lock()
		result.Error = err
		mu.Unlock()
	})

	// Load or generate test audio
	sampleRate := t.config.InputSampleRate
	if sampleRate == 0 {
		sampleRate = 16000
	}

	// Try to load real speech sample first
	speechSamples, err := loadTestSpeech()
	useRealSpeech := err == nil && len(speechSamples) > 0
	
	if useRealSpeech {
		fmt.Println("   📂 Using real speech sample")
	} else {
		fmt.Println("   🔊 Using synthetic audio (no test_speech.wav found)")
	}

	startTime := time.Now()
	audioSent := 0

	// Send audio in chunks (simulating real-time streaming)
	chunkDuration := 100 * time.Millisecond
	chunkSamples := int(float64(sampleRate) * chunkDuration.Seconds())

	if useRealSpeech {
		// Send real speech in chunks
		for i := 0; i < len(speechSamples); i += chunkSamples {
			select {
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result
			default:
			}

			end := i + chunkSamples
			if end > len(speechSamples) {
				end = len(speechSamples)
			}
			chunk := speechSamples[i:end]

			// Convert to bytes
			pcm16 := make([]byte, len(chunk)*2)
			for j, sample := range chunk {
				pcm16[j*2] = byte(sample & 0xFF)
				pcm16[j*2+1] = byte(sample >> 8)
			}

			if err := t.pipeline.SendAudio(pcm16); err != nil {
				result.Error = err
				return result
			}
			audioSent += len(pcm16)

			// Simulate real-time streaming
			time.Sleep(chunkDuration)
		}
	} else {
		// Fall back to synthetic audio
		totalChunks := int(audioDuration / chunkDuration)
		for i := 0; i < totalChunks; i++ {
			select {
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result
			default:
			}

			// Generate audio chunk (speech-like signal with some noise)
			chunk := generateSpeechLikeAudio(chunkSamples, sampleRate, i)

			// Convert to bytes
			pcm16 := make([]byte, len(chunk)*2)
			for j, sample := range chunk {
				pcm16[j*2] = byte(sample & 0xFF)
				pcm16[j*2+1] = byte(sample >> 8)
			}

			if err := t.pipeline.SendAudio(pcm16); err != nil {
				result.Error = err
				return result
			}
			audioSent += len(pcm16)

			// Simulate real-time streaming
			time.Sleep(chunkDuration)
		}
	}

	lastAudioSent := time.Now()
	result.AudioSent = audioSent

	// Wait for response (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		case <-timeout:
			result.Error = fmt.Errorf("timeout waiting for response")
			return result
		case <-ticker.C:
			mu.Lock()
			hasAudio := !firstAudioTime.IsZero()
			done := responseComplete
			received := audioReceived
			first := firstAudioTime
			mu.Unlock()

			if hasAudio && done {
				result.AudioReceived = received
				result.PipelineLatency = first.Sub(lastAudioSent)
				result.TotalLatency = time.Since(startTime)
				result.Metrics = t.pipeline.Metrics()
				return result
			}

			// If we have audio but response not marked complete, wait a bit more
			if hasAudio && time.Since(first) > 5*time.Second {
				result.AudioReceived = received
				result.PipelineLatency = first.Sub(lastAudioSent)
				result.TotalLatency = time.Since(startTime)
				result.Metrics = t.pipeline.Metrics()
				return result
			}
		}
	}
}

// loadTestSpeech loads the pre-recorded speech sample from testdata.
func loadTestSpeech() ([]int16, error) {
	// Try to find testdata relative to executable or current directory
	paths := []string{
		"cmd/test-voice/testdata/test_speech.wav",
		"testdata/test_speech.wav",
		"../testdata/test_speech.wav",
	}

	var data []byte
	var err error
	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("could not load test_speech.wav: %w", err)
	}

	// Parse WAV file (simple parser for 16-bit PCM)
	// WAV header is 44 bytes for standard PCM
	if len(data) < 44 {
		return nil, fmt.Errorf("WAV file too small")
	}

	// Skip header, read samples
	audioData := data[44:]
	samples := make([]int16, len(audioData)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(audioData[i*2]) | int16(audioData[i*2+1])<<8
	}

	return samples, nil
}

// generateSpeechLikeAudio creates audio that sounds like speech patterns.
// This is a fallback if no test audio file is available.
func generateSpeechLikeAudio(samples int, sampleRate int, chunkIndex int) []int16 {
	audio := make([]int16, samples)
	
	// Simulate speech with varying frequencies and amplitudes
	baseFreq := 200.0 + float64(chunkIndex%5)*50 // Vary pitch
	amplitude := 8000.0

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		
		// Fundamental frequency
		sample := math.Sin(2 * math.Pi * baseFreq * t)
		
		// Add harmonics (characteristic of voice)
		sample += 0.5 * math.Sin(2 * math.Pi * baseFreq * 2 * t)
		sample += 0.25 * math.Sin(2 * math.Pi * baseFreq * 3 * t)
		
		// Add some modulation (like syllables)
		envelope := 0.5 + 0.5*math.Sin(2*math.Pi*4*t)
		sample *= envelope
		
		// Add slight noise
		noise := (float64(i%7) - 3) / 100
		sample += noise
		
		audio[i] = int16(sample * amplitude)
	}
	
	return audio
}

// printResults displays the test results summary.
func printResults(provider voice.Provider, results []TestResult) {
	fmt.Println()
	fmt.Println("📊 Results Summary")
	fmt.Println("==================")
	fmt.Printf("Provider: %s\n", provider)
	fmt.Printf("Tests run: %d\n", len(results))
	fmt.Println()

	if len(results) == 0 {
		fmt.Println("No results to display.")
		return
	}

	// Calculate averages
	var totalPipeline, totalLatency time.Duration
	var successCount int
	var minPipeline, maxPipeline time.Duration

	for i, r := range results {
		if r.Error != nil {
			continue
		}
		successCount++
		totalPipeline += r.PipelineLatency
		totalLatency += r.TotalLatency

		if i == 0 || r.PipelineLatency < minPipeline {
			minPipeline = r.PipelineLatency
		}
		if r.PipelineLatency > maxPipeline {
			maxPipeline = r.PipelineLatency
		}
	}

	if successCount == 0 {
		fmt.Println("❌ All tests failed.")
		return
	}

	avgPipeline := totalPipeline / time.Duration(successCount)
	avgTotal := totalLatency / time.Duration(successCount)

	fmt.Println("┌─────────────────────────────────────────┐")
	fmt.Println("│           LATENCY METRICS               │")
	fmt.Println("├─────────────────────────────────────────┤")
	fmt.Printf("│  Pipeline (avg): %-22s│\n", formatDuration(avgPipeline))
	fmt.Printf("│  Pipeline (min): %-22s│\n", formatDuration(minPipeline))
	fmt.Printf("│  Pipeline (max): %-22s│\n", formatDuration(maxPipeline))
	fmt.Printf("│  Total (avg):    %-22s│\n", formatDuration(avgTotal))
	fmt.Printf("│  Success rate:   %d/%d                    │\n", successCount, len(results))
	fmt.Println("└─────────────────────────────────────────┘")

	// Detailed results table
	fmt.Println()
	fmt.Println("Detailed Results:")
	fmt.Println("┌──────┬────────────────┬────────────────┬──────────────┐")
	fmt.Println("│ Loop │ Pipeline       │ Total          │ Status       │")
	fmt.Println("├──────┼────────────────┼────────────────┼──────────────┤")
	for _, r := range results {
		status := "✅ OK"
		if r.Error != nil {
			status = "❌ Error"
		}
		fmt.Printf("│ %4d │ %14s │ %14s │ %12s │\n",
			r.Loop,
			formatDuration(r.PipelineLatency),
			formatDuration(r.TotalLatency),
			status)
	}
	fmt.Println("└──────┴────────────────┴────────────────┴──────────────┘")
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "---"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

