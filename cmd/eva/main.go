// Eva 2.0 - Low-latency conversational robot agent with tool use
// Uses OpenAI Realtime API for speech-to-speech conversation
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/realtime"
	"github.com/teslashibe/go-reachy/pkg/tracking"
	"github.com/teslashibe/go-reachy/pkg/video"
	"github.com/teslashibe/go-reachy/pkg/web"
)

const (
	robotIP = "192.168.68.74"
	sshUser = "pollen"
	sshPass = "root"
)

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
	realtimeClient *realtime.Client
	videoClient    *video.Client
	audioPlayer    *realtime.AudioPlayer
	robot          *realtime.SimpleRobotController
	memory         *realtime.Memory
	webServer      *web.Server
	headTracker    *tracking.Tracker

	speaking   bool
	speakingMu sync.Mutex

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
	fmt.Println("🤖 Eva 2.0 - Low-Latency Conversational Agent")
	fmt.Println("==============================================")

	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		fmt.Println("❌ Set OPENAI_API_KEY!")
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
	if err := initialize(openaiKey); err != nil {
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
	headTracker, err = tracking.New(tracking.DefaultConfig(), robot, videoClient, modelPath)
	if err != nil {
		fmt.Printf("⚠️  Disabled: %v\n", err)
		fmt.Println("   (Download model with: curl -L https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx -o models/face_detection_yunet.onnx)")
	} else {
		fmt.Println("✅")
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

func initialize(_ string) error {
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

func startWebDashboard(_ context.Context) {
	// Create web server
	webServer = web.NewServer("8181")

	// Configure tool trigger callback
	webServer.OnToolTrigger = func(name string, args map[string]interface{}) (string, error) {
		// Get tool config
		cfg := realtime.EvaToolsConfig{
			Robot:        robot,
			Memory:       memory,
			Vision:       &videoVisionAdapter{videoClient},
			GoogleAPIKey: os.Getenv("GOOGLE_API_KEY"),
			AudioPlayer:  audioPlayer,
		}

		// Get tools and find the one requested
		tools := realtime.EvaTools(cfg)
		for _, tool := range tools {
			if tool.Name == name {
				return tool.Handler(args)
			}
		}
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

	// Start server (blocks)
	if err := webServer.Start(); err != nil {
		fmt.Printf("⚠️  Web server error: %v\n", err)
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

func connectRealtime(apiKey string) error {
	realtimeClient = realtime.NewClient(apiKey)

	// Set OpenAI key on audio player for timer announcements
	audioPlayer.SetOpenAIKey(apiKey)

	// Register Eva's tools with vision and tracking support
	toolsCfg := realtime.EvaToolsConfig{
		Robot:        robot,
		Memory:       memory,
		Vision:       &videoVisionAdapter{videoClient},
		GoogleAPIKey: os.Getenv("GOOGLE_API_KEY"),
		AudioPlayer:  audioPlayer,
		Tracker:      headTracker, // For body rotation sync
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

func streamAudioToRealtime(ctx context.Context) {
	// Buffer for accumulating audio
	var audioBuffer []int16
	const chunkSize = 2400 // 100ms at 24kHz

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

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
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Record a small chunk
		videoClient.StartRecording()
		time.Sleep(100 * time.Millisecond)
		pcmData := videoClient.StopRecording()

		if len(pcmData) == 0 {
			continue
		}

		// Resample from 48kHz to 24kHz (OpenAI Realtime uses 24kHz)
		resampled := realtime.Resample(pcmData, 48000, 24000)
		audioBuffer = append(audioBuffer, resampled...)

		// Send when we have enough
		if len(audioBuffer) >= chunkSize {
			// Convert to bytes
			pcm16Bytes := realtime.ConvertInt16ToPCM16(audioBuffer[:chunkSize])
			audioBuffer = audioBuffer[chunkSize:]

			// Send to Realtime API
			if realtimeClient != nil && realtimeClient.IsConnected() {
				realtimeClient.SendAudio(pcm16Bytes)
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
