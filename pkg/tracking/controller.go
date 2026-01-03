package tracking

import (
	"math"
	"time"
)

// PDController implements proportional-derivative control for smooth head tracking
type PDController struct {
	// Yaw gains
	Kp float64 // Proportional gain
	Kd float64 // Derivative gain

	// Pitch gains
	KpPitch float64 // Proportional gain for pitch
	KdPitch float64 // Derivative gain for pitch

	// Yaw limits
	MaxYaw    float64 // Maximum yaw (±radians)
	SoftLimit float64 // Start slowing down here
	MaxSpeed  float64 // Maximum movement speed per tick

	// Pitch limits
	MaxPitchUp   float64 // Maximum pitch looking up (positive)
	MaxPitchDown float64 // Maximum pitch looking down (stored positive, applied negative)

	// Velocity limiting (prevents target from jumping too fast)
	MaxTargetVelocity float64 // Maximum target change per tick (radians)

	// Dead zone
	DeadZone      float64 // Ignore yaw errors smaller than this (radians)
	PitchDeadZone float64 // Ignore pitch errors smaller than this (radians)

	// Yaw state
	lastError  float64
	lastOutput float64
	currentYaw float64
	targetYaw  float64
	isSettled  bool // True when yaw within dead zone

	// Pitch state
	lastPitchError  float64
	lastPitchOutput float64
	currentPitch    float64
	targetPitch     float64
	isPitchSettled  bool // True when pitch within dead zone

	// Interpolation state (for smooth return to neutral)
	interpStart     time.Time
	interpDuration  time.Duration
	interpFrom      float64
	interpTo        float64
	isInterpolating bool
}

// NewPDController creates a new PD controller with default values
func NewPDController(config Config) *PDController {
	return &PDController{
		// Yaw
		Kp:        config.Kp,
		Kd:        config.Kd,
		MaxYaw:    config.YawRange,
		SoftLimit: config.YawRange * 0.85, // 85% of max
		MaxSpeed:  config.MaxSpeed,
		DeadZone:  config.ControlDeadZone,
		// Pitch
		KpPitch:       config.EffectiveKpPitch(),
		KdPitch:       config.EffectiveKdPitch(),
		MaxPitchUp:    config.PitchRangeUp,
		MaxPitchDown:  config.PitchRangeDown,
		PitchDeadZone: config.EffectivePitchDeadZone(),
		// Velocity limiting
		MaxTargetVelocity: config.MaxTargetVelocity,
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

// SetTargetFromOffset sets the target based on a camera-relative offset.
// offset: how much to turn from current position (positive = left, negative = right)
// This is self-correcting: offset should approach 0 as face becomes centered.
// Velocity limiting is applied to prevent the target from jumping too fast.
func (c *PDController) SetTargetFromOffset(offset float64) {
	c.isInterpolating = false

	// Calculate desired new target
	desiredTarget := clamp(c.currentYaw+offset, -c.MaxYaw, c.MaxYaw)

	// Apply velocity limiting: limit how fast target can change
	if c.MaxTargetVelocity > 0 {
		delta := desiredTarget - c.targetYaw
		if delta > c.MaxTargetVelocity {
			delta = c.MaxTargetVelocity
		} else if delta < -c.MaxTargetVelocity {
			delta = -c.MaxTargetVelocity
		}
		c.targetYaw = clamp(c.targetYaw+delta, -c.MaxYaw, c.MaxYaw)
	} else {
		// No velocity limiting
		c.targetYaw = desiredTarget
	}

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

// SetTargetPitch sets the desired pitch angle.
// Positive = look up, negative = look down.
func (c *PDController) SetTargetPitch(pitch float64) {
	c.targetPitch = c.clampPitch(pitch)
	c.isPitchSettled = false
}

// SetTargetPitchFromOffset sets the pitch target based on a camera-relative offset.
// offset: how much to tilt from current position (positive = down, negative = up for Reachy)
// Velocity limiting is applied to prevent the target from jumping too fast.
func (c *PDController) SetTargetPitchFromOffset(offset float64) {
	// Calculate desired new target
	desiredTarget := c.clampPitch(c.currentPitch + offset)

	// Apply velocity limiting: limit how fast target can change
	if c.MaxTargetVelocity > 0 {
		delta := desiredTarget - c.targetPitch
		if delta > c.MaxTargetVelocity {
			delta = c.MaxTargetVelocity
		} else if delta < -c.MaxTargetVelocity {
			delta = -c.MaxTargetVelocity
		}
		c.targetPitch = c.clampPitch(c.targetPitch + delta)
	} else {
		// No velocity limiting
		c.targetPitch = desiredTarget
	}

	c.isPitchSettled = false
}

// GetCurrentPitch returns the current head pitch
func (c *PDController) GetCurrentPitch() float64 {
	return c.currentPitch
}

// SetCurrentPitch sets the current pitch (for initialization)
func (c *PDController) SetCurrentPitch(pitch float64) {
	c.currentPitch = pitch
}

// GetTargetPitch returns the current target pitch
func (c *PDController) GetTargetPitch() float64 {
	return c.targetPitch
}

// IsPitchSettled returns true if pitch is at target (within dead zone)
func (c *PDController) IsPitchSettled() bool {
	return c.isPitchSettled
}

// UpdatePitch calculates the next pitch position
// Returns the new pitch and whether the head should move
func (c *PDController) UpdatePitch() (float64, bool) {
	// Calculate error
	error := c.targetPitch - c.currentPitch

	// Dead zone
	if math.Abs(error) < c.PitchDeadZone {
		c.isPitchSettled = true
		c.lastPitchError = error
		return c.currentPitch, false
	}

	// PD control
	pTerm := c.KpPitch * error
	dTerm := c.KdPitch * (error - c.lastPitchError)
	output := pTerm + dTerm

	// Rate limit
	output = clamp(output, -c.MaxSpeed, c.MaxSpeed)

	// Apply output
	newPitch := c.currentPitch + output
	newPitch = c.clampPitch(newPitch)

	// Update state
	c.lastPitchError = error
	c.lastPitchOutput = output
	c.currentPitch = newPitch

	return newPitch, true
}

// clampPitch limits pitch to mechanical limits (asymmetric)
// Note: Reachy Mini uses negative pitch for looking UP, positive for DOWN
func (c *PDController) clampPitch(pitch float64) float64 {
	if pitch < -c.MaxPitchUp {
		return -c.MaxPitchUp // Looking up limit (negative)
	}
	if pitch > c.MaxPitchDown {
		return c.MaxPitchDown // Looking down limit (positive)
	}
	return pitch
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

// AdjustForBodyRotation compensates head position when body rotates.
// Called after body rotation to maintain gaze on target.
// bodyDelta: how much body rotated (radians, positive = left)
func (c *PDController) AdjustForBodyRotation(bodyDelta float64) {
	// Counter-rotate head to maintain gaze
	// If body rotates left (positive), head needs to rotate right (negative) to keep looking at same spot
	c.currentYaw -= bodyDelta
	c.targetYaw -= bodyDelta

	// Clamp to limits
	c.currentYaw = clamp(c.currentYaw, -c.MaxYaw, c.MaxYaw)
	c.targetYaw = clamp(c.targetYaw, -c.MaxYaw, c.MaxYaw)

	// Reset error tracking to avoid derivative spike
	c.lastError = c.targetYaw - c.currentYaw
}

// SetMaxTargetVelocity updates the velocity limit for runtime tuning.
// velocity: max radians per tick (0 = no limit)
func (c *PDController) SetMaxTargetVelocity(velocity float64) {
	if velocity < 0 {
		velocity = 0
	}
	c.MaxTargetVelocity = velocity
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

