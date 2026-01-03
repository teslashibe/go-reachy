package tracking

import "testing"

func TestDefaultConfig_ResponseScale(t *testing.T) {
	cfg := DefaultConfig()

	// ResponseScale should be 0.6 (matches Python reachy)
	if cfg.ResponseScale != 0.6 {
		t.Errorf("Expected ResponseScale=0.6, got %v", cfg.ResponseScale)
	}

	// PD gains should be tuned for smooth tracking
	if cfg.Kp != 0.10 {
		t.Errorf("Expected Kp=0.10, got %v", cfg.Kp)
	}
	if cfg.Kd != 0.08 {
		t.Errorf("Expected Kd=0.08, got %v", cfg.Kd)
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

