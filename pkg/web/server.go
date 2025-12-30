// Package web provides a real-time dashboard for Eva
package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/websocket/v2"
)

// EvaState represents the current state of Eva for the dashboard
type EvaState struct {
	RobotConnected   bool    `json:"robot_connected"`
	OpenAIConnected  bool    `json:"openai_connected"`
	WebRTCConnected  bool    `json:"webrtc_connected"`
	Speaking         bool    `json:"speaking"`
	Listening        bool    `json:"listening"`
	HeadYaw          float64 `json:"head_yaw"`
	FacePosition     float64 `json:"face_position"` // 0-100%
	ActiveTimer      string  `json:"active_timer"`
	LastUserMessage  string  `json:"last_user_message"`
	LastEvaMessage   string  `json:"last_eva_message"`
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

	// WebSocket clients
	logClients      map[*websocket.Conn]bool
	logClientsMu    sync.RWMutex
	cameraClients   map[*websocket.Conn]bool
	cameraClientsMu sync.RWMutex
	statusClients   map[*websocket.Conn]bool
	statusClientsMu sync.RWMutex

	// Camera frame channel
	cameraFrameChan chan []byte

	// Tool trigger callback
	OnToolTrigger func(name string, args map[string]interface{}) (string, error)

	// Frame capture callback
	OnCaptureFrame func() ([]byte, error)
}

// NewServer creates a new web dashboard server
func NewServer(port string) *Server {
	s := &Server{
		port:            port,
		logs:            make([]LogEntry, 0, 500),
		conversation:    make([]ConversationEntry, 0, 100),
		logClients:      make(map[*websocket.Conn]bool),
		cameraClients:   make(map[*websocket.Conn]bool),
		statusClients:   make(map[*websocket.Conn]bool),
		cameraFrameChan: make(chan []byte, 5),
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
	go s.cameraBroadcaster()
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
	s.stateMu.Unlock()

	// Broadcast to status clients
	s.broadcastStatus()
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

	// Broadcast to log clients
	s.broadcastLog(entry)
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
	select {
	case s.cameraFrameChan <- jpegData:
	default:
		// Drop frame if channel is full
	}
}

// cameraBroadcaster broadcasts camera frames to all clients
func (s *Server) cameraBroadcaster() {
	for frame := range s.cameraFrameChan {
		s.cameraClientsMu.RLock()
		for client := range s.cameraClients {
			if err := client.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				client.Close()
				go func(c *websocket.Conn) {
					s.cameraClientsMu.Lock()
					delete(s.cameraClients, c)
					s.cameraClientsMu.Unlock()
				}(client)
			}
		}
		s.cameraClientsMu.RUnlock()
	}
}

// broadcastLog sends a log entry to all connected log clients
func (s *Server) broadcastLog(entry LogEntry) {
	s.logClientsMu.RLock()
	defer s.logClientsMu.RUnlock()

	for client := range s.logClients {
		if err := client.WriteJSON(entry); err != nil {
			client.Close()
			go func(c *websocket.Conn) {
				s.logClientsMu.Lock()
				delete(s.logClients, c)
				s.logClientsMu.Unlock()
			}(client)
		}
	}
}

// broadcastStatus sends status update to all connected status clients
func (s *Server) broadcastStatus() {
	s.stateMu.RLock()
	state := s.state
	s.stateMu.RUnlock()

	s.statusClientsMu.RLock()
	defer s.statusClientsMu.RUnlock()

	for client := range s.statusClients {
		if err := client.WriteJSON(state); err != nil {
			client.Close()
			go func(c *websocket.Conn) {
				s.statusClientsMu.Lock()
				delete(s.statusClients, c)
				s.statusClientsMu.Unlock()
			}(client)
		}
	}
}

