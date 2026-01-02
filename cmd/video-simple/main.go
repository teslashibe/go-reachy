package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const robotIP = "192.168.68.80"

func main() {
	fmt.Println("📹 Reachy Mini Video MVP (Simple)")
	fmt.Println("==================================")
	fmt.Printf("Robot: %s\n\n", robotIP)

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n👋 Goodbye!")
		os.Exit(0)
	}()

	// Step 1: Capture a single frame via SSH + GStreamer
	fmt.Println("📷 Capturing frame from robot camera...")

	// Use SSH to run GStreamer and capture a frame
	cmd := exec.Command("sshpass", "-p", "root",
		"ssh", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("pollen@%s", robotIP),
		`timeout 5 gst-launch-1.0 libcamerasrc ! 'video/x-raw,width=640,height=480' ! videoconvert ! jpegenc ! filesink location=/tmp/frame.jpg 2>/dev/null && cat /tmp/frame.jpg`,
	)

	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("❌ Failed to capture: %v\n", err)
		fmt.Println("\nTrying alternative method...")

		// Alternative: use libcamera-jpeg if available
		cmd2 := exec.Command("sshpass", "-p", "root",
			"ssh", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("pollen@%s", robotIP),
			"libcamera-jpeg -o /tmp/frame.jpg --width 640 --height 480 -t 1 2>/dev/null && cat /tmp/frame.jpg",
		)
		output, err = cmd2.Output()
		if err != nil {
			fmt.Printf("❌ Alternative also failed: %v\n", err)
			os.Exit(1)
		}
	}

	if len(output) < 100 {
		fmt.Println("❌ No image data received")
		os.Exit(1)
	}

	// Save the frame
	err = os.WriteFile("reachy_frame.jpg", output, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to save: %v\n", err)
		os.Exit(1)
	}

	// Decode to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(output))
	if err != nil {
		fmt.Printf("⚠️  Saved %d bytes but couldn't decode JPEG: %v\n", len(output), err)
	} else {
		bounds := img.Bounds()
		fmt.Printf("✅ Captured %dx%d frame (%d bytes)\n", bounds.Dx(), bounds.Dy(), len(output))
	}

	fmt.Println("✅ Saved to: reachy_frame.jpg")

	// Step 2: Continuous capture loop
	fmt.Println("\n🎬 Starting continuous capture (Ctrl+C to stop)...")
	fmt.Println("   Frames will be saved to reachy_frame.jpg")

	frameCount := 0
	startTime := time.Now()

	for {
		// Capture frame via HTTP if there's a simple endpoint, or via SSH
		frame, err := captureFrame(robotIP)
		if err != nil {
			fmt.Printf("\r⚠️  Frame error: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		frameCount++
		elapsed := time.Since(startTime).Seconds()
		fps := float64(frameCount) / elapsed

		// Save latest frame
		os.WriteFile("reachy_frame.jpg", frame, 0644)

		fmt.Printf("\r📷 Frame %d | %.1f fps | %d bytes     ", frameCount, fps, len(frame))

		// Don't hammer the robot too fast
		time.Sleep(200 * time.Millisecond)
	}
}

func captureFrame(ip string) ([]byte, error) {
	// First try: check if there's an HTTP endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s:8000/api/camera/frame", ip))
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	// Fallback: SSH + GStreamer
	cmd := exec.Command("sshpass", "-p", "root",
		"ssh", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("pollen@%s", ip),
		`gst-launch-1.0 -q libcamerasrc ! 'video/x-raw,width=320,height=240' ! videoconvert ! jpegenc ! fdsink 2>/dev/null`,
	)

	return cmd.Output()
}
