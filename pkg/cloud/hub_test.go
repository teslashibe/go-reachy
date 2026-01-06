package cloud

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/teslashibe/go-reachy/pkg/protocol"
)

func TestNewHub(t *testing.T) {
	hub := NewHub(false)

	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	if hub.RobotCount() != 0 {
		t.Error("RobotCount should be 0 initially")
	}
}

func TestGetStats(t *testing.T) {
	hub := NewHub(false)

	stats := hub.GetStats()

	if stats.RobotCount != 0 {
		t.Error("RobotCount should be 0")
	}
	if stats.MessagesReceived != 0 {
		t.Error("MessagesReceived should be 0")
	}
	if stats.MessagesSent != 0 {
		t.Error("MessagesSent should be 0")
	}
}

func TestCallbackSetters(t *testing.T) {
	hub := NewHub(false)

	// Set all callbacks - should not panic
	hub.OnFrame(func(robotID string, frame *protocol.FrameData) {})
	hub.OnDOA(func(robotID string, doa *protocol.DOAData) {})
	hub.OnMic(func(robotID string, mic *protocol.MicData) {})
	hub.OnState(func(robotID string, state *protocol.StateData) {})
}

func TestGetRobotNotFound(t *testing.T) {
	hub := NewHub(false)

	robot := hub.GetRobot("nonexistent")
	if robot != nil {
		t.Error("GetRobot should return nil for nonexistent robot")
	}
}

func TestGetRobots(t *testing.T) {
	hub := NewHub(false)

	robots := hub.GetRobots()
	if len(robots) != 0 {
		t.Error("GetRobots should return empty slice initially")
	}
}

func TestGetRobotInfos(t *testing.T) {
	hub := NewHub(false)

	infos := hub.GetRobotInfos()
	if len(infos) != 0 {
		t.Error("GetRobotInfos should return empty slice initially")
	}
}

func TestGenerateRobotID(t *testing.T) {
	id := generateRobotID()

	if id == "" {
		t.Error("generateRobotID should return non-empty string")
	}

	if len(id) < 10 {
		t.Error("Robot ID should be reasonably long")
	}
}

func setupTestServer(hub *Hub) (*fiber.App, string) {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)
	hub.RegisterAPIRoutes(app.Group("/api"))

	// Start test server
	go app.Listen(":0")
	time.Sleep(100 * time.Millisecond)

	return app, ""
}

func TestRegisterRoutes(t *testing.T) {
	hub := NewHub(false)
	app := fiber.New()

	// Should not panic
	hub.RegisterRoutes(app)
	hub.RegisterAPIRoutes(app.Group("/api"))
}

func TestWebSocketConnection(t *testing.T) {
	hub := NewHub(true)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)

	// Start server
	go app.Listen(":18080")
	defer app.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect WebSocket
	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:18080/ws/robot/test-robot", nil)
	if err != nil {
		t.Fatalf("WebSocket dial error: %v", err)
	}
	defer ws.Close()

	// Wait for connection to be registered
	time.Sleep(50 * time.Millisecond)

	if hub.RobotCount() != 1 {
		t.Errorf("RobotCount = %d, want 1", hub.RobotCount())
	}

	robot := hub.GetRobot("test-robot")
	if robot == nil {
		t.Error("GetRobot should return the connected robot")
	}

	// Close and verify disconnect
	ws.Close()
	time.Sleep(100 * time.Millisecond)

	if hub.RobotCount() != 0 {
		t.Errorf("RobotCount = %d, want 0 after disconnect", hub.RobotCount())
	}
}

func TestFrameCallback(t *testing.T) {
	hub := NewHub(false)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)

	var frameReceived atomic.Bool
	var receivedRobotID string

	hub.OnFrame(func(robotID string, frame *protocol.FrameData) {
		receivedRobotID = robotID
		frameReceived.Store(true)
	})

	go app.Listen(":18081")
	defer app.Shutdown()
	time.Sleep(100 * time.Millisecond)

	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:18081/ws/robot/frame-test", nil)
	if err != nil {
		t.Fatalf("WebSocket dial error: %v", err)
	}
	defer ws.Close()

	// Send a frame message
	msg, _ := protocol.NewFrameMessage(640, 480, []byte("test"), 1)
	data, _ := msg.Bytes()
	ws.WriteMessage(websocket.TextMessage, data)

	time.Sleep(100 * time.Millisecond)

	if !frameReceived.Load() {
		t.Error("Frame callback should have been called")
	}

	if receivedRobotID != "frame-test" {
		t.Errorf("Robot ID = %s, want frame-test", receivedRobotID)
	}

	stats := hub.GetStats()
	if stats.FramesReceived < 1 {
		t.Error("FramesReceived should be at least 1")
	}
}

func TestSendMotorCommand(t *testing.T) {
	hub := NewHub(false)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)

	go app.Listen(":18082")
	defer app.Shutdown()
	time.Sleep(100 * time.Millisecond)

	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:18082/ws/robot/motor-test", nil)
	if err != nil {
		t.Fatalf("WebSocket dial error: %v", err)
	}
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Send motor command
	head := protocol.HeadTarget{X: 0.1, Y: 0.2}
	err = hub.SendMotorCommand("motor-test", head, [2]float64{0.5, 0.5}, 0.1)
	if err != nil {
		t.Fatalf("SendMotorCommand error: %v", err)
	}

	// Read the message
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	var msg protocol.Message
	json.Unmarshal(data, &msg)

	if msg.Type != protocol.TypeMotor {
		t.Errorf("Type = %s, want motor", msg.Type)
	}
}

func TestSendToNonexistentRobot(t *testing.T) {
	hub := NewHub(false)

	err := hub.SendMotorCommand("nonexistent", protocol.HeadTarget{}, [2]float64{}, 0)
	if err == nil {
		t.Error("SendMotorCommand should return error for nonexistent robot")
	}
}

func TestAPIListRobots(t *testing.T) {
	hub := NewHub(false)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)
	hub.RegisterAPIRoutes(app.Group("/api"))

	req := httptest.NewRequest("GET", "/api/robots/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "robots") {
		t.Error("Response should contain 'robots' field")
	}
}

func TestAPIStats(t *testing.T) {
	hub := NewHub(false)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)
	hub.RegisterAPIRoutes(app.Group("/api"))

	req := httptest.NewRequest("GET", "/api/robots/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}
}

func TestBroadcast(t *testing.T) {
	hub := NewHub(false)

	// Broadcast to empty hub should not panic
	msg, _ := protocol.NewMessage(protocol.TypePing, nil)
	hub.Broadcast(msg)
}

func TestPingPong(t *testing.T) {
	hub := NewHub(false)
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	hub.RegisterRoutes(app)

	go app.Listen(":18083")
	defer app.Shutdown()
	time.Sleep(100 * time.Millisecond)

	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:18083/ws/robot/ping-test", nil)
	if err != nil {
		t.Fatalf("WebSocket dial error: %v", err)
	}
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Send ping
	msg, _ := protocol.NewMessage(protocol.TypePing, nil)
	data, _ := msg.Bytes()
	ws.WriteMessage(websocket.TextMessage, data)

	// Read pong
	_, respData, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	var resp protocol.Message
	json.Unmarshal(respData, &resp)

	if resp.Type != protocol.TypePong {
		t.Errorf("Type = %s, want pong", resp.Type)
	}
}




