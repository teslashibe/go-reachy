package tracking

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/teslashibe/go-reachy/pkg/tracking/detection"
)

// RobotController interface for head movement
type RobotController interface {
	SetHeadPose(roll, pitch, yaw float64) error
}

// VideoSource interface for capturing frames
type VideoSource interface {
	CaptureJPEG() ([]byte, error)
}

// StateUpdater interface for updating dashboard state
type StateUpdater interface {
	UpdateFacePosition(position, yaw float64)
	AddLog(logType, message string)
}

// Offset represents head position adjustments (matches robot.Offset)
type Offset struct {
	Roll, Pitch, Yaw float64
}

// OffsetHandler is called when tracker computes a new head offset.
// If set, the tracker operates in "offset mode" and outputs offsets
// instead of directly controlling the robot.
type OffsetHandler func(offset Offset)

// Tracker handles head tracking with world-coordinate awareness
type Tracker struct {
	config Config
	robot  RobotController
	video  VideoSource
	state  StateUpdater

	// Core components
	detector   detection.Detector
	world      *WorldModel
	controller *PDController
	perception *Perception

	// Offset mode: if set, output offsets instead of direct control
	onOffset OffsetHandler

	// State
	mu            sync.RWMutex
	lastLoggedYaw float64
	isRunning     bool

	// Scanning state
	isScanning     bool
	scanDirection  float64   // 1 = right, -1 = left
	scanStartTime  time.Time
	lastFaceSeenAt time.Time

	// Interpolation for smooth return to neutral
	interpStartedAt time.Time
	isInterpolating bool
}

// New creates a new head tracker with local face detection
func New(config Config, robot RobotController, video VideoSource, modelPath string) (*Tracker, error) {
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
		robot:         robot,
		video:         video,
		detector:      detector,
		world:         NewWorldModel(),
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

// GetCurrentYaw returns the current head yaw
func (t *Tracker) GetCurrentYaw() float64 {
	return t.controller.GetCurrentYaw()
}

// GetWorld returns the world model for inspection
func (t *Tracker) GetWorld() *WorldModel {
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

	fmt.Printf("👁️  Head tracker started (local YuNet face detection)\n")
	fmt.Printf("    Detection: %v, Movement: %v, Range: ±%.1f rad\n",
		t.config.DetectionInterval, t.config.MovementInterval, t.config.YawRange)
	fmt.Printf("    PD Control: Kp=%.2f, Kd=%.2f, DeadZone=%.2f rad\n",
		t.config.Kp, t.config.Kd, t.config.ControlDeadZone)

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

		case <-decayTicker.C:
			dt := time.Since(lastDecay).Seconds()
			t.world.DecayConfidence(dt)
			lastDecay = time.Now()
		}
	}
}

// updateMovement uses the PD controller to smoothly move toward target
func (t *Tracker) updateMovement() {
	// Get target from world model (body-relative angle)
	targetAngle, hasTarget := t.world.GetTargetWorldAngle()

	if !hasTarget {
		// No target - check if we should scan or interpolate to neutral
		t.updateNoTarget()
		return
	}

	// We have a target - stop scanning and interpolation
	if t.isScanning {
		t.isScanning = false
		fmt.Printf("👁️  Found face, stopping scan\n")
	}
	t.isInterpolating = false
	t.lastFaceSeenAt = time.Now()

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
		handler(Offset{Roll: 0, Pitch: 0, Yaw: yaw})
	} else if t.robot != nil {
		// Direct mode: control robot directly
		err := t.robot.SetHeadPose(0, 0, yaw)
		if err == nil {
			// Log significant movements
			if math.Abs(yaw-t.lastLoggedYaw) > t.config.LogThreshold {
				fmt.Printf("🔄 Head: yaw=%.2f (target=%.2f, error=%.2f)\n",
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
			fmt.Printf("👁️  Face lost, returning to neutral\n")
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
			fmt.Printf("👀 Starting scan for faces...\n")
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
			fmt.Printf("👁️  Lost face (5 consecutive misses)\n")
		}
		return
	}

	// Update world model with detection (room coordinates)
	// Using "primary" as the entity ID for single-person tracking
	t.world.UpdateEntity("primary", roomAngle, framePos)

	// Log detection
	fmt.Printf("👁️  Face at %.0f%% → room %.2f rad (head=%.2f, body=%.2f)\n",
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
		fmt.Printf("👀 Scan: reversing to left\n")
	} else if newYaw < -t.config.ScanRange {
		newYaw = -t.config.ScanRange
		t.scanDirection = 1.0
		fmt.Printf("👀 Scan: reversing to right\n")
	}

	// Update controller state
	t.controller.SetCurrentYaw(newYaw)

	// Output the scan position
	t.outputYaw(newYaw, 0)

	// Log occasionally
	if math.Abs(newYaw-t.lastLoggedYaw) > 0.2 {
		fmt.Printf("👀 Scanning: yaw=%.2f\n", newYaw)
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
