package tracking

import (
	"math"
	"time"
)

// PDController implements proportional-derivative control for smooth head tracking
type PDController struct {
	// Gains
	Kp float64 // Proportional gain
	Kd float64 // Derivative gain

	// Limits
	MaxYaw    float64 // Maximum yaw (±radians)
	SoftLimit float64 // Start slowing down here
	MaxSpeed  float64 // Maximum movement speed per tick

	// Dead zone
	DeadZone float64 // Ignore errors smaller than this (radians)

	// State
	lastError    float64
	lastOutput   float64
	currentYaw   float64
	targetYaw    float64
	isSettled    bool // True when within dead zone

	// Interpolation state (for smooth return to neutral)
	interpStart    time.Time
	interpDuration time.Duration
	interpFrom     float64
	interpTo       float64
	isInterpolating bool
}

// NewPDController creates a new PD controller with default values
func NewPDController(config Config) *PDController {
	return &PDController{
		Kp:        config.Kp,
		Kd:        config.Kd,
		MaxYaw:    config.YawRange,
		SoftLimit: config.YawRange * 0.85, // 85% of max
		MaxSpeed:  config.MaxSpeed,
		DeadZone:  config.ControlDeadZone,
	}
}

// SetTarget sets the desired world angle to look at.
// This cancels any active interpolation.
func (c *PDController) SetTarget(worldAngle float64) {
	// Cancel interpolation when we get a new target
	c.isInterpolating = false
	// Clamp to mechanical limits
	c.targetYaw = clamp(worldAngle, -c.MaxYaw, c.MaxYaw)
	c.isSettled = false
}

// InterpolateToNeutral starts smooth interpolation back to center.
// The head will smoothly move from current position to 0 over the given duration.
func (c *PDController) InterpolateToNeutral(duration time.Duration) {
	c.interpStart = time.Now()
	c.interpDuration = duration
	c.interpFrom = c.currentYaw
	c.interpTo = 0
	c.isInterpolating = true
}

// InterpolateTo starts smooth interpolation to a target position.
func (c *PDController) InterpolateTo(target float64, duration time.Duration) {
	c.interpStart = time.Now()
	c.interpDuration = duration
	c.interpFrom = c.currentYaw
	c.interpTo = clamp(target, -c.MaxYaw, c.MaxYaw)
	c.isInterpolating = true
}

// IsInterpolating returns true if currently interpolating.
func (c *PDController) IsInterpolating() bool {
	return c.isInterpolating
}

// GetCurrentYaw returns the current head yaw
func (c *PDController) GetCurrentYaw() float64 {
	return c.currentYaw
}

// SetCurrentYaw sets the current yaw (for initialization)
func (c *PDController) SetCurrentYaw(yaw float64) {
	c.currentYaw = yaw
}

// Update calculates the next yaw position
// Returns the new yaw and whether the head should move
func (c *PDController) Update() (float64, bool) {
	// Handle interpolation mode
	if c.isInterpolating {
		return c.updateInterpolation()
	}

	// Calculate error (difference between target and current)
	error := c.targetYaw - c.currentYaw

	// Dead zone: if error is small enough, we're settled
	if math.Abs(error) < c.DeadZone {
		c.isSettled = true
		c.lastError = error
		return c.currentYaw, false
	}

	// PD control
	pTerm := c.Kp * error
	dTerm := c.Kd * (error - c.lastError)
	output := pTerm + dTerm

	// Apply soft limit - reduce speed near mechanical limits
	if math.Abs(c.currentYaw) > c.SoftLimit {
		// How close are we to the limit (0 = at soft limit, 1 = at max)
		limitFactor := (math.Abs(c.currentYaw) - c.SoftLimit) / (c.MaxYaw - c.SoftLimit)
		// Reduce output as we approach limit
		output *= (1.0 - limitFactor*0.8) // Reduce by up to 80% at limit
	}

	// Rate limit the output
	output = clamp(output, -c.MaxSpeed, c.MaxSpeed)

	// Apply output to current yaw
	newYaw := c.currentYaw + output

	// Hard clamp to mechanical limits
	newYaw = clamp(newYaw, -c.MaxYaw, c.MaxYaw)

	// Update state
	c.lastError = error
	c.lastOutput = output
	c.currentYaw = newYaw

	return newYaw, true
}

// updateInterpolation handles smooth interpolation to target
func (c *PDController) updateInterpolation() (float64, bool) {
	elapsed := time.Since(c.interpStart)
	
	// Calculate interpolation progress (0 to 1)
	t := float64(elapsed) / float64(c.interpDuration)
	if t >= 1.0 {
		// Interpolation complete
		c.isInterpolating = false
		c.currentYaw = c.interpTo
		c.targetYaw = c.interpTo
		c.isSettled = true
		return c.interpTo, true
	}

	// Linear interpolation
	c.currentYaw = c.interpFrom + (c.interpTo-c.interpFrom)*t
	return c.currentYaw, true
}

// IsSettled returns true if the head is at the target (within dead zone)
func (c *PDController) IsSettled() bool {
	return c.isSettled
}

// GetError returns the current tracking error
func (c *PDController) GetError() float64 {
	return c.targetYaw - c.currentYaw
}

// GetTargetYaw returns the current target yaw
func (c *PDController) GetTargetYaw() float64 {
	return c.targetYaw
}

// NeedsBodyRotation checks if head is at mechanical limits and body should rotate.
// threshold is the fraction of MaxYaw (0-1) at which rotation triggers.
// step is how much to rotate the body (radians).
// Returns (needsRotation, rotationAmount) where positive = rotate left, negative = rotate right.
func (c *PDController) NeedsBodyRotation(threshold, step float64) (bool, float64) {
	limit := c.MaxYaw * threshold

	// If target exceeds what head can comfortably reach, rotate body
	if c.targetYaw > limit && c.currentYaw >= limit*0.95 {
		return true, step // Rotate body left to bring target into range
	}
	if c.targetYaw < -limit && c.currentYaw <= -limit*0.95 {
		return true, -step // Rotate body right
	}
	return false, 0
}

// clamp limits a value to a range
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

