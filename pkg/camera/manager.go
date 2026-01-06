package camera

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Manager holds the current camera configuration and handles updates.
type Manager struct {
	config Config
	mu     sync.RWMutex

	// Callback when config changes (for applying to camera)
	OnConfigChange func(cfg Config) error
}

// NewManager creates a new camera manager with default config.
func NewManager() *Manager {
	return &Manager{
		config: DefaultConfig(),
	}
}

// GetConfig returns the current camera configuration.
func (m *Manager) GetConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// SetConfig updates the camera configuration.
func (m *Manager) SetConfig(cfg Config) error {
	// Validate
	if errors := cfg.Validate(); len(errors) > 0 {
		return fmt.Errorf("validation failed: %v", errors)
	}

	m.mu.Lock()
	m.config = cfg
	callback := m.OnConfigChange
	m.mu.Unlock()

	// Notify callback if set
	if callback != nil {
		if err := callback(cfg); err != nil {
			return fmt.Errorf("failed to apply config: %w", err)
		}
	}

	return nil
}

// UpdateConfig updates specific fields of the configuration.
// Accepts a map of field names to values.
func (m *Manager) UpdateConfig(params map[string]interface{}) error {
	m.mu.Lock()
	cfg := m.config
	m.mu.Unlock()

	// Check for preset first
	if presetName, ok := params["preset"].(string); ok {
		preset := GetPreset(presetName)
		if preset == nil {
			return fmt.Errorf("unknown preset: %s", presetName)
		}
		cfg = *preset
		// Remove preset from params so we can still apply other overrides
		delete(params, "preset")
	}

	// Apply individual parameters
	for key, value := range params {
		switch key {
		case "width":
			if v, ok := toInt(value); ok {
				cfg.Width = v
			}
		case "height":
			if v, ok := toInt(value); ok {
				cfg.Height = v
			}
		case "framerate":
			if v, ok := toInt(value); ok {
				cfg.Framerate = v
			}
		case "quality":
			if v, ok := toInt(value); ok {
				cfg.Quality = v
			}
		case "exposure_mode":
			if v, ok := value.(string); ok {
				cfg.ExposureMode = v
			}
		case "constraint_mode":
			if v, ok := value.(string); ok {
				cfg.ConstraintMode = v
			}
		case "exposure_value":
			if v, ok := toFloat(value); ok {
				cfg.ExposureValue = v
			}
		case "brightness":
			if v, ok := toFloat(value); ok {
				cfg.Brightness = v
			}
		case "analogue_gain":
			if v, ok := toFloat(value); ok {
				cfg.AnalogueGain = v
			}
		case "exposure_time":
			if v, ok := toInt(value); ok {
				cfg.ExposureTime = v
			}
		case "zoom_level":
			if v, ok := toFloat(value); ok {
				cfg.ZoomLevel = v
			}
		case "crop_x":
			if v, ok := toInt(value); ok {
				cfg.CropX = v
			}
		case "crop_y":
			if v, ok := toInt(value); ok {
				cfg.CropY = v
			}
		case "crop_width":
			if v, ok := toInt(value); ok {
				cfg.CropWidth = v
			}
		case "crop_height":
			if v, ok := toInt(value); ok {
				cfg.CropHeight = v
			}
		case "af_mode":
			if v, ok := value.(string); ok {
				cfg.AfMode = v
			}
		}
	}

	return m.SetConfig(cfg)
}

// GetConfigJSON returns the current config as a map for JSON serialization.
func (m *Manager) GetConfigJSON() map[string]interface{} {
	cfg := m.GetConfig()
	
	// Convert to map via JSON for consistent serialization
	data, _ := json.Marshal(cfg)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	
	return result
}

// Helper functions for type conversion

func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case json.Number:
		i, err := val.Int64()
		if err == nil {
			return int(i), true
		}
	}
	return 0, false
}

func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		if err == nil {
			return f, true
		}
	}
	return 0, false
}




