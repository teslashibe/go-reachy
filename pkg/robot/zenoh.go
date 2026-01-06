package robot

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"

	zenoh "github.com/teslashibe/zenoh-go"
)

// ZenohController implements RobotController using Zenoh pub/sub.
// This provides direct communication with the robot daemon at 100Hz+
// without the overhead of HTTP requests.
//
// Zenoh topics:
//   - {prefix}/command - motor commands (head pose, antennas, body yaw)
//   - {prefix}/joint_positions - joint state feedback (subscriber)
//   - {prefix}/head_pose - current head pose (subscriber)
type ZenohController struct {
	prefix  string
	session zenoh.Session
	cmdPub  zenoh.Publisher

	mu     sync.RWMutex
	closed bool
}

// NewZenohController creates a new Zenoh-based robot controller.
// robotIP should be the robot's IP address (e.g., "192.168.68.83").
// The Zenoh endpoint is on port 7447.
func NewZenohController(robotIP string) (*ZenohController, error) {
	endpoint := fmt.Sprintf("tcp/%s:7447", robotIP)

	session, err := zenoh.Open(zenoh.ClientConfig(endpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to open zenoh session: %w", err)
	}

	// Default prefix for Reachy Mini
	prefix := "reachy_mini"

	cmdPub, err := session.Publisher(zenoh.KeyExpr(prefix + "/command"))
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to create command publisher: %w", err)
	}

	return &ZenohController{
		prefix:  prefix,
		session: session,
		cmdPub:  cmdPub,
	}, nil
}

// sendCommand publishes a JSON command to the robot.
func (z *ZenohController) sendCommand(cmd map[string]interface{}) error {
	z.mu.RLock()
	if z.closed {
		z.mu.RUnlock()
		return fmt.Errorf("zenoh controller is closed")
	}
	z.mu.RUnlock()

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	return z.cmdPub.Put(data)
}

// rpyToMatrix converts roll, pitch, yaw (radians) to a 4x4 transformation matrix.
// Uses ZYX rotation order (yaw, then pitch, then roll) matching reachy_mini.
func rpyToMatrix(roll, pitch, yaw float64) [4][4]float64 {
	cr, sr := math.Cos(roll), math.Sin(roll)
	cp, sp := math.Cos(pitch), math.Sin(pitch)
	cy, sy := math.Cos(yaw), math.Sin(yaw)

	// ZYX rotation matrix (Rz * Ry * Rx)
	return [4][4]float64{
		{cy*cp, cy*sp*sr - sy*cr, cy*sp*cr + sy*sr, 0},
		{sy*cp, sy*sp*sr + cy*cr, sy*sp*cr - cy*sr, 0},
		{-sp, cp*sr, cp*cr, 0},
		{0, 0, 0, 1},
	}
}

// SetHeadPose sets the robot's head position using Zenoh.
func (z *ZenohController) SetHeadPose(roll, pitch, yaw float64) error {
	// Convert roll/pitch/yaw to 4x4 pose matrix
	pose := rpyToMatrix(roll, pitch, yaw)

	// Convert to nested slice for JSON
	poseList := make([][]float64, 4)
	for i := 0; i < 4; i++ {
		poseList[i] = pose[i][:]
	}

	return z.sendCommand(map[string]interface{}{
		"head_pose": poseList,
	})
}

// SetAntennas sets the robot's antenna positions using Zenoh.
func (z *ZenohController) SetAntennas(left, right float64) error {
	return z.sendCommand(map[string]interface{}{
		"antennas_joint_positions": []float64{left, right},
	})
}

// SetAntennasSmooth sets antenna positions (duration is ignored for Zenoh).
func (z *ZenohController) SetAntennasSmooth(left, right, _ float64) error {
	return z.SetAntennas(left, right)
}

// SetBodyYaw rotates the robot's body using Zenoh.
func (z *ZenohController) SetBodyYaw(yaw float64) error {
	return z.sendCommand(map[string]interface{}{
		"body_yaw": yaw,
	})
}

// SetPose sets head, antennas, and body yaw in a single Zenoh message.
// This is the most efficient method for the control loop.
func (z *ZenohController) SetPose(head *Offset, antennas *[2]float64, bodyYaw *float64) error {
	cmd := make(map[string]interface{})

	if head != nil {
		pose := rpyToMatrix(head.Roll, head.Pitch, head.Yaw)
		poseList := make([][]float64, 4)
		for i := 0; i < 4; i++ {
			poseList[i] = pose[i][:]
		}
		cmd["head_pose"] = poseList
	}

	if antennas != nil {
		cmd["antennas_joint_positions"] = []float64{antennas[0], antennas[1]}
	}

	if bodyYaw != nil {
		cmd["body_yaw"] = *bodyYaw
	}

	if len(cmd) == 0 {
		return nil // Nothing to send
	}

	return z.sendCommand(cmd)
}

// GetDaemonStatus returns the robot daemon status.
// Note: This still uses HTTP since status is not real-time critical.
func (z *ZenohController) GetDaemonStatus() (string, error) {
	// Zenoh doesn't provide a request/response pattern easily,
	// so we fall back to HTTP for status queries.
	// TODO: Subscribe to daemon_status topic instead
	return "unknown", fmt.Errorf("GetDaemonStatus not implemented for Zenoh (use HTTP)")
}

// SetVolume sets the robot's speaker volume.
// Note: This still uses HTTP since volume is not real-time critical.
func (z *ZenohController) SetVolume(_ int) error {
	// Volume control goes through HTTP API, not Zenoh
	return fmt.Errorf("SetVolume not implemented for Zenoh (use HTTP)")
}

// Close closes the Zenoh session and releases resources.
func (z *ZenohController) Close() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.closed {
		return nil
	}
	z.closed = true

	if z.cmdPub != nil {
		z.cmdPub.Close()
	}
	if z.session != nil {
		return z.session.Close()
	}
	return nil
}

// Ensure ZenohController implements MotionController (for RateController)
var _ MotionController = (*ZenohController)(nil)
