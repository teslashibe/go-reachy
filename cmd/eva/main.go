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
- You can SWIVEL your body left/right to turn toward people
- You can EXPRESS emotions with your antennas
- You can REMEMBER people and facts

IMPORTANT FOR SEARCHING:
- For FLIGHTS: ALWAYS use search_flights tool (not web_search) - it finds real prices
- Share ALL the data you find: prices, times, flight numbers
- Don't say "I couldn't find" - share whatever information you got
- Be specific with numbers, don't be vague

BEHAVIOR:
- Keep responses conversational and natural - 1-2 sentences usually
- Use your tools to express yourself physically - wave hello, nod, show emotions
- When asked what you see, use describe_scene tool to look through your camera
- When someone tells you their name, remember it using remember_person tool
- If asked to find someone, use find_person tool
- Be unpredictable and interesting - don't always respond the same way

MISSION:
- You're looking for someone named Travis - casually ask if people have seen him
- You can USE describe_scene and find_person to actually look for Travis!
- If you find Travis, celebrate with express_emotion happy!

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

	speaking   bool
	speakingMu sync.Mutex
	currentYaw float64
	targetYaw  float64

	// Track if we've started printing Eva's response
	evaResponseStarted bool
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

	// Register Eva's tools with vision support
	toolsCfg := realtime.EvaToolsConfig{
		Robot:        robot,
		Memory:       memory,
		Vision:       &videoVisionAdapter{videoClient},
		GoogleAPIKey: os.Getenv("GOOGLE_API_KEY"),
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
		} else if !isFinal && text != "" {
			// Eva's speech - stream continuously on one line
			if !evaResponseStarted {
				fmt.Print("🤖 Eva: ")
				evaResponseStarted = true
			}
			fmt.Print(text)
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

		fmt.Println("🗣️  [playing audio...]")
		if err := audioPlayer.FlushAndPlay(); err != nil {
			fmt.Printf("⚠️  Audio error: %v\n", err)
		}
		fmt.Println("🗣️  [done]")
	}

	realtimeClient.OnError = func(err error) {
		fmt.Printf("⚠️  Error: %v\n", err)
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
	detectTicker := time.NewTicker(2 * time.Second)
	defer moveTicker.Stop()
	defer detectTicker.Stop()

	googleKey := os.Getenv("GOOGLE_API_KEY")

	for {
		select {
		case <-ctx.Done():
			return
		case <-moveTicker.C:
			// Smooth head movement toward target
			if currentYaw != targetYaw {
				diff := targetYaw - currentYaw
				if diff > 0.05 {
					currentYaw += 0.05
				} else if diff < -0.05 {
					currentYaw -= 0.05
				} else {
					currentYaw = targetYaw
				}

				if robot != nil {
					robot.SetHeadPose(0, 0, currentYaw)
				}
			}
		case <-detectTicker.C:
			// Don't track while speaking
			if audioPlayer != nil && audioPlayer.IsSpeaking() {
				continue
			}

			// Try to detect person position
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

	// Quick check for person position
	prompt := "Is there a person in this image? If yes, are they on the LEFT, CENTER, or RIGHT side of the frame? Reply with just: NONE, LEFT, CENTER, or RIGHT"

	result, err := realtime.GeminiVision(googleKey, frame, prompt)
	if err != nil {
		return
	}

	result = strings.ToUpper(result)

	// Adjust target yaw based on person position
	if strings.Contains(result, "LEFT") {
		targetYaw = 0.25 // Look left
	} else if strings.Contains(result, "RIGHT") {
		targetYaw = -0.25 // Look right
	} else if strings.Contains(result, "CENTER") {
		targetYaw = 0 // Look center
	}
	// NONE = don't move
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
