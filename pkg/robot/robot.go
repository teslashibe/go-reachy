package robot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// ControlLoopHz is the frequency of the control loop
const ControlLoopHz = 30

// JointPositions represents the robot's joint positions
type JointPositions struct {
	Neck         [3]float64 `json:"neck"` // roll, pitch, yaw
	LeftAntenna  float64    `json:"l_antenna"`
	RightAntenna float64    `json:"r_antenna"`
	BodyYaw      float64    `json:"body_yaw"`
}

// HeadPose represents the robot's head pose in 3D space
type HeadPose struct {
	Position    [3]float64 `json:"position"`    // x, y, z
	Orientation [4]float64 `json:"orientation"` // quaternion
}

// Command represents a command to send to the robot
type Command struct {
	Head     [4]float64 `json:"head"`     // x, y, z, yaw
	Antennas [2]float64 `json:"antennas"` // left, right
	BodyYaw  float64    `json:"body_yaw"`
}

// Status represents the robot's status
type Status struct {
	Connected bool   `json:"connected"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

// Reachy represents a connection to a Reachy Mini robot
type Reachy struct {
	ip    string
	debug bool

	mu             sync.RWMutex
	jointPositions JointPositions
	headPose       HeadPose
	status         Status

	// Target state (what we want the robot to do)
	targetMu       sync.RWMutex
	targetHead     [4]float64
	targetAntennas [2]float64
	targetBodyYaw  float64

	// Zenoh connection
	zenoh *ZenohClient

	// Callbacks
	onJointUpdate func(JointPositions)
}

// Connect establishes a connection to the Reachy Mini robot
func Connect(ctx context.Context, ip string, debug bool) (*Reachy, error) {
	r := &Reachy{
		ip:    ip,
		debug: debug,
		status: Status{
			Connected: false,
			State:     "connecting",
		},
	}

	// Connect via Zenoh
	zenohAddr := fmt.Sprintf("tcp/%s:7447", ip)
	zenoh, err := NewZenohClient(ctx, zenohAddr, "reachy_mini", debug)
	if err != nil {
		return nil, fmt.Errorf("zenoh connection failed: %w", err)
	}
	r.zenoh = zenoh

	// Set up callbacks
	zenoh.OnJointPositions = r.handleJointPositions
	zenoh.OnHeadPose = r.handleHeadPose
	zenoh.OnStatus = r.handleStatus

	// Wait for first joint position update (confirms connection)
	select {
	case <-zenoh.Connected():
		r.status.Connected = true
		r.status.State = "connected"
	case <-time.After(5 * time.Second):
		zenoh.Close()
		return nil, fmt.Errorf("timeout waiting for robot connection")
	case <-ctx.Done():
		zenoh.Close()
		return nil, ctx.Err()
	}

	return r, nil
}

// Close closes the connection to the robot
func (r *Reachy) Close() error {
	if r.zenoh != nil {
		return r.zenoh.Close()
	}
	return nil
}

// Run starts the control loop
func (r *Reachy) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second / ControlLoopHz)
	defer ticker.Stop()

	if r.debug {
		log.Printf("Control loop started at %d Hz", ControlLoopHz)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.sendCommand()
		}
	}
}

// sendCommand sends the current target state to the robot
func (r *Reachy) sendCommand() {
	r.targetMu.RLock()
	cmd := Command{
		Head:     r.targetHead,
		Antennas: r.targetAntennas,
		BodyYaw:  r.targetBodyYaw,
	}
	r.targetMu.RUnlock()

	data, err := json.Marshal(cmd)
	if err != nil {
		log.Printf("Failed to marshal command: %v", err)
		return
	}

	if err := r.zenoh.Publish("command", data); err != nil {
		log.Printf("Failed to send command: %v", err)
	}
}

// SetHead sets the target head position
func (r *Reachy) SetHead(x, y, z, yaw float64) {
	r.targetMu.Lock()
	r.targetHead = [4]float64{x, y, z, yaw}
	r.targetMu.Unlock()
}

// SetAntennas sets the target antenna positions
func (r *Reachy) SetAntennas(left, right float64) {
	r.targetMu.Lock()
	r.targetAntennas = [2]float64{left, right}
	r.targetMu.Unlock()
}

// SetBodyYaw sets the target body rotation
func (r *Reachy) SetBodyYaw(yaw float64) {
	r.targetMu.Lock()
	r.targetBodyYaw = yaw
	r.targetMu.Unlock()
}

// GetJointPositions returns the current joint positions
func (r *Reachy) GetJointPositions() JointPositions {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.jointPositions
}

// GetHeadPose returns the current head pose
func (r *Reachy) GetHeadPose() HeadPose {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headPose
}

// IsConnected returns whether the robot is connected
func (r *Reachy) IsConnected() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status.Connected
}

// Handlers for Zenoh messages
func (r *Reachy) handleJointPositions(data []byte) {
	var jp JointPositions
	if err := json.Unmarshal(data, &jp); err != nil {
		log.Printf("Failed to parse joint positions: %v", err)
		return
	}
	r.mu.Lock()
	r.jointPositions = jp
	r.mu.Unlock()

	if r.onJointUpdate != nil {
		r.onJointUpdate(jp)
	}
}

func (r *Reachy) handleHeadPose(data []byte) {
	var hp HeadPose
	if err := json.Unmarshal(data, &hp); err != nil {
		log.Printf("Failed to parse head pose: %v", err)
		return
	}
	r.mu.Lock()
	r.headPose = hp
	r.mu.Unlock()
}

func (r *Reachy) handleStatus(data []byte) {
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("Failed to parse status: %v", err)
		return
	}
	r.mu.Lock()
	r.status = s
	r.mu.Unlock()
}

// Dance makes the robot perform a dance move
func (r *Reachy) Dance(name string) error {
	cmd := map[string]interface{}{
		"action": "dance",
		"name":   name,
	}
	data, _ := json.Marshal(cmd)
	return r.zenoh.Publish("command", data)
}

// PlayEmotion makes the robot play an emotion
func (r *Reachy) PlayEmotion(emotion string) error {
	cmd := map[string]interface{}{
		"action":  "emotion",
		"emotion": emotion,
	}
	data, _ := json.Marshal(cmd)
	return r.zenoh.Publish("command", data)
}
