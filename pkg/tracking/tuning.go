package tracking

import "time"

// TuningParams holds the real-time adjustable tracking parameters.
// These can be modified via the tuning API without restarting Eva.
type TuningParams struct {
	// Smoothing
	OffsetSmoothingAlpha float64 `json:"offset_smoothing_alpha"` // EMA alpha (0.3=smooth, 0.6=responsive)
	PositionSmoothing    float64 `json:"position_smoothing"`     // Frame position smoothing

	// Velocity limiting
	MaxTargetVelocity float64 `json:"max_target_velocity"` // Max target change per tick (rad)

	// PD Controller
	Kp              float64 `json:"kp"`                // Proportional gain
	Kd              float64 `json:"kd"`                // Derivative gain
	ControlDeadZone float64 `json:"control_dead_zone"` // Dead zone (rad)
	ResponseScale   float64 `json:"response_scale"`    // Response scaling (0-1)

	// Detection rate
	DetectionHz float64 `json:"detection_hz"` // Face detection frequency (4-20 Hz)

	// Tuning mode
	TuningModeEnabled bool `json:"tuning_mode_enabled"` // Disables secondary features for clean tuning
}

// GetTuningParams returns current tuning parameters from the tracker.
func (t *Tracker) GetTuningParams() TuningParams {
	t.mu.RLock()
	defer t.mu.RUnlock()

	detectionHz := 1.0 / t.config.DetectionInterval.Seconds()

	return TuningParams{
		OffsetSmoothingAlpha: t.perception.offsetSmoothingAlpha,
		PositionSmoothing:    t.perception.smoothingFactor,
		MaxTargetVelocity:    t.controller.MaxTargetVelocity,
		Kp:                   t.controller.Kp,
		Kd:                   t.controller.Kd,
		ControlDeadZone:      t.controller.DeadZone,
		ResponseScale:        t.config.ResponseScale,
		DetectionHz:          detectionHz,
		TuningModeEnabled:    !t.config.AudioSwitchEnabled && !t.config.BreathingEnabled,
	}
}

// SetTuningParams updates tuning parameters at runtime.
// Only non-zero values are applied.
func (t *Tracker) SetTuningParams(params TuningParams) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Smoothing
	if params.OffsetSmoothingAlpha > 0 {
		t.perception.SetOffsetSmoothingAlpha(params.OffsetSmoothingAlpha)
	}
	if params.PositionSmoothing > 0 {
		t.perception.smoothingFactor = clamp(params.PositionSmoothing, 0.0, 1.0)
	}

	// Velocity limiting
	if params.MaxTargetVelocity > 0 {
		t.controller.SetMaxTargetVelocity(params.MaxTargetVelocity)
	}

	// PD Controller
	if params.Kp > 0 {
		t.controller.Kp = params.Kp
		t.controller.KpPitch = params.Kp // Apply to pitch too
	}
	if params.Kd > 0 {
		t.controller.Kd = params.Kd
		t.controller.KdPitch = params.Kd // Apply to pitch too
	}
	if params.ControlDeadZone > 0 {
		t.controller.DeadZone = params.ControlDeadZone
		t.controller.PitchDeadZone = params.ControlDeadZone
	}
	if params.ResponseScale > 0 {
		t.config.ResponseScale = clamp(params.ResponseScale, 0.0, 1.0)
	}

	// Detection rate (handled outside lock via channel)
	if params.DetectionHz > 0 {
		t.setDetectionHz(params.DetectionHz)
	}
}

// setDetectionHz updates the detection rate at runtime.
// Valid range: 1-20 Hz (50ms to 1000ms interval)
func (t *Tracker) setDetectionHz(hz float64) {
	// Clamp to valid range
	if hz < 1 {
		hz = 1
	}
	if hz > 20 {
		hz = 20
	}

	interval := time.Duration(float64(time.Second) / hz)

	// Send to the ticker reset channel (non-blocking)
	select {
	case t.detectTickerReset <- interval:
		// Sent successfully
	default:
		// Channel full, skip (previous update still pending)
	}
}

// EnableTuningMode disables secondary features for clean tuning.
// When enabled: no audio switch, no breathing, no scanning.
func (t *Tracker) EnableTuningMode(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if enabled {
		// Disable secondary features for clean face tracking tuning
		t.config.AudioSwitchEnabled = false
		t.config.BreathingEnabled = false
		// Set very long scan delay to effectively disable scanning
		t.config.ScanStartDelay = 999 * 1e9 // ~999 seconds
	} else {
		// Restore defaults
		defaults := DefaultConfig()
		t.config.AudioSwitchEnabled = defaults.AudioSwitchEnabled
		t.config.BreathingEnabled = defaults.BreathingEnabled
		t.config.ScanStartDelay = defaults.ScanStartDelay
	}
}
