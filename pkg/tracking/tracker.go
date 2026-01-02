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

	// State
	mu            sync.RWMutex
	lastLoggedYaw float64
	isRunning     bool

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

// SetBodyYaw updates the world model with current body orientation.
// Call this when the body rotates so tracking remains accurate.
func (t *Tracker) SetBodyYaw(yaw float64) {
	t.world.SetBodyYaw(yaw)
}

// GetBodyYaw returns the current body orientation from the world model.
func (t *Tracker) GetBodyYaw() float64 {
	return t.world.GetBodyYaw()
}

// SetAudioClient enables audio DOA integration with go-eva
func (t *Tracker) SetAudioClient(client *audio.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.audioClient = client
}

// GetCurrentYaw returns the current head yaw
func (t *Tracker) GetCurrentYaw() float64 {
	return t.controller.GetCurrentYaw()
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
	audioTicker := time.NewTicker(100 * time.Millisecond) // 10Hz audio polling
	defer moveTicker.Stop()
	defer detectTicker.Stop()
	defer decayTicker.Stop()
	defer audioTicker.Stop()

	t.isRunning = true

	fmt.Println("👁️  Head tracker started (local YuNet face detection)")
	debug.Log("    Detection: %v, Movement: %v, Range: ±%.1f rad\n",
		t.config.DetectionInterval, t.config.MovementInterval, t.config.YawRange)
	debug.Log("    PD Control: Kp=%.2f, Kd=%.2f, DeadZone=%.2f rad\n",
		t.config.Kp, t.config.Kd, t.config.ControlDeadZone)

	// Check for audio DOA
	t.mu.RLock()
	hasAudio := t.audioClient != nil
	t.mu.RUnlock()
	if hasAudio {
		fmt.Println("🎤 Audio DOA enabled (go-eva integration)")
	}

	lastDecay := time.Now()

	for {
		select {
		case <-ctx.Done():
			t.isRunning = false
			return

		case <-moveTicker.C:
			t.updateMovement()

		case <-detectTicker.C:
			if t.video != nil {
				go t.detectAndUpdate()
			}

		case <-audioTicker.C:
			go t.pollAudioDOA()

		case <-decayTicker.C:
			dt := time.Since(lastDecay).Seconds()
			t.world.DecayConfidence(dt)
			lastDecay = time.Now()
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
	newYaw, shouldMove := t.controller.Update()

	if !shouldMove {
		return
	}

	// Output the result
	t.outputYaw(newYaw, targetAngle)
}

// outputYaw sends the yaw to either offset handler or direct robot control
func (t *Tracker) outputYaw(yaw float64, targetAngle float64) {
	t.mu.RLock()
	handler := t.onOffset
	t.mu.RUnlock()

	if handler != nil {
		// Offset mode: output for fusion with unified controller
		handler(robot.Offset{Roll: 0, Pitch: 0, Yaw: yaw})
	} else if t.robot != nil {
		// Direct mode: control robot directly
		err := t.robot.SetHeadPose(0, 0, yaw)
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
				debug.Log("🔄 Head: yaw=%.2f (target=%.2f, error=%.2f)\n",
					yaw, targetAngle, t.controller.GetError())
				t.lastLoggedYaw = yaw
			}
		}
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
			t.outputYaw(newYaw, 0)
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

// detectAndUpdate detects faces and updates the world model
func (t *Tracker) detectAndUpdate() {
	currentYaw := t.controller.GetCurrentYaw()
	bodyYaw := t.world.GetBodyYaw()

	// Detect face in current frame using local detector (room coordinates)
	framePos, roomAngle, found := t.perception.DetectFaceRoom(t.video, currentYaw, bodyYaw)

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
	t.world.UpdateEntity("primary", roomAngle, framePos)

	// Log detection
	debug.Log("👁️  Face at %.0f%% → room %.2f rad (head=%.2f, body=%.2f)\n",
		framePos, roomAngle, currentYaw, bodyYaw)

	// Update dashboard
	if t.state != nil {
		t.state.UpdateFacePosition(framePos, roomAngle)
		t.state.AddLog("face", fmt.Sprintf("Face at %.0f%% → room %.2f", framePos, roomAngle))
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

	// Output the scan position
	t.outputYaw(newYaw, 0)

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
