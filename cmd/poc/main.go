// POC - Proof of concept for Reachy Mini control
//
// Deprecated: Use cmd/eva for the main conversational agent.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/teslashibe/go-reachy/internal/config"
)

var robotIP = config.RobotIP("192.168.68.80")

func main() {
	fmt.Println("🤖 Reachy Mini Go PoC")
	fmt.Println("====================")
	fmt.Printf("Robot IP: %s\n\n", robotIP)

	baseURL := config.RobotAPIURL(robotIP)

	// Step 1: Check if robot is reachable
	fmt.Print("1. Checking robot connection... ")
	status, err := getDaemonStatus(baseURL)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		fmt.Println("\n⚠️  Make sure the robot is powered on and connected to WiFi!")
		os.Exit(1)
	}
	fmt.Printf("✅ Connected!\n")
	fmt.Printf("   State: %v\n", status["state"])
	fmt.Printf("   Version: %v\n", status["version"])

	// Step 2: Start daemon if not running
	state, _ := status["state"].(string)
	if state != "running" {
		fmt.Print("\n2. Starting robot daemon... ")
		if err := startDaemon(baseURL); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Started!")

		// Wait for it to initialize
		fmt.Print("   Waiting for initialization... ")
		time.Sleep(8 * time.Second)
		fmt.Println("done")
	} else {
		fmt.Println("\n2. Daemon already running ✅")
	}

	// Step 3: Get current state
	fmt.Print("\n3. Getting robot state... ")
	state2, err := getDaemonStatus(baseURL)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Println("✅")
		if bs, ok := state2["backend_status"].(map[string]interface{}); ok {
			fmt.Printf("   Motor control: %v\n", bs["motor_control_mode"])
			if stats, ok := bs["control_loop_stats"].(map[string]interface{}); ok {
				fmt.Printf("   Control frequency: %.1f Hz\n", stats["mean_control_loop_frequency"])
			}
		}
	}

	// Step 4: Try to play an emotion
	fmt.Print("\n4. Playing 'happy' emotion... ")
	if err := playEmotion(baseURL, "happy"); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		fmt.Println("   (This might not be implemented in the HTTP API)")
	} else {
		fmt.Println("✅ Sent!")
	}

	// Step 5: Try to move the head
	fmt.Print("\n5. Moving head... ")
	if err := moveHead(baseURL, 0.0, 0.0, 0.05, 0.0); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Println("✅ Command sent!")
	}

	time.Sleep(1 * time.Second)

	// Move back
	fmt.Print("   Moving head back... ")
	if err := moveHead(baseURL, 0.0, 0.0, 0.0, 0.0); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Println("✅")
	}

	fmt.Println("\n" + repeatStr("=", 40))
	fmt.Println("🎉 PoC Complete! Go can control Reachy!")
	fmt.Println(repeatStr("=", 40))
}

func getDaemonStatus(baseURL string) (map[string]interface{}, error) {
	resp, err := http.Get(baseURL + "/api/daemon/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var status map[string]interface{}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}

	return status, nil
}

func startDaemon(baseURL string) error {
	resp, err := http.Post(baseURL+"/api/daemon/start?wake_up=true", "application/json", nil)
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

func playEmotion(baseURL string, emotion string) error {
	// Try the move API endpoint for emotions
	cmd := map[string]interface{}{
		"emotion": emotion,
	}
	data, _ := json.Marshal(cmd)

	resp, err := http.Post(baseURL+"/api/move/emotion", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func moveHead(baseURL string, x, y, z, yaw float64) error {
	cmd := map[string]interface{}{
		"x":   x,
		"y":   y,
		"z":   z,
		"yaw": yaw,
	}
	data, _ := json.Marshal(cmd)

	resp, err := http.Post(baseURL+"/api/move/head", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept 404 as "endpoint doesn't exist yet" which is ok for PoC
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Helper to repeat a string
func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
