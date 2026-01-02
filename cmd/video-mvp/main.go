// Video MVP - Simple frame capture from Reachy Mini camera
//
// Captures frames using SSH + GStreamer on the robot.
package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/internal/config"
)

var (
	robotIP = config.RobotIP("192.168.68.80")
	sshUser = config.SSHUser()
	sshPass = config.SSHPass()
)

func main() {
	fmt.Println("📹 Reachy Mini Video MVP")
	fmt.Println("========================")
	fmt.Printf("Robot: %s\n\n", robotIP)

	// Handle Ctrl+C
	done := make(chan bool)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		done <- true
	}()

	// First, trigger a capture on the robot
	fmt.Print("📷 Triggering camera capture... ")
	if err := triggerCapture(); err != nil {
		fmt.Printf("⚠️  %v (using existing frame)\n", err)
	} else {
		fmt.Println("✅")
	}

	// Fetch the frame via SCP
	fmt.Print("📥 Fetching frame... ")
	frame, err := fetchFrame()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	// Decode to verify
	img, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil {
		fmt.Printf("❌ Invalid JPEG: %v\n", err)
		os.Exit(1)
	}
	bounds := img.Bounds()
	fmt.Printf("✅ %dx%d (%d KB)\n", bounds.Dx(), bounds.Dy(), len(frame)/1024)

	// Save it
	os.WriteFile("frame_latest.jpg", frame, 0644)
	fmt.Println("💾 Saved to: frame_latest.jpg")

	// Continuous mode
	fmt.Println("\n🎬 Continuous mode (Ctrl+C to stop)")
	fmt.Println("   Updating frame_latest.jpg...")

	frameCount := 1
	startTime := time.Now()

	for {
		select {
		case <-done:
			elapsed := time.Since(startTime).Seconds()
			fmt.Printf("\n\n📊 %d frames in %.1fs (%.2f fps)\n",
				frameCount, elapsed, float64(frameCount)/elapsed)
			return
		default:
			// Trigger new capture
			triggerCapture()
			time.Sleep(500 * time.Millisecond) // Wait for capture

			// Fetch frame
			frame, err := fetchFrame()
			if err != nil {
				fmt.Printf("\r⚠️  Fetch error        ")
				time.Sleep(time.Second)
				continue
			}

			frameCount++
			os.WriteFile("frame_latest.jpg", frame, 0644)

			elapsed := time.Since(startTime).Seconds()
			fps := float64(frameCount) / elapsed
			fmt.Printf("\r📷 Frame %d | %.2f fps | %d KB    ",
				frameCount, fps, len(frame)/1024)

			time.Sleep(300 * time.Millisecond)
		}
	}
}

// triggerCapture runs GStreamer on robot to capture a frame
func triggerCapture() error {
	// Simple command to capture one frame
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 %s@%s "pkill -f gst-launch 2>/dev/null; nohup gst-launch-1.0 -q libcamerasrc ! video/x-raw,width=640,height=480 ! videoconvert ! jpegenc ! filesink location=/tmp/cam.jpg >/dev/null 2>&1 & sleep 1; pkill -f gst-launch"`,
		sshPass, sshUser, robotIP))

	return cmd.Run()
}

// fetchFrame copies the captured frame from robot
func fetchFrame() ([]byte, error) {
	// Create temp file
	tmpFile := "/tmp/reachy_frame.jpg"

	// SCP the frame
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" scp -o StrictHostKeyChecking=no -o ConnectTimeout=2 %s@%s:/tmp/cam.jpg %s 2>/dev/null || sshpass -p "%s" scp -o StrictHostKeyChecking=no %s@%s:/tmp/frame.jpg %s`,
		sshPass, sshUser, robotIP, tmpFile,
		sshPass, sshUser, robotIP, tmpFile))

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("SCP failed: %v", err)
	}

	// Read the file
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read failed: %v", err)
	}

	if len(data) < 1000 {
		return nil, fmt.Errorf("frame too small")
	}

	return data, nil
}
