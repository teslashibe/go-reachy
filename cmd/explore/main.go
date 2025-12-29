// Explore - Reachy looks around and describes what he sees
//
// Combines: Head movement + Vision + Speech
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
)

const robotIP = "192.168.68.80"
const robotAPI = "http://192.168.68.80:8000"
const sshPass = "root"
const sshUser = "pollen"
const geminiAPI = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

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

	// Start daemon
	fmt.Print("🤖 Waking up... ")
	startDaemon()
	fmt.Println("✅")
	time.Sleep(2 * time.Second)

	// Speak introduction
	speak("Hello! Let me look around and tell you what I see.")
	time.Sleep(2 * time.Second)

	// Exploration loop
	observations := []string{}

	// Look left
	fmt.Println("\n👈 Looking left...")
	moveHead(0, 0, 0, 0.4) // Turn head left
	time.Sleep(1 * time.Second)
	
	frame, _ := captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "What do you see on the left side?")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "On my left: "+desc)
		}
	}

	// Look center
	fmt.Println("\n👀 Looking center...")
	moveHead(0, 0, 0, 0) // Center
	time.Sleep(1 * time.Second)
	
	frame, _ = captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "What do you see in front of you?")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "In front of me: "+desc)
		}
	}

	// Look right
	fmt.Println("\n👉 Looking right...")
	moveHead(0, 0, 0, -0.4) // Turn head right
	time.Sleep(1 * time.Second)
	
	frame, _ = captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "What do you see on the right side?")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "On my right: "+desc)
		}
	}

	// Look up
	fmt.Println("\n👆 Looking up...")
	moveHead(0, 0, 0.05, 0) // Tilt up
	time.Sleep(1 * time.Second)
	
	frame, _ = captureFrame()
	if frame != nil {
		desc, err := analyzeWithGemini(ctx, apiKey, frame, "What do you see above?")
		if err == nil {
			fmt.Printf("   I see: %s\n", desc)
			observations = append(observations, "Above: "+desc)
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

// speak uses espeak-ng on the robot
func speak(text string) {
	// Escape quotes for shell
	text = strings.ReplaceAll(text, "'", "'\\''")
	text = strings.ReplaceAll(text, "\"", "\\\"")
	
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "espeak-ng -v en -s 140 -p 50 '%s' 2>/dev/null"`,
		sshPass, sshUser, robotIP, text))
	cmd.Run()
}

// captureFrame grabs a JPEG from the camera
func captureFrame() ([]byte, error) {
	tmpFile := "/tmp/reachy_explore.jpg"
	
	// Trigger capture
	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 %s@%s "pkill -f gst-launch 2>/dev/null; nohup gst-launch-1.0 -q libcamerasrc ! video/x-raw,width=640,height=480 ! videoconvert ! jpegenc quality=80 ! filesink location=/tmp/cam.jpg >/dev/null 2>&1 & sleep 1; pkill -f gst-launch" 2>/dev/null`,
		sshPass, sshUser, robotIP)).Run()
	
	time.Sleep(500 * time.Millisecond)
	
	// Fetch via SCP
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" scp -o StrictHostKeyChecking=no %s@%s:/tmp/cam.jpg %s 2>/dev/null || sshpass -p "%s" scp -o StrictHostKeyChecking=no %s@%s:/tmp/frame.jpg %s`,
		sshPass, sshUser, robotIP, tmpFile,
		sshPass, sshUser, robotIP, tmpFile))
	
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	
	return os.ReadFile(tmpFile)
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

