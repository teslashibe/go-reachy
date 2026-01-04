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

	// Hubs for websocket broadcast (thread-safe!)
	statusHub *hub.Hub
	logHub    *hub.Hub
	cameraHub *hub.Hub

	// Tool trigger callback
	OnToolTrigger func(name string, args map[string]interface{}) (string, error)

	// Frame capture callback
	OnCaptureFrame func() ([]byte, error)

	// Tuning callbacks (for tracking parameter adjustment)
	OnGetTuningParams func() interface{}
	OnSetTuningParams func(params map[string]interface{})
	OnSetTuningMode   func(enabled bool)

	// Camera callbacks (for camera configuration)
	OnGetCameraConfig func() interface{}
	OnSetCameraConfig func(params map[string]interface{}) error

	// Spark callbacks (for Google Docs integration)
	OnSparkGetStatus    func() interface{}
	OnSparkAuthStart    func() string // Returns auth URL
	OnSparkAuthCallback func(code string) error
	OnSparkDisconnect   func() error
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

	// Tuning API routes
	api.Get("/tracking/params", s.handleGetTuningParams)
	api.Post("/tracking/params", s.handleSetTuningParams)
	api.Post("/tracking/tuning-mode", s.handleSetTuningMode)

	// Camera API routes
	api.Get("/camera/config", s.handleGetCameraConfig)
	api.Post("/camera/config", s.handleSetCameraConfig)
	api.Get("/camera/presets", s.handleGetCameraPresets)
	api.Get("/camera/capabilities", s.handleGetCameraCapabilities)

	// Spark API routes (Google Docs integration)
	api.Get("/spark/status", s.handleSparkStatus)
	api.Get("/spark/auth", s.handleSparkAuthStart)
	api.Get("/spark/callback", s.handleSparkCallback)
	api.Post("/spark/disconnect", s.handleSparkDisconnect)

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

// handleGetTuningParams returns current tracking parameters
func (s *Server) handleGetTuningParams(c *fiber.Ctx) error {
	if s.OnGetTuningParams == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Tuning not available (tracker not connected)",
		})
	}
	return c.JSON(s.OnGetTuningParams())
}

// handleSetTuningParams updates tracking parameters
func (s *Server) handleSetTuningParams(c *fiber.Ctx) error {
	if s.OnSetTuningParams == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Tuning not available (tracker not connected)",
		})
	}

	var params map[string]interface{}
	if err := c.BodyParser(&params); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid JSON: " + err.Error(),
		})
	}

	s.OnSetTuningParams(params)
	return c.JSON(fiber.Map{
		"status": "ok",
		"updated": params,
	})
}

// handleSetTuningMode enables/disables tuning mode
func (s *Server) handleSetTuningMode(c *fiber.Ctx) error {
	if s.OnSetTuningMode == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Tuning not available (tracker not connected)",
		})
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid JSON: " + err.Error(),
		})
	}

	s.OnSetTuningMode(req.Enabled)

	mode := "normal"
	if req.Enabled {
		mode = "tuning (secondary features disabled)"
	}
	return c.JSON(fiber.Map{
		"status": "ok",
		"mode":   mode,
	})
}

// handleGetCameraConfig returns current camera configuration
func (s *Server) handleGetCameraConfig(c *fiber.Ctx) error {
	if s.OnGetCameraConfig == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Camera config not available",
		})
	}
	return c.JSON(s.OnGetCameraConfig())
}

// handleSetCameraConfig updates camera configuration
func (s *Server) handleSetCameraConfig(c *fiber.Ctx) error {
	if s.OnSetCameraConfig == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Camera config not available",
		})
	}

	var params map[string]interface{}
	if err := c.BodyParser(&params); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid JSON: " + err.Error(),
		})
	}

	if err := s.OnSetCameraConfig(params); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "ok",
		"updated": params,
	})
}

// handleGetCameraPresets returns available camera presets
func (s *Server) handleGetCameraPresets(c *fiber.Ctx) error {
	// Import camera package presets
	presets := []string{
		"default", "legacy", "720p", "1080p", "4k",
		"night", "bright", "zoom2x", "zoom4x",
	}
	return c.JSON(fiber.Map{
		"presets": presets,
	})
}

// handleGetCameraCapabilities returns camera sensor capabilities
func (s *Server) handleGetCameraCapabilities(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"sensor":           "imx708_wide",
		"max_width":        4608,
		"max_height":       2592,
		"max_gain":         16.0,
		"max_exposure_us":  120000,
		"max_zoom":         4.0,
		"exposure_modes":   []string{"normal", "short", "long"},
		"constraint_modes": []string{"normal", "highlight", "shadows"},
		"af_modes":         []string{"manual", "auto", "continuous"},
	})
}

// Spark API handlers

// handleSparkStatus returns the Google Docs connection status
func (s *Server) handleSparkStatus(c *fiber.Ctx) error {
	if s.OnSparkGetStatus == nil {
		return c.JSON(fiber.Map{
			"connected": false,
			"error":     "Spark not configured",
		})
	}
	return c.JSON(s.OnSparkGetStatus())
}

// handleSparkAuthStart redirects to Google OAuth
func (s *Server) handleSparkAuthStart(c *fiber.Ctx) error {
	if s.OnSparkAuthStart == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Spark not configured",
		})
	}
	authURL := s.OnSparkAuthStart()
	return c.Redirect(authURL, fiber.StatusTemporaryRedirect)
}

// handleSparkCallback processes the OAuth callback
func (s *Server) handleSparkCallback(c *fiber.Ctx) error {
	if s.OnSparkAuthCallback == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Spark not configured",
		})
	}

	code := c.Query("code")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing authorization code",
		})
	}

	if err := s.OnSparkAuthCallback(code); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head><title>Spark - Error</title></head>
<body style="font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f44336; color: white;">
    <div style="text-align: center;">
        <h1>❌ Connection Failed</h1>
        <p>%s</p>
        <p><small>Please close this window and try again.</small></p>
    </div>
</body>
</html>
`, err.Error()))
	}

	// Success page
	return c.SendString(`
<!DOCTYPE html>
<html>
<head>
    <title>Spark - Connected!</title>
    <style>
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex; 
            justify-content: center; 
            align-items: center; 
            height: 100vh; 
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }
        .container { 
            text-align: center; 
            padding: 40px;
            background: rgba(255,255,255,0.1);
            border-radius: 16px;
            backdrop-filter: blur(10px);
        }
        h1 { margin-bottom: 10px; }
        p { opacity: 0.9; }
        .emoji { font-size: 48px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="emoji">🔥</div>
        <h1>Spark Connected!</h1>
        <p>Your ideas will now sync to Google Docs.</p>
        <p><small>You can close this window.</small></p>
    </div>
    <script>
        setTimeout(function() { window.close(); }, 3000);
    </script>
</body>
</html>
`)
}

// handleSparkDisconnect disconnects from Google
func (s *Server) handleSparkDisconnect(c *fiber.Ctx) error {
	if s.OnSparkDisconnect == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Spark not configured",
		})
	}

	if err := s.OnSparkDisconnect(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"success": true})
}
