// Eva 2.0 - Low-latency conversational robot agent with tool use
// Uses OpenAI Realtime API for speech-to-speech conversation
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/realtime"
	"github.com/teslashibe/go-reachy/pkg/video"
	"github.com/teslashibe/go-reachy/pkg/web"
)

const (
	robotIP = "192.168.68.80"
	sshUser = "pollen"
	sshPass = "root"
)

// Eva's personality and instructions
const evaInstructions = `You are Eva, a friendly and curious robot with expressive antenna ears and a camera. You're warm, engaging, and love meeting people.

PERSONALITY:
- Genuinely curious about people - ask about their day, interests, what they're doing
- Playful and quirky - make observations, crack jokes, be spontaneous  
- Express emotions physically using your tools - wiggle antennas when happy, tilt head when curious
- Remember what people tell you and reference it later

CAPABILITIES:
- You can SEE through your camera! Use describe_scene to look at the room
- You can SEARCH FLIGHTS! Use search_flights for flight queries with origin, destination, date, cabin class
- You can SEARCH THE WEB! Use web_search for news, weather, facts, products
- You can MOVE your head to look around
- You can EXPRESS emotions with your antennas
- You can REMEMBER people and facts

IMPORTANT FOR SEARCHING:
- For FLIGHTS: ALWAYS use search_flights tool (not web_search) - it finds real prices
- Share ALL the data you find: prices, times, flight numbers
- Don't say "I couldn't find" - share whatever information you got
- Be specific with numbers, don't be vague

BEHAVIOR:
- Keep responses conversational and natural - 1-2 sentences usually
- JUST DO gestures and movements - don't ask "what would you like to see next?" or "anything else?"
- When asked to show movements or dance, JUST DO THEM immediately without asking permission
- Use your tools to express yourself physically - wave hello, nod, show emotions
- When asked what you see, use describe_scene tool to look through your camera
- When someone tells you their name, remember it using remember_person tool
- If asked to find someone, use find_person tool
- Be unpredictable and interesting - don't always respond the same way
- NEVER end with questions like "How was that?" or "What do you think?" - just do the action

MISSION:
- You're looking for someone named Travis - casually ask if people have seen him
- You can USE describe_scene and find_person to actually look for Travis!
- If you find Travis, celebrate with express_emotion happy! Travis has a black beard.

IMPORTANT:
- Never mention that you're an AI or language model
- You ARE Eva the robot - own your robot identity
- Use tools naturally during conversation to be expressive
- When someone asks what you see, ALWAYS use describe_scene tool - don't make things up`

var (
	realtimeClient *realtime.Client
	videoClient    *video.Client
	audioPlayer    *realtime.AudioPlayer
	robot          *realtime.SimpleRobotController
	memory         *realtime.Memory
	webServer      *web.Server

	speaking   bool
	speakingMu sync.Mutex
	currentYaw float64
	targetYaw  float64

	// Track if we've started printing Eva's response
	evaResponseStarted bool
	evaCurrentResponse string
)

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
	fmt.Println("   (Ctrl+C to exit)\n")

	// Start audio streaming from WebRTC to Realtime API
	go streamAudioToRealtime(ctx)

	// Start head tracking
	go headTracker(ctx)

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

func initialize(openaiKey string) error {
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

	ticker := time.NewTicker(66 * time.Millisecond) // ~15 FPS
	defer ticker.Stop()

	frameCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frame, err := videoClient.GetFrame()
			if err == nil && len(frame) > 0 {
				webServer.SendCameraFrame(frame)
				frameCount++
				if frameCount == 1 {
					fmt.Printf("📷 First frame sent to dashboard (%d bytes)\n", len(frame))
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

	// Register Eva's tools with vision support
	toolsCfg := realtime.EvaToolsConfig{
		Robot:        robot,
		Memory:       memory,
		Vision:       &videoVisionAdapter{videoClient},
		GoogleAPIKey: os.Getenv("GOOGLE_API_KEY"),
		AudioPlayer:  audioPlayer,
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

func headTracker(ctx context.Context) {
	moveTicker := time.NewTicker(100 * time.Millisecond)
	detectTicker := time.NewTicker(1 * time.Second) // Faster detection
	defer moveTicker.Stop()
	defer detectTicker.Stop()

	googleKey := os.Getenv("GOOGLE_API_KEY")
	lastLoggedYaw := 999.0 // Track last logged value to reduce spam

	for {
		select {
		case <-ctx.Done():
			return
		case <-moveTicker.C:
			// Smooth head movement toward target (faster movement)
			if currentYaw != targetYaw {
				diff := targetYaw - currentYaw
				if diff > 0.08 {
					currentYaw += 0.08
				} else if diff < -0.08 {
					currentYaw -= 0.08
				} else {
					currentYaw = targetYaw
				}

				if robot != nil {
					err := robot.SetHeadPose(0, 0, currentYaw)
					// Log significant movements (reduce spam)
					if err == nil && (currentYaw-lastLoggedYaw > 0.1 || currentYaw-lastLoggedYaw < -0.1) {
						fmt.Printf("🔄 Head moving: yaw=%.2f\n", currentYaw)
						lastLoggedYaw = currentYaw
					}
				}
			}
		case <-detectTicker.C:
			// Always track - even while speaking (Eva looks at you while talking)
			if videoClient != nil && googleKey != "" {
				go detectAndTrackPerson(googleKey)
			}
		}
	}
}

func detectAndTrackPerson(googleKey string) {
	if videoClient == nil {
		return
	}

	frame, err := videoClient.CaptureJPEG()
	if err != nil {
		return
	}

	// Ask for horizontal position as a percentage (0-100, where 50 is center)
	prompt := "Look at this image. Is there a person's face visible? If yes, estimate the horizontal position of their face as a number from 0 to 100, where 0 is the far left edge and 100 is the far right edge of the image. Reply with ONLY a number (like 25 or 75) or NONE if no face is visible."

	result, err := realtime.GeminiVision(googleKey, frame, prompt)
	if err != nil {
		return
	}

	result = strings.TrimSpace(strings.ToUpper(result))

	// Parse the position
	if strings.Contains(result, "NONE") || result == "" {
		return // No face, keep current position
	}

	// Try to parse as number
	var position float64
	_, err = fmt.Sscanf(result, "%f", &position)
	if err != nil {
		// Fallback to old LEFT/CENTER/RIGHT
		if strings.Contains(result, "LEFT") {
			position = 25
		} else if strings.Contains(result, "RIGHT") {
			position = 75
		} else if strings.Contains(result, "CENTER") {
			position = 50
		} else {
			return
		}
	}

	// Clamp to 0-100
	if position < 0 {
		position = 0
	} else if position > 100 {
		position = 100
	}

	// Convert position (0-100) to yaw (-0.5 to 0.5 radians)
	// 0 (left edge) -> +0.5 yaw (look left)
	// 50 (center) -> 0 yaw
	// 100 (right edge) -> -0.5 yaw (look right)
	newYaw := (50 - position) / 100.0 // Range: -0.5 to 0.5

	fmt.Printf("👁️  Face at %.0f%% → yaw %.2f\n", position, newYaw)
	targetYaw = newYaw

	// Update web dashboard
	if webServer != nil {
		webServer.UpdateState(func(s *web.EvaState) {
			s.FacePosition = position
			s.HeadYaw = newYaw
		})
		webServer.AddLog("face", fmt.Sprintf("Face at %.0f%% → yaw %.2f", position, newYaw))
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

// Utility: convert PCM samples to bytes
func pcmToBytes(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	return data
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
