// Eva 2.0 - Low-latency conversational robot agent with tool use
// Supports OpenAI Realtime API and ElevenLabs Agents Platform
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/conversation"
	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/inference"
	"github.com/teslashibe/go-reachy/pkg/realtime"
	"github.com/teslashibe/go-reachy/pkg/tracking"
	"github.com/teslashibe/go-reachy/pkg/tts"
	"github.com/teslashibe/go-reachy/pkg/video"
	"github.com/teslashibe/go-reachy/pkg/web"
)

var (
	robotIP = getEnv("ROBOT_IP", "192.168.68.77")
	sshUser = getEnv("SSH_USER", "pollen")
	sshPass = getEnv("SSH_PASS", "root")
)

// getEnv returns environment variable or default value
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
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
	// Conversation provider (ElevenLabs or OpenAI)
	convProvider conversation.Provider

	// Legacy realtime client (for tool registration until fully migrated)
	realtimeClient *realtime.Client

	videoClient *video.Client
	audioPlayer *realtime.AudioPlayer
	robot       *realtime.SimpleRobotController
	memory      *realtime.Memory
	webServer   *web.Server
	headTracker *tracking.Tracker

	speaking   bool
	speakingMu sync.Mutex

	// Track if we've started printing Eva's response
	evaResponseStarted bool
	evaCurrentResponse string

	// Tool handlers map for conversation provider
	toolHandlers map[string]func(args map[string]any) (string, error)
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
	flag.Parse()
	debug.Enabled = *debugFlag

	fmt.Println("🤖 Eva 2.0 - Low-Latency Conversational Agent")
	fmt.Println("==============================================")
	if debug.Enabled {
		fmt.Println("🐛 Debug mode enabled")
	}

	openaiKey := os.Getenv("OPENAI_API_KEY")
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	elevenLabsVoice := os.Getenv("ELEVENLABS_VOICE_ID")
	provider := getEnv("CONVERSATION_PROVIDER", "elevenlabs")

	// Validate we have at least one provider configured
	if openaiKey == "" && elevenLabsKey == "" {
		fmt.Println("❌ Set OPENAI_API_KEY or ELEVENLABS_API_KEY!")
		os.Exit(1)
	}

	// For ElevenLabs, use default voice if not specified
	if provider == "elevenlabs" || provider == "11labs" {
		if elevenLabsKey == "" {
			fmt.Println("❌ ELEVENLABS_API_KEY required for ElevenLabs provider!")
			os.Exit(1)
		}
		if elevenLabsVoice == "" {
			os.Setenv("ELEVENLABS_VOICE_ID", "EXAVITQu4vr4xnSDxMaL") // Sarah - default
			fmt.Println("🎤 Using default voice: Sarah")
		}
	} else if openaiKey == "" {
		fmt.Println("❌ OPENAI_API_KEY required for OpenAI provider!")
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
	// Use SlowConfig for smoother, less jittery tracking
	headTracker, err = tracking.New(tracking.SlowConfig(), robot, videoClient, modelPath)
	if err != nil {
		fmt.Printf("⚠️  Disabled: %v\n", err)
		fmt.Println("   (Download model with: curl -L https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx -o models/face_detection_yunet.onnx)")
	} else {
		fmt.Println("✅")

		// Connect audio DOA from go-eva
		fmt.Print("🎤 Connecting to go-eva audio DOA... ")
		audioClient := audio.NewClient(robotIP)
		if err := audioClient.Health(); err != nil {
			fmt.Printf("⚠️  %v (audio DOA disabled)\n", err)
		} else {
			headTracker.SetAudioClient(audioClient)
			fmt.Println("✅")
		}
	}

	// Initialize conversation provider
	fmt.Print("🧠 Initializing conversation provider... ")
	convProvider, err = initConversationProvider()
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Connect to conversation API
	fmt.Print("🔌 Connecting to conversation API... ")
	if err := connectConversation(ctx, openaiKey); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Configure session
	fmt.Print("⚙️  Configuring Eva's personality... ")
	sessionOpts := conversation.SessionOptions{
		SystemPrompt: evaInstructions,
		Voice:        getEnv("CONVERSATION_VOICE", "shimmer"),
	}
	// Register tools with session
	for name := range toolHandlers {
		sessionOpts.Tools = append(sessionOpts.Tools, conversation.Tool{
			Name: name,
		})
	}
	if err := convProvider.ConfigureSession(sessionOpts); err != nil {
		fmt.Printf("⚠️  Session config warning: %v\n", err)
	}
	fmt.Println("✅")

	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n🎤 Eva is listening! Speak to start a conversation...")
	fmt.Println("   (Ctrl+C to exit)")

	// Start audio streaming from WebRTC to conversation provider
	go streamAudioToConversation(ctx)

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
				s.OpenAIConnected = convProvider != nil && convProvider.IsConnected()
				s.WebRTCConnected = videoClient != nil
				s.Listening = true
			})
			providerName := getEnv("CONVERSATION_PROVIDER", "openai")
			webServer.AddLog("info", fmt.Sprintf("Eva 2.0 started (conversation: %s)", providerName))
		}
	}()

	// Keep running
	<-ctx.Done()
}

func initialize() error {
	// Create robot controller
	robot = realtime.NewSimpleRobotController(robotIP)

	// Create persistent memory (saves to ~/.eva/memory.json)
	homeDir, _ := os.UserHomeDir()
	memoryPath := homeDir + "/.eva/memory.json"
	memory = realtime.NewMemoryWithFile(memoryPath)
	fmt.Printf("📝 Memory loaded from %s\n", memoryPath)

	// Create audio player
	audioPlayer = realtime.NewAudioPlayer(robotIP, sshUser, sshPass)
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

	return nil
}

func startWebDashboard(ctx context.Context) {
	// Create web server
	webServer = web.NewServer("8181")

	// Configure tool trigger callback
	webServer.OnToolTrigger = func(name string, args map[string]interface{}) (string, error) {
		fmt.Printf("🎮 Dashboard tool: %s (args: %v)\n", name, args)

		// Get tool config
		cfg := realtime.EvaToolsConfig{
			Robot:        robot,
			Memory:       memory,
			Vision:       &videoVisionAdapter{videoClient},
			GoogleAPIKey: os.Getenv("GOOGLE_API_KEY"),
			AudioPlayer:  audioPlayer,
			Tracker:      headTracker,
		}

		// Get tools and find the one requested
		tools := realtime.EvaTools(cfg)
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

	ticker := time.NewTicker(33 * time.Millisecond) // ~30 FPS
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
	status, err := robot.GetDaemonStatus()
	if err != nil {
		return err
	}
	if status != "running" {
		return fmt.Errorf("daemon not running: %s", status)
	}
	// Set volume to max
	robot.SetVolume(100)
	return nil
}

func connectWebRTC() error {
	videoClient = video.NewClient(robotIP)
	return videoClient.Connect()
}

// initTTSProvider creates the TTS provider chain (ElevenLabs → OpenAI fallback)
func initTTSProvider(openaiKey string) (tts.Provider, error) {
	var providers []tts.Provider

	// Check for ElevenLabs configuration
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	elevenLabsVoice := os.Getenv("ELEVENLABS_VOICE_ID")

	if elevenLabsKey != "" && elevenLabsVoice != "" {
		el, err := tts.NewElevenLabs(
			tts.WithAPIKey(elevenLabsKey),
			tts.WithVoice(elevenLabsVoice),
			tts.WithModel(tts.ModelTurboV2_5),
			tts.WithOutputFormat(tts.EncodingPCM24),
		)
		if err != nil {
			fmt.Printf("⚠️  ElevenLabs init failed: %v\n", err)
		} else {
			providers = append(providers, el)
			fmt.Println("🎤 TTS: ElevenLabs (custom voice)")
		}
	}

	// Add OpenAI as fallback
	if openaiKey != "" {
		oai, err := tts.NewOpenAI(
			tts.WithAPIKey(openaiKey),
			tts.WithVoice(tts.VoiceShimmer),
		)
		if err != nil {
			fmt.Printf("⚠️  OpenAI TTS init failed: %v\n", err)
		} else {
			providers = append(providers, oai)
			if len(providers) == 1 {
				fmt.Println("🎤 TTS: OpenAI (shimmer)")
			} else {
				fmt.Println("🎤 TTS: OpenAI (fallback)")
			}
		}
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no TTS providers available")
	}

	// If only one provider, return it directly
	if len(providers) == 1 {
		return providers[0], nil
	}

	// Create chain for fallback
	return tts.NewChain(providers...)
}

// initInferenceProvider creates the inference provider for vision
func initInferenceProvider() (inference.Provider, error) {
	var providers []inference.Provider

	// Check for Google API key (for Gemini vision)
	googleKey := os.Getenv("GOOGLE_API_KEY")
	if googleKey != "" {
		gemini, err := inference.NewGemini(
			inference.WithAPIKey(googleKey),
		)
		if err != nil {
			fmt.Printf("⚠️  Gemini init failed: %v\n", err)
		} else {
			providers = append(providers, gemini)
			fmt.Println("👁️  Vision: Gemini Flash")
		}
	}

	// Check for OpenAI key (fallback for vision)
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey != "" {
		client, err := inference.NewClient(
			inference.WithAPIKey(openaiKey),
			inference.WithModel("gpt-4o-mini"),
			inference.WithVisionModel("gpt-4o"),
		)
		if err != nil {
			fmt.Printf("⚠️  OpenAI inference init failed: %v\n", err)
		} else {
			providers = append(providers, client)
			if len(providers) == 1 {
				fmt.Println("👁️  Vision: OpenAI GPT-4o")
			} else {
				fmt.Println("👁️  Vision: OpenAI GPT-4o (fallback)")
			}
		}
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no inference providers available (set GOOGLE_API_KEY or OPENAI_API_KEY)")
	}

	if len(providers) == 1 {
		return providers[0], nil
	}

	return inference.NewChain(providers...)
}

// initConversationProvider creates the conversation provider based on environment
func initConversationProvider() (conversation.Provider, error) {
	providerType := strings.ToLower(getEnv("CONVERSATION_PROVIDER", "elevenlabs"))

	switch providerType {
	case "elevenlabs", "11labs":
		apiKey := os.Getenv("ELEVENLABS_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ELEVENLABS_API_KEY required for ElevenLabs provider")
		}

		agentID := os.Getenv("ELEVENLABS_AGENT_ID")
		voiceID := os.Getenv("ELEVENLABS_VOICE_ID")
		llm := getEnv("ELEVENLABS_LLM", "gemini-2.0-flash")

		// Mode 1: Programmatic agent (recommended) - VoiceID set, no AgentID
		if voiceID != "" && agentID == "" {
			provider, err := conversation.NewElevenLabs(
				conversation.WithAPIKey(apiKey),
				conversation.WithVoiceID(voiceID),
				conversation.WithLLM(llm),
				conversation.WithSystemPrompt(evaInstructions),
				conversation.WithAgentName("eva-agent"),
				conversation.WithFirstMessage("Hello! I'm Eva, your friendly robot companion. How can I help you today?"),
				conversation.WithAutoCreateAgent(true),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create ElevenLabs provider: %w", err)
			}

			fmt.Printf("🎤 Conversation: ElevenLabs (programmatic, %s)\n", llm)
			return provider, nil
		}

		// Mode 2: Dashboard agent (legacy) - AgentID set
		if agentID != "" {
			provider, err := conversation.NewElevenLabs(
				conversation.WithAPIKey(apiKey),
				conversation.WithAgentID(agentID),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create ElevenLabs provider: %w", err)
			}

			fmt.Println("🎤 Conversation: ElevenLabs Agents (dashboard)")
			return provider, nil
		}

		return nil, fmt.Errorf("ELEVENLABS_VOICE_ID (programmatic) or ELEVENLABS_AGENT_ID (dashboard) required")

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY required for OpenAI provider")
		}

		provider, err := conversation.NewOpenAI(
			conversation.WithAPIKey(apiKey),
			conversation.WithVoice(conversation.VoiceShimmer),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
		}

		fmt.Println("🎤 Conversation: OpenAI Realtime (shimmer)")
		return provider, nil

	default:
		return nil, fmt.Errorf("unknown conversation provider: %s (use 'openai' or 'elevenlabs')", providerType)
	}
}

// connectConversation sets up callbacks and connects the conversation provider
func connectConversation(ctx context.Context, openaiKey string) error {
	// Initialize TTS provider (ElevenLabs with OpenAI fallback) for announcements
	ttsProvider, err := initTTSProvider(openaiKey)
	if err != nil {
		fmt.Printf("⚠️  TTS init warning: %v\n", err)
	}
	if ttsProvider != nil {
		audioPlayer.SetTTSProvider(ttsProvider)
	}

	// Configure audio player sample rate based on conversation provider
	caps := convProvider.Capabilities()
	if caps.OutputSampleRate > 0 {
		audioPlayer.SetSampleRate(caps.OutputSampleRate)
		fmt.Printf("🔊 Audio: %dHz output sample rate\n", caps.OutputSampleRate)
	}

	// Initialize inference provider for vision tools
	inferenceProvider, err := initInferenceProvider()
	if err != nil {
		fmt.Printf("⚠️  Inference init warning: %v\n", err)
	}

	// Build tool handlers map from Eva's tools
	toolsCfg := realtime.EvaToolsConfig{
		Robot:           robot,
		Memory:          memory,
		Vision:          &videoVisionAdapter{videoClient},
		InferenceClient: inferenceProvider,
		GoogleAPIKey:    os.Getenv("GOOGLE_API_KEY"),
		AudioPlayer:     audioPlayer,
		Tracker:         headTracker,
	}
	tools := realtime.EvaTools(toolsCfg)

	toolHandlers = make(map[string]func(args map[string]any) (string, error))
	for _, tool := range tools {
		// Register tool with conversation provider
		convProvider.RegisterTool(conversation.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})

		// Store handler for execution
		handler := tool.Handler // Capture in closure
		toolHandlers[tool.Name] = func(args map[string]any) (string, error) {
			// Convert map[string]any to map[string]interface{}
			argsIface := make(map[string]interface{})
			for k, v := range args {
				argsIface[k] = v
			}
			return handler(argsIface)
		}
	}

	// Latency tracking for response time measurement
	var (
		userSpeechEndTime   time.Time
		firstAudioTime      time.Time
		firstTranscriptTime time.Time
		audioChunkCount     int
		responseStarted     bool
	)

	// Set up conversation callbacks
	convProvider.OnAudio(func(audio []byte) {
		// Track first audio response latency
		if !responseStarted && !userSpeechEndTime.IsZero() {
			firstAudioTime = time.Now()
			latency := firstAudioTime.Sub(userSpeechEndTime)
			debug.Log("⚡ First audio response latency: %v\n", latency)
			responseStarted = true

			// Tell head tracker Eva is speaking (suppress DOA noise from her own voice)
			if headTracker != nil {
				headTracker.SetSpeaking(true)
			}
		}
		audioChunkCount++

		// Stream audio to robot
		if err := audioPlayer.AppendAudioBytes(audio); err != nil {
			debug.Log("⚠️  Audio append error: %v\n", err)
		}
	})

	convProvider.OnAudioDone(func() {
		// Gap detection triggered this callback - audio turn is complete
		fmt.Println("\n🔇 Audio gap detected - flushing audio")

		// Log latency summary (now tracked internally by provider too)
		if !userSpeechEndTime.IsZero() {
			totalTime := time.Since(userSpeechEndTime)
			debug.Log("📊 Response complete: %d audio chunks, total time: %v\n", audioChunkCount, totalTime)
			if !firstAudioTime.IsZero() {
				debug.Log("📊 Time to first audio: %v\n", firstAudioTime.Sub(userSpeechEndTime))
			}
			if !firstTranscriptTime.IsZero() {
				debug.Log("📊 Time to first transcript: %v\n", firstTranscriptTime.Sub(userSpeechEndTime))
			}
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
			webServer.AddLog("speech", "Audio gap detected, flushing...")
		}

		fmt.Println("🗣️  [flushing audio pipeline...]")
		if err := audioPlayer.FlushAndPlay(); err != nil {
			fmt.Printf("⚠️  Audio flush error: %v\n", err)
		}
		fmt.Println("🗣️  [done - resuming listening]")

		// Tell head tracker Eva is done speaking (resume DOA tracking)
		if headTracker != nil {
			headTracker.SetSpeaking(false)
		}

		// Update web dashboard
		if webServer != nil {
			webServer.UpdateState(func(s *web.EvaState) {
				s.Speaking = false
				s.Listening = true
			})
			webServer.AddLog("speech", "Audio done, listening resumed")
		}
		evaCurrentResponse = ""

		// Reset latency tracking for next turn
		userSpeechEndTime = time.Time{}
		firstAudioTime = time.Time{}
		firstTranscriptTime = time.Time{}
	})

	convProvider.OnTranscript(func(role, text string, isFinal bool) {
		if role == "user" && isFinal && text != "" {
			// User's final transcript - start latency timer
			userSpeechEndTime = time.Now()
			responseStarted = false
			audioChunkCount = 0
			debug.Log("⏱️  User speech ended, waiting for response...\n")

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
		} else if role == "agent" && text != "" {
			// Track first transcript response latency
			if !evaResponseStarted && !userSpeechEndTime.IsZero() {
				firstTranscriptTime = time.Now()
				latency := firstTranscriptTime.Sub(userSpeechEndTime)
				debug.Log("⚡ First transcript latency: %v\n", latency)
			}

			// Eva's speech - stream continuously on one line
			if !evaResponseStarted {
				fmt.Print("🤖 Eva: ")
				evaResponseStarted = true
				evaCurrentResponse = ""
			}
			fmt.Print(text)
			evaCurrentResponse += text
		}
	})

	convProvider.OnToolCall(func(callID, name string, args map[string]any) {
		fmt.Printf("🔧 Tool called: %s\n", name)

		// Execute the tool
		var result string
		if handler, ok := toolHandlers[name]; ok {
			var err error
			result, err = handler(args)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}
			fmt.Printf("🔧 Tool result: %s\n", result)
		} else {
			result = "Function not found"
			fmt.Printf("⚠️  Tool not found: %s\n", name)
		}

		// Submit result back to conversation
		if err := convProvider.SubmitToolResult(callID, result); err != nil {
			fmt.Printf("⚠️  Failed to submit tool result: %v\n", err)
		}
	})

	convProvider.OnError(func(err error) {
		fmt.Printf("⚠️  Conversation error: %v\n", err)
		if webServer != nil {
			webServer.AddLog("error", err.Error())
		}
	})

	convProvider.OnInterruption(func() {
		// User started speaking - if Eva is talking, interrupt her
		if audioPlayer != nil && audioPlayer.IsSpeaking() {
			fmt.Println("🛑 [interrupted]")
			audioPlayer.Cancel()
			convProvider.CancelResponse()
		}
	})

	// Connect to conversation service
	return convProvider.Connect(ctx)
}

func connectRealtime(apiKey string) error {
	realtimeClient = realtime.NewClient(apiKey)

	// Initialize TTS provider (ElevenLabs with OpenAI fallback)
	ttsProvider, err := initTTSProvider(apiKey)
	if err != nil {
		fmt.Printf("⚠️  TTS init warning: %v\n", err)
	}
	if ttsProvider != nil {
		audioPlayer.SetTTSProvider(ttsProvider)
	}

	// Initialize inference provider for vision
	inferenceProvider, err := initInferenceProvider()
	if err != nil {
		fmt.Printf("⚠️  Inference init warning: %v\n", err)
	}

	// Register Eva's tools with vision and tracking support
	toolsCfg := realtime.EvaToolsConfig{
		Robot:           robot,
		Memory:          memory,
		Vision:          &videoVisionAdapter{videoClient},
		InferenceClient: inferenceProvider,
		GoogleAPIKey:    os.Getenv("GOOGLE_API_KEY"),
		AudioPlayer:     audioPlayer,
		Tracker:         headTracker, // For body rotation sync
	}
	tools := realtime.EvaTools(toolsCfg)
	for _, tool := range tools {
		realtimeClient.RegisterTool(tool)
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
		}
	}

	realtimeClient.OnAudioDelta = func(audioBase64 string) {
		if err := audioPlayer.AppendAudio(audioBase64); err != nil {
			fmt.Printf("⚠️  Audio append error: %v\n", err)
		}
	}

	realtimeClient.OnAudioDone = func() {
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

func streamAudioToConversation(ctx context.Context) {
	// Buffer for accumulating audio
	var audioBuffer []int16

	// Get sample rate from provider capabilities
	caps := convProvider.Capabilities()
	targetRate := caps.InputSampleRate
	if targetRate == 0 {
		targetRate = 24000 // Default to 24kHz
	}

	// Calculate chunk size based on sample rate (50ms for lower latency)
	chunkSize := targetRate / 20 // 50ms of samples (was 100ms)

	// Counters for debug logging
	var loopCount, emptyCount, sentCount int
	lastLogTime := time.Now()

	// Latency tracking
	var lastSendTime time.Time
	var totalLatency time.Duration
	var latencyCount int

	debug.Logln("🎵 Audio streaming goroutine started (low-latency mode)")
	debug.Log("🎵 Target sample rate: %d Hz, chunk size: %d (50ms)\n", targetRate, chunkSize)

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

		// Record a small chunk (50ms for lower latency)
		videoClient.StartRecording()
		time.Sleep(50 * time.Millisecond)
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

		// Resample from 48kHz to target rate
		resampled := realtime.Resample(pcmData, 48000, targetRate)
		audioBuffer = append(audioBuffer, resampled...)

		// Send when we have enough
		if len(audioBuffer) >= chunkSize {
			// Convert to bytes
			pcm16Bytes := realtime.ConvertInt16ToPCM16(audioBuffer[:chunkSize])
			audioBuffer = audioBuffer[chunkSize:]

			// Send to conversation provider
			if convProvider == nil {
				debug.Logln("🎵 convProvider is nil!")
			} else if !convProvider.IsConnected() {
				debug.Logln("🎵 convProvider not connected!")
			} else {
				sendStart := time.Now()
				if err := convProvider.SendAudio(pcm16Bytes); err != nil {
					debug.Log("🎵 SendAudio error: %v\n", err)
				} else {
					sendLatency := time.Since(sendStart)
					sentCount++
					totalLatency += sendLatency
					latencyCount++

					// Track time between sends
					if !lastSendTime.IsZero() {
						gap := time.Since(lastSendTime)
						if gap > 100*time.Millisecond && sentCount > 10 {
							debug.Log("⚠️  Audio gap detected: %v (should be ~50ms)\n", gap)
						}
					}
					lastSendTime = time.Now()

					// Log first send and then every 100 sends with latency stats
					if sentCount == 1 {
						debug.Log("🎵 First audio sent! (%d bytes, latency: %v)\n", len(pcm16Bytes), sendLatency)
					} else if sentCount%100 == 0 {
						avgLatency := totalLatency / time.Duration(latencyCount)
						debug.Log("🎵 Audio stats: sent=%d chunks, avg send latency: %v\n", sentCount, avgLatency)
					}
				}
			}
		}
	}
}

func shutdown() {
	if convProvider != nil {
		convProvider.Close()
	}
	if realtimeClient != nil {
		realtimeClient.Close()
	}
	if videoClient != nil {
		videoClient.Close()
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

func (v *videoVisionAdapter) CaptureImage() (image.Image, error) {
	if v.client == nil {
		return nil, fmt.Errorf("video client not connected")
	}
	return v.client.CaptureImage()
}
