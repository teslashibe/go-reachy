package web

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// ToolInfo describes an available tool
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Available tools for the dashboard
var availableTools = []ToolInfo{
	{Name: "wave_hello", Description: "Wave antennas to greet"},
	{Name: "look_around", Description: "Look around the room"},
	{Name: "express_emotion", Description: "Express an emotion (happy, sad, curious, excited)"},
	{Name: "move_head", Description: "Move head (left, right, up, down, center)"},
	{Name: "describe_scene", Description: "Describe what Eva sees"},
	{Name: "nod_yes", Description: "Nod head yes"},
	{Name: "shake_head_no", Description: "Shake head no"},
	{Name: "get_time", Description: "Get current time"},
	{Name: "set_timer", Description: "Set a timer"},
}

// handleStatus returns Eva's current state
func (s *Server) handleStatus(c *fiber.Ctx) error {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return c.JSON(s.state)
}

// handleListTools returns available tools
func (s *Server) handleListTools(c *fiber.Ctx) error {
	return c.JSON(availableTools)
}

// TriggerToolRequest is the request body for triggering a tool
type TriggerToolRequest struct {
	Args map[string]interface{} `json:"args"`
}

// handleTriggerTool triggers a tool manually
func (s *Server) handleTriggerTool(c *fiber.Ctx) error {
	name := c.Params("name")

	var req TriggerToolRequest
	if err := c.BodyParser(&req); err != nil {
		req.Args = make(map[string]interface{})
	}

	if s.OnToolTrigger == nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Tool trigger not configured",
		})
	}

	result, err := s.OnToolTrigger(name, req.Args)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	s.AddLog("tool", "Manual: "+name+" → "+result)

	return c.JSON(fiber.Map{
		"tool":   name,
		"result": result,
	})
}

// handleGetLogs returns recent log entries
func (s *Server) handleGetLogs(c *fiber.Ctx) error {
	s.logsMu.RLock()
	defer s.logsMu.RUnlock()
	return c.JSON(s.logs)
}

// handleGetConversation returns recent conversation
func (s *Server) handleGetConversation(c *fiber.Ctx) error {
	s.conversationMu.RLock()
	defer s.conversationMu.RUnlock()
	return c.JSON(s.conversation)
}

// handleLogsWS handles WebSocket connections for live logs
func (s *Server) handleLogsWS(c *websocket.Conn) {
	// Register client
	s.logClientsMu.Lock()
	s.logClients[c] = true
	s.logClientsMu.Unlock()

	// Send recent logs
	s.logsMu.RLock()
	for _, entry := range s.logs {
		c.WriteJSON(entry)
	}
	s.logsMu.RUnlock()

	// Keep connection alive
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			break
		}
	}

	// Unregister client
	s.logClientsMu.Lock()
	delete(s.logClients, c)
	s.logClientsMu.Unlock()
}

// handleCameraWS handles WebSocket connections for camera feed
func (s *Server) handleCameraWS(c *websocket.Conn) {
	// Register client
	s.cameraClientsMu.Lock()
	s.cameraClients[c] = true
	s.cameraClientsMu.Unlock()

	// Keep connection alive
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			break
		}
	}

	// Unregister client
	s.cameraClientsMu.Lock()
	delete(s.cameraClients, c)
	s.cameraClientsMu.Unlock()
}

// handleStatusWS handles WebSocket connections for status updates
func (s *Server) handleStatusWS(c *websocket.Conn) {
	// Register client
	s.statusClientsMu.Lock()
	s.statusClients[c] = true
	s.statusClientsMu.Unlock()

	// Send current status
	s.stateMu.RLock()
	c.WriteJSON(s.state)
	s.stateMu.RUnlock()

	// Keep connection alive
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			break
		}
	}

	// Unregister client
	s.statusClientsMu.Lock()
	delete(s.statusClients, c)
	s.statusClientsMu.Unlock()
}



