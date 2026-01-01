package tracking

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
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

// Tracker handles head tracking with world-coordinate awareness
type Tracker struct {
	config     Config
	robot      RobotController
	video      VideoSource
	state      StateUpdater
	apiKey     string

	// Core components
	world      *WorldModel
	controller *PDController
	perception *Perception

	// State
	mu            sync.RWMutex
	lastLoggedYaw float64
	isRunning     bool
}

// New creates a new head tracker with world-coordinate awareness
func New(config Config, robot RobotController, video VideoSource, apiKey string) *Tracker {
	return &Tracker{
		config:        config,
		robot:         robot,
		video:         video,
		apiKey:        apiKey,
		world:         NewWorldModel(),
		controller:    NewPDController(config),
		perception:    NewPerception(config),
		lastLoggedYaw: 999.0,
	}
}

// SetStateUpdater sets the dashboard state updater
func (t *Tracker) SetStateUpdater(state StateUpdater) {
	t.state = state
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

	fmt.Printf("👁️  Head tracker started (world-aware mode)\n")
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
			if t.video != nil && t.apiKey != "" {
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
	// Get target from world model
	targetAngle, hasTarget := t.world.GetTargetWorldAngle()

	if !hasTarget {
		// No target - could implement scan behavior here
		// For now, just hold position
		return
	}

	// Update controller target
	t.controller.SetTarget(targetAngle)

	// Tell perception we're about to move (for motion blur detection)
	isMoving := !t.controller.IsSettled()
	t.perception.SetMoving(isMoving)

	// Get next yaw from PD controller
	newYaw, shouldMove := t.controller.Update()

	if !shouldMove {
		return
	}

	// Apply to robot
	if t.robot != nil {
		err := t.robot.SetHeadPose(0, 0, newYaw)
		if err == nil {
			// Log significant movements
			if math.Abs(newYaw-t.lastLoggedYaw) > t.config.LogThreshold {
				fmt.Printf("🔄 Head: yaw=%.2f (target=%.2f, error=%.2f)\n",
					newYaw, targetAngle, t.controller.GetError())
				t.lastLoggedYaw = newYaw
			}
		}
	}
}

// detectAndUpdate detects faces and updates the world model
func (t *Tracker) detectAndUpdate() {
	currentYaw := t.controller.GetCurrentYaw()

	// Detect face in current frame
	framePos, worldAngle, found := t.perception.DetectFace(t.video, t.apiKey, currentYaw)

	if !found {
		// Log occasional misses
		misses := t.perception.GetConsecutiveMisses()
		if misses == 5 {
			fmt.Printf("👁️  Lost face (5 consecutive misses)\n")
		}
		return
	}

	// Update world model with detection
	// Using "primary" as the entity ID for single-person tracking
	t.world.UpdateEntity("primary", worldAngle, framePos)

	// Log detection
	fmt.Printf("👁️  Face at %.0f%% → world %.2f rad (head at %.2f)\n",
		framePos, worldAngle, currentYaw)

	// Update dashboard
	if t.state != nil {
		t.state.UpdateFacePosition(framePos, worldAngle)
		t.state.AddLog("face", fmt.Sprintf("Face at %.0f%% → world %.2f", framePos, worldAngle))
	}
}

// --- Legacy compatibility ---

// These methods maintain compatibility with the old API

// GetTargetYaw returns the target yaw (for compatibility)
func (t *Tracker) GetTargetYaw() float64 {
	angle, _ := t.world.GetTargetWorldAngle()
	return angle
}
