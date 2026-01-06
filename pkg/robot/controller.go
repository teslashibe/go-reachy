package robot

import (
	"fmt"
	"sync"
	"time"
)

// Physical head limits (radians) - matches Python reachy move_head.py
// These are safety limits to prevent sending impossible commands to the daemon.
const (
	MaxHeadRoll  = 0.35 // ±20° (conservative)
	MaxHeadPitch = 0.52 // ±30° (matches Python)
	MaxHeadYaw   = 0.70 // ±40° (matches Python)
)

// clamp restricts v to the range [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// Offset represents additive head adjustments (roll, pitch, yaw in radians)
type Offset struct {
	Roll, Pitch, Yaw float64
}

// Clamp returns a new Offset with values clamped to physical head limits.
func (o Offset) Clamp() Offset {
	return Offset{
		Roll:  clamp(o.Roll, -MaxHeadRoll, MaxHeadRoll),
		Pitch: clamp(o.Pitch, -MaxHeadPitch, MaxHeadPitch),
		Yaw:   clamp(o.Yaw, -MaxHeadYaw, MaxHeadYaw),
	}
}

// Add returns a new Offset that is the sum of o and other
func (o Offset) Add(other Offset) Offset {
	return Offset{
		Roll:  o.Roll + other.Roll,
		Pitch: o.Pitch + other.Pitch,
		Yaw:   o.Yaw + other.Yaw,
	}
}

// MotionController is the interface needed by the rate-limited Controller.
// It combines head, antenna, body, and batched pose control for the control loop.
// The PoseController interface is used by tick() to send all updates in one HTTP call.
type MotionController interface {
	HeadController
	AntennaController
	BodyController
	PoseController
}

// Dead-zone thresholds (match Python reachy's _issue_control_command)
// Skip sending if position hasn't changed enough - reduces network traffic by 50-80% when idle
const (
	DeadZoneHeadRad    = 0.005  // ~0.3 degrees
	DeadZoneAntennaRad = 0.009  // ~0.5 degrees (np.deg2rad(0.5))
	DeadZoneBodyRad    = 0.009  // ~0.5 degrees
)

// RateController provides unified robot control at a fixed rate.
// All movement requests flow through here to prevent conflicts.
// It fuses base poses (from tools/moves) with tracking offsets (from face tracker).
type RateController struct {
	robot MotionController

	mu           sync.RWMutex
	baseHead     Offset     // Primary pose from tools/moves
	trackingHead Offset     // Secondary offset from face tracker
	antennas     [2]float64 // Left, right antenna positions
	bodyYaw      float64    // Body rotation in radians

	rate time.Duration // Control loop tick rate
	stop chan struct{}

	// Dead-zone filtering (matches Python reachy)
	lastSentHead     Offset     // Last sent head pose
	lastSentAntennas [2]float64 // Last sent antenna positions
	lastSentBodyYaw  float64    // Last sent body yaw
	skippedTicks     uint64     // Ticks skipped due to dead-zone

	// Diagnostics (Issue #136)
	tickCount     uint64    // Total ticks since start
	errorCount    uint64    // Number of SetPose errors
	lastErrorTime time.Time // Last error timestamp (avoid spam)
}

// NewRateController creates a rate-limited controller running at the given rate.
// Typical rate is 10ms (100Hz) for smooth motion.
func NewRateController(robot MotionController, rate time.Duration) *RateController {
	return &RateController{
		robot: robot,
		rate:  rate,
		stop:  make(chan struct{}),
	}
}

// SetBaseHead sets the primary head pose (from tools/moves).
// This is the "base" position before tracking offsets are applied.
func (c *RateController) SetBaseHead(offset Offset) {
	c.mu.Lock()
	c.baseHead = offset
	c.mu.Unlock()
}

// SetTrackingOffset sets the face tracking offset (additive).
// This is combined with the base head pose each tick.
func (c *RateController) SetTrackingOffset(offset Offset) {
	c.mu.Lock()
	c.trackingHead = offset
	c.mu.Unlock()
}

// SetAntennas sets the antenna positions.
func (c *RateController) SetAntennas(left, right float64) {
	c.mu.Lock()
	c.antennas = [2]float64{left, right}
	c.mu.Unlock()
}

// SetBodyYaw sets the body rotation.
func (c *RateController) SetBodyYaw(yaw float64) {
	c.mu.Lock()
	c.bodyYaw = yaw
	c.mu.Unlock()
}

// BodyYaw returns the current body orientation.
func (c *RateController) BodyYaw() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bodyYaw
}

// BaseHead returns the current base head pose.
func (c *RateController) BaseHead() Offset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseHead
}

// TrackingOffset returns the current tracking offset.
func (c *RateController) TrackingOffset() Offset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trackingHead
}

// CombinedHead returns the fused head pose (base + tracking).
func (c *RateController) CombinedHead() Offset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseHead.Add(c.trackingHead)
}

// Run starts the control loop. Blocks until Stop is called.
func (c *RateController) Run() {
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

// abs returns the absolute value of x.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// max returns the larger of a or b.
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// tick executes one control cycle: fuse poses and send to robot.
// Uses batched SetPose() to send all updates in ONE HTTP call instead of three.
// This prevents robot daemon flooding (Issue #135).
func (c *RateController) tick() {
	c.mu.RLock()
	combined := c.baseHead.Add(c.trackingHead)
	antennas := c.antennas
	bodyYaw := c.bodyYaw
	c.mu.RUnlock()

	// Clamp to physical head limits (Issue #141)
	// Prevents sending impossible commands when tracking outputs world-model values
	combined = combined.Clamp()

	if c.robot == nil {
		return
	}

	c.tickCount++

	// Dead-zone filtering (matches Python reachy's _issue_control_command)
	// Skip sending if position hasn't changed enough - reduces network traffic significantly
	headDiff := max(max(abs(combined.Roll-c.lastSentHead.Roll), abs(combined.Pitch-c.lastSentHead.Pitch)), abs(combined.Yaw-c.lastSentHead.Yaw))
	antennaDiff := max(abs(antennas[0]-c.lastSentAntennas[0]), abs(antennas[1]-c.lastSentAntennas[1]))
	bodyDiff := abs(bodyYaw - c.lastSentBodyYaw)

	if headDiff < DeadZoneHeadRad && antennaDiff < DeadZoneAntennaRad && bodyDiff < DeadZoneBodyRad {
		c.skippedTicks++
		// Heartbeat log even when skipping (every ~5 seconds)
		if c.tickCount%100 == 0 {
			fmt.Printf("💓 RateController: %d ticks, %d skipped, %d errors, head=(%.2f,%.2f,%.2f)\n",
				c.tickCount, c.skippedTicks, c.errorCount, combined.Roll, combined.Pitch, combined.Yaw)
		}
		return // Skip sending - position hasn't changed enough
	}

	// Single batched call - prevents daemon flooding
	err := c.robot.SetPose(&combined, &antennas, &bodyYaw)

	if err == nil {
		// Update last sent values on success
		c.lastSentHead = combined
		c.lastSentAntennas = antennas
		c.lastSentBodyYaw = bodyYaw
	} else {
		// Log errors (but don't spam - max once per 5 seconds)
		c.errorCount++
		if c.lastErrorTime.IsZero() || time.Since(c.lastErrorTime) > 5*time.Second {
			fmt.Printf("⚠️  RateController.SetPose error: %v (total errors: %d)\n", err, c.errorCount)
			c.lastErrorTime = time.Now()
		}
	}

	// Heartbeat log every ~5 seconds (100 ticks at 50ms)
	if c.tickCount%100 == 0 {
		fmt.Printf("💓 RateController: %d ticks, %d skipped, %d errors, head=(%.2f,%.2f,%.2f)\n",
			c.tickCount, c.skippedTicks, c.errorCount, combined.Roll, combined.Pitch, combined.Yaw)
	}
}

// Stop halts the control loop gracefully.
func (c *RateController) Stop() {
	close(c.stop)
}
