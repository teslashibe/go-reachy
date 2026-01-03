// Package cloud provides WebSocket hub for robot connections
package cloud

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/teslashibe/go-reachy/pkg/protocol"
)

// RobotConnection represents a connected robot
type RobotConnection struct {
	ID        string
	Conn      *websocket.Conn
	Connected time.Time
	LastSeen  time.Time

	mu sync.Mutex
}

// Send sends a message to the robot
func (r *RobotConnection) Send(msg *protocol.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := msg.Bytes()
	if err != nil {
		return err
	}

	return r.Conn.WriteMessage(websocket.TextMessage, data)
}

// Hub manages WebSocket connections from robots
type Hub struct {
	mu      sync.RWMutex
	robots  map[string]*RobotConnection
	debug   bool

	// Callbacks
	onFrame    func(robotID string, frame *protocol.FrameData)
	onDOA      func(robotID string, doa *protocol.DOAData)
	onMic      func(robotID string, mic *protocol.MicData)
	onState    func(robotID string, state *protocol.StateData)

	// Stats
	messagesReceived atomic.Uint64
	messagesSent     atomic.Uint64
	framesReceived   atomic.Uint64
}

// NewHub creates a new robot hub
func NewHub(debug bool) *Hub {
	return &Hub{
		robots: make(map[string]*RobotConnection),
		debug:  debug,
	}
}

// OnFrame sets the callback for incoming video frames
func (h *Hub) OnFrame(callback func(robotID string, frame *protocol.FrameData)) {
	h.mu.Lock()
	h.onFrame = callback
	h.mu.Unlock()
}

// OnDOA sets the callback for incoming DOA data
func (h *Hub) OnDOA(callback func(robotID string, doa *protocol.DOAData)) {
	h.mu.Lock()
	h.onDOA = callback
	h.mu.Unlock()
}

// OnMic sets the callback for incoming microphone data
func (h *Hub) OnMic(callback func(robotID string, mic *protocol.MicData)) {
	h.mu.Lock()
	h.onMic = callback
	h.mu.Unlock()
}

// OnState sets the callback for incoming robot state
func (h *Hub) OnState(callback func(robotID string, state *protocol.StateData)) {
	h.mu.Lock()
	h.onState = callback
	h.mu.Unlock()
}

// RegisterRoutes registers WebSocket routes on a Fiber app
func (h *Hub) RegisterRoutes(app *fiber.App) {
	// WebSocket upgrade middleware
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// Robot connection endpoint
	app.Get("/ws/robot", websocket.New(h.handleRobot))
	app.Get("/ws/robot/:id", websocket.New(h.handleRobot))
}

// handleRobot handles a robot WebSocket connection
func (h *Hub) handleRobot(c *websocket.Conn) {
	// Get robot ID from path or generate one
	robotID := c.Params("id")
	if robotID == "" {
		robotID = generateRobotID()
	}

	robot := &RobotConnection{
		ID:        robotID,
		Conn:      c,
		Connected: time.Now(),
		LastSeen:  time.Now(),
	}

	// Register robot
	h.mu.Lock()
	h.robots[robotID] = robot
	robotCount := len(h.robots)
	h.mu.Unlock()

	if h.debug {
		log.Printf("🤖 Robot connected: %s (total: %d)", robotID, robotCount)
	}

	defer func() {
		h.mu.Lock()
		delete(h.robots, robotID)
		robotCount := len(h.robots)
		h.mu.Unlock()

		if h.debug {
			log.Printf("🤖 Robot disconnected: %s (total: %d)", robotID, robotCount)
		}
	}()

	// Read loop
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			if h.debug {
				log.Printf("⚠️  Robot %s read error: %v", robotID, err)
			}
			return
		}

		robot.mu.Lock()
		robot.LastSeen = time.Now()
		robot.mu.Unlock()

		h.messagesReceived.Add(1)
		h.handleMessage(robotID, data)
	}
}

// handleMessage processes an incoming message from a robot
func (h *Hub) handleMessage(robotID string, data []byte) {
	msg, err := protocol.ParseMessage(data)
	if err != nil {
		if h.debug {
			log.Printf("⚠️  Parse error from %s: %v", robotID, err)
		}
		return
	}

	h.mu.RLock()
	frameCb := h.onFrame
	doaCb := h.onDOA
	micCb := h.onMic
	stateCb := h.onState
	h.mu.RUnlock()

	switch msg.Type {
	case protocol.TypeFrame:
		h.framesReceived.Add(1)
		if frameCb != nil {
			frame, err := msg.GetFrameData()
			if err == nil {
				frameCb(robotID, frame)
			}
		}

	case protocol.TypeDOA:
		if doaCb != nil {
			doa, err := msg.GetDOAData()
			if err == nil {
				doaCb(robotID, doa)
			}
		}

	case protocol.TypeMic:
		if micCb != nil {
			mic, err := msg.GetMicData()
			if err == nil {
				micCb(robotID, mic)
			}
		}

	case protocol.TypeState:
		if stateCb != nil {
			state, err := msg.GetStateData()
			if err == nil {
				stateCb(robotID, state)
			}
		}

	case protocol.TypePing:
		// Respond with pong
		h.SendPong(robotID, msg.Timestamp)
	}
}

// SendMotorCommand sends a motor command to a robot
func (h *Hub) SendMotorCommand(robotID string, head protocol.HeadTarget, antennas [2]float64, bodyYaw float64) error {
	msg, err := protocol.NewMotorMessage(head, antennas, bodyYaw)
	if err != nil {
		return err
	}
	return h.sendToRobot(robotID, msg)
}

// SendEmotion sends an emotion command to a robot
func (h *Hub) SendEmotion(robotID string, name string, duration float64) error {
	msg, err := protocol.NewEmotionMessage(name, duration)
	if err != nil {
		return err
	}
	return h.sendToRobot(robotID, msg)
}

// SendSpeak sends TTS audio to a robot
func (h *Hub) SendSpeak(robotID string, audioData []byte, format string, sampleRate int) error {
	msg, err := protocol.NewSpeakMessage(audioData, format, sampleRate)
	if err != nil {
		return err
	}
	return h.sendToRobot(robotID, msg)
}

// SendConfig sends a configuration update to a robot
func (h *Hub) SendConfig(robotID string, camera *protocol.CameraConfig, audio *protocol.AudioConfig) error {
	msg, err := protocol.NewConfigMessage(camera, audio)
	if err != nil {
		return err
	}
	return h.sendToRobot(robotID, msg)
}

// SendPong sends a pong response to a robot
func (h *Hub) SendPong(robotID string, pingTS int64) error {
	msg, err := protocol.NewPongMessage("", pingTS, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	return h.sendToRobot(robotID, msg)
}

// sendToRobot sends a message to a specific robot
func (h *Hub) sendToRobot(robotID string, msg *protocol.Message) error {
	h.mu.RLock()
	robot, ok := h.robots[robotID]
	h.mu.RUnlock()

	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "robot not connected")
	}

	h.messagesSent.Add(1)
	return robot.Send(msg)
}

// Broadcast sends a message to all connected robots
func (h *Hub) Broadcast(msg *protocol.Message) {
	h.mu.RLock()
	robots := make([]*RobotConnection, 0, len(h.robots))
	for _, r := range h.robots {
		robots = append(robots, r)
	}
	h.mu.RUnlock()

	for _, robot := range robots {
		h.messagesSent.Add(1)
		if err := robot.Send(msg); err != nil {
			if h.debug {
				log.Printf("⚠️  Broadcast error to %s: %v", robot.ID, err)
			}
		}
	}
}

// GetRobot returns a robot connection by ID
func (h *Hub) GetRobot(robotID string) *RobotConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.robots[robotID]
}

// GetRobots returns all connected robots
func (h *Hub) GetRobots() []*RobotConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()

	robots := make([]*RobotConnection, 0, len(h.robots))
	for _, r := range h.robots {
		robots = append(robots, r)
	}
	return robots
}

// RobotCount returns the number of connected robots
func (h *Hub) RobotCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.robots)
}

// Stats contains hub statistics
type Stats struct {
	RobotCount       int    `json:"robot_count"`
	MessagesReceived uint64 `json:"messages_received"`
	MessagesSent     uint64 `json:"messages_sent"`
	FramesReceived   uint64 `json:"frames_received"`
}

// GetStats returns hub statistics
func (h *Hub) GetStats() Stats {
	return Stats{
		RobotCount:       h.RobotCount(),
		MessagesReceived: h.messagesReceived.Load(),
		MessagesSent:     h.messagesSent.Load(),
		FramesReceived:   h.framesReceived.Load(),
	}
}

// RobotInfo contains info about a connected robot
type RobotInfo struct {
	ID        string    `json:"id"`
	Connected time.Time `json:"connected"`
	LastSeen  time.Time `json:"last_seen"`
}

// GetRobotInfos returns info about all connected robots
func (h *Hub) GetRobotInfos() []RobotInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	infos := make([]RobotInfo, 0, len(h.robots))
	for _, r := range h.robots {
		r.mu.Lock()
		infos = append(infos, RobotInfo{
			ID:        r.ID,
			Connected: r.Connected,
			LastSeen:  r.LastSeen,
		})
		r.mu.Unlock()
	}
	return infos
}

// RegisterAPIRoutes registers API routes for robot management
func (h *Hub) RegisterAPIRoutes(api fiber.Router) {
	robots := api.Group("/robots")

	// List connected robots
	robots.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"robots": h.GetRobotInfos(),
			"count":  h.RobotCount(),
		})
	})

	// Get hub stats
	robots.Get("/stats", func(c *fiber.Ctx) error {
		return c.JSON(h.GetStats())
	})

	// Send motor command to robot
	robots.Post("/:id/motor", func(c *fiber.Ctx) error {
		robotID := c.Params("id")

		var cmd struct {
			Head     protocol.HeadTarget `json:"head"`
			Antennas [2]float64          `json:"antennas"`
			BodyYaw  float64             `json:"body_yaw"`
		}
		if err := c.BodyParser(&cmd); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		if err := h.SendMotorCommand(robotID, cmd.Head, cmd.Antennas, cmd.BodyYaw); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"status": "sent"})
	})

	// Send emotion to robot
	robots.Post("/:id/emotion", func(c *fiber.Ctx) error {
		robotID := c.Params("id")

		var cmd struct {
			Name     string  `json:"name"`
			Duration float64 `json:"duration"`
		}
		if err := c.BodyParser(&cmd); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		if err := h.SendEmotion(robotID, cmd.Name, cmd.Duration); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"status": "sent"})
	})
}

// generateRobotID generates a unique robot ID
func generateRobotID() string {
	return time.Now().Format("20060102150405")
}

