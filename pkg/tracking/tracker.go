package tracking

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/robot"
	"github.com/teslashibe/go-reachy/pkg/tracking/detection"
	"github.com/teslashibe/go-reachy/pkg/worldmodel"
)

// VideoSource interface for capturing frames
type VideoSource interface {
	CaptureJPEG() ([]byte, error)
}

// StateUpdater interface for updating dashboard state
type StateUpdater interface {
	UpdateFacePosition(position, yaw float64)
	AddLog(logType, message string)
}

// OffsetHandler is called when tracker computes a new head offset.
// If set, the tracker operates in "offset mode" and outputs offsets
// instead of directly controlling the robot.
// Uses robot.Offset for consistency with the robot package.
type OffsetHandler func(offset robot.Offset)

// BodyRotationHandler is called when the head reaches its mechanical limits
// and the body should rotate to bring the target back into range.
// direction: positive = rotate body left, negative = rotate body right (radians)
type BodyRotationHandler func(direction float64)

// Tracker handles head tracking with world-coordinate awareness
type Tracker struct {
	config Config
	robot  robot.HeadController
	video  VideoSource
	state  StateUpdater

	// Core components
	detector   detection.Detector
	world      *worldmodel.WorldModel
	controller *PDController
	perception *Perception

	// Audio DOA client (optional, from go-eva)
	audioClient *audio.Client

	// Offset mode: if set, output offsets instead of direct control
	onOffset OffsetHandler

	// Body rotation callback: if set, called when head reaches limits
	onBodyRotation BodyRotationHandler

	// State
	mu            sync.RWMutex
	lastLoggedYaw float64
	isRunning     bool

	// Enable/disable state (master toggle)
	isEnabled  bool      // Whether tracking is active (default: true)
	disabledAt time.Time // When tracking was disabled (for delayed return to neutral)

	// Granular enable/disable (independent of master toggle)
	isFaceEnabled  bool // Whether face tracking is active (default: true)
	isAudioEnabled bool // Whether audio DOA tracking is active (default: true)

	// Scanning state
	isScanning     bool
	scanDirection  float64 // 1 = right, -1 = left
	scanStartTime  time.Time
	lastFaceSeenAt time.Time
	scanCyclesDone int // Number of complete scan cycles

	// Breathing state (idle animation)
	isBreathing    bool
	breathingPhase float64 // Current phase in radians (0 to 2π)

	// Speech wobble offsets (additive to tracking output)
	speechOffsets robot.Offset

	// Interpolation for smooth return to neutral
	interpStartedAt time.Time
	isInterpolating bool

	// Error tracking (avoid log spam)
	lastRobotError time.Time

	// Camera-relative offsets (from latest detection)
	lastYawOffset   float64 // How much to turn horizontally
	lastPitchOffset float64 // How much to tilt vertically
	hasFaceTarget   bool    // Whether we have a recent face detection
}

// New creates a new head tracker with local face detection.
// The robot parameter only needs to implement HeadController (not the full Controller).
func New(config Config, robotCtrl robot.HeadController, video VideoSource, modelPath string) (*Tracker, error) {
	// Initialize detector
	detConfig := detection.Config{
		ModelPath:        modelPath,
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	detector, err := detection.NewYuNet(detConfig)
	if err != nil {
		return nil, fmt.Errorf("init detector: %w", err)
	}

	return &Tracker{
		config:         config,
		robot:          robotCtrl,
		video:          video,
		detector:       detector,
		world:          worldmodel.New(),
		controller:     NewPDController(config),
		perception:     NewPerception(config, detector),
		lastLoggedYaw:  999.0,
		isEnabled:      true, // Tracking enabled by default
		isFaceEnabled:  true, // Face tracking enabled by default
		isAudioEnabled: true, // Audio tracking enabled by default
	}, nil
}

// Close releases resources
func (t *Tracker) Close() error {
	if t.detector != nil {
		return t.detector.Close()
	}
	return nil
}

// SetStateUpdater sets the dashboard state updater
func (t *Tracker) SetStateUpdater(state StateUpdater) {
	t.state = state
}

// SetOffsetHandler enables offset mode.
// When set, the tracker outputs head offsets via this callback
// instead of directly controlling the robot.
// This allows integration with a unified motion controller.
func (t *Tracker) SetOffsetHandler(handler OffsetHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onOffset = handler
}

// SetBodyRotationHandler sets the callback for automatic body rotation.
// When the head reaches its mechanical limits while tracking a target,
// this callback is invoked with the rotation direction (radians).
// The caller should rotate the body and call SetBodyYaw to update the world model.
func (t *Tracker) SetBodyRotationHandler(handler BodyRotationHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBodyRotation = handler
}

// SetBodyYaw updates the world model with current body orientation.
// Call this when the body rotates so tracking remains accurate.
func (t *Tracker) SetBodyYaw(yaw float64) {
	t.world.SetBodyYaw(yaw)
}

// GetBodyYaw returns the current body orientation from the world model.
func (t *Tracker) GetBodyYaw() float64 {
	return t.world.GetBodyYaw()
}

// SetEnabled enables or disables head tracking.
// When disabled, the tracker stops detecting faces and smoothly returns to neutral.
// When re-enabled, tracking resumes immediately.
func (t *Tracker) SetEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isEnabled == enabled {
		return // No change
	}

	t.isEnabled = enabled

	if enabled {
		// Re-enabling: clear disabled state, restore granular toggles
		t.disabledAt = time.Time{}
		t.isFaceEnabled = true
		t.isAudioEnabled = true
		debug.Logln("👁️  Head tracking enabled (face + audio)")
	} else {
		// Disabling: record time for delayed return to neutral
		t.disabledAt = time.Now()
		debug.Logln("👁️  Head tracking disabled (all)")
	}
}

// IsEnabled returns whether head tracking is currently enabled.
func (t *Tracker) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isEnabled
}

// SetFaceTrackingEnabled enables or disables face tracking independently.
// When disabled, face detection is skipped but audio tracking continues.
// This is independent of the master SetEnabled toggle.
func (t *Tracker) SetFaceTrackingEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isFaceEnabled == enabled {
		return
	}

	t.isFaceEnabled = enabled
	if enabled {
		debug.Logln("👁️  Face tracking enabled")
	} else {
		debug.Logln("👁️  Face tracking disabled")
	}
}

// IsFaceTrackingEnabled returns whether face tracking is enabled.
func (t *Tracker) IsFaceTrackingEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isFaceEnabled
}

// SetAudioTrackingEnabled enables or disables audio DOA tracking independently.
// When disabled, audio direction is ignored but face tracking continues.
// This is independent of the master SetEnabled toggle.
func (t *Tracker) SetAudioTrackingEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isAudioEnabled == enabled {
		return
	}

	t.isAudioEnabled = enabled
	if enabled {
		debug.Logln("🎤 Audio tracking enabled")
	} else {
		debug.Logln("🎤 Audio tracking disabled")
	}
}

// IsAudioTrackingEnabled returns whether audio DOA tracking is enabled.
func (t *Tracker) IsAudioTrackingEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isAudioEnabled
}

// SetAudioClient enables audio DOA integration with go-eva
func (t *Tracker) SetAudioClient(client *audio.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.audioClient = client
}

// handleAudioDOA is called when a DOA reading is received via WebSocket
func (t *Tracker) handleAudioDOA(doa *audio.DOAResult) {
	// Skip if audio tracking is disabled
	t.mu.RLock()
	audioEnabled := t.isAudioEnabled
	t.mu.RUnlock()
	if !audioEnabled {
		return
	}

	// Update world model with audio source
	t.world.UpdateAudioSource(doa.Angle, doa.Confidence, doa.Speaking)

	// Try to associate audio with a visible face
	if doa.Speaking {
		if entityID := t.world.AssociateAudio(doa.Angle, doa.Speaking, doa.Confidence); entityID != "" {
			debug.Log("🎤 DOA (ws): %.2f rad → matched face %s\n", doa.Angle, entityID)
		} else {
			debug.Log("🎤 DOA (ws): %.2f rad, confidence=%.2f (no face match)\n", doa.Angle, doa.Confidence)
		}
	}
}

// GetCurrentYaw returns the current head yaw
func (t *Tracker) GetCurrentYaw() float64 {
	return t.controller.GetCurrentYaw()
}

// GetCurrentPitch returns the current head pitch
func (t *Tracker) GetCurrentPitch() float64 {
	return t.controller.GetCurrentPitch()
}

// SetSpeechOffsets sets additive offsets from speech animation.
// These are added to the tracking output for natural speaking gestures.
// Thread-safe: can be called from TTS audio processing goroutines.
func (t *Tracker) SetSpeechOffsets(roll, pitch, yaw float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.speechOffsets = robot.Offset{Roll: roll, Pitch: pitch, Yaw: yaw}
}

// ClearSpeechOffsets resets speech offsets to zero.
// Call this when speech ends.
func (t *Tracker) ClearSpeechOffsets() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.speechOffsets = robot.Offset{}
}

// GetSpeechOffsets returns the current speech offsets.
func (t *Tracker) GetSpeechOffsets() robot.Offset {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.speechOffsets
}

// GetWorld returns the world model for inspection
func (t *Tracker) GetWorld() *worldmodel.WorldModel {
	return t.world
}

// Run starts the head tracking loops
func (t *Tracker) Run(ctx context.Context) {
	moveTicker := time.NewTicker(t.config.MovementInterval)
	detectTicker := time.NewTicker(t.config.DetectionInterval)
	decayTicker := time.NewTicker(t.config.DecayInterval)
	defer moveTicker.Stop()
	defer detectTicker.Stop()
	defer decayTicker.Stop()

	t.isRunning = true

	fmt.Println("👁️  Head tracker started (local YuNet face detection)")
	debug.Log("    Detection: %v, Movement: %v, Range: ±%.1f rad\n",
		t.config.DetectionInterval, t.config.MovementInterval, t.config.YawRange)
	debug.Log("    PD Control: Kp=%.2f, Kd=%.2f, DeadZone=%.2f rad\n",
		t.config.Kp, t.config.Kd, t.config.ControlDeadZone)

	// Start audio DOA streaming (WebSocket) or fall back to polling
	t.mu.RLock()
	audioClient := t.audioClient
	t.mu.RUnlock()

	var audioTicker *time.Ticker
	usePolling := false

	if audioClient != nil {
		// Try WebSocket streaming first
		err := audioClient.StreamDOA(ctx, t.handleAudioDOA)
		if err != nil {
			fmt.Printf("🎤 WebSocket DOA failed (%v), falling back to polling\n", err)
			usePolling = true
			audioTicker = time.NewTicker(100 * time.Millisecond) // 10Hz polling fallback
		} else {
			fmt.Println("🎤 Audio DOA streaming (WebSocket, 10Hz push)")
		}
	}

	if audioTicker != nil {
		defer audioTicker.Stop()
	}

	lastDecay := time.Now()

	// Build channel for audio polling (only if WebSocket failed)
	var audioChan <-chan time.Time
	if usePolling && audioTicker != nil {
		audioChan = audioTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			t.isRunning = false
			// Close WebSocket connection
			if audioClient != nil {
				audioClient.Close()
			}
			return

		case <-moveTicker.C:
			t.updateMovement()

		case <-detectTicker.C:
			if t.video != nil {
				go t.detectAndUpdate()
			}

		case <-decayTicker.C:
			dt := time.Since(lastDecay).Seconds()
			t.world.DecayConfidence(dt)
			lastDecay = time.Now()

		case <-audioChan:
			// Polling fallback (only used if WebSocket failed)
			go t.pollAudioDOA()
		}
	}
}

// pollAudioDOA fetches DOA from go-eva and updates the world model
func (t *Tracker) pollAudioDOA() {
	t.mu.RLock()
	client := t.audioClient
	audioEnabled := t.isAudioEnabled
	t.mu.RUnlock()

	// Skip if audio tracking is disabled
	if !audioEnabled || client == nil {
		return
	}

	doa, err := client.GetDOA()
	if err != nil {
		// Don't spam logs for connection errors
		return
	}

	// Update world model with audio source
	t.world.UpdateAudioSource(doa.Angle, doa.Confidence, doa.Speaking)

	// Try to associate audio with a visible face
	if doa.Speaking {
		if entityID := t.world.AssociateAudio(doa.Angle, doa.Speaking, doa.Confidence); entityID != "" {
			debug.Log("🎤 DOA: %.2f rad → matched face %s\n", doa.Angle, entityID)
		} else {
			debug.Log("🎤 DOA: %.2f rad, confidence=%.2f (no face match)\n", doa.Angle, doa.Confidence)
		}
	}
}

// updateMovement uses the PD controller to smoothly move toward target
func (t *Tracker) updateMovement() {
	// Check if tracking is disabled
	t.mu.RLock()
	enabled := t.isEnabled
	disabledAt := t.disabledAt
	hasFace := t.hasFaceTarget
	yawOffset := t.lastYawOffset
	pitchOffset := t.lastPitchOffset
	t.mu.RUnlock()

	if !enabled {
		// Tracking disabled - return to neutral
		t.updateDisabled(disabledAt)
		return
	}

	// Check for audio-only target (when no face but audio DOA active)
	audioAngle, hasAudio := t.getAudioTarget()

	if !hasFace && !hasAudio {
		// No target - check if we should scan or interpolate to neutral
		t.updateNoTarget()
		return
	}

	// Determine source and target
	var source string
	if hasFace {
		source = "face"
	} else {
		source = "audio"
	}

	// We have a target - stop scanning, breathing, and interpolation
	if t.isScanning || t.isBreathing {
		if t.isScanning {
			t.isScanning = false
		}
		if t.isBreathing {
			t.isBreathing = false
			t.breathingPhase = 0
		}
		if source == "face" {
			debug.Logln("👁️  Found face, stopping idle behavior")
		} else if source == "audio" {
			debug.Logln("🎤 Heard voice, turning toward sound")
		}
	}
	t.isInterpolating = false

	// Only update lastFaceSeenAt for visual targets
	if source == "face" {
		t.lastFaceSeenAt = time.Now()
	}

	// Apply response scaling to offsets
	scale := t.config.ResponseScale
	if scale <= 0 {
		scale = 1.0
	}

	// Set controller targets using offset-based approach
	if hasFace {
		// Face tracking: use camera-relative offsets (self-correcting)
		t.controller.SetTargetFromOffset(yawOffset * scale)
		t.controller.SetTargetPitchFromOffset(pitchOffset * scale)
	} else if hasAudio {
		// Audio tracking: turn toward audio direction
		// Audio angle is already relative to Eva's current orientation
		currentYaw := t.controller.GetCurrentYaw()
		t.controller.SetTarget(currentYaw + audioAngle*scale)
	}

	// Get next yaw from PD controller
	newYaw, yawShouldMove := t.controller.Update()

	// Get next pitch from PD controller
	newPitch, pitchShouldMove := t.controller.UpdatePitch()

	// Debug: log when we're not moving and why
	if !yawShouldMove && !pitchShouldMove {
		// Only log occasionally to avoid spam
		if hasFace {
			debug.Log("⏸️  Not moving: yaw error=%.3f (deadzone=%.3f), at limit=%v\n",
				t.controller.GetError(), t.config.ControlDeadZone,
				math.Abs(t.controller.GetCurrentYaw()) > t.config.YawRange*0.95)
		}
		return
	}

	// Output the result (combined yaw and pitch)
	t.outputPose(newYaw, newPitch, t.controller.GetTargetYaw())

	// Check if body rotation is needed using offset-based trigger
	t.checkBodyRotationOffset(yawOffset)
}

// outputPose sends yaw and pitch to either offset handler or direct robot control
func (t *Tracker) outputPose(yaw, pitch, targetAngle float64) {
	t.mu.RLock()
	handler := t.onOffset
	speech := t.speechOffsets
	t.mu.RUnlock()

	// Note: Response scaling is now applied to offsets in updateMovement,
	// so yaw/pitch coming in are already scaled appropriately.

	// Add speech wobble offsets for natural speaking gestures
	finalYaw := yaw + speech.Yaw
	finalPitch := pitch + speech.Pitch
	finalRoll := speech.Roll // Roll only from speech (tracking doesn't use roll)

	if handler != nil {
		// Offset mode: output for fusion with unified controller
		handler(robot.Offset{Roll: finalRoll, Pitch: finalPitch, Yaw: finalYaw})
	} else if t.robot != nil {
		// Direct mode: control robot directly
		err := t.robot.SetHeadPose(finalRoll, finalPitch, finalYaw)
		if err != nil {
			// Log errors but don't spam - only log every 5 seconds
			if t.lastRobotError.IsZero() || time.Since(t.lastRobotError) > 5*time.Second {
				fmt.Printf("⚠️  SetHeadPose error: %v\n", err)
				t.lastRobotError = time.Now()
			}
		} else {
			t.lastRobotError = time.Time{} // Clear error state on success
			// Log significant movements
			if math.Abs(yaw-t.lastLoggedYaw) > t.config.LogThreshold {
				debug.Log("🔄 Head: yaw=%.2f pitch=%.2f (target=%.2f, error=%.2f)\n",
					finalYaw, finalPitch, targetAngle, t.controller.GetError())
				t.lastLoggedYaw = yaw
			}
		}
	}
}

// checkBodyRotation checks if head is at limits and triggers body rotation
func (t *Tracker) checkBodyRotation() {
	t.mu.RLock()
	handler := t.onBodyRotation
	t.mu.RUnlock()

	if handler == nil {
		return
	}

	needsRotation, direction := t.controller.NeedsBodyRotation(
		t.config.BodyRotationThreshold,
		t.config.BodyRotationStep,
	)

	if needsRotation {
		debug.Log("🔄 Body rotation triggered: direction=%.2f rad\n", direction)
		handler(direction)
	}
}

// checkBodyRotationOffset checks if body rotation is needed using offset-based trigger.
// This triggers when: head is near limit AND offset still points in same direction.
func (t *Tracker) checkBodyRotationOffset(yawOffset float64) {
	t.mu.RLock()
	handler := t.onBodyRotation
	t.mu.RUnlock()

	if handler == nil {
		return
	}

	currentYaw := t.controller.GetCurrentYaw()
	limit := t.config.YawRange * t.config.BodyRotationThreshold
	step := t.config.BodyRotationStep

	// Trigger if head near limit AND face still off-center in that direction
	// Positive yaw = looking left, positive offset = face on left (need to turn more left)
	if currentYaw > limit*0.9 && yawOffset > 0.05 {
		debug.Log("🔄 Body rotation triggered (offset): head at %.2f, offset %.2f → rotate left\n", currentYaw, yawOffset)
		handler(step)
	} else if currentYaw < -limit*0.9 && yawOffset < -0.05 {
		debug.Log("🔄 Body rotation triggered (offset): head at %.2f, offset %.2f → rotate right\n", currentYaw, yawOffset)
		handler(-step)
	}
}

// getAudioTarget returns audio-based target offset if available
func (t *Tracker) getAudioTarget() (float64, bool) {
	t.mu.RLock()
	audioEnabled := t.isAudioEnabled
	t.mu.RUnlock()

	if !audioEnabled {
		return 0, false
	}

	// Check if there's an active audio source
	audio := t.world.GetAudioSource()
	if audio == nil || !audio.Speaking || audio.Confidence < 0.3 {
		return 0, false
	}

	// Audio angle is relative to Eva's current orientation
	// Return as offset from current position
	return audio.Angle, true
}

// updateNoTarget handles the case when no face is detected
func (t *Tracker) updateNoTarget() {
	// If breathing, continue breathing animation
	if t.isBreathing {
		t.updateBreathing()
		return
	}

	// Check if we should start interpolating to neutral
	if !t.isInterpolating && !t.isScanning {
		if t.lastFaceSeenAt.IsZero() {
			t.lastFaceSeenAt = time.Now()
		}

		timeSinceFace := time.Since(t.lastFaceSeenAt)
		if timeSinceFace >= t.config.ScanStartDelay {
			// Start smooth interpolation to neutral
			t.isInterpolating = true
			t.interpStartedAt = time.Now()
			t.controller.InterpolateToNeutral(1 * time.Second)
			debug.Logln("👁️  Face lost, returning to neutral")
		}
		return
	}

	// If interpolating, continue the interpolation
	if t.isInterpolating {
		newYaw, shouldMove := t.controller.Update()
		if shouldMove {
			// During interpolation to neutral, also return pitch to 0
			t.outputPose(newYaw, 0, 0)
		}

		// Check if interpolation is complete, then start scanning
		if !t.controller.IsInterpolating() {
			t.isInterpolating = false
			t.isScanning = true
			t.scanStartTime = time.Now()
			t.scanDirection = 1.0
			t.scanCyclesDone = 0 // Reset cycle counter
			debug.Logln("👀 Starting scan for faces...")
			if t.state != nil {
				t.state.AddLog("scan", "Scanning for faces")
			}
		}
		return
	}

	// Continue scanning
	t.updateScanning()
}

// updateDisabled handles movement when tracking is disabled.
// Smoothly returns to neutral position.
func (t *Tracker) updateDisabled(disabledAt time.Time) {
	// Stop scanning and breathing when disabled
	if t.isScanning {
		t.isScanning = false
	}
	if t.isBreathing {
		t.isBreathing = false
		t.breathingPhase = 0
	}

	// Start interpolation to neutral if not already interpolating
	if !t.isInterpolating {
		t.isInterpolating = true
		t.interpStartedAt = time.Now()
		t.controller.InterpolateToNeutral(1 * time.Second)
		debug.Logln("👁️  Tracking disabled, returning to neutral")
	}

	// Continue interpolation
	newYaw, shouldMove := t.controller.Update()
	if shouldMove {
		t.outputPose(newYaw, 0, 0)
	}

	// When interpolation completes, just hold position
	if !t.controller.IsInterpolating() {
		t.isInterpolating = false
		// Hold at neutral - no further movement needed
	}
}

// detectAndUpdate detects faces and updates the world model
func (t *Tracker) detectAndUpdate() {
	// Skip detection if tracking or face tracking is disabled
	t.mu.RLock()
	enabled := t.isEnabled
	faceEnabled := t.isFaceEnabled
	t.mu.RUnlock()
	if !enabled || !faceEnabled {
		return
	}

	// Detect face using camera-relative offset approach
	// No dependency on knowing head or body position - self-correcting
	yawOffset, pitchOffset, faceWidth, found := t.perception.DetectFaceOffset(t.video)

	if !found {
		// Clear face target when no face detected
		t.mu.Lock()
		t.hasFaceTarget = false
		t.mu.Unlock()

		// Log occasional misses
		misses := t.perception.GetConsecutiveMisses()
		if misses == 5 {
			debug.Logln("👁️  Lost face (5 consecutive misses)")
		}
		return
	}

	// Get frame position for logging and world model
	frameX, frameY := t.perception.GetFramePosition()

	// Update world model with frame-based detection
	// Store frame position, not room angle - world model handles confidence
	t.world.UpdateEntityWithDepth("primary", frameX, frameX, faceWidth)

	// Store the current offsets for use by updateMovement
	t.mu.Lock()
	t.lastYawOffset = yawOffset
	t.lastPitchOffset = pitchOffset
	t.hasFaceTarget = true
	t.mu.Unlock()

	// Log detection with distance
	entity := t.world.GetFocusTarget()
	dist := float64(0)
	if entity != nil {
		dist = entity.Distance
	}
	if dist > 0 {
		debug.Log("👁️  Face at (%.0f%%, %.0f%%) → yaw offset %.2f rad, pitch offset %.2f rad, dist %.1fm\n",
			frameX, frameY, yawOffset, pitchOffset, dist)
	} else {
		debug.Log("👁️  Face at (%.0f%%, %.0f%%) → yaw offset %.2f rad, pitch offset %.2f rad\n",
			frameX, frameY, yawOffset, pitchOffset)
	}

	// Update dashboard
	if t.state != nil {
		currentYaw := t.controller.GetCurrentYaw()
		t.state.UpdateFacePosition(frameX, currentYaw)
		t.state.AddLog("face", fmt.Sprintf("Face at %.0f%% → offset %.2f", frameX, yawOffset))
	}
}

// updateScanning implements scan behavior when no face is detected
func (t *Tracker) updateScanning() {
	// Calculate scan position
	currentYaw := t.controller.GetCurrentYaw()

	// Move in scan direction
	dt := t.config.MovementInterval.Seconds()
	scanStep := t.config.ScanSpeed * dt * t.scanDirection
	newYaw := currentYaw + scanStep

	// Reverse direction at scan limits and count cycles
	if newYaw > t.config.ScanRange {
		newYaw = t.config.ScanRange
		t.scanDirection = -1.0
		debug.Logln("👀 Scan: reversing to left")
	} else if newYaw < -t.config.ScanRange {
		newYaw = -t.config.ScanRange
		t.scanDirection = 1.0
		t.scanCyclesDone++ // Complete cycle when returning to right
		debug.Logln("👀 Scan: reversing to right")

		// After one full scan cycle, switch to breathing if enabled
		if t.scanCyclesDone >= 1 && t.config.BreathingEnabled {
			t.isScanning = false
			t.isBreathing = true
			t.breathingPhase = 0
			debug.Logln("😮‍💨 Scan complete, starting breathing animation")
			if t.state != nil {
				t.state.AddLog("breathing", "Breathing animation started")
			}
			return
		}
	}

	// Update controller state
	t.controller.SetCurrentYaw(newYaw)

	// Output the scan position (pitch neutral during scanning)
	t.outputPose(newYaw, 0, 0)

	// Log occasionally
	if math.Abs(newYaw-t.lastLoggedYaw) > 0.2 {
		debug.Log("👀 Scanning: yaw=%.2f\n", newYaw)
		t.lastLoggedYaw = newYaw
	}
}

// updateBreathing implements gentle breathing animation when idle
func (t *Tracker) updateBreathing() {
	// Advance phase based on frequency
	dt := t.config.MovementInterval.Seconds()
	t.breathingPhase += dt * t.config.BreathingFrequency * 2 * math.Pi

	// Keep phase in 0 to 2π range
	if t.breathingPhase > 2*math.Pi {
		t.breathingPhase -= 2 * math.Pi
	}

	// Calculate breathing offsets using sinusoidal motion
	// Pitch: gentle up/down nodding
	pitch := t.config.BreathingAmplitude * math.Sin(t.breathingPhase)

	// Roll: subtle side-to-side at slightly different frequency for natural feel
	roll := t.config.BreathingRollAmp * math.Sin(t.breathingPhase*0.7)

	// Yaw stays at 0 (centered) during breathing
	t.outputPose(0, pitch, 0)

	// Note: roll is computed but not used yet since outputPose only takes yaw/pitch
	// TODO: Add roll support to outputPose when robot supports it
	_ = roll
}

// --- Legacy compatibility ---

// These methods maintain compatibility with the old API

// GetTargetYaw returns the target yaw (for compatibility)
func (t *Tracker) GetTargetYaw() float64 {
	angle, _ := t.world.GetTargetWorldAngle()
	return angle
}
