package robot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teslashibe/go-reachy/pkg/debug"
)

// ZenohClient wraps the connection to the robot
// Note: Uses HTTP/WebSocket since zenoh-go doesn't exist yet
// TODO: Switch to native Zenoh when zenoh-go is available
type ZenohClient struct {
	httpBase string
	wsBase   string
	prefix   string
	debug    bool

	wsConn        *websocket.Conn
	connected     chan struct{}
	connectedOnce sync.Once

	// Message callbacks
	OnJointPositions func([]byte)
	OnHeadPose       func([]byte)
	OnStatus         func([]byte)

	mu sync.RWMutex
}

// NewZenohClient creates a new client connection to the robot
// Uses HTTP API + WebSocket for state updates
func NewZenohClient(ctx context.Context, endpoint string, prefix string, debug bool) (*ZenohClient, error) {
	// Parse endpoint (tcp/IP:PORT) -> http://IP:8000
	var ip string
	fmt.Sscanf(endpoint, "tcp/%s", &ip)
	// Remove port from IP if present
	if len(ip) > 0 {
		for i := len(ip) - 1; i >= 0; i-- {
			if ip[i] == ':' {
				ip = ip[:i]
				break
			}
		}
	}

	httpBase := fmt.Sprintf("http://%s:8000", ip)
	wsBase := fmt.Sprintf("ws://%s:8000", ip)

	if debug {
		log.Printf("Connecting to robot at %s", httpBase)
	}

	zc := &ZenohClient{
		httpBase:  httpBase,
		wsBase:    wsBase,
		prefix:    prefix,
		debug:     debug,
		connected: make(chan struct{}),
	}

	// Check if daemon is running
	status, err := zc.GetDaemonStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get daemon status: %w", err)
	}

	if debug {
		log.Printf("Daemon status: %s", status["state"])
	}

	// Connect WebSocket for state updates
	if err := zc.connectWebSocket(ctx); err != nil {
		log.Printf("WebSocket connection failed (non-fatal): %v", err)
	}

	// Mark as connected
	zc.connectedOnce.Do(func() {
		close(zc.connected)
	})

	if debug {
		log.Println("Robot client initialized successfully")
	}

	return zc, nil
}

// GetDaemonStatus gets the current daemon status via HTTP
func (zc *ZenohClient) GetDaemonStatus() (map[string]interface{}, error) {
	resp, err := httpClient.Get(zc.httpBase + "/api/daemon/status")
	if err != nil {
		debug.Log("⚠️  GetDaemonStatus error: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var status map[string]interface{}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}

	return status, nil
}

// StartDaemon starts the robot daemon
func (zc *ZenohClient) StartDaemon(wakeUp bool) error {
	url := fmt.Sprintf("%s/api/daemon/start?wake_up=%v", zc.httpBase, wakeUp)
	resp, err := httpClient.Post(url, "application/json", nil)
	if err != nil {
		debug.Log("⚠️  StartDaemon error: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to start daemon: %s", string(body))
	}

	return nil
}

// connectWebSocket establishes WebSocket connection for state updates
func (zc *ZenohClient) connectWebSocket(ctx context.Context) error {
	wsURL := zc.wsBase + "/api/state/ws/full"

	if zc.debug {
		log.Printf("Connecting WebSocket to %s", wsURL)
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	zc.wsConn = conn

	// Start reading messages
	go zc.readWebSocket(ctx)

	return nil
}

// readWebSocket reads messages from WebSocket
func (zc *ZenohClient) readWebSocket(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := zc.wsConn.ReadMessage()
			if err != nil {
				if zc.debug {
					log.Printf("WebSocket read error: %v", err)
				}
				return
			}

			// Parse the full state message
			var state map[string]interface{}
			if err := json.Unmarshal(message, &state); err != nil {
				continue
			}

			// Extract and dispatch to callbacks
			if jp, ok := state["joint_positions"]; ok {
				if data, err := json.Marshal(jp); err == nil && zc.OnJointPositions != nil {
					zc.OnJointPositions(data)
				}
			}
			if hp, ok := state["head_pose"]; ok {
				if data, err := json.Marshal(hp); err == nil && zc.OnHeadPose != nil {
					zc.OnHeadPose(data)
				}
			}
		}
	}
}

// Publish sends a command to the robot via HTTP
// Note: In real implementation, this would go through Zenoh
func (zc *ZenohClient) Publish(topic string, data []byte) error {
	// For now, commands go through HTTP API
	// TODO: Implement proper Zenoh publish when zenoh-go exists
	if zc.debug {
		log.Printf("Publish to %s: %s", topic, string(data))
	}
	return nil
}

// SendMoveCommand sends a movement command via HTTP
func (zc *ZenohClient) SendMoveCommand(head [4]float64, antennas [2]float64, bodyYaw float64) error {
	cmd := map[string]interface{}{
		"head":     head,
		"antennas": antennas,
		"body_yaw": bodyYaw,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	// POST to move endpoint (uses shared httpClient with 2s timeout)
	resp, err := httpClient.Post(zc.httpBase+"/api/move/target", "application/json",
		io.NopCloser(jsonReader(data)))
	if err != nil {
		debug.Log("⚠️  SendMoveCommand error: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	return nil
}

// jsonReader creates a reader from JSON bytes
func jsonReader(data []byte) io.Reader {
	return &jsonBytesReader{data: data}
}

type jsonBytesReader struct {
	data []byte
	pos  int
}

func (r *jsonBytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// Connected returns a channel that closes when connected
func (zc *ZenohClient) Connected() <-chan struct{} {
	return zc.connected
}

// Close closes the connection
func (zc *ZenohClient) Close() error {
	if zc.debug {
		log.Println("Closing robot connection...")
	}

	if zc.wsConn != nil {
		zc.wsConn.Close()
	}

	return nil
}

// WaitForConnection waits for the robot connection with timeout
func (zc *ZenohClient) WaitForConnection(timeout time.Duration) error {
	select {
	case <-zc.connected:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("connection timeout after %v", timeout)
	}
}
