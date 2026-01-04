// Eva 2.0 - Low-latency conversational robot agent with tool use
// Uses OpenAI Realtime API for speech-to-speech conversation
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/camera"
	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/emotions"
	"github.com/teslashibe/go-reachy/pkg/eva"
	"github.com/teslashibe/go-reachy/pkg/memory"
	"github.com/teslashibe/go-reachy/pkg/openai"
	"github.com/teslashibe/go-reachy/pkg/robot"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/speech"
	"github.com/teslashibe/go-reachy/pkg/tracking"
	"github.com/teslashibe/go-reachy/pkg/tracking/detection"
	"github.com/teslashibe/go-reachy/pkg/tts"
	"github.com/teslashibe/go-reachy/pkg/video"
	"github.com/teslashibe/go-reachy/pkg/web"
)

const (
	defaultRobotIP = "192.168.68.77"
	sshUser        = "pollen"
	sshPass        = "root"
)

var robotIP = defaultRobotIP

func init() {
	if ip := os.Getenv("ROBOT_IP"); ip != "" {
		robotIP = ip
	}
}

// Eva's personality and instructions
const evaInstructions = `You are Eva, a friendly and curious robot with expressive antenna ears and a camera. You're warm, engaging, and love meeting people.

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
- When you can't see or hear something, use your tools to actually look`

var (
	realtimeClient   *openai.Client
	videoClient      *video.Client
	audioPlayer      *audio.Player
	robotCtrl        *robot.HTTPController
	memoryStore      *memory.Memory
	sparkStore       *spark.JSONStore
	sparkGemini      *spark.GeminiClient
	webServer        *web.Server
	headTracker      *tracking.Tracker
	ttsProvider      tts.Provider      // HTTP TTS provider
	ttsStreaming     *tts.ElevenLabsWS // WebSocket streaming TTS
	objectDetector   *detection.YOLODetector
	speechWobbler    *speech.Wobbler        // Speech-synced head movement
	cameraManager    *camera.Manager        // Camera configuration manager
	emotionRegistry  *emotions.Registry     // Pre-recorded emotion animations

	speaking   bool
	speakingMu sync.Mutex

	// TTS mode: "realtime", "elevenlabs", "elevenlabs-streaming", or "openai-tts"
	ttsMode string

	// Track if we've started printing Eva's response
	evaResponseStarted bool
	evaCurrentResponse string
)

// webStateAdapter adapts web.Server to tracking.StateUpdater interface
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

func main() {
	// Parse flags
	debugFlag := flag.Bool("debug", false, "Enable verbose debug logging")
	robotIPFlag := flag.String("robot-ip", "", "Robot IP address (overrides ROBOT_IP env var)")
	ttsFlag := flag.String("tts", "realtime", "TTS provider: realtime, elevenlabs, elevenlabs-streaming (lowest latency), openai-tts")
	ttsVoice := flag.String("tts-voice", "", "Voice ID for ElevenLabs (required if --tts=elevenlabs)")
	flag.Parse()
	debug.Enabled = *debugFlag
	if *robotIPFlag != "" {
		robotIP = *robotIPFlag
	}

	fmt.Println("🤖 Eva 2.0 - Low-Latency Conversational Agent")
	fmt.Println("==============================================")
	if debug.Enabled {
		fmt.Println("🐛 Debug mode enabled")
	}

	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		fmt.Println("❌ Set OPENAI_API_KEY!")
		os.Exit(1)
	}

	// Set TTS mode
	ttsMode = *ttsFlag
	switch ttsMode {
	case "realtime":
		fmt.Println("🎙️  TTS: OpenAI Realtime (streaming audio)")
	case "elevenlabs":
		elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
		if elevenLabsKey == "" {
			fmt.Println("❌ Set ELEVENLABS_API_KEY for ElevenLabs TTS!")
			os.Exit(1)
		}
		voiceID := *ttsVoice
		if voiceID == "" {
			voiceID = os.Getenv("ELEVENLABS_VOICE_ID")
		}
		if voiceID == "" {
			voiceID = "charlotte" // Default
		}
		// Map named presets to voice IDs
		voicePresets := map[string]string{
			"charlotte": "XB0fDUnXU5powFXDhCwa", // British female, warm
			"aria":      "9BWtsMINqrJLrRacOk9x", // American female, expressive
			"sarah":     "EXAVITQu4vr4xnSDxMaL", // American female, soft
			"lily":      "pFZP5JQG7iQjIQuC4Bku", // British female, warm
			"rachel":    "21m00Tcm4TlvDq8ikWAM", // American female, calm
			"domi":      "AZnzlk1XvdvUeBnXmlld", // American female, strong
			"bella":     "EXAVITQu4vr4xnSDxMaL", // American female, soft
			"elli":      "MF3mGyEYCl7XYWbV9V6O", // American female, young
			"josh":      "TxGEqnHWrfWFTfGW9XjX", // American male, deep
			"adam":      "pNInz6obpgDQGcFmaJgB", // American male, deep
			"sam":       "yoZ06aMxZJJ28mfd3POQ", // American male, raspy
		}
		if preset, ok := voicePresets[voiceID]; ok {
			fmt.Printf("   Voice preset: %s\n", voiceID)
			voiceID = preset
		}
		var err error
		ttsProvider, err = tts.NewElevenLabs(
			tts.WithAPIKey(elevenLabsKey),
			tts.WithVoice(voiceID),
			tts.WithModel(tts.ModelFlashV2_5), // Fastest model (~150ms)
			tts.WithOutputFormat(tts.EncodingPCM24),
		)
		if err != nil {
			fmt.Printf("❌ ElevenLabs init failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("🎙️  TTS: ElevenLabs (voice: %s)\n", voiceID)
	case "elevenlabs-streaming":
		elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
		if elevenLabsKey == "" {
			fmt.Println("❌ Set ELEVENLABS_API_KEY for ElevenLabs TTS!")
			os.Exit(1)
		}
		voiceID := *ttsVoice
		if voiceID == "" {
			voiceID = os.Getenv("ELEVENLABS_VOICE_ID")
		}
		if voiceID == "" {
			voiceID = "charlotte"
		}
		// Map named presets to voice IDs
		voicePresets := map[string]string{
			"charlotte": "XB0fDUnXU5powFXDhCwa",
			"aria":      "9BWtsMINqrJLrRacOk9x",
			"sarah":     "EXAVITQu4vr4xnSDxMaL",
			"lily":      "pFZP5JQG7iQjIQuC4Bku",
			"rachel":    "21m00Tcm4TlvDq8ikWAM",
			"domi":      "AZnzlk1XvdvUeBnXmlld",
			"bella":     "EXAVITQu4vr4xnSDxMaL",
			"elli":      "MF3mGyEYCl7XYWbV9V6O",
			"josh":      "TxGEqnHWrfWFTfGW9XjX",
			"adam":      "pNInz6obpgDQGcFmaJgB",
			"sam":       "yoZ06aMxZJJ28mfd3POQ",
		}
		if preset, ok := voicePresets[voiceID]; ok {
			fmt.Printf("   Voice preset: %s\n", voiceID)
			voiceID = preset
		}
		var err error
		ttsStreaming, err = tts.NewElevenLabsWS(
			tts.WithAPIKey(elevenLabsKey),
			tts.WithVoice(voiceID),
			tts.WithModel(tts.ModelFlashV2_5),
			tts.WithOutputFormat(tts.EncodingPCM24),
		)
		if err != nil {
			fmt.Printf("❌ ElevenLabs streaming init failed: %v\n", err)
			os.Exit(1)
		}
		// Pre-warm WebSocket connection
		fmt.Printf("🎙️  TTS: ElevenLabs WebSocket Streaming (voice: %s)\n", voiceID)
		fmt.Println("   Pre-warming WebSocket connection...")
		if err := ttsStreaming.Connect(context.Background()); err != nil {
			fmt.Printf("⚠️  WebSocket pre-warm failed (will retry): %v\n", err)
		} else {
			fmt.Println("   WebSocket connected ✓")
		}
	case "openai-tts":
		var err error
		ttsProvider, err = tts.NewOpenAI(
			tts.WithAPIKey(openaiKey),
			tts.WithVoice("shimmer"),
			tts.WithOutputFormat(tts.EncodingPCM24),
		)
		if err != nil {
			fmt.Printf("❌ OpenAI TTS init failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("🎙️  TTS: OpenAI TTS API")
	default:
		fmt.Printf("❌ Unknown TTS provider: %s (use: realtime, elevenlabs, elevenlabs-streaming, openai-tts)\n", ttsMode)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n👋 Goodbye!")
		shutdown()
		cancel()
		os.Exit(0)
	}()

	// Initialize components
	fmt.Print("🔧 Initializing... ")
	if err := initialize(); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Start robot
	fmt.Print("🤖 Waking up Eva... ")
	if err := wakeUpRobot(); err != nil {
		fmt.Printf("⚠️  %v\n", err)
	} else {
		fmt.Println("✅")
	}

	// Connect to WebRTC for audio input
	fmt.Print("📹 Connecting to camera/microphone... ")
	if err := connectWebRTC(); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Initialize head tracking BEFORE connecting to realtime API (so tools can reference it)
	fmt.Print("👁️  Initializing head tracking... ")
	modelPath := "models/face_detection_yunet.onnx"
	var err error
	headTracker, err = tracking.New(tracking.DefaultConfig(), robotCtrl, videoClient, modelPath)
	if err != nil {
		fmt.Printf("⚠️  Disabled: %v\n", err)
		fmt.Println("   (Download model with: curl -L https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx -o models/face_detection_yunet.onnx)")
	} else {
		fmt.Println("✅")
	}

	// Initialize YOLO object detection
	fmt.Print("🔍 Initializing object detection... ")
	objectDetector, err = detection.NewYOLO(detection.DefaultYOLOConfig())
	if err != nil {
		fmt.Printf("⚠️  Disabled: %v\n", err)
	} else {
		fmt.Println("✅")
	}

	// Initialize emotion system (81 pre-recorded animations)
	fmt.Print("🎭 Initializing emotions... ")
	emotionRegistry = emotions.NewRegistry()
	if err := emotionRegistry.LoadBuiltIn(); err != nil {
		fmt.Printf("⚠️  %v\n", err)
	} else {
		fmt.Printf("✅ (%d emotions loaded)\n", emotionRegistry.Count())
		// Set up callback to control robot during emotion playback
		emotionRegistry.SetCallback(func(pose emotions.Pose, elapsed time.Duration) bool {
			if robotCtrl != nil {
				// Convert emotion pose to robot commands
				robotCtrl.SetHeadPose(pose.Head.Roll, pose.Head.Pitch, pose.Head.Yaw)
				robotCtrl.SetAntennas(pose.Antennas[0], pose.Antennas[1])
				robotCtrl.SetBodyYaw(pose.BodyYaw)
			}
			return true // Continue playback
		})
	}

	// Connect audio DOA from go-eva
	if headTracker != nil {
		fmt.Print("🎤 Connecting to go-eva audio DOA... ")
		audioClient := audio.NewClient(robotIP)
		if err := audioClient.Health(); err != nil {
			fmt.Printf("⚠️  %v (audio DOA disabled)\n", err)
		} else {
			headTracker.SetAudioClient(audioClient)
			fmt.Println("✅")
		}

		// Set up automatic body rotation when head reaches limits
		// Returns actual delta for head counter-rotation (Issue #79 fix)
		headTracker.SetBodyRotationHandler(func(direction float64) float64 {
			currentBody := headTracker.GetBodyYaw()
			newBody := currentBody + direction

			// Use world model's limit (matches Python reachy: 0.9*π ≈ ±162°)
			limit := headTracker.GetWorld().GetBodyYawLimit()
			if newBody > limit {
				newBody = limit
			} else if newBody < -limit {
				newBody = -limit
			}

			// Calculate actual delta after clamping
			actualDelta := newBody - currentBody

			debug.Log("🔄 Body rotation: %.2f → %.2f rad (delta: %.3f, limit: ±%.2f)\n",
				currentBody, newBody, actualDelta, limit)

			robotCtrl.SetBodyYaw(newBody)
			headTracker.SetBodyYaw(newBody) // Sync world model

			return actualDelta // Return actual movement for head counter-rotation
		})
		fmt.Println("🔄 Auto body rotation enabled")

		// Enable antenna breathing animation (matches Python reachy)
		headTracker.SetAntennaController(robotCtrl)
		fmt.Println("😮‍💨 Breathing antenna sway enabled")

		// Initialize speech wobbler for natural speaking gestures
		speechWobbler = speech.NewWobbler(func(roll, pitch, yaw float64) {
			headTracker.SetSpeechOffsets(roll, pitch, yaw)
		})
		fmt.Println("😮‍💨 Speech wobble enabled")
	}

	// Connect to OpenAI Realtime API
	fmt.Print("🧠 Connecting to OpenAI Realtime API... ")
	if err := connectRealtime(openaiKey); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Configure session
	fmt.Print("⚙️  Configuring Eva's personality... ")
	if err := realtimeClient.ConfigureSession(evaInstructions, "shimmer"); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Wait for session ready
	for i := 0; i < 50; i++ {
		if realtimeClient.IsReady() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\n🎤 Eva is listening! Speak to start a conversation...")
	fmt.Println("   (Ctrl+C to exit)")

	// Start audio streaming from WebRTC to Realtime API
	go streamAudioToRealtime(ctx)

	// Start head tracking loop
	if headTracker != nil {
		go headTracker.Run(ctx)
	}

	// Start web dashboard
	go startWebDashboard(ctx)

	// Start camera streaming to web
	go streamCameraToWeb(ctx)

	// Update web dashboard with initial connection state
	go func() {
		time.Sleep(500 * time.Millisecond) // Wait for web server to start
		if webServer != nil {
			webServer.UpdateState(func(s *web.EvaState) {
				s.RobotConnected = true
				s.OpenAIConnected = realtimeClient != nil && realtimeClient.IsConnected()
				s.WebRTCConnected = videoClient != nil
				s.Listening = true
			})
			webServer.AddLog("info", "Eva 2.0 started")
		}
	}()

	// Keep running
	<-ctx.Done()
}

func initialize() error {
	// Create robot controller
	robotCtrl = robot.NewHTTPController(robotIP)

	// Create persistent memory (saves to ~/.eva/memory.json)
	homeDir, _ := os.UserHomeDir()
	memoryPath := homeDir + "/.eva/memory.json"
	memoryStore = memory.NewWithFile(memoryPath)
	fmt.Printf("📝 Memory loaded from %s\n", memoryPath)

	// Create Spark store (idea collection)
	var sparkErr error
	sparkStore, sparkErr = spark.NewDefaultStore()
	if sparkErr != nil {
		fmt.Printf("⚠️  Spark store error: %v\n", sparkErr)
	} else {
		fmt.Printf("🔥 Spark loaded (%d sparks) from %s\n", sparkStore.Count(), sparkStore.Path())
	}

	// Create Spark Gemini client for AI title/tag generation
	if googleAPIKey := os.Getenv("GOOGLE_API_KEY"); googleAPIKey != "" {
		sparkGemini = spark.NewGeminiClient(spark.GeminiConfig{
			APIKey:         googleAPIKey,
			MaxRequestsMin: 10,
		})
		fmt.Println("🔥 Spark Gemini integration enabled")
	}

	// Create audio player
	audioPlayer = audio.NewPlayer(robotIP, sshUser, sshPass)
	audioPlayer.OnPlaybackStart = func() {
		speakingMu.Lock()
		speaking = true
		speakingMu.Unlock()
	}
	audioPlayer.OnPlaybackEnd = func() {
		speakingMu.Lock()
		speaking = false
		speakingMu.Unlock()
	}

	// Wire up streaming TTS audio callback (if using WebSocket streaming)
	if ttsStreaming != nil {
		ttsStreaming.OnAudio = func(pcmData []byte) {
			// Stream audio chunks directly to the player for lowest latency
			if err := audioPlayer.AppendPCMChunk(pcmData); err != nil {
				debug.Log("⚠️  Streaming audio chunk error: %v\n", err)
			}

			// Feed audio to speech wobbler for head movement
			if speechWobbler != nil && len(pcmData) > 0 {
				// Convert bytes to int16 samples (little-endian PCM)
				samples := make([]int16, len(pcmData)/2)
				for i := 0; i < len(samples); i++ {
					samples[i] = int16(pcmData[i*2]) | int16(pcmData[i*2+1])<<8
				}
				speechWobbler.Feed(samples, 24000) // ElevenLabs outputs 24kHz
			}
		}
		ttsStreaming.OnConnected = func() {
			debug.Log("🔌 ElevenLabs WebSocket connected\n")
		}
		ttsStreaming.OnDisconnect = func() {
			debug.Log("🔌 ElevenLabs WebSocket disconnected\n")
		}
		ttsStreaming.OnError = func(err error) {
			fmt.Printf("⚠️  Streaming TTS error: %v\n", err)
		}
		ttsStreaming.OnStreamComplete = func() {
			// Audio stream complete, flush the player to finish playback
			fmt.Println("🗣️  [streaming audio complete, flushing...]")
			if err := audioPlayer.FlushAndPlay(); err != nil {
				fmt.Printf("⚠️  Audio flush error: %v\n", err)
			}
			fmt.Println("🗣️  [done]")

			// Reset speech wobbler and clear offsets
			if speechWobbler != nil {
				speechWobbler.Reset()
			}
			if headTracker != nil {
				headTracker.ClearSpeechOffsets()
			}

			// Update web dashboard
			if webServer != nil {
				webServer.UpdateState(func(s *web.EvaState) {
					s.Speaking = false
					s.Listening = true
				})
				webServer.AddLog("speech", "Streaming audio done")
			}
		}
	}

	return nil
}

func startWebDashboard(ctx context.Context) {
	// Create web server
	webServer = web.NewServer("8181")

	// Configure tool trigger callback
	webServer.OnToolTrigger = func(name string, args map[string]interface{}) (string, error) {
		fmt.Printf("🎮 Dashboard tool: %s (args: %v)\n", name, args)

		// Get tool config
		cfg := eva.ToolsConfig{
			Robot:          robotCtrl,
			Memory:         memoryStore,
			Vision:         &videoVisionAdapter{videoClient},
			ObjectDetector: &yoloAdapter{objectDetector},
			GoogleAPIKey:   os.Getenv("GOOGLE_API_KEY"),
			AudioPlayer:    audioPlayer,
			Tracker:        headTracker,
		}

		// Get tools and find the one requested
		tools := eva.Tools(cfg)
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

	// Configure frame capture callback
	webServer.OnCaptureFrame = func() ([]byte, error) {
		if videoClient == nil {
			return nil, fmt.Errorf("video client not connected")
		}
		return videoClient.GetFrame()
	}

	// Configure tuning API callbacks
	if headTracker != nil {
		webServer.OnGetTuningParams = func() interface{} {
			return headTracker.GetTuningParams()
		}
		webServer.OnSetTuningParams = func(params map[string]interface{}) {
			tp := tracking.TuningParams{}

			// === Smoothing ===
			if v, ok := params["offset_smoothing_alpha"].(float64); ok {
				tp.OffsetSmoothingAlpha = v
			}
			if v, ok := params["position_smoothing"].(float64); ok {
				tp.PositionSmoothing = v
			}

			// === Velocity limiting ===
			if v, ok := params["max_target_velocity"].(float64); ok {
				tp.MaxTargetVelocity = v
			}

			// === PD Controller (yaw) ===
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

			// === Body alignment ===
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

			// === Pitch-specific ===
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

			// === Audio tracking ===
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

			// === Breathing ===
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

			// === Range/speed ===
			if v, ok := params["max_speed"].(float64); ok {
				tp.MaxSpeed = v
			}
			if v, ok := params["yaw_range"].(float64); ok {
				tp.YawRange = v
			}
			if v, ok := params["body_yaw_limit"].(float64); ok {
				tp.BodyYawLimit = v
			}

			// === Scan behavior ===
			if v, ok := params["scan_start_delay"].(float64); ok {
				tp.ScanStartDelay = v
			}
			if v, ok := params["scan_speed"].(float64); ok {
				tp.ScanSpeed = v
			}
			if v, ok := params["scan_range"].(float64); ok {
				tp.ScanRange = v
			}

			headTracker.SetTuningParams(tp)
			fmt.Printf("🎛️  Tuning params updated: %+v\n", tp)
		}
		webServer.OnSetTuningMode = func(enabled bool) {
			headTracker.EnableTuningMode(enabled)
			fmt.Printf("🎛️  Tuning mode: %v\n", enabled)
		}
	}

	// Initialize camera configuration manager
	cameraManager = camera.NewManager()
	cfg := cameraManager.GetConfig()
	fmt.Printf("📷 Camera config: %dx%d @ %dfps (default: 1080p for better tracking)\n",
		cfg.Width, cfg.Height, cfg.Framerate)

	// Wire up camera API callbacks
	webServer.OnGetCameraConfig = func() interface{} {
		return cameraManager.GetConfigJSON()
	}
	webServer.OnSetCameraConfig = func(params map[string]interface{}) error {
		if err := cameraManager.UpdateConfig(params); err != nil {
			return err
		}
		cfg := cameraManager.GetConfig()
		fmt.Printf("📷 Camera config updated: %dx%d @ %dfps\n",
			cfg.Width, cfg.Height, cfg.Framerate)
		return nil
	}

	// Connect head tracker to web dashboard for state updates
	if headTracker != nil {
		headTracker.SetStateUpdater(&webStateAdapter{webServer})
	}

	// Start server in goroutine
	go func() {
		if err := webServer.Start(); err != nil {
			fmt.Printf("⚠️  Web server error: %v\n", err)
		}
	}()

	// Wait for context cancellation and gracefully shutdown
	<-ctx.Done()
	if err := webServer.Shutdown(); err != nil {
		fmt.Printf("⚠️  Web server shutdown error: %v\n", err)
	}
}

func streamCameraToWeb(ctx context.Context) {
	// Wait for web server to be ready
	for i := 0; i < 50; i++ {
		if webServer != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if videoClient == nil {
		fmt.Println("⚠️  Camera stream: video client not available")
		return
	}
	if webServer == nil {
		fmt.Println("⚠️  Camera stream: web server not available")
		return
	}

	fmt.Println("📷 Camera streaming to dashboard started")

	// Stream at 10 FPS to dashboard - much smoother than trying to hit 30 FPS
	// The H264 decoder can't keep up with 30 FPS anyway, and 10 FPS is 
	// plenty smooth for monitoring purposes while saving ~70% CPU
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
			frame, err := videoClient.GetFrame()
			if err != nil {
				// Log errors periodically
				if time.Since(lastLogTime) > 5*time.Second {
					fmt.Printf("📷 GetFrame error: %v\n", err)
					lastLogTime = time.Now()
				}
				continue
			}
			if len(frame) > 0 {
				webServer.SendCameraFrame(frame)
				frameCount++
				if frameCount == 1 {
					fmt.Printf("📷 First frame sent to dashboard (%d bytes)\n", len(frame))
				}
				// Log every 5 seconds if frame size changes
				if len(frame) != lastFrameSize && time.Since(lastLogTime) > 5*time.Second {
					fmt.Printf("📷 Streaming: %d frames sent, latest %d bytes\n", frameCount, len(frame))
					lastLogTime = time.Now()
					lastFrameSize = len(frame)
				}
			}
		}
	}
}

func wakeUpRobot() error {
	status, err := robotCtrl.GetDaemonStatus()
	if err != nil {
		return err
	}
	if status != "running" {
		return fmt.Errorf("daemon not running: %s", status)
	}
	// Set volume to max
	robotCtrl.SetVolume(100)

	// Reset body to neutral position at startup
	// This ensures known initial state and matches Python reachy behavior
	if err := robotCtrl.SetBodyYaw(0.0); err != nil {
		debug.Log("⚠️  Failed to reset body to neutral: %v\n", err)
	} else {
		debug.Log("🔄 Body reset to neutral (0.0 rad)\n")
		// Sync head tracker's world model with the physical robot state
		if headTracker != nil {
			headTracker.SetBodyYaw(0.0)
			debug.Log("🔄 World model synced: body=0.0 rad\n")
		}
	}

	return nil
}

func connectWebRTC() error {
	videoClient = video.NewClient(robotIP)
	return videoClient.Connect()
}

func connectRealtime(apiKey string) error {
	realtimeClient = openai.NewClient(apiKey)

	// Set OpenAI key on audio player for timer announcements
	audioPlayer.SetOpenAIKey(apiKey)

	// Register Eva's tools with vision and tracking support
	toolsCfg := eva.ToolsConfig{
		Robot:          robotCtrl,
		Memory:         memoryStore,
		Vision:         &videoVisionAdapter{videoClient},
		ObjectDetector: &yoloAdapter{objectDetector},
		GoogleAPIKey:   os.Getenv("GOOGLE_API_KEY"),
		AudioPlayer:    audioPlayer,
		Tracker:        headTracker, // For body rotation sync
		Emotions:       emotionRegistry,
		SparkStore:     sparkStore,  // Idea collection
		SparkGemini:    sparkGemini, // Gemini for title/tag generation
	}
	tools := eva.Tools(toolsCfg)
	for _, tool := range tools {
		realtimeClient.RegisterTool(openai.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Handler:     tool.Handler,
		})
	}

	// Set up callbacks
	realtimeClient.OnTranscript = func(text string, isFinal bool) {
		if isFinal && text != "" {
			// User's final transcript
			fmt.Printf("👤 User: %s\n", text)
			evaResponseStarted = false
			// Update web dashboard
			if webServer != nil {
				webServer.UpdateState(func(s *web.EvaState) {
					s.LastUserMessage = text
					s.Listening = true
					s.Speaking = false
				})
				webServer.AddConversation("user", text)
			}
		} else if !isFinal && text != "" {
			// Eva's speech - stream continuously on one line
			if !evaResponseStarted {
				fmt.Print("🤖 Eva: ")
				evaResponseStarted = true
				evaCurrentResponse = ""
			}
			fmt.Print(text)
			evaCurrentResponse += text

			// Stream text to ElevenLabs WebSocket for lowest latency
			if ttsMode == "elevenlabs-streaming" && ttsStreaming != nil {
				if err := ttsStreaming.SendText(text); err != nil {
					debug.Log("⚠️  Streaming TTS send error: %v\n", err)
				}
			}
		}
	}

	realtimeClient.OnAudioDelta = func(audioBase64 string) {
		// Only use OpenAI audio in realtime mode
		if ttsMode == "realtime" {
			if err := audioPlayer.AppendAudio(audioBase64); err != nil {
				fmt.Printf("⚠️  Audio append error: %v\n", err)
			}
		}
	}

	realtimeClient.OnAudioDone = func() {
		// Only handle realtime TTS mode here
		if ttsMode != "realtime" {
			return // External TTS handled in OnTranscriptDone
		}

		// End the Eva response line
		if evaResponseStarted {
			fmt.Println() // newline after streaming text
			evaResponseStarted = false
		}

		// Update web dashboard with Eva's response
		if webServer != nil && evaCurrentResponse != "" {
			webServer.UpdateState(func(s *web.EvaState) {
				s.Speaking = true
				s.Listening = false
				s.LastEvaMessage = evaCurrentResponse
			})
			webServer.AddConversation("eva", evaCurrentResponse)
			webServer.AddLog("speech", "Playing audio...")
		}

		// Use OpenAI Realtime audio
		fmt.Println("🗣️  [playing audio...]")
		if err := audioPlayer.FlushAndPlay(); err != nil {
			fmt.Printf("⚠️  Audio error: %v\n", err)
		}
		fmt.Println("🗣️  [done]")

		// Update web dashboard
		if webServer != nil {
			webServer.UpdateState(func(s *web.EvaState) {
				s.Speaking = false
				s.Listening = true
			})
			webServer.AddLog("speech", "Audio done")
		}
		evaCurrentResponse = ""
	}

	// OnTranscriptDone fires when OpenAI's transcript is complete
	// Use this for external TTS (ElevenLabs/OpenAI TTS) to ensure we have full text
	realtimeClient.OnTranscriptDone = func() {
		// Only handle external TTS modes here
		if ttsMode == "realtime" {
			return // Realtime audio handled in OnAudioDone
		}

		// End the Eva response line
		if evaResponseStarted {
			fmt.Println() // newline after streaming text
			evaResponseStarted = false
		}

		// Skip if no text to synthesize
		if evaCurrentResponse == "" {
			return
		}

		// Update web dashboard with Eva's response
		if webServer != nil {
			webServer.UpdateState(func(s *web.EvaState) {
				s.Speaking = true
				s.Listening = false
				s.LastEvaMessage = evaCurrentResponse
			})
			webServer.AddConversation("eva", evaCurrentResponse)
			webServer.AddLog("speech", "Synthesizing with "+ttsMode+"...")
		}

		// Streaming TTS: just flush to signal end of text
		if ttsMode == "elevenlabs-streaming" && ttsStreaming != nil {
			debug.Log("🗣️  Flushing streaming TTS...\n")
			if err := ttsStreaming.Flush(); err != nil {
				fmt.Printf("⚠️  Streaming TTS flush error: %v\n", err)
			}
			// Audio playback handled by ttsStreaming.OnAudio callback
			evaCurrentResponse = ""
			return
		}

		// Use HTTP TTS provider (ElevenLabs or OpenAI TTS)
		if ttsProvider != nil {
			fmt.Printf("🗣️  [synthesizing with %s...]\n", ttsMode)
			go func(text string) {
				result, err := ttsProvider.Synthesize(context.Background(), text)
				if err != nil {
					fmt.Printf("⚠️  TTS error: %v\n", err)
					return
				}
				fmt.Printf("🗣️  TTS: %d bytes, %d latency\n", len(result.Audio), result.LatencyMs)

				// Play the PCM audio
				if err := audioPlayer.PlayPCM(result.Audio); err != nil {
					fmt.Printf("⚠️  Audio playback error: %v\n", err)
				}
				fmt.Println("🗣️  [done]")

				// Update web dashboard
				if webServer != nil {
					webServer.UpdateState(func(s *web.EvaState) {
						s.Speaking = false
						s.Listening = true
					})
					webServer.AddLog("speech", "Audio done")
				}
			}(evaCurrentResponse)
		}

		evaCurrentResponse = ""
	}

	realtimeClient.OnError = func(err error) {
		fmt.Printf("⚠️  Error: %v\n", err)
		if webServer != nil {
			webServer.AddLog("error", err.Error())
		}
	}

	realtimeClient.OnSessionCreated = func() {
		fmt.Println("   Session created!")
	}

	realtimeClient.OnSpeechStarted = func() {
		// User started speaking - if Eva is talking, interrupt her
		if audioPlayer != nil && audioPlayer.IsSpeaking() {
			fmt.Println("🛑 [interrupted]")
			audioPlayer.Cancel()
			realtimeClient.CancelResponse()
		}
	}

	return realtimeClient.Connect()
}

func streamAudioToRealtime(ctx context.Context) {
	// Buffer for accumulating audio
	var audioBuffer []int16
	const chunkSize = 2400 // 100ms at 24kHz

	// Counters for debug logging
	var loopCount, emptyCount, sentCount int
	lastLogTime := time.Now()

	debug.Logln("🎵 Audio streaming goroutine started")

	for {
		select {
		case <-ctx.Done():
			debug.Logln("🎵 Audio streaming stopped (context cancelled)")
			return
		default:
		}

		loopCount++

		// Don't send audio while speaking (to avoid echo)
		speakingMu.Lock()
		isSpeaking := speaking
		speakingMu.Unlock()

		if isSpeaking {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Get audio from WebRTC (48kHz)
		if videoClient == nil {
			if loopCount == 1 {
				debug.Logln("🎵 videoClient is nil!")
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Record a small chunk
		videoClient.StartRecording()
		time.Sleep(100 * time.Millisecond)
		pcmData := videoClient.StopRecording()

		if len(pcmData) == 0 {
			emptyCount++
			// Log every 5 seconds if getting empty audio
			if time.Since(lastLogTime) > 5*time.Second {
				debug.Log("🎵 Audio stats: loops=%d, empty=%d, sent=%d (empty audio!)\n", loopCount, emptyCount, sentCount)
				lastLogTime = time.Now()
			}
			continue
		}

		// First time we get audio
		if sentCount == 0 && emptyCount == 0 {
			debug.Log("🎵 First audio chunk: %d samples\n", len(pcmData))
		}

		// Resample from 48kHz to 24kHz (OpenAI Realtime uses 24kHz)
		resampled := audio.Resample(pcmData, 48000, 24000)
		audioBuffer = append(audioBuffer, resampled...)

		// Send when we have enough
		if len(audioBuffer) >= chunkSize {
			// Convert to bytes
			pcm16Bytes := audio.ConvertInt16ToPCM16(audioBuffer[:chunkSize])
			audioBuffer = audioBuffer[chunkSize:]

			// Send to Realtime API
			if realtimeClient == nil {
				debug.Logln("🎵 realtimeClient is nil!")
			} else if !realtimeClient.IsConnected() {
				debug.Logln("🎵 realtimeClient not connected!")
			} else {
				if err := realtimeClient.SendAudio(pcm16Bytes); err != nil {
					debug.Log("🎵 SendAudio error: %v\n", err)
				} else {
					sentCount++
					// Log first send and then every 50 sends
					if sentCount == 1 {
						debug.Log("🎵 First audio sent to OpenAI! (%d bytes)\n", len(pcm16Bytes))
					} else if sentCount%50 == 0 {
						debug.Log("🎵 Audio stats: sent=%d chunks to OpenAI\n", sentCount)
					}
				}
			}
		}
	}
}

func shutdown() {
	if realtimeClient != nil {
		realtimeClient.Close()
	}
	if videoClient != nil {
		videoClient.Close()
	}
	if ttsProvider != nil {
		ttsProvider.Close()
	}
	if ttsStreaming != nil {
		ttsStreaming.Close()
	}
}

// videoVisionAdapter wraps video.Client to implement VisionProvider
type videoVisionAdapter struct {
	client *video.Client
}

func (v *videoVisionAdapter) CaptureFrame() ([]byte, error) {
	if v.client == nil {
		return nil, fmt.Errorf("video client not connected")
	}
	return v.client.CaptureJPEG()
}

// yoloAdapter wraps YOLO detector to implement ObjectDetector interface
type yoloAdapter struct {
	detector *detection.YOLODetector
}

func (y *yoloAdapter) Detect(jpeg []byte) ([]eva.ObjectDetectionResult, error) {
	if y.detector == nil {
		return nil, fmt.Errorf("object detector not initialized")
	}
	detections, err := y.detector.Detect(jpeg)
	if err != nil {
		return nil, err
	}
	// Convert to eva package type
	results := make([]eva.ObjectDetectionResult, len(detections))
	for i, det := range detections {
		results[i] = eva.ObjectDetectionResult{
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
