package tracking

import (
	"fmt"
	"math"

	"github.com/teslashibe/go-reachy/pkg/tracking/detection"
)

// Perception handles converting camera frame detections to world coordinates
type Perception struct {
	// Detector
	detector detection.Detector

	// Camera properties
	CameraFOV float64 // Horizontal field of view in radians

	// Smoothing
	smoothedPosition float64
	hasLastPosition  bool
	smoothingFactor  float64 // 0-1, higher = more weight on new reading

	// Detection state
	lastValidPosition float64
	consecutiveMisses int
}

// NewPerception creates a new perception system
func NewPerception(config Config, detector detection.Detector) *Perception {
	return &Perception{
		detector:        detector,
		CameraFOV:       config.CameraFOV,
		smoothingFactor: config.PositionSmoothing,
	}
}

// FrameToWorld converts a frame position (0-100%) to a world angle
// currentYaw is the current head yaw in radians
func (p *Perception) FrameToWorld(framePosition float64, currentYaw float64) float64 {
	// Frame offset from center: -0.5 to +0.5
	frameOffset := (framePosition - 50) / 100.0

	// Convert to camera-relative angle
	// At 0% position, we're at -FOV/2 from camera center
	// At 100% position, we're at +FOV/2 from camera center
	cameraAngle := frameOffset * p.CameraFOV

	// Add current head yaw to get world angle
	// Note: positive yaw = looking left, so we subtract camera angle
	// because positive frame position = right side of frame
	worldAngle := currentYaw - cameraAngle

	return worldAngle
}

// WorldToFrame converts a world angle to expected frame position
// Returns the expected position (0-100%) if the target were visible
func (p *Perception) WorldToFrame(worldAngle float64, currentYaw float64) float64 {
	// Calculate camera-relative angle
	cameraAngle := currentYaw - worldAngle

	// Convert to frame position
	frameOffset := cameraAngle / p.CameraFOV
	framePosition := 50 - frameOffset*100

	return framePosition
}

// IsInFrame returns true if a world angle would be visible in the current frame
func (p *Perception) IsInFrame(worldAngle float64, currentYaw float64) bool {
	cameraAngle := math.Abs(currentYaw - worldAngle)
	return cameraAngle < p.CameraFOV/2
}

// DetectFace captures a frame and detects face position using local vision
// Returns (framePosition, worldAngle, found)
func (p *Perception) DetectFace(video VideoSource, currentYaw float64) (float64, float64, bool) {
	if video == nil || p.detector == nil {
		return 0, 0, false
	}

	frame, err := video.CaptureJPEG()
	if err != nil {
		return 0, 0, false
	}

	// Run local face detection
	detections, err := p.detector.Detect(frame)
	if err != nil {
		fmt.Printf("👁️  Detection error: %v\n", err)
		p.consecutiveMisses++
		return 0, 0, false
	}

	// Select best face if multiple found
	best := detection.SelectBest(detections)
	if best == nil {
		p.consecutiveMisses++
		return 0, 0, false
	}

	// Convert detection center to frame position (0-100%)
	cx, _ := best.Center()
	position := cx * 100.0 // Normalized 0-1 to 0-100%

	// Clamp to 0-100
	position = clamp(position, 0, 100)

	// Apply smoothing
	if p.hasLastPosition {
		position = p.smoothingFactor*position + (1-p.smoothingFactor)*p.smoothedPosition
	}
	p.smoothedPosition = position
	p.hasLastPosition = true
	p.lastValidPosition = position
	p.consecutiveMisses = 0

	// Convert to world coordinates
	worldAngle := p.FrameToWorld(position, currentYaw)

	return position, worldAngle, true
}

// GetConsecutiveMisses returns how many consecutive detections have failed
func (p *Perception) GetConsecutiveMisses() int {
	return p.consecutiveMisses
}

// GetLastValidPosition returns the last successfully detected frame position
func (p *Perception) GetLastValidPosition() float64 {
	return p.lastValidPosition
}
