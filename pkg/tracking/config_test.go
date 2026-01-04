package tracking

import "testing"

func TestDefaultConfig_ResponseScale(t *testing.T) {
	cfg := DefaultConfig()

	// ResponseScale is tuned to 0.45 (intentionally lower than Python's 0.6 for smoother tracking)
	// Can be tuned via API at runtime
	if cfg.ResponseScale != 0.45 {
		t.Errorf("Expected ResponseScale=0.45, got %v", cfg.ResponseScale)
	}

	// PD gains should be tuned for smooth tracking
	if cfg.Kp != 0.10 {
		t.Errorf("Expected Kp=0.10, got %v", cfg.Kp)
	}
	if cfg.Kd != 0.12 { // Tuned up from 0.08 for better dampening
		t.Errorf("Expected Kd=0.12, got %v", cfg.Kd)
	}
	if cfg.ControlDeadZone != 0.05 {
		t.Errorf("Expected ControlDeadZone=0.05, got %v", cfg.ControlDeadZone)
	}
}

func TestSlowConfig_ResponseScale(t *testing.T) {
	cfg := SlowConfig()

	// SlowConfig should have even more scaling
	if cfg.ResponseScale != 0.5 {
		t.Errorf("Expected ResponseScale=0.5, got %v", cfg.ResponseScale)
	}
}

func TestAggressiveConfig_ResponseScale(t *testing.T) {
	cfg := AggressiveConfig()

	// AggressiveConfig should have less scaling
	if cfg.ResponseScale != 0.8 {
		t.Errorf("Expected ResponseScale=0.8, got %v", cfg.ResponseScale)
	}
}

func TestResponseScale_ValidRange(t *testing.T) {
	// All configs should have ResponseScale in valid range (0, 1]
	configs := []struct {
		name string
		cfg  Config
	}{
		{"Default", DefaultConfig()},
		{"Slow", SlowConfig()},
		{"Aggressive", AggressiveConfig()},
	}

	for _, tc := range configs {
		if tc.cfg.ResponseScale <= 0 || tc.cfg.ResponseScale > 1 {
			t.Errorf("%s: ResponseScale=%v out of range (0, 1]",
				tc.name, tc.cfg.ResponseScale)
		}
	}
}

func TestDefaultConfig_AudioSwitch(t *testing.T) {
	cfg := DefaultConfig()

	// Audio switch should be enabled by default
	if !cfg.AudioSwitchEnabled {
		t.Error("Expected AudioSwitchEnabled=true by default")
	}

	// Threshold should be ~30 degrees (0.52 rad)
	if cfg.AudioSwitchThreshold < 0.5 || cfg.AudioSwitchThreshold > 0.6 {
		t.Errorf("Expected AudioSwitchThreshold ~0.52 rad, got %v", cfg.AudioSwitchThreshold)
	}

	// Min confidence should be reasonably high to avoid false triggers
	if cfg.AudioSwitchMinConfidence < 0.5 {
		t.Errorf("Expected AudioSwitchMinConfidence >= 0.5, got %v", cfg.AudioSwitchMinConfidence)
	}

	// Look duration should be reasonable (1-3 seconds)
	if cfg.AudioSwitchLookDuration.Seconds() < 1 || cfg.AudioSwitchLookDuration.Seconds() > 3 {
		t.Errorf("Expected AudioSwitchLookDuration 1-3s, got %v", cfg.AudioSwitchLookDuration)
	}
}

// Issue #79: Tests for body and pitch limits matching Python reachy

func TestDefaultConfig_BodyYawLimit(t *testing.T) {
	cfg := DefaultConfig()

	// BodyYawLimit should match Python reachy's 0.9*π ≈ 2.827 rad (162°)
	expectedLimit := DefaultBodyYawLimit
	if cfg.BodyYawLimit != expectedLimit {
		t.Errorf("Expected BodyYawLimit=%v (0.9*π), got %v", expectedLimit, cfg.BodyYawLimit)
	}

	// Verify it's approximately 162 degrees
	degreesLimit := Degrees(cfg.BodyYawLimit)
	if degreesLimit < 160 || degreesLimit > 165 {
		t.Errorf("BodyYawLimit should be ~162°, got %.1f°", degreesLimit)
	}
}

func TestDefaultConfig_PitchRanges(t *testing.T) {
	cfg := DefaultConfig()

	// Pitch ranges should be ±30° (0.523 rad) matching Python reachy
	expectedPitch := DefaultPitchRangeUp // 30° = 0.523 rad

	if cfg.PitchRangeUp != expectedPitch {
		t.Errorf("Expected PitchRangeUp=%v (30°), got %v", expectedPitch, cfg.PitchRangeUp)
	}

	if cfg.PitchRangeDown != expectedPitch {
		t.Errorf("Expected PitchRangeDown=%v (30°), got %v", expectedPitch, cfg.PitchRangeDown)
	}

	// Verify they're approximately 30 degrees
	if Degrees(cfg.PitchRangeUp) < 29 || Degrees(cfg.PitchRangeUp) > 31 {
		t.Errorf("PitchRangeUp should be ~30°, got %.1f°", Degrees(cfg.PitchRangeUp))
	}
}

func TestLimitsConstants(t *testing.T) {
	// Verify limits constants match Python reachy values

	// Body: 0.9 * π ≈ 2.827 rad ≈ 162°
	if Degrees(DefaultBodyYawLimit) < 160 || Degrees(DefaultBodyYawLimit) > 165 {
		t.Errorf("DefaultBodyYawLimit should be ~162°, got %.1f°", Degrees(DefaultBodyYawLimit))
	}

	// Pitch: 30° = π/6 ≈ 0.523 rad
	if Degrees(DefaultPitchRangeUp) < 29 || Degrees(DefaultPitchRangeUp) > 31 {
		t.Errorf("DefaultPitchRangeUp should be ~30°, got %.1f°", Degrees(DefaultPitchRangeUp))
	}

	if Degrees(DefaultPitchRangeDown) < 29 || Degrees(DefaultPitchRangeDown) > 31 {
		t.Errorf("DefaultPitchRangeDown should be ~30°, got %.1f°", Degrees(DefaultPitchRangeDown))
	}

	// Head yaw: 0.9 * π ≈ 2.827 rad ≈ 162° (matches Python reachy)
	if Degrees(DefaultHeadYawRange) < 161 || Degrees(DefaultHeadYawRange) > 163 {
		t.Errorf("DefaultHeadYawRange should be ~162°, got %.1f°", Degrees(DefaultHeadYawRange))
	}
}

func TestDegreesRadiansConversion(t *testing.T) {
	// Test round-trip conversion
	degrees := 45.0
	radians := Radians(degrees)
	backToDegrees := Degrees(radians)

	if backToDegrees < 44.9 || backToDegrees > 45.1 {
		t.Errorf("Round-trip conversion failed: 45° -> %v rad -> %.1f°", radians, backToDegrees)
	}
}

