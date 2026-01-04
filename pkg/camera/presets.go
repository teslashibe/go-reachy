package camera

// Preset names for common configurations
const (
	PresetDefault = "default"
	PresetLegacy  = "legacy"
	Preset720p    = "720p"
	Preset1080p   = "1080p"
	Preset4K      = "4k"
	PresetNight   = "night"
	PresetBright  = "bright"
	PresetZoom2x  = "zoom2x"
	PresetZoom4x  = "zoom4x"
)

// Presets returns all available preset configurations.
func Presets() map[string]Config {
	return map[string]Config{
		PresetDefault: DefaultConfig(),
		PresetLegacy:  LegacyConfig(),
		Preset720p:    HD720Config(),
		Preset1080p:   HD1080Config(),
		Preset4K:      UHD4KConfig(),
		PresetNight:   NightModeConfig(),
		PresetBright:  BrightModeConfig(),
		PresetZoom2x:  Zoom2xConfig(),
		PresetZoom4x:  Zoom4xConfig(),
	}
}

// PresetNames returns the list of available preset names.
func PresetNames() []string {
	return []string{
		PresetDefault,
		PresetLegacy,
		Preset720p,
		Preset1080p,
		Preset4K,
		PresetNight,
		PresetBright,
		PresetZoom2x,
		PresetZoom4x,
	}
}

// GetPreset returns a preset config by name, or nil if not found.
func GetPreset(name string) *Config {
	presets := Presets()
	if cfg, ok := presets[name]; ok {
		return &cfg
	}
	return nil
}

// HD720Config returns 720p HD configuration.
// Good balance of quality and performance.
func HD720Config() Config {
	cfg := DefaultConfig()
	cfg.Width = 1280
	cfg.Height = 720
	return cfg
}

// HD1080Config returns 1080p Full HD configuration.
// Best for face tracking accuracy.
func HD1080Config() Config {
	cfg := DefaultConfig()
	cfg.Width = 1920
	cfg.Height = 1080
	return cfg
}

// UHD4KConfig returns 4K UHD configuration.
// Maximum quality, higher CPU usage.
func UHD4KConfig() Config {
	cfg := DefaultConfig()
	cfg.Width = 3840
	cfg.Height = 2160
	cfg.Framerate = 15 // Lower framerate for 4K
	return cfg
}

// NightModeConfig returns configuration optimized for low light.
// Uses long exposure mode with shadow priority.
func NightModeConfig() Config {
	cfg := DefaultConfig()
	cfg.Width = 1280  // Lower res for faster processing in low light
	cfg.Height = 720
	cfg.ExposureMode = "long"
	cfg.ConstraintMode = "shadows"
	cfg.ExposureValue = 1.0 // +1 stop brighter
	return cfg
}

// BrightModeConfig returns configuration optimized for bright scenes.
// Prevents overexposure in highlights.
func BrightModeConfig() Config {
	cfg := DefaultConfig()
	cfg.ConstraintMode = "highlight"
	cfg.ExposureValue = -0.5 // Slightly darker to preserve highlights
	return cfg
}

// Zoom2xConfig returns 2x digital zoom configuration.
func Zoom2xConfig() Config {
	cfg := DefaultConfig()
	cfg.ZoomLevel = 2.0
	return cfg
}

// Zoom4xConfig returns 4x digital zoom configuration.
func Zoom4xConfig() Config {
	cfg := DefaultConfig()
	cfg.ZoomLevel = 4.0
	return cfg
}


