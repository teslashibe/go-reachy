package tracking

import "time"

// Config holds all tunable parameters for head tracking
type Config struct {
	// Timing
	DetectionInterval time.Duration // How often to detect faces
	MovementInterval  time.Duration // How often to update head position

	// Movement speeds (radians per tick)
	MaxSpeed    float64 // Fastest movement speed
	MediumSpeed float64 // Medium movement speed

	// Range
	YawRange float64 // Maximum yaw in radians (symmetric: ±YawRange)

	// Thresholds
	JitterThreshold  float64 // Ignore position changes smaller than this %
	FastThreshold    float64 // Use MaxSpeed when distance > this (radians)
	MediumThreshold  float64 // Use MediumSpeed when distance > this (radians)
	LogThreshold     float64 // Only log movements larger than this (radians)
}

// DefaultConfig returns the recommended configuration for responsive tracking
func DefaultConfig() Config {
	return Config{
		// Timing - fast and responsive
		DetectionInterval: 250 * time.Millisecond, // 4 detections per second
		MovementInterval:  50 * time.Millisecond,  // 20 updates per second

		// Movement speeds
		MaxSpeed:    0.25, // Fast when far from target
		MediumSpeed: 0.10, // Medium for smaller adjustments

		// Range - almost full 180° rotation
		YawRange: 1.5, // ±1.5 rad = ±86° = 172° total

		// Thresholds
		JitterThreshold: 3.0,  // Ignore <3% position changes (noise)
		FastThreshold:   0.3,  // Use fast speed when >0.3 rad away
		MediumThreshold: 0.1,  // Use medium speed when >0.1 rad away
		LogThreshold:    0.05, // Log movements >0.05 rad
	}
}

// SlowConfig returns a configuration for slower, smoother tracking
func SlowConfig() Config {
	cfg := DefaultConfig()
	cfg.DetectionInterval = 500 * time.Millisecond
	cfg.MaxSpeed = 0.15
	cfg.MediumSpeed = 0.05
	return cfg
}

// AggressiveConfig returns a configuration for very fast tracking
func AggressiveConfig() Config {
	cfg := DefaultConfig()
	cfg.DetectionInterval = 150 * time.Millisecond
	cfg.MaxSpeed = 0.35
	cfg.MediumSpeed = 0.15
	cfg.JitterThreshold = 2.0
	return cfg
}

