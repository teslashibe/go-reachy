package hub

import (
	"time"

	"github.com/gofiber/websocket/v2"
)

const (
	// writeWait is how long to wait for a write to complete
	writeWait = 10 * time.Second

	// pongWait is how long to wait for a pong response
	pongWait = 60 * time.Second

	// pingPeriod must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize is the maximum message size allowed
	maxMessageSize = 512 * 1024 // 512KB for camera frames
)

// Client represents a single websocket connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan Message
}

// NewClient creates a new client and registers it with the hub
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan Message, 256), // Buffered channel for backpressure
	}
	hub.register <- client
	return client
}

// Run starts the client's read and write pumps
// This should be called in the websocket handler
func (c *Client) Run() {
	go c.writePump()
	c.readPump() // Blocks until connection closes
}

// readPump reads messages from the websocket connection
// It keeps the connection alive and detects disconnection
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// We don't expect messages from clients, but we need to read
		// to detect disconnection and receive pong responses
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// writePump writes messages to the websocket connection
// Only this goroutine writes to the connection - no race conditions!
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel - send close frame
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Determine websocket message type
			wsType := websocket.TextMessage
			if message.Type == BinaryMessage {
				wsType = websocket.BinaryMessage
			}

			if err := c.conn.WriteMessage(wsType, message.Data); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
