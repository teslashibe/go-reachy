// Package audio provides DOA (Direction of Arrival) integration with go-eva
package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DOAResult represents a DOA reading from go-eva
type DOAResult struct {
	Angle      float64   `json:"angle"`       // Radians in Eva coordinates (0=front, +π/2=left, -π/2=right)
	Speaking   bool      `json:"speaking"`    // Voice activity detected
	Confidence float64   `json:"confidence"`  // 0-1 confidence score
	Timestamp  time.Time `json:"timestamp"`   // When this reading was taken
	RawAngle   float64   `json:"raw_angle"`   // Original XVF3800 angle
}

// wsMessage represents a WebSocket message from go-eva
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// DOAHandler is called when a new DOA reading is received
type DOAHandler func(result *DOAResult)

// Client connects to go-eva's DOA API
type Client struct {
	robotIP    string
	baseURL    string
	wsURL      string
	httpClient *http.Client

	// WebSocket state
	mu        sync.RWMutex
	conn      *websocket.Conn
	connected bool
	handler   DOAHandler
	cancel    context.CancelFunc
}

// NewClient creates a new DOA client
func NewClient(robotIP string) *Client {
	return &Client{
		robotIP: robotIP,
		baseURL: fmt.Sprintf("http://%s:9000", robotIP),
		wsURL:   fmt.Sprintf("ws://%s:9000/api/audio/doa/stream", robotIP),
		httpClient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
	}
}

// GetDOA fetches the current DOA reading via HTTP (fallback)
func (c *Client) GetDOA() (*DOAResult, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/audio/doa")
	if err != nil {
		return nil, fmt.Errorf("DOA request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DOA returned status %d", resp.StatusCode)
	}

	var result DOAResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("DOA decode failed: %w", err)
	}

	return &result, nil
}

// Health checks if go-eva is running
func (c *Client) Health() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("go-eva not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("go-eva unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

// StreamDOA connects to go-eva's WebSocket and streams DOA updates.
// The handler is called for each DOA reading received.
// Returns immediately; streaming happens in a goroutine.
// Call Close() to stop streaming.
func (c *Client) StreamDOA(ctx context.Context, handler DOAHandler) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("already streaming")
	}
	c.handler = handler
	c.mu.Unlock()

	// Parse WebSocket URL
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	// Connect with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket connect failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// Create cancellable context for the read loop
	streamCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	// Start read loop in goroutine
	go c.readLoop(streamCtx)

	return nil
}

// readLoop reads WebSocket messages and calls the handler
func (c *Client) readLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		handler := c.handler
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		_, message, err := conn.ReadMessage()
		if err != nil {
			// Check if context was cancelled
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Connection error - try to reconnect
			c.handleDisconnect(ctx)
			return
		}

		// Parse message
		var msg wsMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		// Handle DOA messages
		if msg.Type == "doa" && handler != nil {
			var result DOAResult
			if err := json.Unmarshal(msg.Data, &result); err == nil {
				handler(&result)
			}
		}
	}
}

// handleDisconnect attempts to reconnect after a disconnect
func (c *Client) handleDisconnect(ctx context.Context) {
	c.mu.Lock()
	handler := c.handler
	c.connected = false
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	// Exponential backoff reconnection
	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Try to reconnect
		u, _ := url.Parse(c.wsURL)
		dialer := websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
		}

		conn, _, err := dialer.DialContext(ctx, u.String(), nil)
		if err != nil {
			// Increase backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reconnected!
		c.mu.Lock()
		c.conn = conn
		c.connected = true
		c.mu.Unlock()

		// Restart read loop
		go c.readLoop(ctx)
		
		// Log reconnection if handler exists
		if handler != nil {
			// Send a synthetic reading to indicate reconnection
			// (handler can check timestamp to see if it's recent)
		}
		return
	}
}

// IsStreaming returns true if WebSocket streaming is active
func (c *Client) IsStreaming() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Close stops WebSocket streaming and closes the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.connected = false
		return err
	}

	return nil
}
