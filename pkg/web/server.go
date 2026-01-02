// Package web provides a real-time dashboard for Eva
package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/websocket/v2"
	"github.com/teslashibe/go-reachy/pkg/hub"
)

// EvaState represents the current state of Eva for the dashboard
type EvaState struct {
	RobotConnected  bool    `json:"robot_connected"`
	OpenAIConnected bool    `json:"openai_connected"`
	WebRTCConnected bool    `json:"webrtc_connected"`
	Speaking        bool    `json:"speaking"`
	Listening       bool    `json:"listening"`
	HeadYaw         float64 `json:"head_yaw"`
	FacePosition    float64 `json:"face_position"` // 0-100%
	ActiveTimer     string  `json:"active_timer"`
	LastUserMessage string  `json:"last_user_message"`
	LastEvaMessage  string  `json:"last_eva_message"`
}

// LogEntry represents a log line for the dashboard
type LogEntry struct {
	Time    string `json:"time"`
	Type    string `json:"type"` // info, tool, speech, error, face
	Message string `json:"message"`
}

// ConversationEntry represents a message in the conversation
type ConversationEntry struct {
	Time    string `json:"time"`
	Role    string `json:"role"` // user, eva, tool
	Message string `json:"message"`
}

// Server is the web dashboard server
type Server struct {
	app  *fiber.App
	port string

	// State
	state   EvaState
	stateMu sync.RWMutex

	// Log buffer (last 500 entries)
	logs   []LogEntry
	logsMu sync.RWMutex

	// Conversation buffer
	conversation   []ConversationEntry
	conversationMu sync.RWMutex

	// Hubs for websocket broadcast (thread-safe!)
	statusHub *hub.Hub
	logHub    *hub.Hub
	cameraHub *hub.Hub

	// Tool trigger callback
	OnToolTrigger func(name string, args map[string]interface{}) (string, error)

	// Frame capture callback
	OnCaptureFrame func() ([]byte, error)
}

// NewServer creates a new web dashboard server
func NewServer(port string) *Server {
	s := &Server{
		port:         port,
		logs:         make([]LogEntry, 0, 500),
		conversation: make([]ConversationEntry, 0, 100),
		statusHub:    hub.New("status"),
		logHub:       hub.New("logs"),
		cameraHub:    hub.New("camera"),
	}

	app := fiber.New(fiber.Config{
		AppName:               "Eva Dashboard",
		DisableStartupMessage: true,
	})

	// CORS for local development
	app.Use(cors.New())

	// Static files
	app.Static("/", "./web")

	// API routes
	api := app.Group("/api")
	api.Get("/status", s.handleStatus)
	api.Get("/tools", s.handleListTools)
	api.Post("/tools/:name", s.handleTriggerTool)
	api.Get("/logs", s.handleGetLogs)
	api.Get("/conversation", s.handleGetConversation)

	// WebSocket upgrade middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// WebSocket routes
	app.Get("/ws/logs", websocket.New(s.handleLogsWS))
	app.Get("/ws/camera", websocket.New(s.handleCameraWS))
	app.Get("/ws/status", websocket.New(s.handleStatusWS))

	s.app = app
	return s
}

// Start starts the web server
func (s *Server) Start() error {
	fmt.Printf("🌐 Web dashboard: http://localhost:%s\n", s.port)

	// Start all hubs
	go s.statusHub.Run()
	go s.logHub.Run()
	go s.cameraHub.Run()

	return s.app.Listen(":" + s.port)
}

// StartAsync starts the web server in a goroutine
func (s *Server) StartAsync() {
	go func() {
		if err := s.Start(); err != nil {
			fmt.Printf("⚠️  Web server error: %v\n", err)
		}
	}()
}

// UpdateState updates Eva's state and broadcasts to clients
func (s *Server) UpdateState(update func(*EvaState)) {
	s.stateMu.Lock()
	update(&s.state)
	state := s.state // Copy for broadcast
	s.stateMu.Unlock()

	// Broadcast via hub (thread-safe!)
	s.statusHub.BroadcastJSON(state)
}

// AddLog adds a log entry and broadcasts to clients
func (s *Server) AddLog(logType, message string) {
	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Type:    logType,
		Message: message,
	}

	s.logsMu.Lock()
	s.logs = append(s.logs, entry)
	if len(s.logs) > 500 {
		s.logs = s.logs[1:]
	}
	s.logsMu.Unlock()

	// Broadcast via hub (thread-safe!)
	s.logHub.BroadcastJSON(entry)
}

// AddConversation adds a conversation entry
func (s *Server) AddConversation(role, message string) {
	entry := ConversationEntry{
		Time:    time.Now().Format("15:04:05"),
		Role:    role,
		Message: message,
	}

	s.conversationMu.Lock()
	s.conversation = append(s.conversation, entry)
	if len(s.conversation) > 100 {
		s.conversation = s.conversation[1:]
	}
	s.conversationMu.Unlock()
}

// SendCameraFrame sends a camera frame to all connected clients
func (s *Server) SendCameraFrame(jpegData []byte) {
	// Broadcast via hub (thread-safe!)
	s.cameraHub.BroadcastBinary(jpegData)
}

// GetStatusHub returns the status hub for external use
func (s *Server) GetStatusHub() *hub.Hub {
	return s.statusHub
}

// GetLogHub returns the log hub for external use
func (s *Server) GetLogHub() *hub.Hub {
	return s.logHub
}

// GetCameraHub returns the camera hub for external use
func (s *Server) GetCameraHub() *hub.Hub {
	return s.cameraHub
}

// Shutdown gracefully stops the web server
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
