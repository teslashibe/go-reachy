// Package detection provides face detection using computer vision
package detection

// Detection represents a detected face
type Detection struct {
	X, Y       float64 // Center position (0-1 normalized)
	W, H       float64 // Width and height (0-1 normalized)
	Confidence float64 // Detection confidence (0-1)
}

// Center returns the center point of the detection
func (d Detection) Center() (x, y float64) {
	return d.X + d.W/2, d.Y + d.H/2
}

// Area returns the area of the bounding box
func (d Detection) Area() float64 {
	return d.W * d.H
}

// Detector is the interface for face detection backends
type Detector interface {
	// Detect finds faces in the image and returns their positions
	Detect(jpeg []byte) ([]Detection, error)

	// Close releases resources
	Close() error
}

// Config holds detector configuration
type Config struct {
	ModelPath        string  // Path to ONNX model
	ConfidenceThresh float64 // Minimum confidence (default 0.5)
	InputWidth       int     // Model input width
	InputHeight      int     // Model input height
}

// DefaultConfig returns production defaults for YuNet
func DefaultConfig() Config {
	return Config{
		ModelPath:        "models/face_detection_yunet.onnx",
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}
}

// SelectBest picks the best face from multiple detections
// Priority: confidence * 0.7 + area * 0.3 (matches Reachy Python)
func SelectBest(dets []Detection) *Detection {
	if len(dets) == 0 {
		return nil
	}

	if len(dets) == 1 {
		return &dets[0]
	}

	// Find max area for normalization
	maxArea := 0.0
	for _, d := range dets {
		if d.Area() > maxArea {
			maxArea = d.Area()
		}
	}

	// Score each detection
	bestScore := -1.0
	var best *Detection

	for i := range dets {
		score := dets[i].Confidence*0.7 + (dets[i].Area()/maxArea)*0.3
		if score > bestScore {
			bestScore = score
			best = &dets[i]
		}
	}

	return best
}
