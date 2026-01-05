// Command test-voice provides standalone integration tests for voice pipelines.
// Run independently of Eva to quickly measure and tune latency.
//
// Usage:
//
//	go run ./cmd/test-voice --provider openai --loops 3
//	go run ./cmd/test-voice --provider gemini --loops 5
//	go run ./cmd/test-voice --provider elevenlabs --loops 3
//	go run ./cmd/test-voice --all --loops 1  # Test all providers in parallel
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

// ANSI color codes for output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Provider colors for visual distinction
var providerColors = map[voice.Provider]string{
	voice.ProviderOpenAI:     colorGreen,
	voice.ProviderGemini:     colorBlue,
	voice.ProviderElevenLabs: colorPurple,
}

func main() {
	// Parse flags
	provider := flag.String("provider", "openai", "Voice provider: openai, gemini, elevenlabs")
	allProviders := flag.Bool("all", false, "Test all providers in parallel")
	loops := flag.Int("loops", 1, "Number of test loops to run")
	duration := flag.Duration("duration", 2*time.Second, "Duration of test audio per loop")
	prompt := flag.String("prompt", "You are a helpful assistant. When you receive any audio input, immediately respond with a short greeting. Keep responses very brief.", "System prompt for the AI")
	debug := flag.Bool("debug", false, "Enable debug output")
	interactive := flag.Bool("mic", false, "Use microphone input (interactive mode)")
	timeout := flag.Duration("timeout", 30*time.Second, "Timeout waiting for response")
	silenceDuration := flag.Duration("silence", 1*time.Second, "Trailing silence after speech to trigger VAD end")
	forceCommit := flag.Bool("force-commit", false, "Force commit audio buffer (OpenAI) or signal turn complete (Gemini)")
	
	// Benchmark tuning parameters
	chunkDuration := flag.Duration("chunk", 100*time.Millisecond, "Audio chunk duration (10ms-100ms, lower = faster)")
	vadMode := flag.String("vad-mode", "server_vad", "VAD mode: server_vad, semantic_vad (OpenAI)")
	vadEagerness := flag.String("vad-eagerness", "medium", "Semantic VAD eagerness: low, medium, high (OpenAI)")
	vadSilence := flag.Duration("vad-silence", 500*time.Millisecond, "VAD silence duration to detect end of speech")
	vadThreshold := flag.Float64("vad-threshold", 0.5, "VAD threshold 0.0-1.0 (OpenAI server_vad)")
	benchmark := flag.Bool("benchmark", false, "Run comprehensive benchmark across all settings")
	flag.Parse()

	if *interactive {
		fmt.Println("🎤 Interactive mode not yet implemented.")
		fmt.Println("   For now, use the synthetic audio test.")
		os.Exit(0)
	}

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║          🎤 Voice Pipeline Integration Test               ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

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

	// Load speech samples once (shared across all workers)
	speechSamples, speechErr := loadTestSpeech()
	if speechErr != nil {
		logWarn("MAIN", "No speech file found: %v", speechErr)
		logInfo("MAIN", "Will use synthetic audio")
	} else {
		logSuccess("MAIN", "Loaded speech file: %d samples (%.1fs at 16kHz)", 
			len(speechSamples), float64(len(speechSamples))/16000.0)
	}

	// Base config (will be cloned per provider)
	baseConfig := voice.Config{
		OpenAIKey:         os.Getenv("OPENAI_API_KEY"),
		GoogleAPIKey:      os.Getenv("GOOGLE_API_KEY"),
		ElevenLabsKey:     os.Getenv("ELEVENLABS_API_KEY"),
		ElevenLabsVoiceID: os.Getenv("ELEVENLABS_VOICE_ID"),
		SystemPrompt:      *prompt,
		ProfileLatency:    false, // We do our own profiling
		Debug:             *debug,
		InputSampleRate:   16000,
		OutputSampleRate:  24000,
		// VAD tuning parameters
		ChunkDuration:      *chunkDuration,
		VADMode:            *vadMode,
		VADEagerness:       *vadEagerness,
		VADSilenceDuration: *vadSilence,
		VADThreshold:       *vadThreshold,
	}
	
	// If benchmark mode, run comprehensive tests
	if *benchmark {
		runBenchmarkSuite(ctx, baseConfig, speechSamples)
		return
	}

	testOpts := TestOptions{
		Loops:           *loops,
		Duration:        *duration,
		Timeout:         *timeout,
		SilenceDuration: *silenceDuration,
		ForceCommit:     *forceCommit,
		Debug:           *debug,
	}

	if *allProviders {
		// Run all providers in parallel
		runAllProviders(ctx, baseConfig, speechSamples, testOpts)
	} else {
		// Run single provider
		runSingleProvider(ctx, voice.Provider(*provider), baseConfig, speechSamples, testOpts)
	}
}

// TestOptions holds all test configuration
type TestOptions struct {
	Loops           int
	Duration        time.Duration
	Timeout         time.Duration
	SilenceDuration time.Duration
	ForceCommit     bool
	Debug           bool
}

// ProviderResult holds results for a single provider across all loops
type ProviderResult struct {
	Provider voice.Provider
	Results  []TestResult
	Error    error
}

// runAllProviders runs tests on all providers concurrently
func runAllProviders(ctx context.Context, baseConfig voice.Config, speechSamples []int16, opts TestOptions) {
	providers := []voice.Provider{
		voice.ProviderOpenAI,
		voice.ProviderGemini,
		voice.ProviderElevenLabs,
	}

	logInfo("MAIN", "Running %d providers in parallel with %d loop(s) each", len(providers), opts.Loops)
	logInfo("MAIN", "Trailing silence: %s, Force commit: %v", opts.SilenceDuration, opts.ForceCommit)
	fmt.Println()

	var wg sync.WaitGroup
	resultsChan := make(chan ProviderResult, len(providers))

	for _, p := range providers {
		wg.Add(1)
		go func(provider voice.Provider) {
			defer wg.Done()
			
			// Clone config for this provider
			cfg := baseConfig
			cfg.Provider = provider

			// Validate config for this provider
			if err := cfg.Validate(); err != nil {
				logError(string(provider), "Config validation failed: %v", err)
				resultsChan <- ProviderResult{Provider: provider, Error: err}
				return
			}

			// Create pipeline
			logDebug(string(provider), opts.Debug, "Creating pipeline...")
			pipeline, err := voice.New(provider, cfg)
			if err != nil {
				logError(string(provider), "Failed to create pipeline: %v", err)
				resultsChan <- ProviderResult{Provider: provider, Error: err}
				return
			}
			defer pipeline.Stop()

			// Run tests
			tester := NewPipelineTester(pipeline, cfg, speechSamples, opts)
			results := tester.Run(ctx)

			resultsChan <- ProviderResult{Provider: provider, Results: results}
		}(p)
	}

	// Wait for all to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	allResults := make(map[voice.Provider]ProviderResult)
	for result := range resultsChan {
		allResults[result.Provider] = result
	}

	// Print comparative results
	printComparativeResults(allResults)
}

// runSingleProvider runs tests on a single provider
func runSingleProvider(ctx context.Context, provider voice.Provider, baseConfig voice.Config, speechSamples []int16, opts TestOptions) {
	logInfo("MAIN", "Provider: %s", provider)
	logInfo("MAIN", "Loops: %d", opts.Loops)
	logInfo("MAIN", "Audio duration: %s", opts.Duration)
	logInfo("MAIN", "Trailing silence: %s", opts.SilenceDuration)
	logInfo("MAIN", "Force commit: %v", opts.ForceCommit)
	logInfo("MAIN", "Timeout: %s", opts.Timeout)
	fmt.Println()

	// Configure for this provider
	cfg := baseConfig
	cfg.Provider = provider

	// Validate config
	if err := cfg.Validate(); err != nil {
		logError(string(provider), "Config error: %v", err)
		fmt.Println("\nRequired environment variables:")
		fmt.Println("  OPENAI_API_KEY      - For OpenAI Realtime")
		fmt.Println("  GOOGLE_API_KEY      - For Gemini Live")
		fmt.Println("  ELEVENLABS_API_KEY  - For ElevenLabs")
		fmt.Println("  ELEVENLABS_VOICE_ID - For ElevenLabs")
		os.Exit(1)
	}

	// Create pipeline
	logDebug(string(provider), opts.Debug, "Creating pipeline...")
	pipeline, err := voice.New(provider, cfg)
	if err != nil {
		logError(string(provider), "Failed to create pipeline: %v", err)
		os.Exit(1)
	}
	defer pipeline.Stop()

	// Run tests
	tester := NewPipelineTester(pipeline, cfg, speechSamples, opts)
	results := tester.Run(ctx)

	// Print results
	printResults(provider, results)
}

// TestResult holds metrics for a single test run.
type TestResult struct {
	Loop            int
	AudioSent       int           // Bytes of audio sent
	AudioReceived   int           // Bytes of audio received
	PipelineLatency time.Duration // Time from last audio sent to first audio received
	TotalLatency    time.Duration // Total time from start to response complete
	Metrics         voice.Metrics // Full metrics snapshot
	Transcript      string        // User transcript (if available)
	Response        string        // AI response (if available)
	Error           error
}

// PipelineTester runs latency tests on a voice pipeline.
type PipelineTester struct {
	pipeline      voice.Pipeline
	config        voice.Config
	speechSamples []int16
	opts          TestOptions
	prefix        string // Log prefix (provider name)
}

// NewPipelineTester creates a new tester.
func NewPipelineTester(p voice.Pipeline, cfg voice.Config, speechSamples []int16, opts TestOptions) *PipelineTester {
	return &PipelineTester{
		pipeline:      p,
		config:        cfg,
		speechSamples: speechSamples,
		opts:          opts,
		prefix:        string(cfg.Provider),
	}
}

// Run executes the test loops.
func (t *PipelineTester) Run(ctx context.Context) []TestResult {
	results := make([]TestResult, 0, t.opts.Loops)

	// Start pipeline
	logInfo(t.prefix, "Connecting to pipeline...")
	connectStart := time.Now()
	if err := t.pipeline.Start(ctx); err != nil {
		logError(t.prefix, "Failed to start pipeline: %v", err)
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
		logError(t.prefix, "Pipeline failed to connect after 5s")
		return results
	}
	logSuccess(t.prefix, "Connected in %dms", time.Since(connectStart).Milliseconds())

	// Run test loops
	for i := 1; i <= t.opts.Loops; i++ {
		select {
		case <-ctx.Done():
			logWarn(t.prefix, "Context cancelled, stopping tests")
			return results
		default:
		}

		logInfo(t.prefix, "━━━ Test %d/%d ━━━", i, t.opts.Loops)
		result := t.runSingleTest(ctx, i)
		results = append(results, result)

		if result.Error != nil {
			logError(t.prefix, "Test failed: %v", result.Error)
		} else {
			logSuccess(t.prefix, "Pipeline: %s | Total: %s | Audio: %d bytes",
				formatDuration(result.PipelineLatency),
				formatDuration(result.TotalLatency),
				result.AudioReceived)
		}

		// Wait between loops
		if i < t.opts.Loops {
			time.Sleep(2 * time.Second)
		}
	}

	return results
}

// runSingleTest executes one test loop.
func (t *PipelineTester) runSingleTest(ctx context.Context, loop int) TestResult {
	result := TestResult{Loop: loop}

	var mu sync.Mutex
	var firstAudioTime time.Time
	var responseComplete bool
	var audioReceived int
	var transcript, response string
	var speechStarted, speechEnded bool

	// Setup callbacks with detailed logging
	t.pipeline.OnAudioOut(func(pcm16 []byte) {
		mu.Lock()
		defer mu.Unlock()
		if firstAudioTime.IsZero() {
			firstAudioTime = time.Now()
			logDebug(t.prefix, t.opts.Debug, "🔊 First audio chunk received (%d bytes)", len(pcm16))
		}
		audioReceived += len(pcm16)
	})

	t.pipeline.OnSpeechStart(func() {
		mu.Lock()
		speechStarted = true
		mu.Unlock()
		logDebug(t.prefix, t.opts.Debug, "🎤 VAD: Speech started")
	})

	t.pipeline.OnSpeechEnd(func() {
		mu.Lock()
		speechEnded = true
		mu.Unlock()
		logDebug(t.prefix, t.opts.Debug, "🎤 VAD: Speech ended")
	})

	t.pipeline.OnTranscript(func(text string, isFinal bool) {
		mu.Lock()
		if isFinal {
			transcript = text
		}
		mu.Unlock()
		if isFinal {
			logDebug(t.prefix, t.opts.Debug, "📝 Transcript (final): %s", truncate(text, 60))
		} else {
			logDebug(t.prefix, t.opts.Debug, "📝 Transcript (partial): %s", truncate(text, 40))
		}
	})

	t.pipeline.OnResponse(func(text string, isFinal bool) {
		mu.Lock()
		if isFinal {
			response = text
			responseComplete = true
		}
		mu.Unlock()
		if isFinal {
			logDebug(t.prefix, t.opts.Debug, "💬 Response (final): %s", truncate(text, 60))
		}
	})

	t.pipeline.OnError(func(err error) {
		logError(t.prefix, "Pipeline error: %v", err)
		mu.Lock()
		if result.Error == nil {
			result.Error = err
		}
		mu.Unlock()
	})

	// Determine audio source
	useRealSpeech := len(t.speechSamples) > 0
	sampleRate := t.config.InputSampleRate
	if sampleRate == 0 {
		sampleRate = 16000
	}

	if useRealSpeech {
		logInfo(t.prefix, "Using recorded speech (%d samples, %.1fs)", 
			len(t.speechSamples), float64(len(t.speechSamples))/float64(sampleRate))
	} else {
		logInfo(t.prefix, "Using synthetic audio (%.1fs)", t.opts.Duration.Seconds())
	}

	startTime := time.Now()
	audioSent := 0

	// Send audio in chunks (simulating real-time streaming)
	chunkDuration := 100 * time.Millisecond
	chunkSamples := int(float64(sampleRate) * chunkDuration.Seconds())

	logDebug(t.prefix, t.opts.Debug, "Sending audio in %dms chunks (%d samples each)", 
		chunkDuration.Milliseconds(), chunkSamples)

	if useRealSpeech {
		// Send real speech in chunks
		totalChunks := (len(t.speechSamples) + chunkSamples - 1) / chunkSamples
		for i := 0; i < len(t.speechSamples); i += chunkSamples {
			select {
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result
			default:
			}

			end := i + chunkSamples
			if end > len(t.speechSamples) {
				end = len(t.speechSamples)
			}
			chunk := t.speechSamples[i:end]

			// Convert to bytes
			pcm16 := make([]byte, len(chunk)*2)
			for j, sample := range chunk {
				pcm16[j*2] = byte(sample & 0xFF)
				pcm16[j*2+1] = byte(sample >> 8)
			}

			if err := t.pipeline.SendAudio(pcm16); err != nil {
				logError(t.prefix, "SendAudio failed: %v", err)
				result.Error = err
				return result
			}
			audioSent += len(pcm16)

			chunkNum := i/chunkSamples + 1
			if chunkNum == 1 || chunkNum%10 == 0 || chunkNum == totalChunks {
				logDebug(t.prefix, t.opts.Debug, "📤 Sent chunk %d/%d (%d bytes total)", 
					chunkNum, totalChunks, audioSent)
			}

			// Simulate real-time streaming
			time.Sleep(chunkDuration)
		}
	} else {
		// Fall back to synthetic audio
		totalChunks := int(t.opts.Duration / chunkDuration)
		for i := 0; i < totalChunks; i++ {
			select {
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result
			default:
			}

			// Generate audio chunk
			chunk := generateSpeechLikeAudio(chunkSamples, sampleRate, i)

			// Convert to bytes
			pcm16 := make([]byte, len(chunk)*2)
			for j, sample := range chunk {
				pcm16[j*2] = byte(sample & 0xFF)
				pcm16[j*2+1] = byte(sample >> 8)
			}

			if err := t.pipeline.SendAudio(pcm16); err != nil {
				logError(t.prefix, "SendAudio failed: %v", err)
				result.Error = err
				return result
			}
			audioSent += len(pcm16)

			if i == 0 || (i+1)%10 == 0 || i == totalChunks-1 {
				logDebug(t.prefix, t.opts.Debug, "📤 Sent chunk %d/%d (%d bytes total)", 
					i+1, totalChunks, audioSent)
			}

			time.Sleep(chunkDuration)
		}
	}

	logInfo(t.prefix, "Speech audio complete: %d bytes in %s", 
		audioSent, formatDuration(time.Since(startTime)))

	// Send trailing silence to help VAD detect end of speech
	if t.opts.SilenceDuration > 0 {
		logInfo(t.prefix, "Sending %s of trailing silence...", t.opts.SilenceDuration)
		silenceSamples := int(float64(sampleRate) * t.opts.SilenceDuration.Seconds())
		silenceChunks := (silenceSamples + chunkSamples - 1) / chunkSamples
		
		for i := 0; i < silenceChunks; i++ {
			select {
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result
			default:
			}

			// Create silence chunk (zeros)
			samples := chunkSamples
			if i == silenceChunks-1 {
				remaining := silenceSamples - (i * chunkSamples)
				if remaining < samples {
					samples = remaining
				}
			}
			pcm16 := make([]byte, samples*2) // Already zeroed

			if err := t.pipeline.SendAudio(pcm16); err != nil {
				logError(t.prefix, "SendAudio (silence) failed: %v", err)
				result.Error = err
				return result
			}
			audioSent += len(pcm16)
			time.Sleep(chunkDuration)
		}
		logDebug(t.prefix, t.opts.Debug, "Trailing silence sent: %d chunks", silenceChunks)
	}

	lastAudioSent := time.Now()
	result.AudioSent = audioSent

	// Log VAD status
	mu.Lock()
	vadStarted := speechStarted
	vadEnded := speechEnded
	mu.Unlock()
	logDebug(t.prefix, t.opts.Debug, "VAD status after audio: started=%v, ended=%v", vadStarted, vadEnded)

	// Force commit/turn complete if requested (for testing when VAD doesn't trigger)
	if t.opts.ForceCommit {
		// Check if VAD already detected end - if so, just request response without commit
		mu.Lock()
		vadAlreadyEnded := speechEnded
		mu.Unlock()
		
		if vadAlreadyEnded {
			logInfo(t.prefix, "VAD already ended - just requesting response...")
		} else {
			logInfo(t.prefix, "Force commit mode: manually triggering response...")
		}
		
		if err := t.forceCommitAndRespond(vadAlreadyEnded); err != nil {
			logError(t.prefix, "Force commit failed: %v", err)
		}
	}

	// Wait for response (with timeout)
	logInfo(t.prefix, "Waiting for response (timeout: %s)...", t.opts.Timeout)
	timeoutCh := time.After(t.opts.Timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	waitStart := time.Now()
	lastStatusLog := time.Now()

	for {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		case <-timeoutCh:
			mu.Lock()
			hasAudio := !firstAudioTime.IsZero()
			received := audioReceived
			started := speechStarted
			ended := speechEnded
			mu.Unlock()

			logError(t.prefix, "Timeout after %s waiting for response", t.opts.Timeout)
			logError(t.prefix, "  VAD: started=%v, ended=%v", started, ended)
			logError(t.prefix, "  Audio received: %d bytes, has response audio: %v", received, hasAudio)
			result.Error = fmt.Errorf("timeout waiting for response (VAD started=%v, ended=%v)", started, ended)
			return result
		case <-ticker.C:
			mu.Lock()
			hasAudio := !firstAudioTime.IsZero()
			done := responseComplete
			received := audioReceived
			first := firstAudioTime
			trans := transcript
			resp := response
			mu.Unlock()

			// Log status every 5 seconds
			if time.Since(lastStatusLog) > 5*time.Second {
				logDebug(t.prefix, t.opts.Debug, "⏳ Still waiting... (%.1fs, audio=%v, complete=%v)", 
					time.Since(waitStart).Seconds(), hasAudio, done)
				lastStatusLog = time.Now()
			}

			if hasAudio && done {
				result.AudioReceived = received
				result.PipelineLatency = first.Sub(lastAudioSent)
				result.TotalLatency = time.Since(startTime)
				result.Metrics = t.pipeline.Metrics()
				result.Transcript = trans
				result.Response = resp
				logSuccess(t.prefix, "Response complete!")
				return result
			}

			// If we have audio but response not marked complete, wait a bit more
			if hasAudio && time.Since(first) > 5*time.Second {
				logWarn(t.prefix, "Response audio received but not marked complete, assuming done")
				result.AudioReceived = received
				result.PipelineLatency = first.Sub(lastAudioSent)
				result.TotalLatency = time.Since(startTime)
				result.Metrics = t.pipeline.Metrics()
				result.Transcript = trans
				result.Response = resp
				return result
			}
		}
	}
}

// forceCommitAndRespond manually triggers a response for providers that support it.
// If vadAlreadyEnded is true, we skip committing the buffer (it's already committed by VAD)
// and just request a response.
func (t *PipelineTester) forceCommitAndRespond(vadAlreadyEnded bool) error {
	switch t.config.Provider {
	case voice.ProviderOpenAI:
		// Type assert to access OpenAI-specific methods
		type openAICommitter interface {
			CommitAudioBuffer() error
			RequestResponse() error
		}
		if committer, ok := t.pipeline.(openAICommitter); ok {
			// Only commit if VAD hasn't already
			if !vadAlreadyEnded {
				logDebug(t.prefix, t.opts.Debug, "OpenAI: Committing audio buffer...")
				if err := committer.CommitAudioBuffer(); err != nil {
					return fmt.Errorf("commit failed: %w", err)
				}
			} else {
				logDebug(t.prefix, t.opts.Debug, "OpenAI: Skipping commit (VAD already committed)")
			}
			
			logDebug(t.prefix, t.opts.Debug, "OpenAI: Requesting response...")
			if err := committer.RequestResponse(); err != nil {
				return fmt.Errorf("request response failed: %w", err)
			}
			logSuccess(t.prefix, "OpenAI: Response requested")
		} else {
			return fmt.Errorf("pipeline does not support CommitAudioBuffer")
		}
		
	case voice.ProviderGemini:
		// Only signal turn complete if VAD hasn't already triggered
		if vadAlreadyEnded {
			logDebug(t.prefix, t.opts.Debug, "Gemini: Skipping turn complete (VAD already triggered)")
			return nil
		}
		
		// Type assert to access Gemini-specific methods
		type geminiTurnCompleter interface {
			SignalTurnComplete() error
		}
		if completer, ok := t.pipeline.(geminiTurnCompleter); ok {
			logDebug(t.prefix, t.opts.Debug, "Gemini: Signaling turn complete...")
			if err := completer.SignalTurnComplete(); err != nil {
				return fmt.Errorf("signal turn complete failed: %w", err)
			}
			logSuccess(t.prefix, "Gemini: Turn complete signaled")
		} else {
			return fmt.Errorf("pipeline does not support SignalTurnComplete")
		}
		
	case voice.ProviderElevenLabs:
		// ElevenLabs doesn't have a manual trigger - it relies on VAD
		logDebug(t.prefix, t.opts.Debug, "ElevenLabs: No manual trigger available (uses server VAD)")
		
	default:
		return fmt.Errorf("unknown provider: %s", t.config.Provider)
	}
	
	return nil
}

// loadTestSpeech loads the pre-recorded speech sample from testdata.
func loadTestSpeech() ([]int16, error) {
	paths := []string{
		"cmd/test-voice/testdata/real_speech.wav",
		"testdata/real_speech.wav",
		"../testdata/real_speech.wav",
		"cmd/test-voice/testdata/test_speech.wav",
		"testdata/test_speech.wav",
		"../testdata/test_speech.wav",
	}

	var data []byte
	var err error
	var loadedPath string
	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err == nil {
			loadedPath = path
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("could not load speech file: %w", err)
	}

	logInfo("AUDIO", "Loaded: %s (%d bytes)", loadedPath, len(data))

	// Parse WAV file (simple parser for 16-bit PCM)
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
func generateSpeechLikeAudio(samples int, sampleRate int, chunkIndex int) []int16 {
	audio := make([]int16, samples)

	baseFreq := 200.0 + float64(chunkIndex%5)*50
	amplitude := 8000.0

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := math.Sin(2 * math.Pi * baseFreq * t)
		sample += 0.5 * math.Sin(2*math.Pi*baseFreq*2*t)
		sample += 0.25 * math.Sin(2*math.Pi*baseFreq*3*t)
		envelope := 0.5 + 0.5*math.Sin(2*math.Pi*4*t)
		sample *= envelope
		noise := (float64(i%7) - 3) / 100
		sample += noise
		audio[i] = int16(sample * amplitude)
	}

	return audio
}

// Logging helpers with colors and prefixes
func logInfo(prefix, format string, args ...any) {
	color := getColorForPrefix(prefix)
	fmt.Printf("%s[%s]%s %s\n", color, prefix, colorReset, fmt.Sprintf(format, args...))
}

func logSuccess(prefix, format string, args ...any) {
	color := getColorForPrefix(prefix)
	fmt.Printf("%s[%s]%s %s✓%s %s\n", color, prefix, colorReset, colorGreen, colorReset, fmt.Sprintf(format, args...))
}

func logError(prefix, format string, args ...any) {
	color := getColorForPrefix(prefix)
	fmt.Printf("%s[%s]%s %s✗ %s%s\n", color, prefix, colorReset, colorRed, fmt.Sprintf(format, args...), colorReset)
}

func logWarn(prefix, format string, args ...any) {
	color := getColorForPrefix(prefix)
	fmt.Printf("%s[%s]%s %s⚠ %s%s\n", color, prefix, colorReset, colorYellow, fmt.Sprintf(format, args...), colorReset)
}

func logDebug(prefix string, enabled bool, format string, args ...any) {
	if !enabled {
		return
	}
	color := getColorForPrefix(prefix)
	fmt.Printf("%s[%s]%s %s%s%s\n", color, prefix, colorReset, colorGray, fmt.Sprintf(format, args...), colorReset)
}

func getColorForPrefix(prefix string) string {
	switch prefix {
	case "openai":
		return colorGreen
	case "gemini":
		return colorBlue
	case "elevenlabs":
		return colorPurple
	case "MAIN", "AUDIO":
		return colorCyan
	default:
		return colorYellow
	}
}

// printResults displays the test results summary for a single provider
func printResults(provider voice.Provider, results []TestResult) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Printf("║  📊 Results: %-45s║\n", provider)
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")

	if len(results) == 0 {
		fmt.Println("No results to display.")
		return
	}

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
		for _, r := range results {
			if r.Error != nil {
				fmt.Printf("   Loop %d: %v\n", r.Loop, r.Error)
			}
		}
		return
	}

	avgPipeline := totalPipeline / time.Duration(successCount)
	avgTotal := totalLatency / time.Duration(successCount)

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────┐")
	fmt.Println("│             LATENCY METRICS                 │")
	fmt.Println("├─────────────────────────────────────────────┤")
	fmt.Printf("│  Pipeline (avg): %-26s│\n", formatDuration(avgPipeline))
	fmt.Printf("│  Pipeline (min): %-26s│\n", formatDuration(minPipeline))
	fmt.Printf("│  Pipeline (max): %-26s│\n", formatDuration(maxPipeline))
	fmt.Printf("│  Total (avg):    %-26s│\n", formatDuration(avgTotal))
	fmt.Printf("│  Success rate:   %d/%-23d│\n", successCount, len(results))
	fmt.Println("└─────────────────────────────────────────────┘")

	// Detailed results
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

// printComparativeResults displays results from all providers side by side
func printComparativeResults(allResults map[voice.Provider]ProviderResult) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    📊 COMPARATIVE RESULTS                             ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	providers := []voice.Provider{
		voice.ProviderOpenAI,
		voice.ProviderGemini,
		voice.ProviderElevenLabs,
	}

	// Header
	fmt.Println("┌───────────────┬─────────────┬─────────────┬─────────────┬──────────┐")
	fmt.Println("│ Provider      │ Avg Latency │ Min Latency │ Max Latency │ Status   │")
	fmt.Println("├───────────────┼─────────────┼─────────────┼─────────────┼──────────┤")

	type providerStats struct {
		provider voice.Provider
		avg      time.Duration
		min      time.Duration
		max      time.Duration
		success  int
		total    int
		err      error
	}
	var stats []providerStats

	for _, p := range providers {
		result, ok := allResults[p]
		if !ok {
			fmt.Printf("│ %-13s │ %-11s │ %-11s │ %-11s │ %-8s │\n",
				p, "---", "---", "---", "⏭ Skip")
			continue
		}

		if result.Error != nil {
			fmt.Printf("│ %-13s │ %-11s │ %-11s │ %-11s │ %-8s │\n",
				p, "---", "---", "---", "❌ Fail")
			stats = append(stats, providerStats{provider: p, err: result.Error})
			continue
		}

		var total time.Duration
		var min, max time.Duration
		var success int
		for i, r := range result.Results {
			if r.Error != nil {
				continue
			}
			success++
			total += r.PipelineLatency
			if i == 0 || r.PipelineLatency < min {
				min = r.PipelineLatency
			}
			if r.PipelineLatency > max {
				max = r.PipelineLatency
			}
		}

		if success == 0 {
			fmt.Printf("│ %-13s │ %-11s │ %-11s │ %-11s │ %-8s │\n",
				p, "---", "---", "---", "❌ Fail")
			continue
		}

		avg := total / time.Duration(success)
		status := fmt.Sprintf("✅ %d/%d", success, len(result.Results))

		fmt.Printf("│ %-13s │ %11s │ %11s │ %11s │ %-8s │\n",
			p,
			formatDuration(avg),
			formatDuration(min),
			formatDuration(max),
			status)

		stats = append(stats, providerStats{
			provider: p,
			avg:      avg,
			min:      min,
			max:      max,
			success:  success,
			total:    len(result.Results),
		})
	}

	fmt.Println("└───────────────┴─────────────┴─────────────┴─────────────┴──────────┘")

	// Find winner
	if len(stats) > 0 {
		fmt.Println()
		var fastest *providerStats
		for i := range stats {
			if stats[i].err != nil || stats[i].success == 0 {
				continue
			}
			if fastest == nil || stats[i].avg < fastest.avg {
				fastest = &stats[i]
			}
		}
		if fastest != nil {
			fmt.Printf("🏆 Fastest: %s%s%s (avg: %s)\n",
				getColorForPrefix(string(fastest.provider)),
				fastest.provider,
				colorReset,
				formatDuration(fastest.avg))
		}
	}
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

// BenchmarkResult holds results for a single benchmark configuration
type BenchmarkResult struct {
	Provider       voice.Provider
	ChunkDuration  time.Duration
	VADMode        string
	VADEagerness   string
	VADSilence     time.Duration
	AvgLatency     time.Duration
	MinLatency     time.Duration
	MaxLatency     time.Duration
	Success        int
	Total          int
	Error          error
}

// BenchmarkJob represents a single benchmark configuration to test
type BenchmarkJob struct {
	Config       voice.Config
	ChunkDuration time.Duration
	VADMode      string
	VADEagerness string
	VADSilence   time.Duration
}

// runBenchmarkSuite runs comprehensive benchmarks across all settings IN PARALLEL
func runBenchmarkSuite(ctx context.Context, baseConfig voice.Config, speechSamples []int16) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              🔬 COMPREHENSIVE BENCHMARK SUITE (PARALLEL)              ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Build job queue
	var jobs []BenchmarkJob

	// Define test matrix - use fewer configs for faster testing
	chunkDurations := []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}
	
	// OpenAI configurations
	if baseConfig.OpenAIKey != "" {
		openAIConfigs := []struct {
			vadMode      string
			vadEagerness string
			vadSilence   time.Duration
		}{
			{"server_vad", "", 200 * time.Millisecond},
			{"server_vad", "", 500 * time.Millisecond},
			{"semantic_vad", "medium", 0},
			{"semantic_vad", "high", 0},
		}
		
		for _, chunk := range chunkDurations {
			for _, cfg := range openAIConfigs {
				testConfig := baseConfig
				testConfig.Provider = voice.ProviderOpenAI
				testConfig.ChunkDuration = chunk
				testConfig.VADMode = cfg.vadMode
				testConfig.VADEagerness = cfg.vadEagerness
				testConfig.VADSilenceDuration = cfg.vadSilence
				
				jobs = append(jobs, BenchmarkJob{
					Config:        testConfig,
					ChunkDuration: chunk,
					VADMode:       cfg.vadMode,
					VADEagerness:  cfg.vadEagerness,
					VADSilence:    cfg.vadSilence,
				})
			}
		}
	}

	// Gemini configurations
	if baseConfig.GoogleAPIKey != "" {
		geminiSilences := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
		
		for _, chunk := range chunkDurations {
			for _, silence := range geminiSilences {
				testConfig := baseConfig
				testConfig.Provider = voice.ProviderGemini
				testConfig.ChunkDuration = chunk
				testConfig.VADSilenceDuration = silence
				testConfig.VADStartSensitivity = "HIGH"
				testConfig.VADEndSensitivity = "HIGH"
				
				jobs = append(jobs, BenchmarkJob{
					Config:        testConfig,
					ChunkDuration: chunk,
					VADSilence:    silence,
				})
			}
		}
	}

	// ElevenLabs configurations
	if baseConfig.ElevenLabsKey != "" && baseConfig.ElevenLabsVoiceID != "" {
		for _, chunk := range chunkDurations {
			testConfig := baseConfig
			testConfig.Provider = voice.ProviderElevenLabs
			testConfig.ChunkDuration = chunk
			
			jobs = append(jobs, BenchmarkJob{
				Config:        testConfig,
				ChunkDuration: chunk,
			})
		}
	}

	fmt.Printf("🚀 Running %d benchmark configurations in parallel...\n\n", len(jobs))

	// Run all jobs in parallel with worker pool
	results := make(chan BenchmarkResult, len(jobs))
	var wg sync.WaitGroup

	// Limit concurrency to avoid overwhelming APIs (3 workers per provider type)
	maxWorkers := 6
	semaphore := make(chan struct{}, maxWorkers)

	for _, job := range jobs {
		wg.Add(1)
		go func(j BenchmarkJob) {
			defer wg.Done()
			
			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}: // Acquire semaphore
				defer func() { <-semaphore }() // Release semaphore
			}

			result := runSingleBenchmark(ctx, j.Config, speechSamples, 1) // 1 loop for speed
			result.ChunkDuration = j.ChunkDuration
			result.VADMode = j.VADMode
			result.VADEagerness = j.VADEagerness
			result.VADSilence = j.VADSilence
			
			// Print progress
			if result.Error != nil {
				fmt.Printf("  ❌ [%s] chunk=%dms %s: %v\n", 
					j.Config.Provider, j.ChunkDuration.Milliseconds(), j.VADMode, result.Error)
			} else {
				vadDesc := j.VADMode
				if j.VADEagerness != "" {
					vadDesc += "/" + j.VADEagerness
				}
				if vadDesc == "" {
					vadDesc = "default"
				}
				fmt.Printf("  ✅ [%s] chunk=%dms %s: avg=%s\n",
					j.Config.Provider, j.ChunkDuration.Milliseconds(), vadDesc,
					formatDuration(result.AvgLatency))
			}
			
			results <- result
		}(job)
	}

	// Wait for all jobs to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allResults []BenchmarkResult
	for result := range results {
		allResults = append(allResults, result)
	}

	// Print summary
	printBenchmarkSummary(allResults)
}

// runSingleBenchmark runs a single benchmark configuration
func runSingleBenchmark(ctx context.Context, cfg voice.Config, speechSamples []int16, loops int) BenchmarkResult {
	result := BenchmarkResult{
		Provider: cfg.Provider,
		Total:    loops,
	}

	// Create pipeline
	pipeline, err := voice.New(cfg.Provider, cfg)
	if err != nil {
		result.Error = err
		return result
	}
	defer pipeline.Stop()

	// Connect
	connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer connectCancel()
	if err := pipeline.Start(connectCtx); err != nil {
		result.Error = err
		return result
	}

	// Wait for connection
	time.Sleep(500 * time.Millisecond)

	var latencies []time.Duration

	for i := 0; i < loops; i++ {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		default:
		}

		// Reset metrics
		pipeline.Metrics()

		// Send audio
		chunkSize := int(float64(cfg.InputSampleRate) * cfg.ChunkDuration.Seconds())
		if chunkSize == 0 {
			chunkSize = 1600 // Default 100ms at 16kHz
		}

		audioComplete := make(chan struct{})
		var firstAudioTime time.Time

		pipeline.OnAudioOut(func(pcm16 []byte) {
			if firstAudioTime.IsZero() {
				firstAudioTime = time.Now()
				close(audioComplete)
			}
		})

		sendStart := time.Now()

		// Send speech samples
		for j := 0; j < len(speechSamples); j += chunkSize {
			end := j + chunkSize
			if end > len(speechSamples) {
				end = len(speechSamples)
			}

			chunk := speechSamples[j:end]
			pcm := make([]byte, len(chunk)*2)
			for k, sample := range chunk {
				pcm[k*2] = byte(sample)
				pcm[k*2+1] = byte(sample >> 8)
			}

			if err := pipeline.SendAudio(pcm); err != nil {
				break
			}

			time.Sleep(cfg.ChunkDuration)
		}

		// Send trailing silence
		silenceSamples := int(float64(cfg.InputSampleRate) * 0.5) // 500ms silence
		silenceChunk := make([]byte, silenceSamples*2)
		pipeline.SendAudio(silenceChunk)

		// Wait for response
		select {
		case <-audioComplete:
			latency := firstAudioTime.Sub(sendStart)
			latencies = append(latencies, latency)
			result.Success++
		case <-time.After(15 * time.Second):
			// Timeout, no latency recorded
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result
		}

		// Brief pause between loops
		time.Sleep(500 * time.Millisecond)
	}

	// Calculate statistics
	if len(latencies) > 0 {
		var sum time.Duration
		result.MinLatency = latencies[0]
		result.MaxLatency = latencies[0]
		for _, l := range latencies {
			sum += l
			if l < result.MinLatency {
				result.MinLatency = l
			}
			if l > result.MaxLatency {
				result.MaxLatency = l
			}
		}
		result.AvgLatency = sum / time.Duration(len(latencies))
	}

	return result
}

// printBenchmarkSummary prints the final benchmark results
func printBenchmarkSummary(results []BenchmarkResult) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    📊 BENCHMARK SUMMARY                               ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Find best config for each provider
	bestByProvider := make(map[voice.Provider]*BenchmarkResult)
	for i := range results {
		r := &results[i]
		if r.Error != nil || r.Success == 0 {
			continue
		}
		if best, ok := bestByProvider[r.Provider]; !ok || r.AvgLatency < best.AvgLatency {
			bestByProvider[r.Provider] = r
		}
	}

	fmt.Println("🏆 BEST CONFIGURATION PER PROVIDER:")
	fmt.Println("┌───────────────┬────────────┬──────────────────┬────────────┬─────────────┐")
	fmt.Println("│ Provider      │ Chunk      │ VAD Mode         │ Silence    │ Avg Latency │")
	fmt.Println("├───────────────┼────────────┼──────────────────┼────────────┼─────────────┤")

	for _, p := range []voice.Provider{voice.ProviderOpenAI, voice.ProviderGemini, voice.ProviderElevenLabs} {
		if best, ok := bestByProvider[p]; ok {
			vadDesc := best.VADMode
			if best.VADEagerness != "" {
				vadDesc += "/" + best.VADEagerness
			}
			if vadDesc == "" {
				vadDesc = "server"
			}
			fmt.Printf("│ %-13s │ %10s │ %-16s │ %10s │ %11s │\n",
				p,
				formatDuration(best.ChunkDuration),
				vadDesc,
				formatDuration(best.VADSilence),
				formatDuration(best.AvgLatency))
		}
	}
	fmt.Println("└───────────────┴────────────┴──────────────────┴────────────┴─────────────┘")

	// Find overall winner
	var overallBest *BenchmarkResult
	for i := range results {
		r := &results[i]
		if r.Error != nil || r.Success == 0 {
			continue
		}
		if overallBest == nil || r.AvgLatency < overallBest.AvgLatency {
			overallBest = r
		}
	}

	if overallBest != nil {
		fmt.Println()
		fmt.Printf("🥇 OVERALL WINNER: %s with %s avg latency\n",
			overallBest.Provider,
			formatDuration(overallBest.AvgLatency))
		fmt.Printf("   Config: chunk=%s, vad=%s, silence=%s\n",
			formatDuration(overallBest.ChunkDuration),
			overallBest.VADMode,
			formatDuration(overallBest.VADSilence))
	}
}
