// Reachy - Basic robot controller
//
// Deprecated: Use cmd/eva for the main conversational agent.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/teslashibe/go-reachy/pkg/robot"
)

func main() {
	// Command line flags
	robotIP := flag.String("robot", "", "Robot IP address (or set ROBOT_IP env)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Get robot IP from env if not provided
	ip := *robotIP
	if ip == "" {
		ip = os.Getenv("ROBOT_IP")
	}
	if ip == "" {
		log.Fatal("Robot IP required: use -robot flag or set ROBOT_IP env")
	}

	fmt.Println("🤖 Reachy Mini Go Controller (deprecated - use cmd/eva)")
	fmt.Printf("   Robot: %s\n", ip)
	fmt.Printf("   Debug: %v\n", *debug)
	fmt.Println()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n👋 Shutting down...")
		cancel()
	}()

	// Connect to robot
	reachy, err := robot.Connect(ctx, ip, *debug)
	if err != nil {
		log.Fatalf("Failed to connect to robot: %v", err)
	}
	defer reachy.Close()

	fmt.Println("✅ Connected to Reachy Mini!")

	// Run the control loop
	if err := reachy.Run(ctx); err != nil {
		log.Printf("Robot control ended: %v", err)
	}

	fmt.Println("👋 Goodbye!")
}
