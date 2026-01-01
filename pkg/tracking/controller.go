package tracking

import (
	"math"
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

// SetTarget sets the desired world angle to look at
func (c *PDController) SetTarget(worldAngle float64) {
	// Clamp to mechanical limits
	c.targetYaw = clamp(worldAngle, -c.MaxYaw, c.MaxYaw)
	c.isSettled = false
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

// IsSettled returns true if the head is at the target (within dead zone)
func (c *PDController) IsSettled() bool {
	return c.isSettled
}

// GetError returns the current tracking error
func (c *PDController) GetError() float64 {
	return c.targetYaw - c.currentYaw
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

