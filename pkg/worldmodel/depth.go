package worldmodel

// Depth estimation constants
// These are calibrated for the Reachy Mini camera
const (
	// Average human face width in meters (~15cm)
	avgFaceWidthMeters = 0.15

	// Calibration constant: when face fills ~20% of frame width, person is ~1m away
	// This gives us: distance = calibrationConstant / faceWidthNorm
	// At 20% width (0.2): distance = 0.2 / 0.2 = 1.0m
	// At 40% width (0.4): distance = 0.2 / 0.4 = 0.5m
	// At 10% width (0.1): distance = 0.2 / 0.1 = 2.0m
	depthCalibrationConstant = 0.2
)

// EstimateDepth calculates approximate distance from normalized face width.
// faceWidth should be the face bounding box width as a fraction of frame width (0-1).
// Returns distance in meters, or 0 if face width is invalid.
//
// This uses a simple inverse relationship: distance ≈ k / faceWidth
// Accuracy is approximately ±30% at distances under 3 meters.
func EstimateDepth(faceWidth float64) float64 {
	if faceWidth <= 0 || faceWidth > 1 {
		return 0 // Invalid or unknown
	}

	// Simple inverse relationship
	distance := depthCalibrationConstant / faceWidth

	// Clamp to reasonable range (0.3m to 5m)
	if distance < 0.3 {
		distance = 0.3
	}
	if distance > 5.0 {
		distance = 5.0
	}

	return distance
}

// DistanceCategory returns a human-readable distance category
func DistanceCategory(distance float64) string {
	if distance <= 0 {
		return "unknown"
	}
	if distance < 0.5 {
		return "very close"
	}
	if distance < 1.0 {
		return "close"
	}
	if distance < 2.0 {
		return "nearby"
	}
	if distance < 3.0 {
		return "moderate"
	}
	return "far"
}

