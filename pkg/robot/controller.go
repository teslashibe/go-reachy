package robot

import (
	"sync"
	"time"
)

// Offset represents additive head adjustments (roll, pitch, yaw in radians)
type Offset struct {
	Roll, Pitch, Yaw float64
}

// Add returns a new Offset that is the sum of o and other
func (o Offset) Add(other Offset) Offset {
	return Offset{
		Roll:  o.Roll + other.Roll,
		Pitch: o.Pitch + other.Pitch,
		Yaw:   o.Yaw + other.Yaw,
	}
}

// RobotAPI defines the interface for robot hardware control
type RobotAPI interface {
	SetHeadPose(roll, pitch, yaw float64) error
	SetAntennas(left, right float64) error
	SetBodyYaw(yaw float64) error
}

// Controller provides unified robot control at a fixed rate.
// All movement requests flow through here to prevent conflicts.
// It fuses base poses (from tools/moves) with tracking offsets (from face tracker).
type Controller struct {
	robot RobotAPI

	mu           sync.RWMutex
	baseHead     Offset     // Primary pose from tools/moves
	trackingHead Offset     // Secondary offset from face tracker
	antennas     [2]float64 // Left, right antenna positions
	bodyYaw      float64    // Body rotation in radians

	rate time.Duration // Control loop tick rate
	stop chan struct{}
}

// NewController creates a controller running at the given rate.
// Typical rate is 10ms (100Hz) for smooth motion.
func NewController(robot RobotAPI, rate time.Duration) *Controller {
	return &Controller{
		robot: robot,
		rate:  rate,
		stop:  make(chan struct{}),
	}
}

// SetBaseHead sets the primary head pose (from tools/moves).
// This is the "base" position before tracking offsets are applied.
func (c *Controller) SetBaseHead(offset Offset) {
	c.mu.Lock()
	c.baseHead = offset
	c.mu.Unlock()
}

// SetTrackingOffset sets the face tracking offset (additive).
// This is combined with the base head pose each tick.
func (c *Controller) SetTrackingOffset(offset Offset) {
	c.mu.Lock()
	c.trackingHead = offset
	c.mu.Unlock()
}

// SetAntennas sets the antenna positions.
func (c *Controller) SetAntennas(left, right float64) {
	c.mu.Lock()
	c.antennas = [2]float64{left, right}
	c.mu.Unlock()
}

// SetBodyYaw sets the body rotation.
func (c *Controller) SetBodyYaw(yaw float64) {
	c.mu.Lock()
	c.bodyYaw = yaw
	c.mu.Unlock()
}

// BodyYaw returns the current body orientation.
func (c *Controller) BodyYaw() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bodyYaw
}

// BaseHead returns the current base head pose.
func (c *Controller) BaseHead() Offset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseHead
}

// TrackingOffset returns the current tracking offset.
func (c *Controller) TrackingOffset() Offset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trackingHead
}

// CombinedHead returns the fused head pose (base + tracking).
func (c *Controller) CombinedHead() Offset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseHead.Add(c.trackingHead)
}

// Run starts the control loop. Blocks until Stop is called.
func (c *Controller) Run() {
	ticker := time.NewTicker(c.rate)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.tick()
		}
	}
}

// tick executes one control cycle: fuse poses and send to robot.
func (c *Controller) tick() {
	c.mu.RLock()
	combined := c.baseHead.Add(c.trackingHead)
	antennas := c.antennas
	bodyYaw := c.bodyYaw
	c.mu.RUnlock()

	if c.robot == nil {
		return
	}

	// Single control point - all robot commands go here
	c.robot.SetHeadPose(combined.Roll, combined.Pitch, combined.Yaw)
	c.robot.SetAntennas(antennas[0], antennas[1])
	c.robot.SetBodyYaw(bodyYaw)
}

// Stop halts the control loop gracefully.
func (c *Controller) Stop() {
	close(c.stop)
}

