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
	"github.com/teslashibe/go-reachy/pkg/speech"
)

func main() {
	// Command line flags
	robotIP := flag.String("robot", "192.168.68.75", "Robot IP address")
	openaiKey := flag.String("openai-key", "", "OpenAI API key (or set OPENAI_API_KEY env)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Get OpenAI key from env if not provided
	apiKey := *openaiKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		log.Println("⚠️  No OpenAI API key provided - speech disabled")
	}

	fmt.Println("🤖 Reachy Mini Go Controller")
	fmt.Printf("   Robot: %s:7447\n", *robotIP)
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
	reachy, err := robot.Connect(ctx, *robotIP, *debug)
	if err != nil {
		log.Fatalf("Failed to connect to robot: %v", err)
	}
	defer reachy.Close()

	fmt.Println("✅ Connected to Reachy Mini!")

	// Start speech handler if API key provided
	if apiKey != "" {
		speechHandler := speech.NewHandler(apiKey, reachy)
		go speechHandler.Run(ctx)
		fmt.Println("🎤 Speech enabled - start talking!")
	}

	// Run the control loop
	if err := reachy.Run(ctx); err != nil {
		log.Printf("Robot control ended: %v", err)
	}

	fmt.Println("👋 Goodbye!")
}

