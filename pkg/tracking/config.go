package tracking

import (
	"math"
	"time"
)

// Config holds all tunable parameters for head tracking
type Config struct {
	// Timing
	DetectionInterval time.Duration // How often to detect faces
	MovementInterval  time.Duration // How often to update head position
	DecayInterval     time.Duration // How often to decay world model confidence

	// Movement speeds (radians per tick)
	MaxSpeed float64 // Maximum movement speed per tick

	// Range
	YawRange float64 // Maximum yaw in radians (symmetric: ±YawRange)

	// PD Controller
	Kp              float64 // Proportional gain
	Kd              float64 // Derivative gain (dampening)
	ControlDeadZone float64 // Don't move if error < this (radians)

	// Perception
	CameraFOV         float64 // Horizontal field of view in radians
	PositionSmoothing float64 // Exponential smoothing factor (0-1, higher = more new data)
	JitterThreshold   float64 // Ignore frame position changes < this %

	// World Model
	ConfidenceDecay float64       // How fast confidence decays (per second)
	ForgetThreshold float64       // Remove entities below this confidence
	ForgetTimeout   time.Duration // Remove entities not seen for this long

	// Logging
	LogThreshold float64 // Only log movements larger than this (radians)
}

// DefaultConfig returns the recommended configuration for responsive tracking
func DefaultConfig() Config {
	return Config{
		// Timing - fast and responsive
		DetectionInterval: 250 * time.Millisecond, // 4 detections per second
		MovementInterval:  50 * time.Millisecond,  // 20 updates per second
		DecayInterval:     100 * time.Millisecond, // 10 decay updates per second

		// Movement speed
		MaxSpeed: 0.15, // Max radians per tick

		// Range - almost full 180° rotation
		YawRange: 1.5, // ±1.5 rad = ±86° = 172° total

		// PD Controller - tuned for smooth tracking
		Kp:              0.15,              // Proportional: respond to error
		Kd:              0.05,              // Derivative: dampen oscillations
		ControlDeadZone: 0.03,              // ~2° dead zone

		// Perception
		CameraFOV:         math.Pi / 2,     // 90° horizontal FOV
		PositionSmoothing: 0.6,             // 60% new, 40% old
		JitterThreshold:   5.0,             // Ignore <5% position changes

		// World Model
		ConfidenceDecay: 0.3,               // Lose 30% confidence per second
		ForgetThreshold: 0.1,               // Forget below 10% confidence
		ForgetTimeout:   10 * time.Second,  // Forget after 10 seconds

		// Logging
		LogThreshold: 0.05, // Log movements >0.05 rad (~3°)
	}
}

// SlowConfig returns a configuration for slower, smoother tracking
func SlowConfig() Config {
	cfg := DefaultConfig()
	cfg.DetectionInterval = 400 * time.Millisecond
	cfg.MaxSpeed = 0.10
	cfg.Kp = 0.10
	cfg.Kd = 0.08 // More dampening
	cfg.ControlDeadZone = 0.05
	return cfg
}

// AggressiveConfig returns a configuration for very fast tracking
func AggressiveConfig() Config {
	cfg := DefaultConfig()
	cfg.DetectionInterval = 150 * time.Millisecond
	cfg.MaxSpeed = 0.25
	cfg.Kp = 0.20
	cfg.Kd = 0.03 // Less dampening
	cfg.ControlDeadZone = 0.02
	cfg.PositionSmoothing = 0.8 // Trust new readings more
	return cfg
}
