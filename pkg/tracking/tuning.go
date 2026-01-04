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

	// PD Controller (yaw)
	Kp              float64 `json:"kp"`                // Proportional gain
	Kd              float64 `json:"kd"`                // Derivative gain
	ControlDeadZone float64 `json:"control_dead_zone"` // Dead zone (rad)
	ResponseScale   float64 `json:"response_scale"`    // Response scaling (0-1)

	// Detection rate
	DetectionHz float64 `json:"detection_hz"` // Face detection frequency (4-20 Hz)

	// Tuning mode
	TuningModeEnabled bool `json:"tuning_mode_enabled"` // Disables secondary features for clean tuning

	// Body alignment (gradual body rotation when locked on target)
	BodyAlignmentEnabled   bool    `json:"body_alignment_enabled"`   // Enable automatic body alignment
	BodyAlignmentDelay     float64 `json:"body_alignment_delay"`     // Seconds before alignment starts
	BodyAlignmentThreshold float64 `json:"body_alignment_threshold"` // Min head yaw to trigger (radians)
	BodyAlignmentSpeed     float64 `json:"body_alignment_speed"`     // Body rotation speed (rad/s)
	BodyAlignmentDeadZone  float64 `json:"body_alignment_dead_zone"` // Stop threshold (radians)
	BodyAlignmentCooldown  float64 `json:"body_alignment_cooldown"`  // Seconds between actions

	// === NEW: Pitch-specific settings ===
	KpPitch        float64 `json:"kp_pitch"`         // Pitch proportional gain (0 = use kp)
	KdPitch        float64 `json:"kd_pitch"`         // Pitch derivative gain (0 = use kd)
	PitchDeadZone  float64 `json:"pitch_dead_zone"`  // Pitch dead zone (0 = use control_dead_zone)
	PitchRangeUp   float64 `json:"pitch_range_up"`   // Max pitch looking up (radians)
	PitchRangeDown float64 `json:"pitch_range_down"` // Max pitch looking down (radians)

	// === NEW: Audio tracking settings ===
	AudioSwitchEnabled       bool    `json:"audio_switch_enabled"`        // Enable turning toward voices
	AudioSwitchThreshold     float64 `json:"audio_switch_threshold"`      // Angle to trigger turn (radians)
	AudioSwitchMinConfidence float64 `json:"audio_switch_min_confidence"` // DOA confidence threshold (0-1)
	AudioSwitchLookDuration  float64 `json:"audio_switch_look_duration"`  // Seconds to look for face at audio direction

	// === NEW: Breathing/idle animation ===
	BreathingEnabled    bool    `json:"breathing_enabled"`     // Enable idle breathing animation
	BreathingAmplitude  float64 `json:"breathing_amplitude"`   // Pitch oscillation (radians)
	BreathingFrequency  float64 `json:"breathing_frequency"`   // Cycles per second (Hz)
	BreathingAntennaAmp float64 `json:"breathing_antenna_amp"` // Antenna sway amplitude (radians)

	// === NEW: Range/speed limits ===
	MaxSpeed     float64 `json:"max_speed"`      // Movement speed (radians per tick)
	YawRange     float64 `json:"yaw_range"`      // Head yaw limit (radians)
	BodyYawLimit float64 `json:"body_yaw_limit"` // Body rotation limit (radians)

	// === NEW: Scan behavior ===
	ScanStartDelay float64 `json:"scan_start_delay"` // Seconds before scanning starts
	ScanSpeed      float64 `json:"scan_speed"`       // Scan speed (rad/s)
	ScanRange      float64 `json:"scan_range"`       // Scan extent (radians)
}

// GetTuningParams returns current tuning parameters from the tracker.
func (t *Tracker) GetTuningParams() TuningParams {
	t.mu.RLock()
	defer t.mu.RUnlock()

	detectionHz := 1.0 / t.config.DetectionInterval.Seconds()

	return TuningParams{
		// Existing params
		OffsetSmoothingAlpha: t.perception.offsetSmoothingAlpha,
		PositionSmoothing:    t.perception.smoothingFactor,
		MaxTargetVelocity:    t.controller.MaxTargetVelocity,
		Kp:                   t.controller.Kp,
		Kd:                   t.controller.Kd,
		ControlDeadZone:      t.controller.DeadZone,
		ResponseScale:        t.config.ResponseScale,
		DetectionHz:          detectionHz,
		TuningModeEnabled:    !t.config.AudioSwitchEnabled && !t.config.BreathingEnabled,

		// Body alignment
		BodyAlignmentEnabled:   t.config.BodyAlignmentEnabled,
		BodyAlignmentDelay:     t.config.BodyAlignmentDelay.Seconds(),
		BodyAlignmentThreshold: t.config.BodyAlignmentThreshold,
		BodyAlignmentSpeed:     t.config.BodyAlignmentSpeed,
		BodyAlignmentDeadZone:  t.config.BodyAlignmentDeadZone,
		BodyAlignmentCooldown:  t.config.BodyAlignmentCooldown.Seconds(),

		// Pitch-specific
		KpPitch:        t.controller.KpPitch,
		KdPitch:        t.controller.KdPitch,
		PitchDeadZone:  t.controller.PitchDeadZone,
		PitchRangeUp:   t.config.PitchRangeUp,
		PitchRangeDown: t.config.PitchRangeDown,

		// Audio tracking
		AudioSwitchEnabled:       t.config.AudioSwitchEnabled,
		AudioSwitchThreshold:     t.config.AudioSwitchThreshold,
		AudioSwitchMinConfidence: t.config.AudioSwitchMinConfidence,
		AudioSwitchLookDuration:  t.config.AudioSwitchLookDuration.Seconds(),

		// Breathing
		BreathingEnabled:    t.config.BreathingEnabled,
		BreathingAmplitude:  t.config.BreathingAmplitude,
		BreathingFrequency:  t.config.BreathingFrequency,
		BreathingAntennaAmp: t.config.BreathingAntennaAmp,

		// Range/speed
		MaxSpeed:     t.config.MaxSpeed,
		YawRange:     t.config.YawRange,
		BodyYawLimit: t.config.BodyYawLimit,

		// Scan behavior
		ScanStartDelay: t.config.ScanStartDelay.Seconds(),
		ScanSpeed:      t.config.ScanSpeed,
		ScanRange:      t.config.ScanRange,
	}
}

// SetTuningParams updates tuning parameters at runtime.
// Only non-zero values are applied (zero values are ignored to allow partial updates).
func (t *Tracker) SetTuningParams(params TuningParams) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// === Smoothing ===
	if params.OffsetSmoothingAlpha > 0 {
		t.perception.SetOffsetSmoothingAlpha(params.OffsetSmoothingAlpha)
	}
	if params.PositionSmoothing > 0 {
		t.perception.smoothingFactor = clamp(params.PositionSmoothing, 0.0, 1.0)
	}

	// === Velocity limiting ===
	if params.MaxTargetVelocity > 0 {
		t.controller.SetMaxTargetVelocity(params.MaxTargetVelocity)
	}

	// === PD Controller (yaw) ===
	if params.Kp > 0 {
		t.controller.Kp = params.Kp
	}
	if params.Kd > 0 {
		t.controller.Kd = params.Kd
	}
	if params.ControlDeadZone > 0 {
		t.controller.DeadZone = params.ControlDeadZone
	}
	if params.ResponseScale > 0 {
		t.config.ResponseScale = clamp(params.ResponseScale, 0.0, 1.0)
	}

	// === Detection rate ===
	if params.DetectionHz > 0 {
		t.setDetectionHz(params.DetectionHz)
	}

	// === Body alignment ===
	// Handle enabled flag specially: explicit false is valid
	// We check if any body params are set to distinguish between "not sent" and "sent as false"
	hasOtherBodyParams := params.BodyAlignmentDelay > 0 || params.BodyAlignmentSpeed > 0 ||
		params.BodyAlignmentThreshold > 0 || params.BodyAlignmentDeadZone > 0 ||
		params.BodyAlignmentCooldown > 0
	if params.BodyAlignmentEnabled {
		t.config.BodyAlignmentEnabled = true
	} else if !hasOtherBodyParams {
		// Only enabled:false was sent, explicitly disable
		t.config.BodyAlignmentEnabled = false
	}
	if params.BodyAlignmentDelay > 0 {
		t.config.BodyAlignmentDelay = time.Duration(params.BodyAlignmentDelay * float64(time.Second))
	}
	if params.BodyAlignmentThreshold > 0 {
		t.config.BodyAlignmentThreshold = params.BodyAlignmentThreshold
	}
	if params.BodyAlignmentSpeed > 0 {
		t.config.BodyAlignmentSpeed = params.BodyAlignmentSpeed
	}
	if params.BodyAlignmentDeadZone > 0 {
		t.config.BodyAlignmentDeadZone = params.BodyAlignmentDeadZone
	}
	if params.BodyAlignmentCooldown > 0 {
		t.config.BodyAlignmentCooldown = time.Duration(params.BodyAlignmentCooldown * float64(time.Second))
	}

	// === Pitch-specific ===
	if params.KpPitch > 0 {
		t.controller.KpPitch = params.KpPitch
	}
	if params.KdPitch > 0 {
		t.controller.KdPitch = params.KdPitch
	}
	if params.PitchDeadZone > 0 {
		t.controller.PitchDeadZone = params.PitchDeadZone
	}
	if params.PitchRangeUp > 0 {
		t.config.PitchRangeUp = params.PitchRangeUp
		t.controller.MaxPitchUp = params.PitchRangeUp
	}
	if params.PitchRangeDown > 0 {
		t.config.PitchRangeDown = params.PitchRangeDown
		t.controller.MaxPitchDown = params.PitchRangeDown
	}

	// === Audio tracking ===
	// Handle audio enabled flag specially (same pattern as body alignment)
	hasOtherAudioParams := params.AudioSwitchThreshold > 0 || params.AudioSwitchMinConfidence > 0 ||
		params.AudioSwitchLookDuration > 0
	if params.AudioSwitchEnabled {
		t.config.AudioSwitchEnabled = true
	} else if !hasOtherAudioParams {
		t.config.AudioSwitchEnabled = false
	}
	if params.AudioSwitchThreshold > 0 {
		t.config.AudioSwitchThreshold = params.AudioSwitchThreshold
	}
	if params.AudioSwitchMinConfidence > 0 {
		t.config.AudioSwitchMinConfidence = clamp(params.AudioSwitchMinConfidence, 0.0, 1.0)
	}
	if params.AudioSwitchLookDuration > 0 {
		t.config.AudioSwitchLookDuration = time.Duration(params.AudioSwitchLookDuration * float64(time.Second))
	}

	// === Breathing ===
	hasOtherBreathingParams := params.BreathingAmplitude > 0 || params.BreathingFrequency > 0 ||
		params.BreathingAntennaAmp > 0
	if params.BreathingEnabled {
		t.config.BreathingEnabled = true
	} else if !hasOtherBreathingParams {
		t.config.BreathingEnabled = false
	}
	if params.BreathingAmplitude > 0 {
		t.config.BreathingAmplitude = params.BreathingAmplitude
	}
	if params.BreathingFrequency > 0 {
		t.config.BreathingFrequency = params.BreathingFrequency
	}
	if params.BreathingAntennaAmp > 0 {
		t.config.BreathingAntennaAmp = params.BreathingAntennaAmp
	}

	// === Range/speed ===
	if params.MaxSpeed > 0 {
		t.config.MaxSpeed = params.MaxSpeed
	}
	if params.YawRange > 0 {
		t.config.YawRange = params.YawRange
		t.controller.MaxYaw = params.YawRange
		t.controller.SoftLimit = params.YawRange * 0.85
	}
	if params.BodyYawLimit > 0 {
		t.config.BodyYawLimit = params.BodyYawLimit
		// Also update world model if available
		if t.world != nil {
			t.world.SetBodyYawLimit(params.BodyYawLimit)
		}
	}

	// === Scan behavior ===
	if params.ScanStartDelay > 0 {
		t.config.ScanStartDelay = time.Duration(params.ScanStartDelay * float64(time.Second))
	}
	if params.ScanSpeed > 0 {
		t.config.ScanSpeed = params.ScanSpeed
	}
	if params.ScanRange > 0 {
		t.config.ScanRange = params.ScanRange
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
		t.config.ScanStartDelay = 999 * time.Second
	} else {
		// Restore defaults
		defaults := DefaultConfig()
		t.config.AudioSwitchEnabled = defaults.AudioSwitchEnabled
		t.config.BreathingEnabled = defaults.BreathingEnabled
		t.config.ScanStartDelay = defaults.ScanStartDelay
	}
}
