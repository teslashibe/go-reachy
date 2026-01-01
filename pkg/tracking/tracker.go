package tracking

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/teslashibe/go-reachy/pkg/realtime"
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

// Tracker handles head tracking based on face detection
type Tracker struct {
	config Config
	robot  RobotController
	video  VideoSource
	state  StateUpdater
	apiKey string

	currentYaw   float64
	targetYaw    float64
	lastPosition float64
	mu           sync.RWMutex

	lastLoggedYaw float64
}

// New creates a new head tracker
func New(config Config, robot RobotController, video VideoSource, apiKey string) *Tracker {
	return &Tracker{
		config:        config,
		robot:         robot,
		video:         video,
		apiKey:        apiKey,
		lastLoggedYaw: 999.0,
	}
}

// SetStateUpdater sets the dashboard state updater
func (t *Tracker) SetStateUpdater(state StateUpdater) {
	t.state = state
}

// GetCurrentYaw returns the current head yaw
func (t *Tracker) GetCurrentYaw() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentYaw
}

// GetTargetYaw returns the target head yaw
func (t *Tracker) GetTargetYaw() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.targetYaw
}

// Run starts the head tracking loops
func (t *Tracker) Run(ctx context.Context) {
	moveTicker := time.NewTicker(t.config.MovementInterval)
	detectTicker := time.NewTicker(t.config.DetectionInterval)
	defer moveTicker.Stop()
	defer detectTicker.Stop()

	fmt.Printf("👁️  Head tracker started (detect: %v, move: %v, range: ±%.1f rad)\n",
		t.config.DetectionInterval, t.config.MovementInterval, t.config.YawRange)

	for {
		select {
		case <-ctx.Done():
			return
		case <-moveTicker.C:
			t.updateMovement()
		case <-detectTicker.C:
			if t.video != nil && t.apiKey != "" {
				go t.detectFace()
			}
		}
	}
}

// updateMovement smoothly moves head toward target using proportional control
func (t *Tracker) updateMovement() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.currentYaw == t.targetYaw {
		return
	}

	diff := t.targetYaw - t.currentYaw
	absDiff := math.Abs(diff)

	// Proportional control: move faster when far, slower when close
	var speed float64
	if absDiff > t.config.FastThreshold {
		speed = t.config.MaxSpeed
	} else if absDiff > t.config.MediumThreshold {
		speed = t.config.MediumSpeed
	} else {
		// Snap to target when very close
		t.currentYaw = t.targetYaw
		t.applyHeadPosition()
		return
	}

	// Move toward target
	if diff > 0 {
		t.currentYaw += speed
		if t.currentYaw > t.targetYaw {
			t.currentYaw = t.targetYaw
		}
	} else {
		t.currentYaw -= speed
		if t.currentYaw < t.targetYaw {
			t.currentYaw = t.targetYaw
		}
	}

	t.applyHeadPosition()
}

// applyHeadPosition sends the current yaw to the robot (must hold lock)
func (t *Tracker) applyHeadPosition() {
	if t.robot == nil {
		return
	}

	err := t.robot.SetHeadPose(0, 0, t.currentYaw)
	if err == nil {
		// Log significant movements
		if math.Abs(t.currentYaw-t.lastLoggedYaw) > t.config.LogThreshold {
			fmt.Printf("🔄 Head: yaw=%.2f\n", t.currentYaw)
			t.lastLoggedYaw = t.currentYaw
		}
	}
}

// detectFace captures a frame and detects face position
func (t *Tracker) detectFace() {
	if t.video == nil {
		return
	}

	frame, err := t.video.CaptureJPEG()
	if err != nil {
		return
	}

	// Ask Gemini for face position
	prompt := "Look at this image. Is there a person's face visible? If yes, estimate the horizontal position of their face as a number from 0 to 100, where 0 is the far left edge and 100 is the far right edge of the image. Reply with ONLY a number (like 25 or 75) or NONE if no face is visible."

	result, err := realtime.GeminiVision(t.apiKey, frame, prompt)
	if err != nil {
		return
	}

	result = strings.TrimSpace(strings.ToUpper(result))

	// Parse the position
	if strings.Contains(result, "NONE") || result == "" {
		return // No face, keep current position
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
			return
		}
	}

	// Clamp to 0-100
	if position < 0 {
		position = 0
	} else if position > 100 {
		position = 100
	}

	// Jitter filter: ignore small changes
	t.mu.RLock()
	lastPos := t.lastPosition
	t.mu.RUnlock()

	if math.Abs(position-lastPos) < t.config.JitterThreshold {
		return // Too small a change, ignore
	}

	// Convert position (0-100) to yaw
	// 0 (left edge) -> +YawRange (look left)
	// 50 (center) -> 0
	// 100 (right edge) -> -YawRange (look right)
	newYaw := (50 - position) / 50.0 * t.config.YawRange

	// Clamp to range
	if newYaw > t.config.YawRange {
		newYaw = t.config.YawRange
	} else if newYaw < -t.config.YawRange {
		newYaw = -t.config.YawRange
	}

	fmt.Printf("👁️  Face at %.0f%% → yaw %.2f\n", position, newYaw)

	t.mu.Lock()
	t.targetYaw = newYaw
	t.lastPosition = position
	t.mu.Unlock()

	// Update dashboard
	if t.state != nil {
		t.state.UpdateFacePosition(position, newYaw)
		t.state.AddLog("face", fmt.Sprintf("Face at %.0f%% → yaw %.2f", position, newYaw))
	}
}

