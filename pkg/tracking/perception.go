package tracking

import (
	"math"

	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/tracking/detection"
)

// Perception handles converting camera frame detections to world coordinates
type Perception struct {
	// Detector
	detector detection.Detector

	// Camera properties
	CameraFOV   float64 // Horizontal field of view in radians
	VerticalFOV float64 // Vertical field of view in radians

	// Smoothing (horizontal)
	smoothedPosition float64
	hasLastPosition  bool
	smoothingFactor  float64 // 0-1, higher = more weight on new reading

	// Smoothing (vertical)
	smoothedPositionY float64
	hasLastPositionY  bool

	// Detection state
	lastValidPosition  float64
	lastValidPositionY float64
	consecutiveMisses  int
}

// NewPerception creates a new perception system
func NewPerception(config Config, detector detection.Detector) *Perception {
	return &Perception{
		detector:        detector,
		CameraFOV:       config.CameraFOV,
		VerticalFOV:     config.VerticalFOV,
		smoothingFactor: config.PositionSmoothing,
	}
}

// FrameToWorld converts a frame position (0-100%) to a body-relative world angle.
// currentYaw is the current head yaw in radians (relative to body).
// The returned angle is body-relative, suitable for storing in the world model.
func (p *Perception) FrameToWorld(framePosition float64, currentYaw float64) float64 {
	// Frame offset from center: -0.5 to +0.5
	frameOffset := (framePosition - 50) / 100.0

	// Convert to camera-relative angle
	// At 0% position, we're at -FOV/2 from camera center
	// At 100% position, we're at +FOV/2 from camera center
	cameraAngle := frameOffset * p.CameraFOV

	// Add current head yaw to get body-relative angle
	// Note: positive yaw = looking left, so we subtract camera angle
	// because positive frame position = right side of frame
	bodyRelativeAngle := currentYaw - cameraAngle

	return bodyRelativeAngle
}

// FrameToRoomAngle converts a frame position to a room-relative world angle.
// This accounts for both head yaw and body yaw to give the absolute room position.
func (p *Perception) FrameToRoomAngle(framePosition float64, headYaw float64, bodyYaw float64) float64 {
	// First get body-relative angle
	bodyRelativeAngle := p.FrameToWorld(framePosition, headYaw)

	// Add body yaw to get room-relative angle
	roomAngle := bodyYaw + bodyRelativeAngle

	return roomAngle
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

// FrameToPitch converts vertical frame position to room-relative pitch angle.
// framePositionY: 0 = top of frame, 100 = bottom of frame
// currentPitch: current head pitch in radians (used to compute room-relative target)
// Returns: target pitch in radians (negative = looking up, positive = looking down)
// Note: Reachy Mini uses negative pitch for looking UP
func (p *Perception) FrameToPitch(framePositionY float64, currentPitch float64) float64 {
	// Frame offset from center: -0.5 (top) to +0.5 (bottom)
	frameOffset := (framePositionY - 50) / 100.0

	// Convert to camera-relative pitch offset
	// Face above center (negative offset) = need to pitch up more (add negative)
	// Face below center (positive offset) = need to pitch down more (add positive)
	cameraPitchOffset := frameOffset * p.VerticalFOV

	// Room-relative target = current pitch + offset to center the face
	// This computes where the face IS in room coordinates, not where to move
	return currentPitch + cameraPitchOffset
}

// DetectFace captures a frame and detects face position using local vision.
// Returns (framePosition, worldAngle, found).
// The worldAngle is body-relative (for backwards compatibility).
// Use DetectFaceRoom for room-relative coordinates.
func (p *Perception) DetectFace(video VideoSource, currentYaw float64) (float64, float64, bool) {
	return p.DetectFaceRoom(video, currentYaw, 0)
}

// DetectFaceRoom captures a frame and detects face position using local vision.
// Returns (framePosition, roomAngle, found).
// The roomAngle is in room coordinates (accounts for body rotation).
// Note: For pitch tracking, use DetectFaceRoomWithPitch instead.
func (p *Perception) DetectFaceRoom(video VideoSource, headYaw float64, bodyYaw float64) (float64, float64, bool) {
	posX, _, roomYaw, _, found := p.DetectFaceRoomWithPitch(video, headYaw, 0, bodyYaw)
	return posX, roomYaw, found
}

// DetectFaceRoomWithPitch captures a frame and detects face position with full 2D tracking.
// Returns (positionX, positionY, roomYaw, targetPitch, found).
// positionX/Y are frame positions (0-100%), roomYaw is in room coordinates,
// targetPitch is the pitch angle needed to center the face vertically.
func (p *Perception) DetectFaceRoomWithPitch(video VideoSource, headYaw, headPitch, bodyYaw float64) (float64, float64, float64, float64, bool) {
	posX, posY, roomYaw, targetPitch, _, found := p.DetectFaceRoomFull(video, headYaw, headPitch, bodyYaw)
	return posX, posY, roomYaw, targetPitch, found
}

// DetectFaceRoomFull captures a frame and detects face position with full 2D tracking and face width.
// Returns (positionX, positionY, roomYaw, targetPitch, faceWidth, found).
// positionX/Y are frame positions (0-100%), roomYaw is in room coordinates,
// targetPitch is the pitch angle needed to center the face vertically,
// faceWidth is the normalized face width (0-1) for depth estimation.
func (p *Perception) DetectFaceRoomFull(video VideoSource, headYaw, headPitch, bodyYaw float64) (float64, float64, float64, float64, float64, bool) {
	if video == nil || p.detector == nil {
		return 0, 0, 0, 0, 0, false
	}

	frame, err := video.CaptureJPEG()
	if err != nil {
		return 0, 0, 0, 0, 0, false
	}

	// Run local face detection
	detections, err := p.detector.Detect(frame)
	if err != nil {
		debug.Log("👁️  Detection error: %v\n", err)
		p.consecutiveMisses++
		return 0, 0, 0, 0, 0, false
	}

	// Select best face if multiple found
	best := detection.SelectBest(detections)
	if best == nil {
		p.consecutiveMisses++
		return 0, 0, 0, 0, 0, false
	}

	// Convert detection center to frame position (0-100%)
	cx, cy := best.Center()
	positionX := clamp(cx*100.0, 0, 100)
	positionY := clamp(cy*100.0, 0, 100)

	// Get face width for depth estimation (already normalized 0-1)
	faceWidth := best.W

	// Apply smoothing to X (horizontal)
	if p.hasLastPosition {
		positionX = p.smoothingFactor*positionX + (1-p.smoothingFactor)*p.smoothedPosition
	}
	p.smoothedPosition = positionX
	p.hasLastPosition = true
	p.lastValidPosition = positionX

	// Apply smoothing to Y (vertical)
	if p.hasLastPositionY {
		positionY = p.smoothingFactor*positionY + (1-p.smoothingFactor)*p.smoothedPositionY
	}
	p.smoothedPositionY = positionY
	p.hasLastPositionY = true
	p.lastValidPositionY = positionY

	p.consecutiveMisses = 0

	// Convert to room coordinates (yaw)
	roomYaw := p.FrameToRoomAngle(positionX, headYaw, bodyYaw)

	// Calculate target pitch
	targetPitch := p.FrameToPitch(positionY, headPitch)

	return positionX, positionY, roomYaw, targetPitch, faceWidth, true
}

// GetConsecutiveMisses returns how many consecutive detections have failed
func (p *Perception) GetConsecutiveMisses() int {
	return p.consecutiveMisses
}

// GetLastValidPosition returns the last successfully detected frame position
func (p *Perception) GetLastValidPosition() float64 {
	return p.lastValidPosition
}

// DetectFaceOffset detects a face and returns camera-relative offsets.
// Returns (yawOffset, pitchOffset, faceWidth, found).
// yawOffset: how much to turn horizontally (positive = turn left, negative = turn right)
// pitchOffset: how much to tilt vertically (positive = tilt down, negative = tilt up)
// faceWidth: normalized face width (0-1) for depth estimation
// This is self-correcting: when face is centered, offsets are 0.
func (p *Perception) DetectFaceOffset(video VideoSource) (yawOffset, pitchOffset, faceWidth float64, found bool) {
	if video == nil || p.detector == nil {
		return 0, 0, 0, false
	}

	frame, err := video.CaptureJPEG()
	if err != nil {
		return 0, 0, 0, false
	}

	// Run local face detection
	detections, err := p.detector.Detect(frame)
	if err != nil {
		debug.Log("👁️  Detection error: %v\n", err)
		p.consecutiveMisses++
		return 0, 0, 0, false
	}

	// Select best face if multiple found
	best := detection.SelectBest(detections)
	if best == nil {
		p.consecutiveMisses++
		return 0, 0, 0, false
	}

	// Convert detection center to frame position (0-100%)
	cx, cy := best.Center()
	positionX := clamp(cx*100.0, 0, 100)
	positionY := clamp(cy*100.0, 0, 100)

	// Get face width for depth estimation (already normalized 0-1)
	faceWidth = best.W

	// Apply smoothing to X (horizontal)
	if p.hasLastPosition {
		positionX = p.smoothingFactor*positionX + (1-p.smoothingFactor)*p.smoothedPosition
	}
	p.smoothedPosition = positionX
	p.hasLastPosition = true
	p.lastValidPosition = positionX

	// Apply smoothing to Y (vertical)
	if p.hasLastPositionY {
		positionY = p.smoothingFactor*positionY + (1-p.smoothingFactor)*p.smoothedPositionY
	}
	p.smoothedPositionY = positionY
	p.hasLastPositionY = true
	p.lastValidPositionY = positionY

	p.consecutiveMisses = 0

	// Calculate camera-relative offsets
	// Face at 50% = centered, offset = 0
	// Face at 75% = 25% right of center, need to turn right (negative yaw)
	// Face at 25% = 25% left of center, need to turn left (positive yaw)
	frameOffsetX := (50 - positionX) / 100.0 // -0.5 to +0.5
	yawOffset = frameOffsetX * p.CameraFOV   // Scale by FOV

	// Pitch: face above center = need to look up (negative pitch for Reachy)
	frameOffsetY := (50 - positionY) / 100.0    // -0.5 to +0.5
	pitchOffset = -frameOffsetY * p.VerticalFOV // Negative because up is negative pitch

	return yawOffset, pitchOffset, faceWidth, true
}

// GetFramePosition returns the last detected frame position (0-100%)
func (p *Perception) GetFramePosition() (x, y float64) {
	return p.lastValidPosition, p.lastValidPositionY
}
