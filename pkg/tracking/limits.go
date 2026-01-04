// Package tracking provides head and body tracking for Eva.
// This file defines mechanical limits matching Python reachy.
package tracking

import "math"

// Mechanical limits matching Python reachy implementation.
// Source: reachy/src/reachy_mini_conversation_app/profiles/example/sweep_look.py
//
// Python uses: max_angle = 0.9 * np.pi  # ≈ 2.83 rad ≈ 162°

const (
	// DefaultBodyYawLimit is the maximum body rotation in radians (±162°).
	// Matches Python reachy's 0.9 * π limit.
	DefaultBodyYawLimit = 0.9 * math.Pi // ≈ 2.827 rad ≈ 162°

	// DefaultPitchRangeUp is the maximum head pitch looking up in radians (30°).
	// Matches Python reachy's move_head tool default.
	DefaultPitchRangeUp = 30.0 * math.Pi / 180.0 // 0.523 rad = 30°

	// DefaultPitchRangeDown is the maximum head pitch looking down in radians (30°).
	// Matches Python reachy's move_head tool default.
	DefaultPitchRangeDown = 30.0 * math.Pi / 180.0 // 0.523 rad = 30°

	// DefaultHeadYawRange is the maximum head yaw in radians (±86°).
	// This is the head-only range before body rotation kicks in.
	DefaultHeadYawRange = 1.5 // ±1.5 rad ≈ ±86°
)

// Degrees converts radians to degrees for logging/display.
func Degrees(radians float64) float64 {
	return radians * 180.0 / math.Pi
}

// Radians converts degrees to radians.
func Radians(degrees float64) float64 {
	return degrees * math.Pi / 180.0
}

