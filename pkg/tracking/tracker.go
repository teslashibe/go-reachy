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

	// Enable/disable state
	isEnabled  bool      // Whether tracking is active (default: true)
	disabledAt time.Time // When tracking was disabled (for delayed return to neutral)

	// Scanning state
	isScanning     bool
	scanDirection  float64 // 1 = right, -1 = left
	scanStartTime  time.Time
	lastFaceSeenAt time.Time

	// Interpolation for smooth return to neutral
	interpStartedAt time.Time
	isInterpolating bool

	// Error tracking (avoid log spam)
	lastRobotError time.Time
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
		config:        config,
		robot:         robotCtrl,
		video:         video,
		detector:      detector,
		world:         worldmodel.New(),
		controller:    NewPDController(config),
		perception:    NewPerception(config, detector),
		lastLoggedYaw: 999.0,
		isEnabled:     true, // Tracking enabled by default
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
		// Re-enabling: clear disabled state, stop any return-to-neutral
		t.disabledAt = time.Time{}
		debug.Logln("👁️  Head tracking enabled")
	} else {
		// Disabling: record time for delayed return to neutral
		t.disabledAt = time.Now()
		debug.Logln("👁️  Head tracking disabled")
	}
}

// IsEnabled returns whether head tracking is currently enabled.
func (t *Tracker) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isEnabled
}

// SetAudioClient enables audio DOA integration with go-eva
func (t *Tracker) SetAudioClient(client *audio.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.audioClient = client
}

// handleAudioDOA is called when a DOA reading is received via WebSocket
func (t *Tracker) handleAudioDOA(doa *audio.DOAResult) {
	// Update world model with audio source
	t.world.UpdateAudioSource(doa.Angle, doa.Confidence, doa.Speaking)

	// Log when speaking detected
	if doa.Speaking {
		debug.Log("🎤 DOA (ws): %.2f rad, confidence=%.2f (speaking)\n", doa.Angle, doa.Confidence)
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
	t.mu.RUnlock()

	if client == nil {
		return
	}

	doa, err := client.GetDOA()
	if err != nil {
		// Don't spam logs for connection errors
		return
	}

	// Update world model with audio source
	t.world.UpdateAudioSource(doa.Angle, doa.Confidence, doa.Speaking)

	// Log when speaking detected
	if doa.Speaking {
		debug.Log("🎤 DOA: %.2f rad, confidence=%.2f (speaking)\n", doa.Angle, doa.Confidence)
	}
}

// updateMovement uses the PD controller to smoothly move toward target
func (t *Tracker) updateMovement() {
	// Check if tracking is disabled
	t.mu.RLock()
	enabled := t.isEnabled
	disabledAt := t.disabledAt
	t.mu.RUnlock()

	if !enabled {
		// Tracking disabled - return to neutral
		t.updateDisabled(disabledAt)
		return
	}

	// Get target from world model (priority: Face > Audio > None)
	targetAngle, source, hasTarget := t.world.GetTarget()

	if !hasTarget {
		// No target - check if we should scan or interpolate to neutral
		t.updateNoTarget()
		return
	}

	// We have a target - stop scanning and interpolation
	if t.isScanning {
		t.isScanning = false
		if source == "face" {
			debug.Logln("👁️  Found face, stopping scan")
		} else if source == "audio" {
			debug.Logln("🎤 Heard voice, turning toward sound")
		}
	}
	t.isInterpolating = false

	// Only update lastFaceSeenAt for visual targets
	if source == "face" {
		t.lastFaceSeenAt = time.Now()
	}

	// Update controller target
	t.controller.SetTarget(targetAngle)

	// Get next yaw from PD controller
	newYaw, yawShouldMove := t.controller.Update()

	// Get next pitch from PD controller
	newPitch, pitchShouldMove := t.controller.UpdatePitch()

	if !yawShouldMove && !pitchShouldMove {
		return
	}

	// Output the result (combined yaw and pitch)
	t.outputPose(newYaw, newPitch, targetAngle)

	// Check if body rotation is needed (head at mechanical limits)
	t.checkBodyRotation()
}

// outputPose sends yaw and pitch to either offset handler or direct robot control
func (t *Tracker) outputPose(yaw, pitch, targetAngle float64) {
	t.mu.RLock()
	handler := t.onOffset
	t.mu.RUnlock()

	if handler != nil {
		// Offset mode: output for fusion with unified controller
		handler(robot.Offset{Roll: 0, Pitch: pitch, Yaw: yaw})
	} else if t.robot != nil {
		// Direct mode: control robot directly (roll=0, pitch, yaw)
		err := t.robot.SetHeadPose(0, pitch, yaw)
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
					yaw, pitch, targetAngle, t.controller.GetError())
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

// updateNoTarget handles the case when no face is detected
func (t *Tracker) updateNoTarget() {
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
	// Stop scanning when disabled
	if t.isScanning {
		t.isScanning = false
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
	// Skip detection if tracking is disabled
	t.mu.RLock()
	enabled := t.isEnabled
	t.mu.RUnlock()
	if !enabled {
		return
	}

	currentYaw := t.controller.GetCurrentYaw()
	currentPitch := t.controller.GetCurrentPitch()
	bodyYaw := t.world.GetBodyYaw()

	// Detect face in current frame using local detector (room coordinates + pitch)
	frameX, frameY, roomYaw, targetPitch, found := t.perception.DetectFaceRoomWithPitch(
		t.video, currentYaw, currentPitch, bodyYaw,
	)

	if !found {
		// Log occasional misses
		misses := t.perception.GetConsecutiveMisses()
		if misses == 5 {
			debug.Logln("👁️  Lost face (5 consecutive misses)")
		}
		return
	}

	// Update world model with detection (room coordinates)
	// Using "primary" as the entity ID for single-person tracking
	t.world.UpdateEntity("primary", roomYaw, frameX)

	// Update pitch target
	t.controller.SetTargetPitch(targetPitch)

	// Log detection
	debug.Log("👁️  Face at (%.0f%%, %.0f%%) → room %.2f rad, pitch %.2f rad\n",
		frameX, frameY, roomYaw, targetPitch)

	// Update dashboard
	if t.state != nil {
		t.state.UpdateFacePosition(frameX, roomYaw)
		t.state.AddLog("face", fmt.Sprintf("Face at %.0f%% → room %.2f", frameX, roomYaw))
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

	// Reverse direction at scan limits
	if newYaw > t.config.ScanRange {
		newYaw = t.config.ScanRange
		t.scanDirection = -1.0
		debug.Logln("👀 Scan: reversing to left")
	} else if newYaw < -t.config.ScanRange {
		newYaw = -t.config.ScanRange
		t.scanDirection = 1.0
		debug.Logln("👀 Scan: reversing to right")
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

// --- Legacy compatibility ---

// These methods maintain compatibility with the old API

// GetTargetYaw returns the target yaw (for compatibility)
func (t *Tracker) GetTargetYaw() float64 {
	angle, _ := t.world.GetTargetWorldAngle()
	return angle
}
