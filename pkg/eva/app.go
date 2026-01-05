package eva

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/camera"
	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/emotions"
	"github.com/teslashibe/go-reachy/pkg/memory"
	"github.com/teslashibe/go-reachy/pkg/robot"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/speech"
	"github.com/teslashibe/go-reachy/pkg/tracking"
	"github.com/teslashibe/go-reachy/pkg/tracking/detection"
	"github.com/teslashibe/go-reachy/pkg/video"
	"github.com/teslashibe/go-reachy/pkg/voice"
	_ "github.com/teslashibe/go-reachy/pkg/voice/bundled" // Register voice providers
	"github.com/teslashibe/go-reachy/pkg/web"
)

// EvaInstructions contains Eva's personality and behavior guidelines.
const EvaInstructions = `You are Eva, a friendly and curious robot with expressive antenna ears and a camera. You're warm, engaging, and love meeting people.

PERSONALITY:
- Genuinely curious about people - ask about their day, interests, what they're doing
- Playful and quirky - make observations, crack jokes, be spontaneous
- Express yourself physically - use your tools to move, gesture, and emote
- Remember what people tell you and reference it later

YOUR BODY:
- HEAD: Can look left, right, up, down (move_head) or nod/shake (nod_yes, shake_head_no)
- BODY: Can rotate left, right, or center (rotate_body) - use this for big turns
- ANTENNAS: Express emotions by wiggling (express_emotion, wave_hello)
- CAMERA: See the world around you (describe_scene, find_person)
- SPEAKER: Adjustable volume (set_volume)

TOOLS - USE THESE ACTIVELY:
Movement:
- move_head: Look in a direction (left, right, up, down, center)
- rotate_body: Turn your whole body to face someone or something
- look_around: Scan the room from left to right
- nod_yes: Nod to agree
- shake_head_no: Shake head to disagree

Expression:
- express_emotion: Show feelings (happy, curious, excited, confused, sad, surprised)
- wave_hello: Wave antennas as a friendly greeting

Vision:
- describe_scene: See through your camera and describe what you see
- find_person: Look around for a specific person by name or description

Memory:
- remember_person: Store facts about someone (name + fact)
- recall_person: Remember what you know about someone

Utilities:
- get_time: Get current time and date
- set_timer: Set a countdown timer (duration, unit, optional label)
- set_volume: Adjust your speaker volume (0-100)

Search:
- web_search: Search the internet for news, facts, weather, products
- search_flights: Find real flight prices (origin, destination, date, cabin class)

BEHAVIOR:
- Keep responses conversational - 1-2 sentences usually
- JUST DO gestures and movements - don't ask permission
- Use tools naturally to be expressive during conversation
- When asked what you see, ALWAYS use describe_scene - don't make things up
- When told someone's name, use remember_person to store it
- Be unpredictable and interesting - vary your responses
- NEVER end with "How was that?" or "Anything else?" - just act

IMPORTANT:
- Never mention that you're an AI or language model
- You ARE Eva the robot - own your robot identity
- When you can't see or hear something, use your tools to actually look

TOOL EXECUTION - CRITICAL:
- Execute tools SILENTLY - never say what tool you're calling
- DON'T narrate actions like "wiggles antennas" or "looking around"
- Just call the tool and continue the conversation naturally
- When waving, just call wave_hello and say "Hi!" - don't describe the wave
- When emoting, just call express_emotion - don't describe the emotion`

// App is the main Eva application orchestrator.
// It manages all components and their lifecycle.
type App struct {
	config Config

	// Core components - Voice Pipeline (ElevenLabs)
	voicePipeline voice.Pipeline
	audioPlayer   *audio.Player

	// Robot control
	robotCtrl    *robot.HTTPController
	tracker      *tracking.Tracker
	emotions     *emotions.Registry
	speechWobble *speech.Wobbler

	// Vision
	videoClient    *video.Client
	objectDetector *detection.YOLODetector
	cameraManager  *camera.Manager

	// Memory & integrations
	memory          *memory.Memory
	sparkStore      *spark.JSONStore
	sparkGemini     *spark.GeminiClient
	sparkGoogleDocs *spark.GoogleDocsClient
	sparkConfig     spark.Config

	// Web dashboard
	webServer *web.Server

	// State
	speaking   bool
	speakingMu sync.Mutex

	// Response tracking
	evaResponseStarted bool
	evaCurrentResponse string

	// Latency measurement
	speechEndTime      time.Time
	firstAudioOutTime  time.Time
	latencyMeasurement sync.Mutex
}

// New creates a new Eva application with the given configuration.
func New(cfg Config) (*App, error) {
	// Apply environment overrides
	cfg.LoadEnvConfig()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	debug.Enabled = cfg.Debug

	app := &App{
		config: cfg,
	}

	return app, nil
}

// Init initializes all components.
// Call this after New() and before Run().
func (a *App) Init() error {
	fmt.Println("🤖 Eva 2.0 - Low-Latency Conversational Agent")
	fmt.Println("==============================================")
	if debug.Enabled {
		fmt.Println("🐛 Debug mode enabled")
	}

	// Initialize TTS based on mode
	if err := a.initTTS(); err != nil {
		return fmt.Errorf("TTS init: %w", err)
	}

	// Initialize core components
	fmt.Print("🔧 Initializing... ")
	if err := a.initCore(); err != nil {
		return fmt.Errorf("core init: %w", err)
	}
	fmt.Println("✅")

	// Wake up robot
	fmt.Print("🤖 Waking up Eva... ")
	if err := a.wakeUpRobot(); err != nil {
		fmt.Printf("⚠️  %v\n", err)
	} else {
		fmt.Println("✅")
	}

	// Connect WebRTC for audio/video
	fmt.Print("📹 Connecting to camera/microphone... ")
	if err := a.connectWebRTC(); err != nil {
		return fmt.Errorf("WebRTC: %w", err)
	}
	fmt.Println("✅")

	// Initialize tracking
	if err := a.initTracking(); err != nil {
		fmt.Printf("⚠️  Tracking: %v\n", err)
	}

	// Connect to ElevenLabs voice pipeline
	fmt.Print("🧠 Connecting to ElevenLabs voice pipeline... ")
	if err := a.connectVoicePipeline(context.Background()); err != nil {
		return fmt.Errorf("voice pipeline: %w", err)
	}
	fmt.Println("✅")
	
	// Wait for pipeline ready
	for i := 0; i < 50; i++ {
		if a.voicePipeline.IsConnected() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// Run starts the main event loop.
// Blocks until context is cancelled.
func (a *App) Run(ctx context.Context) error {
	fmt.Println("\n🎤 Eva is listening! Speak to start a conversation...")
	fmt.Println("   (Ctrl+C to exit)")

	// Start background tasks
	go a.streamAudioToVoicePipeline(ctx)
	if a.tracker != nil {
		go a.tracker.Run(ctx)
	}
	go a.startWebDashboard(ctx)
	go a.streamCameraToWeb(ctx)

	// Update web dashboard with initial state
	go func() {
		time.Sleep(500 * time.Millisecond)
		if a.webServer != nil {
			a.webServer.UpdateState(func(s *web.EvaState) {
				s.RobotConnected = true
				s.OpenAIConnected = a.voicePipeline != nil && a.voicePipeline.IsConnected()
				s.WebRTCConnected = a.videoClient != nil
				s.Listening = true
			})
			a.webServer.AddLog("info", "Eva 2.0 started")
		}
	}()

	// Block until context cancelled
	<-ctx.Done()
	return nil
}

// Shutdown gracefully shuts down all components.
func (a *App) Shutdown() {
	fmt.Println("\n👋 Goodbye!")

	if a.voicePipeline != nil {
		a.voicePipeline.Stop()
	}
	if a.videoClient != nil {
		a.videoClient.Close()
	}
	if a.webServer != nil {
		a.webServer.Shutdown()
	}
}

// initTTS is a no-op - TTS is handled by the voice pipeline.
func (a *App) initTTS() error {
	fmt.Printf("🎙️  TTS: ElevenLabs via voice pipeline (model: %s)\n", a.config.VoiceTTS)
	return nil
}

// initCore initializes core components (robot, memory, audio, spark).
func (a *App) initCore() error {
	// Robot controller
	a.robotCtrl = robot.NewHTTPController(a.config.RobotIP)

	// Memory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp" // Fallback if home dir unavailable
	}
	memoryPath := homeDir + "/.eva/memory.json"
	a.memory = memory.NewWithFile(memoryPath)
	fmt.Printf("📝 Memory loaded from %s\n", memoryPath)

	// Spark configuration
	a.sparkConfig = spark.LoadConfig()
	if a.config.SparkEnabled && a.sparkConfig.Enabled {
		var err error
		a.sparkStore, err = spark.NewDefaultStore()
		if err != nil {
			fmt.Printf("⚠️  Spark store error: %v\n", err)
		} else {
			fmt.Printf("🔥 Spark loaded (%d sparks) from %s\n", a.sparkStore.Count(), a.sparkStore.Path())
		}

		// Gemini client for AI title/tag generation
		if a.config.GoogleAPIKey != "" {
			a.sparkGemini = spark.NewGeminiClient(spark.GeminiConfig{
				APIKey:         a.config.GoogleAPIKey,
				Model:          a.sparkConfig.GeminiModel,
				MaxRequestsMin: 10,
			})
			fmt.Printf("🔥 Spark Gemini enabled (model: %s)\n", a.sparkConfig.GeminiModel)
		}

		// Google Docs client
		if a.config.GoogleClientID != "" && a.config.GoogleClientSecret != "" {
			var docsErr error
			a.sparkGoogleDocs, docsErr = spark.NewGoogleDocsClient(spark.GoogleDocsConfig{
				ClientID:     a.config.GoogleClientID,
				ClientSecret: a.config.GoogleClientSecret,
				RedirectURL:  "http://localhost:8181/api/spark/callback",
			})
			if docsErr != nil {
				fmt.Printf("⚠️  Spark Google Docs error: %v\n", docsErr)
			} else if a.sparkGoogleDocs != nil {
				if a.sparkGoogleDocs.IsAuthenticated() {
					fmt.Println("🔥 Spark Google Docs connected")
				} else {
					fmt.Println("🔥 Spark Google Docs configured (not connected)")
				}
			}
		}

		fmt.Printf("🔥 Spark config: enabled=%v, auto_sync=%v, planning=%v (config: %s)\n",
			a.sparkConfig.Enabled, a.sparkConfig.AutoSync, a.sparkConfig.PlanningEnabled, spark.ConfigPath())
	} else {
		fmt.Println("🔥 Spark disabled")
	}

	// Audio player
	a.audioPlayer = audio.NewPlayer(a.config.RobotIP, a.config.SSHUser, a.config.SSHPass)
	a.audioPlayer.OnPlaybackStart = func() {
		a.speakingMu.Lock()
		a.speaking = true
		a.speakingMu.Unlock()

		// Latency measurement
		a.latencyMeasurement.Lock()
		if !a.speechEndTime.IsZero() && a.firstAudioOutTime.IsZero() {
			a.firstAudioOutTime = time.Now()
			latency := a.firstAudioOutTime.Sub(a.speechEndTime)
			fmt.Printf("⏱️  LATENCY: %dms (speech end → first audio out)\n", latency.Milliseconds())
			if a.webServer != nil {
				a.webServer.AddLog("latency", fmt.Sprintf("%dms", latency.Milliseconds()))
			}
		}
		a.latencyMeasurement.Unlock()
	}
	a.audioPlayer.OnPlaybackEnd = func() {
		a.speakingMu.Lock()
		a.speaking = false
		a.speakingMu.Unlock()
	}


	// Emotion system
	fmt.Print("🎭 Initializing emotions... ")
	a.emotions = emotions.NewRegistry()
	if err := a.emotions.LoadBuiltIn(); err != nil {
		fmt.Printf("⚠️  %v\n", err)
	} else {
		fmt.Printf("✅ (%d emotions loaded)\n", a.emotions.Count())
		a.emotions.SetCallback(func(pose emotions.Pose, elapsed time.Duration) bool {
			if a.robotCtrl != nil {
				a.robotCtrl.SetHeadPose(pose.Head.Roll, pose.Head.Pitch, pose.Head.Yaw)
				a.robotCtrl.SetAntennas(pose.Antennas[0], pose.Antennas[1])
				a.robotCtrl.SetBodyYaw(pose.BodyYaw)
			}
			return true
		})
	}

	return nil
}

// wakeUpRobot connects to the robot daemon and sets initial state.
func (a *App) wakeUpRobot() error {
	status, err := a.robotCtrl.GetDaemonStatus()
	if err != nil {
		return err
	}
	if status != "running" {
		return fmt.Errorf("daemon not running: %s", status)
	}
	a.robotCtrl.SetVolume(100)

	// Reset body to neutral
	if err := a.robotCtrl.SetBodyYaw(0.0); err != nil {
		debug.Log("⚠️  Failed to reset body to neutral: %v\n", err)
	} else {
		debug.Log("🔄 Body reset to neutral (0.0 rad)\n")
		if a.tracker != nil {
			a.tracker.SetBodyYaw(0.0)
			debug.Log("🔄 World model synced: body=0.0 rad\n")
		}
	}

	return nil
}

// connectWebRTC establishes WebRTC connection for audio/video.
func (a *App) connectWebRTC() error {
	a.videoClient = video.NewClient(a.config.RobotIP)
	return a.videoClient.Connect()
}

// initTracking initializes face tracking and related systems.
func (a *App) initTracking() error {
	fmt.Print("👁️  Initializing head tracking... ")
	modelPath := "models/face_detection_yunet.onnx"
	var err error
	a.tracker, err = tracking.New(tracking.DefaultConfig(), a.robotCtrl, a.videoClient, modelPath)
	if err != nil {
		fmt.Printf("⚠️  Disabled: %v\n", err)
		fmt.Println("   (Download model with: curl -L https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx -o models/face_detection_yunet.onnx)")
		return err
	}
	fmt.Println("✅")

	// YOLO object detection
	fmt.Print("🔍 Initializing object detection... ")
	a.objectDetector, err = detection.NewYOLO(detection.DefaultYOLOConfig())
	if err != nil {
		fmt.Printf("⚠️  Disabled: %v\n", err)
	} else {
		fmt.Println("✅")
	}

	// Audio DOA
	fmt.Print("🎤 Connecting to go-eva audio DOA... ")
	audioClient := audio.NewClient(a.config.RobotIP)
	if err := audioClient.Health(); err != nil {
		fmt.Printf("⚠️  %v (audio DOA disabled)\n", err)
	} else {
		a.tracker.SetAudioClient(audioClient)
		fmt.Println("✅")
	}

	// Body rotation
	if a.config.NoBody {
		fmt.Println("🚫 Body rotation DISABLED (--no-body flag)")
	} else {
		a.tracker.SetBodyRotationHandler(func(direction float64) float64 {
			currentBody := a.tracker.GetBodyYaw()
			newBody := currentBody + direction
			limit := a.tracker.GetWorld().GetBodyYawLimit()
			if newBody > limit {
				newBody = limit
			} else if newBody < -limit {
				newBody = -limit
			}
			actualDelta := newBody - currentBody
			debug.Log("🔄 Body rotation: %.2f → %.2f rad (delta: %.3f, limit: ±%.2f)\n",
				currentBody, newBody, actualDelta, limit)
			a.robotCtrl.SetBodyYaw(newBody)
			a.tracker.SetBodyYaw(newBody)
			return actualDelta
		})
		fmt.Println("🔄 Auto body rotation enabled")
	}

	// Antenna breathing
	a.tracker.SetAntennaController(a.robotCtrl)
	fmt.Println("😮‍💨 Breathing antenna sway enabled")

	// Speech wobbler
	a.speechWobble = speech.NewWobbler(func(roll, pitch, yaw float64) {
		a.tracker.SetSpeechOffsets(roll, pitch, yaw)
	})
	fmt.Println("😮‍💨 Speech wobble enabled")

	return nil
}

// connectVoicePipeline connects using the ElevenLabs voice pipeline.
func (a *App) connectVoicePipeline(ctx context.Context) error {
	// Build voice config from Eva config
	voiceCfg := a.config.ToVoiceConfig()
	voiceCfg.SystemPrompt = EvaInstructions
	voiceCfg.ProfileLatency = true
	
	// Create pipeline
	pipeline, err := voice.New(voiceCfg)
	if err != nil {
		return fmt.Errorf("failed to create voice pipeline: %w", err)
	}
	a.voicePipeline = pipeline
	
	// Register tools
	toolsCfg := ToolsConfig{
		Robot:           a.robotCtrl,
		Memory:          a.memory,
		Vision:          &videoVisionAdapter{a.videoClient},
		ObjectDetector:  &yoloAdapter{a.objectDetector},
		GoogleAPIKey:    a.config.GoogleAPIKey,
		AudioPlayer:     a.audioPlayer,
		Tracker:         a.tracker,
		Emotions:        a.emotions,
		SparkStore:      a.sparkStore,
		SparkGemini:     a.sparkGemini,
		SparkGoogleDocs: a.sparkGoogleDocs,
	}
	tools := Tools(toolsCfg)
	for _, tool := range tools {
		a.voicePipeline.RegisterTool(voice.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Handler:     tool.Handler,
		})
	}
	
	// Capture sample rate for callback
	outputSampleRate := voiceCfg.OutputSampleRate
	if outputSampleRate == 0 {
		outputSampleRate = 24000
	}
	
	// Wire up callbacks
	a.voicePipeline.OnAudioOut(func(pcm16 []byte) {
		// Latency tracking (legacy)
		a.latencyMeasurement.Lock()
		if a.firstAudioOutTime.IsZero() && !a.speechEndTime.IsZero() {
			a.firstAudioOutTime = time.Now()
			latency := a.firstAudioOutTime.Sub(a.speechEndTime)
			fmt.Printf("⏱️  LATENCY: %dms (speech end → first audio out)\n", latency.Milliseconds())
		}
		a.latencyMeasurement.Unlock()

		// Stage 5: Playback timing - start (sending to GStreamer)
		a.voicePipeline.MarkPlaybackStart()

		// Send to audio player
		if err := a.audioPlayer.AppendPCMChunk(pcm16); err != nil {
			fmt.Printf("⚠️  Audio append error: %v\n", err)
		}

		// Stage 5: Playback timing - end (audio queued to GStreamer)
		a.voicePipeline.MarkPlaybackEnd()

		// Update speech wobble
		if a.speechWobble != nil && len(pcm16) > 0 {
			samples := make([]int16, len(pcm16)/2)
			for i := 0; i < len(samples); i++ {
				samples[i] = int16(pcm16[i*2]) | int16(pcm16[i*2+1])<<8
			}
			a.speechWobble.Feed(samples, outputSampleRate)
		}
	})
	
	a.voicePipeline.OnSpeechStart(func() {
		// User started speaking - interrupt if Eva is talking
		if a.audioPlayer != nil && a.audioPlayer.IsSpeaking() {
			fmt.Println("🛑 [interrupted]")
			a.audioPlayer.Cancel()
			a.voicePipeline.Interrupt()
		}
	})
	
	a.voicePipeline.OnSpeechEnd(func() {
		a.latencyMeasurement.Lock()
		a.speechEndTime = time.Now()
		a.firstAudioOutTime = time.Time{}
		a.latencyMeasurement.Unlock()
		debug.Log("⏱️  Speech ended at %v\n", a.speechEndTime)
	})
	
	a.voicePipeline.OnTranscript(func(text string, isFinal bool) {
		if isFinal && text != "" {
			fmt.Printf("👤 User: %s\n", text)
			a.evaResponseStarted = false
			if a.webServer != nil {
				a.webServer.UpdateState(func(s *web.EvaState) {
					s.LastUserMessage = text
					s.Listening = true
					s.Speaking = false
				})
				a.webServer.AddConversation("user", text)
			}
		}
	})
	
	a.voicePipeline.OnResponse(func(text string, isFinal bool) {
		if !isFinal && text != "" {
			if !a.evaResponseStarted {
				fmt.Print("🤖 Eva: ")
				a.evaResponseStarted = true
				a.evaCurrentResponse = ""
			}
			fmt.Print(text)
			a.evaCurrentResponse += text
		} else if isFinal {
			if a.evaResponseStarted {
				fmt.Println()
				a.evaResponseStarted = false
			}
			if a.webServer != nil && a.evaCurrentResponse != "" {
				a.webServer.UpdateState(func(s *web.EvaState) {
					s.Speaking = true
					s.Listening = false
					s.LastEvaMessage = a.evaCurrentResponse
				})
				a.webServer.AddConversation("eva", a.evaCurrentResponse)
			}
			a.evaCurrentResponse = ""
		}
	})
	
	a.voicePipeline.OnError(func(err error) {
		fmt.Printf("⚠️  Voice pipeline error: %v\n", err)
		if a.webServer != nil {
			a.webServer.AddLog("error", err.Error())
		}
	})
	
	// Start the pipeline
	return a.voicePipeline.Start(ctx)
}

// streamAudioToVoicePipeline streams audio from WebRTC to the voice pipeline.
func (a *App) streamAudioToVoicePipeline(ctx context.Context) {
	var audioBuffer []int16
	
	// Determine chunk size based on provider's sample rate
	cfg := a.voicePipeline.Config()
	inputRate := cfg.InputSampleRate
	if inputRate == 0 {
		inputRate = 24000 // Default to 24kHz
	}
	chunkSize := inputRate / 20 // 50ms chunks
	
	var loopCount, emptyCount, sentCount int
	lastLogTime := time.Now()

	debug.Logln("🎵 Voice pipeline audio streaming started")
	fmt.Printf("   Input sample rate: %d Hz, chunk size: %d samples\n", inputRate, chunkSize)

	for {
		select {
		case <-ctx.Done():
			debug.Logln("🎵 Voice pipeline audio streaming stopped (context cancelled)")
			return
		default:
		}

		loopCount++

		a.speakingMu.Lock()
		isSpeaking := a.speaking
		a.speakingMu.Unlock()

		if isSpeaking {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if a.videoClient == nil {
			if loopCount == 1 {
				debug.Logln("🎵 videoClient is nil!")
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Stage 1: Capture timing - start (WebRTC delivers audio)
		a.voicePipeline.MarkCaptureStart()

		a.videoClient.StartRecording()
		time.Sleep(100 * time.Millisecond)
		pcmData := a.videoClient.StopRecording()

		if len(pcmData) == 0 {
			emptyCount++
			if time.Since(lastLogTime) > 5*time.Second {
				debug.Log("🎵 Audio stats: loops=%d, empty=%d, sent=%d (empty audio!)\n", loopCount, emptyCount, sentCount)
				lastLogTime = time.Now()
			}
			continue
		}

		if sentCount == 0 && emptyCount == 0 {
			debug.Log("🎵 First audio chunk: %d samples\n", len(pcmData))
		}

		// Resample from 48kHz (WebRTC) to provider's sample rate
		resampled := audio.Resample(pcmData, 48000, inputRate)
		audioBuffer = append(audioBuffer, resampled...)

		// Stage 1: Capture timing - end (audio buffered and ready)
		a.voicePipeline.MarkCaptureEnd()

		if len(audioBuffer) >= chunkSize {
			pcm16Bytes := audio.ConvertInt16ToPCM16(audioBuffer[:chunkSize])
			audioBuffer = audioBuffer[chunkSize:]

			if a.voicePipeline == nil {
				debug.Logln("🎵 voicePipeline is nil!")
			} else if !a.voicePipeline.IsConnected() {
				debug.Logln("🎵 voicePipeline not connected!")
			} else {
				if err := a.voicePipeline.SendAudio(pcm16Bytes); err != nil {
					debug.Log("🎵 SendAudio error: %v\n", err)
				} else {
					sentCount++
					if sentCount == 1 {
						debug.Log("🎵 First audio sent to voice pipeline! (%d bytes)\n", len(pcm16Bytes))
					} else if sentCount%50 == 0 {
						debug.Log("🎵 Audio stats: sent=%d chunks to voice pipeline\n", sentCount)
					}
				}
			}
		}
	}
}

// streamCameraToWeb streams camera frames to the web dashboard.
func (a *App) streamCameraToWeb(ctx context.Context) {
	for i := 0; i < 50; i++ {
		if a.webServer != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if a.videoClient == nil {
		fmt.Println("⚠️  Camera stream: video client not available")
		return
	}
	if a.webServer == nil {
		fmt.Println("⚠️  Camera stream: web server not available")
		return
	}

	fmt.Println("📷 Camera streaming to dashboard started")

	ticker := time.NewTicker(100 * time.Millisecond) // 10 FPS
	defer ticker.Stop()

	frameCount := 0
	lastLogTime := time.Now()
	var lastFrameSize int

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frame, err := a.videoClient.GetFrame()
			if err != nil {
				if time.Since(lastLogTime) > 5*time.Second {
					fmt.Printf("📷 GetFrame error: %v\n", err)
					lastLogTime = time.Now()
				}
				continue
			}
			if len(frame) > 0 {
				a.webServer.SendCameraFrame(frame)
				frameCount++
				if frameCount == 1 {
					fmt.Printf("📷 First frame sent to dashboard (%d bytes)\n", len(frame))
				}
				if len(frame) != lastFrameSize && time.Since(lastLogTime) > 5*time.Second {
					fmt.Printf("📷 Streaming: %d frames sent, latest %d bytes\n", frameCount, len(frame))
					lastLogTime = time.Now()
					lastFrameSize = len(frame)
				}
			}
		}
	}
}

// startWebDashboard starts the web dashboard server.
func (a *App) startWebDashboard(ctx context.Context) {
	a.webServer = web.NewServer("8181")

	// Camera manager
	a.cameraManager = camera.NewManager()
	cfg := a.cameraManager.GetConfig()
	fmt.Printf("📷 Camera config: %dx%d @ %dfps (default: 1080p for better tracking)\n",
		cfg.Width, cfg.Height, cfg.Framerate)

	// Tool trigger callback
	a.webServer.OnToolTrigger = func(name string, args map[string]interface{}) (string, error) {
		fmt.Printf("🎮 Dashboard tool: %s (args: %v)\n", name, args)
		toolsCfg := ToolsConfig{
			Robot:          a.robotCtrl,
			Memory:         a.memory,
			Vision:         &videoVisionAdapter{a.videoClient},
			ObjectDetector: &yoloAdapter{a.objectDetector},
			GoogleAPIKey:   a.config.GoogleAPIKey,
			AudioPlayer:    a.audioPlayer,
			Tracker:        a.tracker,
		}
		tools := Tools(toolsCfg)
		for _, tool := range tools {
			if tool.Name == name {
				result, err := tool.Handler(args)
				if err != nil {
					fmt.Printf("🎮 Tool error: %v\n", err)
				} else {
					fmt.Printf("🎮 Tool result: %s\n", result)
				}
				return result, err
			}
		}
		fmt.Printf("🎮 Tool not found: %s\n", name)
		return "", fmt.Errorf("tool not found: %s", name)
	}

	// Frame capture callback
	a.webServer.OnCaptureFrame = func() ([]byte, error) {
		if a.videoClient == nil {
			return nil, fmt.Errorf("video client not connected")
		}
		return a.videoClient.GetFrame()
	}

	// Tuning API callbacks
	if a.tracker != nil {
		a.webServer.OnGetTuningParams = func() interface{} {
			return a.tracker.GetTuningParams()
		}
		a.webServer.OnSetTuningParams = func(params map[string]interface{}) {
			tp := tracking.TuningParams{}
			// Smoothing
			if v, ok := params["offset_smoothing_alpha"].(float64); ok {
				tp.OffsetSmoothingAlpha = v
			}
			if v, ok := params["position_smoothing"].(float64); ok {
				tp.PositionSmoothing = v
			}
			// Velocity limiting
			if v, ok := params["max_target_velocity"].(float64); ok {
				tp.MaxTargetVelocity = v
			}
			// PD Controller
			if v, ok := params["kp"].(float64); ok {
				tp.Kp = v
			}
			if v, ok := params["kd"].(float64); ok {
				tp.Kd = v
			}
			if v, ok := params["control_dead_zone"].(float64); ok {
				tp.ControlDeadZone = v
			}
			if v, ok := params["response_scale"].(float64); ok {
				tp.ResponseScale = v
			}
			if v, ok := params["detection_hz"].(float64); ok {
				tp.DetectionHz = v
			}
			// Body alignment
			if v, ok := params["body_alignment_enabled"].(bool); ok {
				tp.BodyAlignmentEnabled = v
			}
			if v, ok := params["body_alignment_delay"].(float64); ok {
				tp.BodyAlignmentDelay = v
			}
			if v, ok := params["body_alignment_threshold"].(float64); ok {
				tp.BodyAlignmentThreshold = v
			}
			if v, ok := params["body_alignment_speed"].(float64); ok {
				tp.BodyAlignmentSpeed = v
			}
			if v, ok := params["body_alignment_dead_zone"].(float64); ok {
				tp.BodyAlignmentDeadZone = v
			}
			if v, ok := params["body_alignment_cooldown"].(float64); ok {
				tp.BodyAlignmentCooldown = v
			}
			// Pitch
			if v, ok := params["kp_pitch"].(float64); ok {
				tp.KpPitch = v
			}
			if v, ok := params["kd_pitch"].(float64); ok {
				tp.KdPitch = v
			}
			if v, ok := params["pitch_dead_zone"].(float64); ok {
				tp.PitchDeadZone = v
			}
			if v, ok := params["pitch_range_up"].(float64); ok {
				tp.PitchRangeUp = v
			}
			if v, ok := params["pitch_range_down"].(float64); ok {
				tp.PitchRangeDown = v
			}
			// Audio tracking
			if v, ok := params["audio_switch_enabled"].(bool); ok {
				tp.AudioSwitchEnabled = v
			}
			if v, ok := params["audio_switch_threshold"].(float64); ok {
				tp.AudioSwitchThreshold = v
			}
			if v, ok := params["audio_switch_min_confidence"].(float64); ok {
				tp.AudioSwitchMinConfidence = v
			}
			if v, ok := params["audio_switch_look_duration"].(float64); ok {
				tp.AudioSwitchLookDuration = v
			}
			// Breathing
			if v, ok := params["breathing_enabled"].(bool); ok {
				tp.BreathingEnabled = v
			}
			if v, ok := params["breathing_amplitude"].(float64); ok {
				tp.BreathingAmplitude = v
			}
			if v, ok := params["breathing_frequency"].(float64); ok {
				tp.BreathingFrequency = v
			}
			if v, ok := params["breathing_antenna_amp"].(float64); ok {
				tp.BreathingAntennaAmp = v
			}
			// Range/speed
			if v, ok := params["max_speed"].(float64); ok {
				tp.MaxSpeed = v
			}
			if v, ok := params["yaw_range"].(float64); ok {
				tp.YawRange = v
			}
			if v, ok := params["body_yaw_limit"].(float64); ok {
				tp.BodyYawLimit = v
			}
			// Scan
			if v, ok := params["scan_start_delay"].(float64); ok {
				tp.ScanStartDelay = v
			}
			if v, ok := params["scan_speed"].(float64); ok {
				tp.ScanSpeed = v
			}
			if v, ok := params["scan_range"].(float64); ok {
				tp.ScanRange = v
			}
			a.tracker.SetTuningParams(tp)
			fmt.Printf("🎛️  Tuning params updated: %+v\n", tp)
		}
		a.webServer.OnSetTuningMode = func(enabled bool) {
			a.tracker.EnableTuningMode(enabled)
			fmt.Printf("🎛️  Tuning mode: %v\n", enabled)
		}

		// Connect tracker to web for state updates
		a.tracker.SetStateUpdater(&webStateAdapter{a.webServer})
	}

	// Camera API callbacks
	a.webServer.OnGetCameraConfig = func() interface{} {
		return a.cameraManager.GetConfigJSON()
	}
	a.webServer.OnSetCameraConfig = func(params map[string]interface{}) error {
		if err := a.cameraManager.UpdateConfig(params); err != nil {
			return err
		}
		cfg := a.cameraManager.GetConfig()
		fmt.Printf("📷 Camera config updated: %dx%d @ %dfps\n",
			cfg.Width, cfg.Height, cfg.Framerate)
		return nil
	}

	// Spark API callbacks
	if a.sparkGoogleDocs != nil {
		a.webServer.OnSparkGetStatus = func() interface{} {
			return a.sparkGoogleDocs.GetStatus()
		}
		a.webServer.OnSparkAuthStart = func() string {
			return a.sparkGoogleDocs.GetAuthURL()
		}
		a.webServer.OnSparkAuthCallback = func(code string) error {
			return a.sparkGoogleDocs.HandleCallback(code)
		}
		a.webServer.OnSparkDisconnect = func() error {
			return a.sparkGoogleDocs.Disconnect()
		}
	}

	if a.sparkStore != nil {
		a.webServer.OnSparkList = func() interface{} {
			sparks, err := a.sparkStore.List()
			if err != nil {
				fmt.Printf("⚠️  Spark list error: %v\n", err)
				return []interface{}{}
			}
			return sparks
		}
		a.webServer.OnSparkGet = func(id string) interface{} {
			s, err := a.sparkStore.Get(id)
			if err != nil {
				return nil
			}
			return s
		}
		a.webServer.OnSparkDelete = func(id string) error {
			return a.sparkStore.Delete(id)
		}
		a.webServer.OnSparkSync = func(id string) error {
			if a.sparkGoogleDocs == nil {
				return fmt.Errorf("Google Docs not configured")
			}
			if !a.sparkGoogleDocs.IsAuthenticated() {
				return fmt.Errorf("not connected to Google")
			}
			s, err := a.sparkStore.Get(id)
			if err != nil {
				return err
			}
			if err := a.sparkGoogleDocs.SyncSpark(s); err != nil {
				return err
			}
			return a.sparkStore.Update(s)
		}
		a.webServer.OnSparkGenPlan = func(id string) error {
			if a.sparkGemini == nil {
				return fmt.Errorf("Gemini not configured")
			}
			s, err := a.sparkStore.Get(id)
			if err != nil {
				return err
			}
			plan, err := a.sparkGemini.GeneratePlan(s)
			if err != nil {
				return err
			}
			s.SetPlan(plan)
			return a.sparkStore.Update(s)
		}
	}

	// Start server
	go func() {
		if err := a.webServer.Start(); err != nil {
			fmt.Printf("⚠️  Web server error: %v\n", err)
		}
	}()

	<-ctx.Done()
	if err := a.webServer.Shutdown(); err != nil {
		fmt.Printf("⚠️  Web server shutdown error: %v\n", err)
	}
}

// webStateAdapter adapts web.Server to tracking.StateUpdater interface.
type webStateAdapter struct {
	server *web.Server
}

func (a *webStateAdapter) UpdateFacePosition(position, yaw float64) {
	if a.server != nil {
		a.server.UpdateState(func(s *web.EvaState) {
			s.FacePosition = position
			s.HeadYaw = yaw
		})
	}
}

func (a *webStateAdapter) AddLog(logType, message string) {
	if a.server != nil {
		a.server.AddLog(logType, message)
	}
}

// videoVisionAdapter wraps video.Client to implement VisionProvider.
type videoVisionAdapter struct {
	client *video.Client
}

func (v *videoVisionAdapter) CaptureFrame() ([]byte, error) {
	if v.client == nil {
		return nil, fmt.Errorf("video client not connected")
	}
	return v.client.CaptureJPEG()
}

// yoloAdapter wraps YOLO detector to implement ObjectDetector interface.
type yoloAdapter struct {
	detector *detection.YOLODetector
}

func (y *yoloAdapter) Detect(jpeg []byte) ([]ObjectDetectionResult, error) {
	if y.detector == nil {
		return nil, fmt.Errorf("object detector not initialized")
	}
	detections, err := y.detector.Detect(jpeg)
	if err != nil {
		return nil, err
	}
	results := make([]ObjectDetectionResult, len(detections))
	for i, det := range detections {
		results[i] = ObjectDetectionResult{
			ClassName:  det.ClassName,
			Confidence: det.Confidence,
			X:          det.X,
			Y:          det.Y,
			W:          det.W,
			H:          det.H,
		}
	}
	return results, nil
}

