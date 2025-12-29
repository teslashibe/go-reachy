// Travis Detector - Conversational robot looking for Travis
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/video"
)

const robotIP = "192.168.68.80"
const robotAPI = "http://192.168.68.80:8000"
const sshPass = "root"
const sshUser = "pollen"
const geminiAPI = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
const openaiTTSAPI = "https://api.openai.com/v1/audio/speech"
const whisperAPI = "https://api.openai.com/v1/audio/transcriptions"

var videoClient *video.Client

// State
var (
	currentYaw       float64 = 0
	targetYaw        float64 = 0
	speaking         bool    = false
	speakingMutex    sync.Mutex
	speakEndTime     time.Time // When we stopped speaking
	lastHeard        string
	lastHeardMutex   sync.Mutex
	travisFound      bool = false
	conversationCtx  context.Context
	conversationHist []string // Track what we've said
	peopleMetNames   []string // Remember names
	greetingVariant  int      = 0
	cachedPosition   string   = "none"
	positionExpiry   time.Time
)

const systemPrompt = `You are Reachy, a curious and expressive robot with antenna ears. You're on a fun mission to find someone named Travis.

PERSONALITY:
- Genuinely curious about people - ask about their day, what they're up to
- Playful and a bit quirky - make little observations about what you see
- Warm and engaging - remember details people tell you
- Express yourself physically - mention when you wiggle your antennas

CONVERSATION RULES:
- NEVER repeat the same greeting or question twice
- If you already asked "Are you Travis?", try something different next time
- If someone told you their name, USE IT and remember it
- Ask follow-up questions based on what they said
- Make observations about what you see in the room
- Keep responses to 1-2 SHORT sentences
- Sound natural, like chatting with a friend
- NO emojis or special characters

VARY YOUR APPROACH:
- Sometimes comment on what you observe before asking about Travis
- Sometimes ask what someone is doing
- Sometimes share something about yourself
- Be unpredictable and interesting!`

func main() {
	fmt.Println("🤖 Reachy - Conversational Travis Finder")
	fmt.Println("=========================================")

	geminiKey := os.Getenv("GEMINI_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if geminiKey == "" {
		fmt.Println("❌ Set GEMINI_API_KEY!")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	conversationCtx = ctx

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n👋 Goodbye!")
		if videoClient != nil {
			videoClient.Close()
		}
		cancel()
		os.Exit(0)
	}()

	// Start robot
	fmt.Print("🤖 Waking up... ")
	startDaemon()
	setVolume(100)
	fmt.Println("✅")

	// Connect
	fmt.Println("📹 Connecting...")
	videoClient = video.NewClient(robotIP)
	if err := videoClient.Connect(); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		os.Exit(1)
	}
	defer videoClient.Close()

	time.Sleep(2 * time.Second)

	// Start continuous listening in background
	go continuousListener(ctx, openaiKey)

	// Start head tracking in background
	go smoothHeadTracker(ctx)

	// Opening
	speak("Hello! I'm Reachy, and I'm on a mission to find Travis. Have you seen him around?", openaiKey)
	waveAntenna()

	fmt.Println("\n🔍 Watching and listening continuously...\n")

	lastSpoke := time.Now()
	cooldown := 8 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
			frame, err := videoClient.WaitForFrame(3 * time.Second)
			if err != nil {
				continue
			}

			// Get person position for tracking (cached to reduce API calls)
			var position string
			if time.Since(positionExpiry) > 500*time.Millisecond {
				position = getPersonPosition(ctx, geminiKey, frame)
				cachedPosition = position
				positionExpiry = time.Now()
			} else {
				position = cachedPosition
			}
			updateTracking(position)

			// Check if someone spoke to us
			lastHeardMutex.Lock()
			heard := lastHeard
			lastHeard = "" // Clear it
			lastHeardMutex.Unlock()

			if heard != "" && time.Since(lastSpoke) > cooldown {
				lastSpoke = time.Now()

				fmt.Printf("\n💬 Heard: \"%s\"\n", heard)

				// Add what they said to history
				conversationHist = append(conversationHist, "They said: "+heard)

				// Try to extract their name
				extractName(heard)

				// Generate contextual response
				response := chat(ctx, geminiKey, frame, fmt.Sprintf(
					`The person just said: "%s"

Based on what they said:
- If they confirmed they ARE Travis → be super excited, welcome them!
- If they said they're NOT Travis → acknowledge it, maybe ask their name or what they're up to
- If they mentioned their name → use it! Remember it.
- If they asked you something → answer naturally
- If unclear → ask a follow-up question

Generate your natural spoken response:`, heard))

				// Check if Travis found
				if isTravisConfirmation(ctx, geminiKey, heard) {
					travisFound = true
					happyDance()
					speak(response, openaiKey)
					fmt.Println("\n🎉 TRAVIS FOUND!")
				} else {
					if shouldWiggle(response) {
						waveAntenna()
					}
					speak(response, openaiKey)
				}
			}

			// Occasionally greet if we see someone and haven't spoken recently
			if position != "none" && time.Since(lastSpoke) > 15*time.Second && !travisFound {
				lastSpoke = time.Now()
				greetingVariant++

				// Vary the greeting prompt
				greetingPrompts := []string{
					"You see someone. Make an observation about them or the room, then casually ask if they've seen Travis.",
					"Someone's nearby. Start with a friendly comment about what you observe, then introduce yourself.",
					"You notice a person. Ask what they're up to, then mention you're looking for Travis.",
					"Someone's there! Make a playful observation and see if they want to chat.",
					"You spot a person. Wonder aloud about something you see, then ask if they know Travis.",
				}
				prompt := greetingPrompts[greetingVariant%len(greetingPrompts)]

				greeting := chat(ctx, geminiKey, frame, prompt)
				speak(greeting, openaiKey)
				waveAntenna()
			}

			time.Sleep(200 * time.Millisecond)
		}
	}
}

// continuousListener runs in background, constantly listening
func continuousListener(ctx context.Context, openaiKey string) {
	if openaiKey == "" {
		fmt.Println("⚠️  No OpenAI key - listening disabled")
		return
	}

	fmt.Println("🎤 Continuous listening started")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Don't listen while speaking
			speakingMutex.Lock()
			isSpeaking := speaking
			endTime := speakEndTime
			speakingMutex.Unlock()

			// Wait while speaking OR for 2 seconds after speaking (avoid feedback)
			if isSpeaking || time.Since(endTime) < 2*time.Second {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			// Record 2 seconds of audio (faster response)
			wavFile, err := videoClient.RecordAudio(2 * time.Second)
			if err != nil {
				continue
			}

			// Check if there's actual speech (file size > threshold)
			info, err := os.Stat(wavFile)
			if err != nil || info.Size() < 5000 {
				continue
			}

			// Transcribe
			text := transcribeWithWhisper(openaiKey, wavFile)
			text = strings.TrimSpace(text)

			// Filter out noise/silence transcriptions
			if text != "" && len(text) > 3 && !isNoise(text) {
				lastHeardMutex.Lock()
				lastHeard = text
				lastHeardMutex.Unlock()
				fmt.Printf("\r🎤 [heard: \"%s\"]                    \n", truncate(text, 40))
			}
		}
	}
}

// isNoise filters out common noise transcriptions
func isNoise(text string) bool {
	lower := strings.ToLower(text)
	noisePatterns := []string{
		"thank you",
		"thanks for watching",
		"subscribe",
		"[music]",
		"[applause]",
		"...",
		"hmm",
		"uh",
		"um",
	}
	for _, pattern := range noisePatterns {
		if strings.Contains(lower, pattern) && len(text) < 20 {
			return true
		}
	}
	return false
}

// smoothHeadTracker smoothly moves head to follow target
func smoothHeadTracker(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Smooth interpolation
			diff := targetYaw - currentYaw
			if diff > 0.02 {
				currentYaw += 0.02
			} else if diff < -0.02 {
				currentYaw -= 0.02
			} else {
				currentYaw = targetYaw
			}

			moveHead(0, 0, currentYaw)
		}
	}
}

// getPersonPosition uses vision to detect person position
func getPersonPosition(ctx context.Context, apiKey string, frame []byte) string {
	prompt := `Where is the person in this image? Reply with ONLY one word:
- LEFT (person is on left third)
- CENTER (person is in middle)
- RIGHT (person is on right third)
- NONE (no person visible)

Reply with only the single word.`

	result := callGeminiVision(ctx, apiKey, frame, prompt)
	result = strings.ToUpper(strings.TrimSpace(result))

	// Extract just the position word
	if strings.Contains(result, "LEFT") {
		return "left"
	} else if strings.Contains(result, "RIGHT") {
		return "right"
	} else if strings.Contains(result, "CENTER") {
		return "center"
	}
	return "none"
}

// updateTracking updates target yaw based on person position
func updateTracking(position string) {
	var newTarget float64
	symbol := "·"

	switch position {
	case "left":
		newTarget = 0.35
		symbol = "←"
	case "right":
		newTarget = -0.35
		symbol = "→"
	case "center":
		newTarget = 0
		symbol = "●"
	default:
		// No change if no person
		fmt.Printf("\r👀 Scanning... [head: %.2f]          ", currentYaw)
		return
	}

	targetYaw = newTarget
	fmt.Printf("\r🎯 Tracking: %s [%s] target:%.2f current:%.2f     ", position, symbol, targetYaw, currentYaw)
}

func chat(ctx context.Context, apiKey string, frame []byte, userMessage string) string {
	// Build context with history
	historyContext := ""
	if len(conversationHist) > 0 {
		historyContext = "\n\nRECENT CONVERSATION (don't repeat these):\n"
		// Show last 5 exchanges
		start := 0
		if len(conversationHist) > 10 {
			start = len(conversationHist) - 10
		}
		for _, h := range conversationHist[start:] {
			historyContext += "- " + h + "\n"
		}
	}

	namesContext := ""
	if len(peopleMetNames) > 0 {
		namesContext = "\n\nPeople you've met: " + strings.Join(peopleMetNames, ", ")
	}

	fullPrompt := systemPrompt + historyContext + namesContext + "\n\n" + userMessage

	contents := []map[string]interface{}{
		{
			"parts": []map[string]interface{}{
				{"text": fullPrompt},
			},
		},
	}

	if frame != nil {
		contents[0]["parts"] = append(contents[0]["parts"].([]map[string]interface{}),
			map[string]interface{}{
				"inline_data": map[string]string{
					"mime_type": "image/jpeg",
					"data":      base64.StdEncoding.EncodeToString(frame),
				},
			})
	}

	payload := map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]interface{}{
			"temperature":     1.0, // Higher for more variety
			"maxOutputTokens": 60,
		},
	}

	jsonData, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s?key=%s", geminiAPI, apiKey)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "Hmm, let me think."
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	json.Unmarshal(body, &result)

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		text := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
		text = strings.Trim(text, "\"'")

		// Track what we said
		conversationHist = append(conversationHist, "You said: "+text)

		// Try to extract names from conversation
		extractName(text)

		return text
	}

	return "Well, this is interesting!"
}

// extractName tries to find names mentioned
func extractName(text string) {
	lower := strings.ToLower(text)
	// Look for patterns like "I'm [Name]" or "my name is [Name]"
	patterns := []string{"i'm ", "i am ", "my name is ", "call me "}
	for _, p := range patterns {
		if idx := strings.Index(lower, p); idx != -1 {
			rest := text[idx+len(p):]
			// Get first word after pattern
			parts := strings.Fields(rest)
			if len(parts) > 0 {
				name := strings.Trim(parts[0], ".,!?")
				if len(name) > 1 && name != "not" && name != "travis" {
					// Add if not already known
					found := false
					for _, n := range peopleMetNames {
						if strings.EqualFold(n, name) {
							found = true
							break
						}
					}
					if !found {
						peopleMetNames = append(peopleMetNames, name)
						fmt.Printf("📝 Learned name: %s\n", name)
					}
				}
			}
		}
	}
}

func callGeminiVision(ctx context.Context, apiKey string, frame []byte, prompt string) string {
	b64Image := base64.StdEncoding.EncodeToString(frame)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
					{"inline_data": map[string]string{
						"mime_type": "image/jpeg",
						"data":      b64Image,
					}},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.3,
			"maxOutputTokens": 20,
		},
	}

	jsonData, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s?key=%s", geminiAPI, apiKey)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "NONE"
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	json.Unmarshal(body, &result)

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	}

	return "NONE"
}

func isTravisConfirmation(ctx context.Context, apiKey string, response string) bool {
	prompt := fmt.Sprintf(`Did this person confirm they ARE Travis? "%s"
Reply YES or NO only.`, response)

	result := callGeminiText(ctx, apiKey, prompt)
	return strings.Contains(strings.ToUpper(result), "YES")
}

func callGeminiText(ctx context.Context, apiKey string, prompt string) string {
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]interface{}{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.1,
			"maxOutputTokens": 10,
		},
	}

	jsonData, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s?key=%s", geminiAPI, apiKey)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "NO"
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	json.Unmarshal(body, &result)

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	}

	return "NO"
}

func shouldWiggle(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "hello") || strings.Contains(lower, "hi") ||
		strings.Contains(lower, "nice") || strings.Contains(lower, "great")
}

func transcribeWithWhisper(apiKey, audioFile string) string {
	file, err := os.Open(audioFile)
	if err != nil {
		return ""
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("model", "whisper-1")
	part, _ := writer.CreateFormFile("file", "audio.wav")
	io.Copy(part, file)
	writer.Close()

	req, _ := http.NewRequest("POST", whisperAPI, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result struct {
		Text string `json:"text"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return strings.TrimSpace(result.Text)
}

func speak(text string, openaiKey string) {
	speakingMutex.Lock()
	speaking = true
	speakingMutex.Unlock()

	defer func() {
		speakingMutex.Lock()
		speaking = false
		speakEndTime = time.Now() // Track when we stopped speaking
		speakingMutex.Unlock()
	}()

	fmt.Printf("🗣️  \"%s\"\n", text)

	if openaiKey != "" {
		speakWithOpenAI(text, openaiKey)
	} else {
		speakWithEspeak(text)
	}
}

func speakWithOpenAI(text, apiKey string) {
	payload := map[string]interface{}{
		"model": "tts-1",
		"input": text,
		"voice": "nova",
		"speed": 1.0,
	}

	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", openaiTTSAPI, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	audioData, _ := io.ReadAll(resp.Body)
	tmpFile := "/tmp/speech.mp3"
	os.WriteFile(tmpFile, audioData, 0644)

	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" scp -o StrictHostKeyChecking=no %s %s@%s:/tmp/speech.mp3`,
		sshPass, tmpFile, sshUser, robotIP)).Run()

	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "gst-launch-1.0 filesrc location=/tmp/speech.mp3 ! mpegaudioparse ! mpg123audiodec ! audioconvert ! audioresample ! audio/x-raw,rate=48000 ! opusenc ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 2>/dev/null"`,
		sshPass, sshUser, robotIP)).Run()
}

func speakWithEspeak(text string) {
	text = strings.ReplaceAll(text, "'", "'\\''")
	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "espeak-ng -v en -s 140 '%s' --stdout > /tmp/s.wav 2>/dev/null && gst-launch-1.0 filesrc location=/tmp/s.wav ! wavparse ! audioconvert ! audioresample ! audio/x-raw,rate=48000 ! opusenc ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 2>/dev/null"`,
		sshPass, sshUser, robotIP, text)).Run()
}

func happyDance() {
	for i := 0; i < 4; i++ {
		setAntennas(0.7, -0.7)
		time.Sleep(150 * time.Millisecond)
		setAntennas(-0.7, 0.7)
		time.Sleep(150 * time.Millisecond)
	}
	setAntennas(0, 0)
}

func waveAntenna() {
	go func() {
		for i := 0; i < 2; i++ {
			setAntennas(0.4, -0.4)
			time.Sleep(150 * time.Millisecond)
			setAntennas(-0.4, 0.4)
			time.Sleep(150 * time.Millisecond)
		}
		setAntennas(0, 0)
	}()
}

func setAntennas(left, right float64) {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{"x": 0, "y": 0, "z": 0, "roll": 0, "pitch": 0, "yaw": currentYaw},
		"target_antennas":  []float64{left, right},
		"target_body_yaw":  0,
	}
	data, _ := json.Marshal(payload)
	http.Post(robotAPI+"/api/move/set_target", "application/json", bytes.NewReader(data))
}

func moveHead(roll, pitch, yaw float64) {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{"x": 0, "y": 0, "z": 0, "roll": roll, "pitch": pitch, "yaw": yaw},
		"target_antennas":  []float64{0, 0},
		"target_body_yaw":  0,
	}
	data, _ := json.Marshal(payload)
	http.Post(robotAPI+"/api/move/set_target", "application/json", bytes.NewReader(data))
}

func startDaemon() {
	http.Post(robotAPI+"/api/daemon/start?wake_up=true", "application/json", nil)
}

func setVolume(vol int) {
	http.Post(robotAPI+"/api/volume/set", "application/json", strings.NewReader(fmt.Sprintf(`{"volume":%d}`, vol)))
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
