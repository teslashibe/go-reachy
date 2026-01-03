// eva-cloud: Cloud service for Eva robot fleet management
// Accepts WebSocket connections from robots and provides AI/tracking
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/teslashibe/go-reachy/pkg/cloud"
	"github.com/teslashibe/go-reachy/pkg/protocol"
)

var (
	version = "1.0.0"
	port    = flag.Int("port", 8080, "HTTP server port")
	debug   = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	// Override from environment
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", port)
	}

	fmt.Println()
	fmt.Println("☁️  Eva Cloud v" + version)
	fmt.Println("   Robot fleet management service")
	fmt.Println()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:               "eva-cloud",
		DisableStartupMessage: true,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))
	if *debug {
		app.Use(logger.New())
	}

	// Create robot hub
	hub := cloud.NewHub(*debug)

	// Register WebSocket routes
	hub.RegisterRoutes(app)

	// Register API routes
	api := app.Group("/api")
	hub.RegisterAPIRoutes(api)

	// Health endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"version": version,
			"robots":  hub.RobotCount(),
		})
	})

	// Metrics endpoint
	app.Get("/metrics", func(c *fiber.Ctx) error {
		stats := hub.GetStats()
		return c.SendString(fmt.Sprintf(`# HELP eva_cloud_robots Connected robot count
# TYPE eva_cloud_robots gauge
eva_cloud_robots %d

# HELP eva_cloud_messages_received Total messages received
# TYPE eva_cloud_messages_received counter
eva_cloud_messages_received %d

# HELP eva_cloud_messages_sent Total messages sent
# TYPE eva_cloud_messages_sent counter
eva_cloud_messages_sent %d

# HELP eva_cloud_frames_received Total video frames received
# TYPE eva_cloud_frames_received counter
eva_cloud_frames_received %d
`, stats.RobotCount, stats.MessagesReceived, stats.MessagesSent, stats.FramesReceived))
	})

	// Set up frame callback for processing
	hub.OnFrame(func(robotID string, frame *protocol.FrameData) {
		if *debug {
			log.Printf("📹 Frame from %s: %dx%d", robotID, frame.Width, frame.Height)
		}
		// TODO: Add face detection and tracking here
		// For now, just log frames received
	})

	// Set up DOA callback
	hub.OnDOA(func(robotID string, doa *protocol.DOAData) {
		if *debug {
			log.Printf("🎤 DOA from %s: angle=%.2f speaking=%v", robotID, doa.Angle, doa.Speaking)
		}
		// TODO: Use for audio-based tracking
	})

	// Start server
	go func() {
		addr := fmt.Sprintf(":%d", *port)
		log.Printf("🚀 Starting server on %s", addr)
		log.Printf("   WebSocket: ws://localhost:%d/ws/robot", *port)
		log.Printf("   Health:    http://localhost:%d/health", *port)
		log.Printf("   Robots:    http://localhost:%d/api/robots", *port)
		log.Println()

		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("\n👋 Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("✅ Goodbye!")
}

