// Dance - Reachy Mini dance demo with antenna movements
//
// Makes the robot perform a dance routine with head and antenna animations.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/internal/config"
)

var robotIP = config.RobotIP("192.168.68.80")

func main() {
	fmt.Println("💃 Reachy Mini Dance Demo")
	fmt.Println("========================")
	fmt.Printf("Robot: %s\n\n", robotIP)

	baseURL := config.RobotAPIURL(robotIP)

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\n👋 Stopping dance, resetting position...")
		setTarget(baseURL, 0, 0, 0, 0, 0, 0, 0)
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()

	// Check connection
	fmt.Print("Checking connection... ")
	if err := checkConnection(baseURL); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Make sure daemon is running
	fmt.Print("Starting daemon... ")
	startDaemon(baseURL)
	time.Sleep(2 * time.Second)
	fmt.Println("✅")

	fmt.Println("\n🎵 Let's dance! (Ctrl+C to stop)")

	// Dance loop!
	danceRoutine(baseURL)
}

// setTarget sends a movement command to the robot
func setTarget(baseURL string, headX, headY, headZ, headYaw, leftAnt, rightAnt, bodyYaw float64) error {
	// FullBodyTarget format from OpenAPI schema
	cmd := map[string]interface{}{
		"target_head_pose": map[string]float64{
			"x":     headX,
			"y":     headY,
			"z":     headZ,
			"roll":  0,
			"pitch": 0,
			"yaw":   headYaw,
		},
		"target_antennas": []float64{leftAnt, rightAnt},
		"target_body_yaw": bodyYaw,
	}

	data, _ := json.Marshal(cmd)
	resp, err := http.Post(baseURL+"/api/move/set_target", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func checkConnection(baseURL string) error {
	resp, err := http.Get(baseURL + "/api/daemon/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func startDaemon(baseURL string) error {
	resp, err := http.Post(baseURL+"/api/daemon/start?wake_up=true", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// danceRoutine performs a fun dance!
func danceRoutine(baseURL string) {
	startTime := time.Now()
	frameRate := 30 * time.Millisecond

	moves := []string{
		"🎵 Head bob...",
		"🎵 Antenna wave...",
		"🎵 Body twist...",
		"🎵 Full groove...",
	}
	moveIndex := 0

	for {
		t := time.Since(startTime).Seconds()

		// Change move every 4 seconds
		newMoveIndex := int(t/4) % len(moves)
		if newMoveIndex != moveIndex {
			moveIndex = newMoveIndex
			fmt.Printf("\r%s          ", moves[moveIndex])
		}

		var headX, headY, headZ, headYaw, leftAnt, rightAnt, bodyYaw float64

		switch moveIndex {
		case 0: // Head bob
			headZ = 0.02 * math.Sin(t*4)
			headY = 0.01 * math.Sin(t*2)

		case 1: // Antenna wave
			leftAnt = 0.5 * math.Sin(t*3)
			rightAnt = 0.5 * math.Sin(t*3+math.Pi) // Opposite phase

		case 2: // Body twist
			bodyYaw = 0.3 * math.Sin(t*2)
			headYaw = -0.2 * math.Sin(t*2) // Counter-rotate head

		case 3: // Full groove - combine everything!
			headZ = 0.02 * math.Sin(t*4)
			headY = 0.01 * math.Cos(t*3)
			leftAnt = 0.4 * math.Sin(t*5)
			rightAnt = 0.4 * math.Sin(t*5+math.Pi/2)
			bodyYaw = 0.2 * math.Sin(t*2)
			headYaw = 0.15 * math.Cos(t*3)
		}

		if err := setTarget(baseURL, headX, headY, headZ, headYaw, leftAnt, rightAnt, bodyYaw); err != nil {
			fmt.Printf("\rError: %v", err)
		}

		time.Sleep(frameRate)
	}
}
