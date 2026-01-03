// Package camera provides runtime-configurable camera settings for Eva.
// This follows the same pattern as pkg/tracking for tunable parameters.
package camera

// Config holds all camera configuration parameters.
// These can be modified via the camera API at runtime.
type Config struct {
	// === Resolution ===
	Width     int `json:"width"`     // Frame width in pixels
	Height    int `json:"height"`    // Frame height in pixels
	Framerate int `json:"framerate"` // Target FPS
	Quality   int `json:"quality"`   // JPEG quality 1-100

	// === Low Light Controls ===
	// ExposureMode controls how the AE algorithm balances shutter/gain.
	// Values: "normal", "short", "long"
	ExposureMode string `json:"exposure_mode"`

	// ConstraintMode controls scene brightness optimization.
	// Values: "normal", "highlight", "shadows"
	ConstraintMode string `json:"constraint_mode"`

	// ExposureValue is EV compensation in stops (-2.0 to +2.0).
	// Positive = brighter, negative = darker.
	ExposureValue float64 `json:"exposure_value"`

	// Brightness adjustment (-1.0 to +1.0).
	Brightness float64 `json:"brightness"`

	// AnalogueGain is manual sensor gain (1.0 to 16.0).
	// Set to 0 for auto gain.
	AnalogueGain float64 `json:"analogue_gain"`

	// ExposureTime is manual exposure in microseconds (100 to 120000).
	// Set to 0 for auto exposure.
	ExposureTime int `json:"exposure_time"`

	// === Digital Zoom ===
	// ZoomLevel is digital zoom factor (1.0 to 4.0).
	// Uses scaler-crop internally.
	ZoomLevel float64 `json:"zoom_level"`

	// Manual crop region (overrides ZoomLevel if set).
	// All values in native sensor pixels.
	CropX      int `json:"crop_x"`
	CropY      int `json:"crop_y"`
	CropWidth  int `json:"crop_width"`
	CropHeight int `json:"crop_height"`

	// === Autofocus ===
	// AfMode controls autofocus behavior.
	// Values: "manual", "auto", "continuous"
	AfMode string `json:"af_mode"`
}

// Sensor capabilities for IMX708 Wide
const (
	SensorMaxWidth  = 4608
	SensorMaxHeight = 2592
	SensorMaxGain   = 16.0
	SensorMaxExposure = 120000 // microseconds
	SensorMaxZoom   = 4.0
)

// DefaultConfig returns the recommended high-resolution configuration.
// Uses 1920x1080 (1080p) for optimal face tracking accuracy.
func DefaultConfig() Config {
	return Config{
		// High resolution for better tracking
		Width:     1920,
		Height:    1080,
		Framerate: 30,
		Quality:   85,

		// Auto exposure with balanced settings
		ExposureMode:   "normal",
		ConstraintMode: "normal",
		ExposureValue:  0.0,
		Brightness:     0.0,
		AnalogueGain:   0, // Auto
		ExposureTime:   0, // Auto

		// No zoom
		ZoomLevel:  1.0,
		CropX:      0,
		CropY:      0,
		CropWidth:  0,
		CropHeight: 0,

		// Continuous autofocus for tracking
		AfMode: "continuous",
	}
}

// LegacyConfig returns the original 640x480 configuration.
// Use this if higher resolution causes issues.
func LegacyConfig() Config {
	cfg := DefaultConfig()
	cfg.Width = 640
	cfg.Height = 480
	return cfg
}

// Validate checks if the config values are within valid ranges.
// Returns a list of validation errors, or nil if valid.
func (c *Config) Validate() []string {
	var errors []string

	// Resolution
	if c.Width < 160 || c.Width > SensorMaxWidth {
		errors = append(errors, "width must be between 160 and 4608")
	}
	if c.Height < 120 || c.Height > SensorMaxHeight {
		errors = append(errors, "height must be between 120 and 2592")
	}
	if c.Framerate < 1 || c.Framerate > 120 {
		errors = append(errors, "framerate must be between 1 and 120")
	}
	if c.Quality < 1 || c.Quality > 100 {
		errors = append(errors, "quality must be between 1 and 100")
	}

	// Exposure mode
	validExposureModes := map[string]bool{"normal": true, "short": true, "long": true}
	if c.ExposureMode != "" && !validExposureModes[c.ExposureMode] {
		errors = append(errors, "exposure_mode must be normal, short, or long")
	}

	// Constraint mode
	validConstraintModes := map[string]bool{"normal": true, "highlight": true, "shadows": true}
	if c.ConstraintMode != "" && !validConstraintModes[c.ConstraintMode] {
		errors = append(errors, "constraint_mode must be normal, highlight, or shadows")
	}

	// EV
	if c.ExposureValue < -2.0 || c.ExposureValue > 2.0 {
		errors = append(errors, "exposure_value must be between -2.0 and 2.0")
	}

	// Brightness
	if c.Brightness < -1.0 || c.Brightness > 1.0 {
		errors = append(errors, "brightness must be between -1.0 and 1.0")
	}

	// Gain
	if c.AnalogueGain != 0 && (c.AnalogueGain < 1.0 || c.AnalogueGain > SensorMaxGain) {
		errors = append(errors, "analogue_gain must be 0 (auto) or between 1.0 and 16.0")
	}

	// Exposure time
	if c.ExposureTime != 0 && (c.ExposureTime < 100 || c.ExposureTime > SensorMaxExposure) {
		errors = append(errors, "exposure_time must be 0 (auto) or between 100 and 120000")
	}

	// Zoom
	if c.ZoomLevel < 1.0 || c.ZoomLevel > SensorMaxZoom {
		errors = append(errors, "zoom_level must be between 1.0 and 4.0")
	}

	// AF mode
	validAfModes := map[string]bool{"manual": true, "auto": true, "continuous": true}
	if c.AfMode != "" && !validAfModes[c.AfMode] {
		errors = append(errors, "af_mode must be manual, auto, or continuous")
	}

	return errors
}

// Capabilities returns the camera sensor capabilities.
func Capabilities() map[string]interface{} {
	return map[string]interface{}{
		"sensor":          "imx708_wide",
		"max_width":       SensorMaxWidth,
		"max_height":      SensorMaxHeight,
		"max_gain":        SensorMaxGain,
		"max_exposure_us": SensorMaxExposure,
		"max_zoom":        SensorMaxZoom,
		"exposure_modes":  []string{"normal", "short", "long"},
		"constraint_modes": []string{"normal", "highlight", "shadows"},
		"af_modes":        []string{"manual", "auto", "continuous"},
	}
}

