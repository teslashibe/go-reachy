// Video - WebRTC video client for Reachy Mini camera
//
// Connects to the robot's WebRTC stream and captures frames.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const robotIP = "192.168.68.80"
const streamPort = "5000"

func main() {
	fmt.Println("📹 Reachy Mini Video Stream MVP")
	fmt.Println("================================")
	fmt.Printf("Robot: %s\n\n", robotIP)

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the stream on the robot first
	fmt.Println("Starting video stream on robot...")
	fmt.Println("Run this on the robot first:")
	fmt.Println()
	fmt.Printf("  gst-launch-1.0 libcamerasrc ! video/x-raw,width=640,height=480,framerate=15/1 ! \\\n")
	fmt.Printf("    videoconvert ! jpegenc quality=50 ! \\\n")
	fmt.Printf("    tcpserversink host=0.0.0.0 port=%s\n", streamPort)
	fmt.Println()
	fmt.Println("Press Enter when stream is running...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	// Connect to stream
	fmt.Printf("Connecting to %s:%s...\n", robotIP, streamPort)
	conn, err := net.DialTimeout("tcp", robotIP+":"+streamPort, 5*time.Second)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		fmt.Println("\nMake sure the GStreamer pipeline is running on the robot!")
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("✅ Connected!")

	// Read frames
	frameCount := 0
	startTime := time.Now()
	buffer := make([]byte, 1024*1024) // 1MB buffer

	go func() {
		<-sigChan
		fmt.Printf("\n\n📊 Stats: %d frames in %.1fs (%.1f fps)\n",
			frameCount, time.Since(startTime).Seconds(),
			float64(frameCount)/time.Since(startTime).Seconds())
		os.Exit(0)
	}()

	fmt.Println("\n🎬 Receiving frames (Ctrl+C to stop)...")

	// Simple frame reader - look for JPEG markers
	reader := bufio.NewReader(conn)
	for {
		// Read data
		n, err := reader.Read(buffer)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Stream ended")
				break
			}
			fmt.Printf("Read error: %v\n", err)
			break
		}

		// Look for JPEG start marker (FFD8)
		if n > 2 && buffer[0] == 0xFF && buffer[1] == 0xD8 {
			frameCount++

			// Try to decode to get dimensions
			img, err := jpeg.Decode(bytes.NewReader(buffer[:n]))
			if err == nil {
				bounds := img.Bounds()
				fmt.Printf("\r📷 Frame %d: %dx%d (%d bytes)     ",
					frameCount, bounds.Dx(), bounds.Dy(), n)
			} else {
				fmt.Printf("\r📷 Frame %d: %d bytes     ", frameCount, n)
			}

			// Save first frame as proof
			if frameCount == 1 {
				os.WriteFile("frame_001.jpg", buffer[:n], 0644)
				fmt.Println("\n✅ Saved first frame to frame_001.jpg")
			}
		}
	}
}
