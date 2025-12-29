// Explore - Reachy looks around and describes what he sees
//
// Combines: Head movement + Vision + Speech (with WebRTC video)
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/video"
)

var videoClient *video.Client

const robotIP = "192.168.68.80"
const robotAPI = "http://192.168.68.80:8000"
const sshPass = "root"
const sshUser = "pollen"
const geminiAPI = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
const openaiTTSAPI = "https://api.openai.com/v1/audio/speech"

func main() {
	fmt.Println("🔭 Reachy Mini Explorer")
	fmt.Println("=======================")
	fmt.Println("I'll look around and tell you what I see!\n")

	// Check API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ Set GEMINI_API_KEY first!")
		os.Exit(1)
	}

	// Handle Ctrl+C
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n👋 Stopping exploration...")
		resetPosition()
		cancel()
		os.Exit(0)
	}()

	// Start daemon and set volume to max
	fmt.Print("🤖 Waking up... ")
	startDaemon()
	setVolume(100)
	fmt.Println("✅")
	time.Sleep(2 * time.Second)

	// Connect to video stream via WebRTC
	fmt.Println("\n📹 Connecting to video stream...")
	videoClient = video.NewClient(robotIP)
	if err := videoClient.Connect(); err != nil {
		fmt.Printf("⚠️  Video connection failed: %v\n", err)
		fmt.Println("   Will try SSH fallback for frames...")
	} else {
		defer videoClient.Close()
	}

	// Speak introduction
	speak("Hello! Let me look around and tell you what I see.")
	time.Sleep(2 * time.Second)

	// Exploration loop
	observations := []string{}

	// Look straight ahead first (where humans usually are!)
	fmt.Println("\n👀 Looking straight ahead...")
	moveHead(0, 0, -0.02, 0) // Slightly down to see person in front
	time.Sleep(1 * time.Second)

	frame, _ := captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "Describe any people you see, or what's directly in front of you.")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "In front of me: "+desc)
		}
	}

	// Look left
	fmt.Println("\n👈 Looking left...")
	moveHead(0, 0, -0.02, 0.5) // Turn head left, still slightly down
	time.Sleep(1 * time.Second)

	frame, _ = captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "What do you see on the left? Any people?")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "On my left: "+desc)
		}
	}

	// Look right
	fmt.Println("\n👉 Looking right...")
	moveHead(0, 0, -0.02, -0.5) // Turn head right
	time.Sleep(1 * time.Second)

	frame, _ = captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "What do you see on the right? Any people?")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "On my right: "+desc)
		}
	}

	// Look at the person again (center)
	fmt.Println("\n🧑 Looking for you...")
	moveHead(0, 0, -0.03, 0) // Center, slightly down for human height
	time.Sleep(1 * time.Second)

	frame, _ = captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "Focus on any person in front of you. Describe them briefly.")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "Looking at you: "+desc)
		}
	}

	// Back to center
	fmt.Println("\n🎯 Returning to center...")
	moveHead(0, 0, 0, 0)
	time.Sleep(1 * time.Second)

	// Generate summary
	fmt.Println("\n📝 Generating summary...")
	summary := generateSummary(ctx, apiKey, observations)

	// Speak the summary
	fmt.Println("\n🗣️  Speaking summary...")
	fmt.Println("╭───────────────────────────────────────────")
	fmt.Printf("│ %s\n", summary)
	fmt.Println("╰───────────────────────────────────────────")

	speak(summary)

	// Wave antennas happily
	waveAntennas()

	fmt.Println("\n✅ Exploration complete!")
}

// moveHead sets head position
func moveHead(x, y, z, yaw float64) {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{
			"x": x, "y": y, "z": z,
			"roll": 0, "pitch": 0, "yaw": yaw,
		},
		"target_antennas": []float64{0, 0},
		"target_body_yaw": 0,
		"duration":        0.5,
	}

	data, _ := json.Marshal(payload)
	http.Post(robotAPI+"/api/move/set_target", "application/json", bytes.NewReader(data))
}

// waveAntennas does a happy wave
func waveAntennas() {
	for i := 0; i < 3; i++ {
		payload := map[string]interface{}{
			"target_head_pose": map[string]float64{"x": 0, "y": 0, "z": 0, "roll": 0, "pitch": 0, "yaw": 0},
			"target_antennas":  []float64{0.5, -0.5},
			"target_body_yaw":  0,
		}
		data, _ := json.Marshal(payload)
		http.Post(robotAPI+"/api/move/set_target", "application/json", bytes.NewReader(data))
		time.Sleep(300 * time.Millisecond)

		payload["target_antennas"] = []float64{-0.5, 0.5}
		data, _ = json.Marshal(payload)
		http.Post(robotAPI+"/api/move/set_target", "application/json", bytes.NewReader(data))
		time.Sleep(300 * time.Millisecond)
	}

	// Reset
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{"x": 0, "y": 0, "z": 0, "roll": 0, "pitch": 0, "yaw": 0},
		"target_antennas":  []float64{0, 0},
		"target_body_yaw":  0,
	}
	data, _ := json.Marshal(payload)
	http.Post(robotAPI+"/api/move/set_target", "application/json", bytes.NewReader(data))
}

// resetPosition resets robot to center
func resetPosition() {
	moveHead(0, 0, 0, 0)
}

// startDaemon wakes up the robot
func startDaemon() {
	http.Post(robotAPI+"/api/daemon/start?wake_up=true", "application/json", nil)
}

// setVolume sets the speaker volume (0-100)
func setVolume(vol int) {
	payload := fmt.Sprintf(`{"volume": %d}`, vol)
	http.Post(robotAPI+"/api/volume/set", "application/json", strings.NewReader(payload))
}

// speak uses OpenAI TTS (natural voice) or falls back to espeak-ng
func speak(text string) {
	openaiKey := os.Getenv("OPENAI_API_KEY")

	if openaiKey != "" {
		// Use OpenAI TTS for natural voice
		speakWithOpenAI(text, openaiKey)
	} else {
		// Fallback to espeak-ng (robotic but works)
		speakWithEspeak(text)
	}
}

// speakWithOpenAI uses OpenAI's TTS API for natural speech
func speakWithOpenAI(text, apiKey string) {
	// Request MP3 from OpenAI TTS
	payload := map[string]interface{}{
		"model": "tts-1",
		"input": text,
		"voice": "nova", // Options: alloy, echo, fable, onyx, nova, shimmer
		"speed": 1.0,
	}

	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", openaiTTSAPI, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   ⚠️  TTS error: %v, using espeak\n", err)
		speakWithEspeak(text)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("   ⚠️  TTS API error %d: %s\n", resp.StatusCode, string(body))
		speakWithEspeak(text)
		return
	}

	// Save MP3 to temp file
	audioData, _ := io.ReadAll(resp.Body)
	tmpFile := "/tmp/reachy_speech.mp3"
	os.WriteFile(tmpFile, audioData, 0644)

	// Copy to robot and play via GStreamer
	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" scp -o StrictHostKeyChecking=no %s %s@%s:/tmp/speech.mp3`,
		sshPass, tmpFile, sshUser, robotIP)).Run()

	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "gst-launch-1.0 filesrc location=/tmp/speech.mp3 ! mpegaudioparse ! mpg123audiodec ! audioconvert ! audioresample ! audio/x-raw,rate=48000 ! opusenc ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 2>/dev/null"`,
		sshPass, sshUser, robotIP)).Run()
}

// speakWithEspeak uses local espeak-ng (robotic but fast)
func speakWithEspeak(text string) {
	text = strings.ReplaceAll(text, "'", "'\\''")
	text = strings.ReplaceAll(text, "\"", "\\\"")

	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "espeak-ng -v en -s 140 '%s' --stdout > /tmp/speech.wav 2>/dev/null && gst-launch-1.0 filesrc location=/tmp/speech.wav ! wavparse ! audioconvert ! audioresample ! audio/x-raw,rate=48000 ! opusenc ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 2>/dev/null"`,
		sshPass, sshUser, robotIP, text)).Run()
}

// captureFrame gets a frame from the WebRTC video stream
func captureFrame() ([]byte, error) {
	// Try WebRTC client first
	if videoClient != nil {
		frame, err := videoClient.WaitForFrame(3 * time.Second)
		if err == nil && len(frame) > 5000 {
			return frame, nil
		}
	}

	// Fallback to SSH/Python method
	tmpFile := "/tmp/reachy_explore.jpg"

	captureCmd := fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=15 %s@%s 'source /venvs/mini_daemon/bin/activate && timeout 20 python /tmp/capture_frame.py'`,
		sshPass, sshUser, robotIP)

	cmd := exec.Command("bash", "-c", captureCmd)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "OK") {
		return nil, fmt.Errorf("capture failed: %s", string(output))
	}

	scpCmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" scp -o StrictHostKeyChecking=no %s@%s:/tmp/frame_live.jpg %s`,
		sshPass, sshUser, robotIP, tmpFile))

	if err := scpCmd.Run(); err != nil {
		return nil, fmt.Errorf("SCP failed: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, err
	}

	if len(data) < 5000 {
		return nil, fmt.Errorf("frame too small: %d bytes", len(data))
	}

	return data, nil
}

// analyzeWithGemini sends image to Gemini for description
func analyzeWithGemini(ctx context.Context, apiKey string, imageData []byte, question string) (string, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageData)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": "You are Reachy, a friendly robot. " + question + " Answer in one short sentence (max 15 words). Be friendly and conversational."},
					{"inline_data": map[string]string{"mime_type": "image/jpeg", "data": b64Image}},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 60,
		},
	}

	jsonData, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s?key=%s", geminiAPI, apiKey)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
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
		return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
	}

	return "I'm not sure what I see", nil
}

// generateSummary creates a spoken summary from observations
func generateSummary(ctx context.Context, apiKey string, observations []string) string {
	if len(observations) == 0 {
		return "I couldn't see anything clearly, but I enjoyed looking around!"
	}

	prompt := "You are Reachy, a friendly robot. Summarize what you saw in 2-3 short sentences for speaking aloud. Be friendly and conversational. Here's what you observed:\n\n"
	for _, obs := range observations {
		prompt += "- " + obs + "\n"
	}

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]interface{}{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 100,
		},
	}

	jsonData, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s?key=%s", geminiAPI, apiKey)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "I saw some interesting things around me!"
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

	return "I saw some interesting things around me!"
}
