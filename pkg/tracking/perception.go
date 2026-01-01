package tracking

import (
	"fmt"
	"math"
	"strings"

	"github.com/teslashibe/go-reachy/pkg/realtime"
)

// Perception handles converting camera frame detections to world coordinates
type Perception struct {
	// Camera properties
	CameraFOV float64 // Horizontal field of view in radians

	// Smoothing
	smoothedPosition float64
	hasLastPosition  bool
	smoothingFactor  float64 // 0-1, higher = more weight on new reading

	// Detection state
	lastValidPosition float64
	consecutiveMisses int
	isMoving          bool // True if head is currently moving (may cause blur)
}

// NewPerception creates a new perception system
func NewPerception(config Config) *Perception {
	return &Perception{
		CameraFOV:       config.CameraFOV,
		smoothingFactor: config.PositionSmoothing,
	}
}

// SetMoving indicates whether the head is currently moving
// This can be used to ignore detections during motion blur
func (p *Perception) SetMoving(moving bool) {
	p.isMoving = moving
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

// DetectFace captures a frame and detects face position
// Returns (framePosition, worldAngle, found)
func (p *Perception) DetectFace(video VideoSource, apiKey string, currentYaw float64) (float64, float64, bool) {
	// Skip detection during fast head movement (motion blur)
	if p.isMoving {
		p.consecutiveMisses++
		return 0, 0, false
	}

	if video == nil {
		return 0, 0, false
	}

	frame, err := video.CaptureJPEG()
	if err != nil {
		return 0, 0, false
	}

	// Ask Gemini for face position
	prompt := "Look at this image. Is there a person's face visible? If yes, estimate the horizontal position of their face as a number from 0 to 100, where 0 is the far left edge and 100 is the far right edge of the image. Reply with ONLY a number (like 25 or 75) or NONE if no face is visible."

	result, err := realtime.GeminiVision(apiKey, frame, prompt)
	if err != nil {
		p.consecutiveMisses++
		return 0, 0, false
	}

	result = strings.TrimSpace(strings.ToUpper(result))

	// Parse the position
	if strings.Contains(result, "NONE") || result == "" {
		p.consecutiveMisses++
		return 0, 0, false
	}

	// Try to parse as number
	var position float64
	_, err = fmt.Sscanf(result, "%f", &position)
	if err != nil {
		// Fallback to old LEFT/CENTER/RIGHT
		if strings.Contains(result, "LEFT") {
			position = 25
		} else if strings.Contains(result, "RIGHT") {
			position = 75
		} else if strings.Contains(result, "CENTER") {
			position = 50
		} else {
			p.consecutiveMisses++
			return 0, 0, false
		}
	}

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

