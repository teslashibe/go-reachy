// Vision MVP - Gemini Flash vision for Reachy Mini
//
// Captures frames and sends to Gemini for understanding.
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
	"syscall"
	"time"
)

const robotIP = "192.168.68.80"
const sshPass = "root"
const sshUser = "pollen"

// Gemini API endpoint
const geminiAPI = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

func main() {
	fmt.Println("👁️  Reachy Mini Vision MVP")
	fmt.Println("==========================")
	fmt.Printf("Robot: %s\n", robotIP)

	// Check for API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		fmt.Println("\n❌ GEMINI_API_KEY not set!")
		fmt.Println("   Get one at: https://aistudio.google.com/apikey")
		fmt.Println("   Then: export GEMINI_API_KEY=your-key")
		os.Exit(1)
	}
	fmt.Println("API Key: ✅\n")

	// Handle Ctrl+C
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n👋 Goodbye!")
		cancel()
	}()

	// Main loop
	fmt.Println("🔄 Starting vision loop (Ctrl+C to stop)\n")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Capture frame
			fmt.Print("📷 Capturing... ")
			frame, err := captureFrame()
			if err != nil {
				fmt.Printf("❌ %v\n", err)
				time.Sleep(2 * time.Second)
				continue
			}
			fmt.Printf("✅ (%d KB)\n", len(frame)/1024)

			// Send to Gemini
			fmt.Print("🧠 Analyzing... ")
			description, err := analyzeWithGemini(ctx, apiKey, frame)
			if err != nil {
				fmt.Printf("❌ %v\n", err)
				time.Sleep(2 * time.Second)
				continue
			}

			// Print what Reachy sees
			fmt.Println("✅")
			fmt.Println("╭───────────────────────────────────────────")
			fmt.Printf("│ 👁️  I see: %s\n", description)
			fmt.Println("╰───────────────────────────────────────────\n")

			// Wait before next capture
			time.Sleep(3 * time.Second)
		}
	}
}

// captureFrame gets a JPEG from the robot's camera
func captureFrame() ([]byte, error) {
	// Try to get existing frame via SCP
	tmpFile := "/tmp/reachy_vision.jpg"

	// First trigger a new capture
	exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 %s@%s "pkill -f gst-launch 2>/dev/null; nohup gst-launch-1.0 -q libcamerasrc ! video/x-raw,width=640,height=480 ! videoconvert ! jpegenc quality=80 ! filesink location=/tmp/cam.jpg >/dev/null 2>&1 & sleep 1; pkill -f gst-launch" 2>/dev/null`,
		sshPass, sshUser, robotIP)).Run()

	time.Sleep(500 * time.Millisecond)

	// Fetch frame via SCP
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" scp -o StrictHostKeyChecking=no -o ConnectTimeout=2 %s@%s:/tmp/cam.jpg %s 2>/dev/null || sshpass -p "%s" scp -o StrictHostKeyChecking=no %s@%s:/tmp/frame.jpg %s`,
		sshPass, sshUser, robotIP, tmpFile,
		sshPass, sshUser, robotIP, tmpFile))

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("SCP failed")
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read failed")
	}

	if len(data) < 1000 {
		return nil, fmt.Errorf("frame too small")
	}

	return data, nil
}

// analyzeWithGemini sends the image to Gemini Flash for analysis
func analyzeWithGemini(ctx context.Context, apiKey string, imageData []byte) (string, error) {
	// Encode image as base64
	b64Image := base64.StdEncoding.EncodeToString(imageData)

	// Build request payload
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": "You are Reachy, a friendly robot. Describe what you see in this image in one short sentence (max 20 words). Be conversational and friendly. Focus on people and interesting objects.",
					},
					{
						"inline_data": map[string]string{
							"mime_type": "image/jpeg",
							"data":      b64Image,
						},
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 100,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Make request
	url := fmt.Sprintf("%s?key=%s", geminiAPI, apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "I couldn't understand what I'm seeing", nil
}
