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
	Kp                float64 // Proportional gain
	Kd                float64 // Derivative gain (dampening)
	ControlDeadZone   float64 // Don't move if error < this (radians)
	MaxTargetVelocity float64 // Max target change per tick (radians, 0 = no limit)

	// Perception
	CameraFOV            float64 // Horizontal field of view in radians
	PositionSmoothing    float64 // Exponential smoothing factor (0-1, higher = more new data)
	JitterThreshold      float64 // Ignore frame position changes < this %
	OffsetSmoothingAlpha float64 // EMA alpha for offset smoothing (0.3=smooth, 0.6=responsive, 1.0=off)

	// World Model
	ConfidenceDecay float64       // How fast confidence decays (per second)
	ForgetThreshold float64       // Remove entities below this confidence
	ForgetTimeout   time.Duration // Remove entities not seen for this long

	// Scan behavior (when no face detected)
	ScanStartDelay time.Duration // Start scanning after this long with no face
	ScanSpeed      float64       // Radians per second when scanning
	ScanRange      float64       // How far to scan left/right (radians)

	// Logging
	LogThreshold float64 // Only log movements larger than this (radians)

	// Body rotation (when head reaches mechanical limits)
	BodyRotationThreshold float64 // Trigger rotation when head yaw exceeds this fraction of YawRange (0-1)
	BodyRotationStep      float64 // How much to rotate body per trigger (radians)

	// Pitch (up/down) tracking
	PitchRangeUp   float64 // Max pitch looking up (positive radians)
	PitchRangeDown float64 // Max pitch looking down (positive radians, applied as negative)
	VerticalFOV    float64 // Vertical field of view in radians

	// Pitch PD gains (0 = inherit from Kp/Kd)
	KpPitch       float64 // Proportional gain for pitch (0 = use Kp)
	KdPitch       float64 // Derivative gain for pitch (0 = use Kd)
	PitchDeadZone float64 // Dead zone for pitch (0 = use ControlDeadZone)

	// Breathing animation (idle behavior when not tracking)
	// Matches Python reachy behavior for visibility
	BreathingEnabled    bool    // Enable breathing animation (default: true)
	BreathingAmplitude  float64 // Pitch amplitude in radians
	BreathingFrequency  float64 // Cycles per second (Hz) - Python uses 0.08
	BreathingRollAmp    float64 // Roll amplitude in radians (subtle side-to-side)
	BreathingAntennaAmp float64 // Antenna sway amplitude in radians (Python: ~5°)

	// Response scaling (0-1) reduces overshoot for smoother tracking
	// Python reachy uses 0.6; set to 1.0 for full response
	ResponseScale float64

	// Audio-triggered speaker switching
	// When tracking a face, if audio comes from a different direction, turn toward the voice
	AudioSwitchEnabled       bool          // Enable audio-triggered speaker switching (default: true)
	AudioSwitchThreshold     float64       // Angle difference to trigger switch (radians, default: ~30°)
	AudioSwitchMinConfidence float64       // Minimum audio confidence to trigger (0-1)
	AudioSwitchLookDuration  time.Duration // How long to look for a face at audio direction before returning
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

		// PD Controller - tuned for smooth tracking (matches Python reachy)
		Kp:                0.10, // Proportional: respond to error
		Kd:                0.08, // Derivative: dampen oscillations
		ControlDeadZone:   0.05, // ~3° dead zone
		MaxTargetVelocity: 0.05, // ~3°/tick = ~60°/sec max target velocity

		// Perception
		CameraFOV:            math.Pi / 2, // 90° horizontal FOV
		PositionSmoothing:    0.6,         // 60% new, 40% old
		JitterThreshold:      5.0,         // Ignore <5% position changes
		OffsetSmoothingAlpha: 0.4,         // EMA alpha for offsets (0.4 = smooth but responsive)

		// World Model
		ConfidenceDecay: 0.3,              // Lose 30% confidence per second
		ForgetThreshold: 0.1,              // Forget below 10% confidence
		ForgetTimeout:   10 * time.Second, // Forget after 10 seconds

		// Scan behavior
		ScanStartDelay: 2 * time.Second, // Start scanning after 2s with no face
		ScanSpeed:      0.3,             // 0.3 rad/sec when scanning
		ScanRange:      1.0,             // Scan ±1.0 rad (±57°)

		// Logging
		LogThreshold: 0.05, // Log movements >0.05 rad (~3°)

		// Body rotation
		BodyRotationThreshold: 0.8, // Trigger when head yaw > 80% of max range
		BodyRotationStep:      0.5, // Rotate body by 0.5 rad (~29°) per trigger

		// Pitch tracking (asymmetric range is typical for head mechanics)
		PitchRangeUp:   0.4,         // +23° up
		PitchRangeDown: 0.3,         // -17° down (less mechanical range)
		VerticalFOV:    math.Pi / 3, // 60° vertical FOV (narrower than horizontal)

		// Pitch gains (0 = inherit from yaw gains)
		KpPitch:       0, // Use Kp
		KdPitch:       0, // Use Kd
		PitchDeadZone: 0, // Use ControlDeadZone

		// Breathing animation (matches Python reachy for visibility)
		BreathingEnabled:    true,               // Enable by default
		BreathingAmplitude:  0.05,               // ~3° pitch oscillation
		BreathingFrequency:  0.08,               // Match Python: one breath every ~12.5 seconds
		BreathingRollAmp:    0.02,               // ~1° roll (subtle)
		BreathingAntennaAmp: 5.0 * math.Pi / 180, // 5° antenna sway (like Python)

		// Response scaling (matches Python reachy behavior)
		ResponseScale: 0.6, // Scale down response to prevent overshoot

		// Audio-triggered speaker switching
		AudioSwitchEnabled:       true,                  // Enable by default
		AudioSwitchThreshold:     0.52,                  // ~30° - turn toward voice if more than this from current gaze
		AudioSwitchMinConfidence: 0.6,                   // Require high confidence to avoid false triggers
		AudioSwitchLookDuration:  1500 * time.Millisecond, // Look for face at audio direction for 1.5 seconds
	}
}

// EffectiveKpPitch returns the pitch proportional gain (inherits from Kp if 0)
func (c Config) EffectiveKpPitch() float64 {
	if c.KpPitch > 0 {
		return c.KpPitch
	}
	return c.Kp
}

// EffectiveKdPitch returns the pitch derivative gain (inherits from Kd if 0)
func (c Config) EffectiveKdPitch() float64 {
	if c.KdPitch > 0 {
		return c.KdPitch
	}
	return c.Kd
}

// EffectivePitchDeadZone returns the pitch dead zone (inherits from ControlDeadZone if 0)
func (c Config) EffectivePitchDeadZone() float64 {
	if c.PitchDeadZone > 0 {
		return c.PitchDeadZone
	}
	return c.ControlDeadZone
}

// SlowConfig returns a configuration for slower, smoother tracking
func SlowConfig() Config {
	cfg := DefaultConfig()
	cfg.DetectionInterval = 400 * time.Millisecond
	cfg.MaxSpeed = 0.10
	cfg.Kp = 0.08
	cfg.Kd = 0.10 // More dampening
	cfg.ControlDeadZone = 0.06
	cfg.ResponseScale = 0.5 // Even more scaling
	return cfg
}

// AggressiveConfig returns a configuration for very fast tracking
func AggressiveConfig() Config {
	cfg := DefaultConfig()
	cfg.DetectionInterval = 150 * time.Millisecond
	cfg.MaxSpeed = 0.25
	cfg.Kp = 0.15
	cfg.Kd = 0.05 // Less dampening
	cfg.ControlDeadZone = 0.03
	cfg.PositionSmoothing = 0.8 // Trust new readings more
	cfg.ResponseScale = 0.8     // Less scaling for faster response
	return cfg
}
