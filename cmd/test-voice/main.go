// test-voice: Integration test for ElevenLabs voice pipeline
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/voice"
	_ "github.com/teslashibe/go-reachy/pkg/voice/bundled" // Register ElevenLabs pipeline
)

// ANSI colors for output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
)

func main() {
	// Parse flags - all voice-related flags use --voice- prefix
	loops := flag.Int("loops", 3, "Number of test loops to run")
	prompt := flag.String("prompt", "You are a helpful assistant. When you receive any audio input, immediately respond with a short greeting. Keep responses very brief.", "System prompt for the AI")
	debug := flag.Bool("debug", false, "Enable debug output")
	timeout := flag.Duration("timeout", 30*time.Second, "Timeout waiting for response")

	// Voice tuning flags
	voiceLLM := flag.String("voice-llm", voice.LLMGpt5Mini, "LLM model: gpt-5-mini, gpt-4.1-mini, gemini-2.0-flash, claude-3.5-sonnet")
	voiceTTS := flag.String("voice-tts", voice.TTSFlash, "TTS model: eleven_flash_v2, eleven_turbo_v2, eleven_multilingual_v2")
	voiceSTT := flag.String("voice-stt", voice.STTRealtime, "STT model: scribe_v2_realtime, scribe_v1")
	voiceChunk := flag.Duration("voice-chunk", 50*time.Millisecond, "Audio chunk duration (10ms-100ms)")
	voiceVADSilence := flag.Duration("voice-vad-silence", 500*time.Millisecond, "VAD silence duration to detect end of speech")

	// Benchmark mode
	benchmarkLLMs := flag.Bool("benchmark-llms", false, "Benchmark all LLM models for TTFA comparison")

	flag.Parse()

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║          🎤 ElevenLabs Voice Pipeline Test                ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Validate environment
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	voiceID := os.Getenv("ELEVENLABS_VOICE_ID")
	if apiKey == "" || voiceID == "" {
		fmt.Printf("%s❌ Missing environment variables:%s\n", colorRed, colorReset)
		fmt.Println("   export ELEVENLABS_API_KEY=\"your-api-key\"")
		fmt.Println("   export ELEVENLABS_VOICE_ID=\"your-voice-id\"")
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
		fmt.Println("\n🛑 Interrupted - shutting down...")
		cancel()
	}()

	// Load speech samples
	speechSamples, err := loadTestSpeech()
	if err != nil {
		fmt.Printf("%s⚠️  No speech file: %v%s\n", colorYellow, err, colorReset)
		fmt.Println("   Creating synthetic audio...")
		speechSamples = generateSyntheticSpeech(2 * time.Second)
	} else {
		fmt.Printf("%s✓ Loaded speech: %d samples (%.1fs)%s\n",
			colorGreen, len(speechSamples), float64(len(speechSamples))/16000.0, colorReset)
	}
	fmt.Println()

	// Build config
	cfg := voice.DefaultConfig().
		WithLLM(*voiceLLM).
		WithTTS(*voiceTTS).
		WithSTT(*voiceSTT).
		WithChunkDuration(*voiceChunk).
		WithVAD(*voiceVADSilence).
		WithSystemPrompt(*prompt).
		WithDebug(*debug)

	cfg.ElevenLabsKey = apiKey
	cfg.ElevenLabsVoiceID = voiceID

	// Run benchmark or single test
	if *benchmarkLLMs {
		runLLMBenchmark(ctx, cfg, speechSamples, *loops, *timeout)
	} else {
		runSingleTest(ctx, cfg, speechSamples, *loops, *timeout)
	}
}

// runSingleTest runs the configured test for the specified number of loops
func runSingleTest(ctx context.Context, cfg voice.Config, speechSamples []int16, loops int, timeout time.Duration) {
	fmt.Printf("📋 Configuration:\n")
	fmt.Printf("   LLM:  %s%s%s\n", colorCyan, cfg.LLMModel, colorReset)
	fmt.Printf("   TTS:  %s%s%s\n", colorCyan, cfg.TTSModel, colorReset)
	fmt.Printf("   STT:  %s%s%s\n", colorCyan, cfg.STTModel, colorReset)
	fmt.Printf("   Chunk: %s\n", cfg.ChunkDuration)
	fmt.Println()

	var ttfas []time.Duration
	var errors []string

	for i := 0; i < loops; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fmt.Printf("🔄 Loop %d/%d: ", i+1, loops)

		ttfa, err := runTestLoop(ctx, cfg, speechSamples, timeout)
		if err != nil {
			errors = append(errors, err.Error())
			fmt.Printf("%s❌ %s%s\n", colorRed, err.Error(), colorReset)
		} else {
			ttfas = append(ttfas, ttfa)
			fmt.Printf("%s✅ TTFA: %s%s\n", colorGreen, formatDuration(ttfa), colorReset)
		}
	}

	printSummary(cfg.LLMModel, loops, ttfas, errors)
}

// runTestLoop runs a single test iteration and returns TTFA
func runTestLoop(ctx context.Context, cfg voice.Config, speechSamples []int16, timeout time.Duration) (time.Duration, error) {
	// Create pipeline
	pipeline, err := voice.New(cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Track timing
	var audioSendEndTime, firstAudioTime time.Time
	responseChan := make(chan struct{}, 1)

	// Set up callbacks
	pipeline.OnAudioOut(func(pcm16 []byte) {
		if firstAudioTime.IsZero() {
			firstAudioTime = time.Now()
			select {
			case responseChan <- struct{}{}:
			default:
			}
		}
	})

	pipeline.OnError(func(err error) {
		if cfg.Debug {
			fmt.Printf("   [error] %v\n", err)
		}
	})

	// Start pipeline
	testCtx, testCancel := context.WithTimeout(ctx, timeout)
	defer testCancel()

	if err := pipeline.Start(testCtx); err != nil {
		return 0, fmt.Errorf("failed to start: %w", err)
	}
	defer pipeline.Stop()

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Send audio in chunks
	chunkSamples := int(float64(16000) * cfg.ChunkDuration.Seconds())
	for i := 0; i < len(speechSamples); i += chunkSamples {
		select {
		case <-testCtx.Done():
			return 0, testCtx.Err()
		default:
		}

		end := i + chunkSamples
		if end > len(speechSamples) {
			end = len(speechSamples)
		}

		chunk := speechSamples[i:end]
		pcm16 := samplesToBytes(chunk)
		if err := pipeline.SendAudio(pcm16); err != nil {
			return 0, fmt.Errorf("send error: %w", err)
		}

		time.Sleep(cfg.ChunkDuration)
	}

	audioSendEndTime = time.Now()

	// Send trailing silence to trigger VAD
	silenceSamples := make([]int16, int(float64(16000)*cfg.VADSilenceDuration.Seconds()))
	silenceBytes := samplesToBytes(silenceSamples)
	for i := 0; i < len(silenceBytes); i += chunkSamples * 2 {
		select {
		case <-testCtx.Done():
			return 0, testCtx.Err()
		default:
		}

		end := i + chunkSamples*2
		if end > len(silenceBytes) {
			end = len(silenceBytes)
		}
		pipeline.SendAudio(silenceBytes[i:end])
		time.Sleep(cfg.ChunkDuration)
	}

	// Wait for response
	select {
	case <-responseChan:
		// Success
	case <-time.After(timeout):
		return 0, fmt.Errorf("timeout waiting for response")
	case <-testCtx.Done():
		return 0, testCtx.Err()
	}

	// Calculate TTFA
	if !audioSendEndTime.IsZero() && !firstAudioTime.IsZero() {
		if firstAudioTime.After(audioSendEndTime) {
			return firstAudioTime.Sub(audioSendEndTime), nil
		}
		return 1 * time.Millisecond, nil // Audio started during send (very fast!)
	}

	return 0, fmt.Errorf("timing error")
}

// runLLMBenchmark tests all LLM models for TTFA comparison
func runLLMBenchmark(ctx context.Context, baseCfg voice.Config, speechSamples []int16, loops int, timeout time.Duration) {
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║            🔬 LLM Model Benchmark (TTFA)                  ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	llmModels := []string{
		// Fastest (from prior benchmarks)
		"gpt-5-mini", "gpt-4.1-mini", "gpt-4.1", "gpt-4.1-nano",
		"gemini-2.0-flash", "gemini-2.0-flash-lite",
		"gpt-4o-mini", "gpt-4o",
		"claude-3.5-sonnet", "claude-haiku-4.5",
		// Additional
		"gpt-5", "gpt-5-nano",
		"claude-sonnet-4", "claude-sonnet-4.5",
	}

	type Result struct {
		Model string
		TTFAs []time.Duration
		Errs  []string
	}

	var results []Result

	for _, model := range llmModels {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fmt.Printf("📊 Testing %s%s%s...\n", colorPurple, model, colorReset)

		cfg := baseCfg.WithLLM(model)
		result := Result{Model: model}

		for i := 0; i < loops; i++ {
			ttfa, err := runTestLoop(ctx, cfg, speechSamples, timeout)
			if err != nil {
				result.Errs = append(result.Errs, err.Error())
				fmt.Printf("   Loop %d: %s❌%s\n", i+1, colorRed, colorReset)
			} else {
				result.TTFAs = append(result.TTFAs, ttfa)
				fmt.Printf("   Loop %d: %s%s%s\n", i+1, colorGreen, formatDuration(ttfa), colorReset)
			}
		}

		results = append(results, result)
		fmt.Println()
	}

	// Sort by average TTFA
	sort.Slice(results, func(i, j int) bool {
		avgI, avgJ := avgDuration(results[i].TTFAs), avgDuration(results[j].TTFAs)
		if avgI == 0 {
			return false
		}
		if avgJ == 0 {
			return true
		}
		return avgI < avgJ
	})

	// Print ranked results
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║                    📈 RESULTS (Ranked)                    ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("%-5s %-25s %10s %10s %10s\n", "Rank", "Model", "Avg TTFA", "Min", "Max")
	fmt.Println("───────────────────────────────────────────────────────────────")

	for i, r := range results {
		rank := fmt.Sprintf("%d", i+1)
		if i == 0 {
			rank = "🥇"
		} else if i == 1 {
			rank = "🥈"
		} else if i == 2 {
			rank = "🥉"
		}

		if len(r.TTFAs) == 0 {
			fmt.Printf("%-5s %-25s %s(failed)%s\n", rank, r.Model, colorRed, colorReset)
			continue
		}

		avg := avgDuration(r.TTFAs)
		min, max := minMaxDuration(r.TTFAs)
		fmt.Printf("%-5s %-25s %10s %10s %10s\n", rank, r.Model, formatDuration(avg), formatDuration(min), formatDuration(max))
	}
	fmt.Println()
}

// Helper functions

func loadTestSpeech() ([]int16, error) {
	// Look for real_speech.wav first, then test_speech.wav
	paths := []string{
		filepath.Join("cmd", "test-voice", "testdata", "real_speech.wav"),
		filepath.Join("cmd", "test-voice", "testdata", "test_speech.wav"),
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 44 {
			// Skip WAV header (44 bytes)
			audio := data[44:]
			samples := make([]int16, len(audio)/2)
			for i := 0; i < len(samples); i++ {
				samples[i] = int16(binary.LittleEndian.Uint16(audio[i*2:]))
			}
			return samples, nil
		}
	}

	return nil, fmt.Errorf("no speech file found")
}

func generateSyntheticSpeech(duration time.Duration) []int16 {
	sampleRate := 16000
	samples := int(float64(sampleRate) * duration.Seconds())
	audio := make([]int16, samples)

	// Generate simple tone with some variation
	freq := 440.0 // A4
	for i := range audio {
		t := float64(i) / float64(sampleRate)
		// Add harmonics for richer sound
		val := 8000.0 * (0.5*sine(freq*t) + 0.3*sine(freq*2*t) + 0.2*sine(freq*3*t))
		audio[i] = int16(val)
	}

	return audio
}

func sine(t float64) float64 {
	const twoPi = 6.283185307179586
	return float64(int16(32767 * sinApprox(t*twoPi)))
}

func sinApprox(x float64) float64 {
	// Simple sine approximation
	for x > 3.14159 {
		x -= 6.28318
	}
	for x < -3.14159 {
		x += 6.28318
	}
	return x - x*x*x/6 + x*x*x*x*x/120
}

func samplesToBytes(samples []int16) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, samples)
	return buf.Bytes()
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func avgDuration(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	return sum / time.Duration(len(ds))
}

func minMaxDuration(ds []time.Duration) (min, max time.Duration) {
	if len(ds) == 0 {
		return 0, 0
	}
	min, max = ds[0], ds[0]
	for _, d := range ds[1:] {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return
}

func printSummary(model string, loops int, ttfas []time.Duration, errors []string) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║                       📊 SUMMARY                          ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("   Model:    %s%s%s\n", colorCyan, model, colorReset)
	fmt.Printf("   Loops:    %d\n", loops)
	fmt.Printf("   Success:  %s%d/%d%s\n", colorGreen, len(ttfas), loops, colorReset)

	if len(errors) > 0 {
		fmt.Printf("   Errors:   %s%d%s\n", colorRed, len(errors), colorReset)
	}

	if len(ttfas) > 0 {
		avg := avgDuration(ttfas)
		min, max := minMaxDuration(ttfas)
		fmt.Println()
		fmt.Printf("   TTFA Avg: %s%s%s\n", colorGreen, formatDuration(avg), colorReset)
		fmt.Printf("   TTFA Min: %s\n", formatDuration(min))
		fmt.Printf("   TTFA Max: %s\n", formatDuration(max))
	}
	fmt.Println()
}
