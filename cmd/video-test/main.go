// Video test - measure WebRTC frame rate
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/internal/config"
	"github.com/teslashibe/go-reachy/pkg/video"
)

var robotIP = config.RobotIP("192.168.68.80")

func main() {
	fmt.Println("📹 WebRTC Video FPS Test")
	fmt.Println("========================")
	fmt.Printf("Robot: %s\n\n", robotIP)

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Connect to video
	client := video.NewClient(robotIP)
	if err := client.Connect(); err != nil {
		fmt.Printf("❌ Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("\n🎬 Measuring frame rate (Ctrl+C to stop)...")

	frameCount := 0
	startTime := time.Now()
	lastReport := time.Now()
	lastFrame := []byte{}

	go func() {
		<-sigChan
		elapsed := time.Since(startTime).Seconds()
		fmt.Printf("\n\n📊 Final: %d frames in %.1fs = %.2f fps\n",
			frameCount, elapsed, float64(frameCount)/elapsed)
		os.Exit(0)
	}()

	for {
		frame, err := client.GetFrame()
		if err == nil && len(frame) > 0 {
			// Check if it's a new frame (different from last)
			if len(frame) != len(lastFrame) {
				frameCount++
				lastFrame = frame

				// Save first frame
				if frameCount == 1 {
					os.WriteFile("test_frame.jpg", frame, 0644)
					fmt.Printf("💾 First frame saved: test_frame.jpg (%d bytes)\n", len(frame))
				}
			}
		}

		// Report every second
		if time.Since(lastReport) >= time.Second {
			elapsed := time.Since(startTime).Seconds()
			fps := float64(frameCount) / elapsed
			fmt.Printf("\r📷 Frames: %d | FPS: %.2f | Last size: %d bytes    ",
				frameCount, fps, len(lastFrame))
			lastReport = time.Now()
		}

		time.Sleep(10 * time.Millisecond)
	}
}
