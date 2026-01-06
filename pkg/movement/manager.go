package movement

import (
	"fmt"
	"sync"
	"time"

	"github.com/teslashibe/go-reachy/pkg/robot"
)

// MotionController is the interface for sending commands to the robot.
type MotionController interface {
	SetPose(head *robot.Offset, antennas *[2]float64, bodyYaw *float64) error
}

// BodyYawNotifier is called when body yaw changes (for world model sync).
type BodyYawNotifier interface {
	SetBodyYaw(yaw float64)
}

// Manager orchestrates all robot movement through a single control loop.
// It composes primary moves (emotions) with secondary offsets (face tracking).
//
// Architecture (matches Python reachy_mini_conversation_app):
// - Primary moves are queued and played in order
// - Secondary offsets are continuously applied on top
// - Only ONE command is sent to the robot per tick (30Hz)
type Manager struct {
	robot       MotionController
	bodyNotify  BodyYawNotifier

	mu sync.RWMutex

	// Primary move state
	currentMove    Move          // Currently playing primary move (nil = idle)
	moveStartTime  time.Time     // When current move started
	lastPrimaryPose Pose         // Last pose from primary move (cached for when idle)

	// Secondary offsets (continuously updated)
	secondaryOffset SecondaryOffset

	// Control loop
	rate        time.Duration
	stop        chan struct{}
	running     bool

	// Dead-zone filtering (matches Python reachy)
	lastSentPose Pose
	skippedTicks uint64

	// Rate limiting for large movements
	maxStepRad float64

	// Diagnostics
	tickCount  uint64
	errorCount uint64
}

// NewManager creates a new MovementManager.
// rate should be ~33ms for 30Hz control loop.
func NewManager(robotCtrl MotionController, rate time.Duration) *Manager {
	return &Manager{
		robot:       robotCtrl,
		rate:        rate,
		stop:        make(chan struct{}),
		maxStepRad:  0.05, // ~3 degrees per tick max
	}
}

// SetBodyYawNotifier sets the callback for body yaw changes.
func (m *Manager) SetBodyYawNotifier(notifier BodyYawNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bodyNotify = notifier
}

// Run starts the control loop. Blocks until Stop is called.
func (m *Manager) Run() {
	ticker := time.NewTicker(m.rate)
	defer ticker.Stop()

	m.mu.Lock()
	m.running = true
	m.mu.Unlock()

	fmt.Printf("🎬 MovementManager started (%.0fHz)\n", 1.0/m.rate.Seconds())

	for {
		select {
		case <-m.stop:
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			fmt.Println("🎬 MovementManager stopped")
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

// Stop halts the control loop.
func (m *Manager) Stop() {
	close(m.stop)
}

// tick executes one control cycle.
func (m *Manager) tick() {
	m.mu.RLock()
	
	// 1. Get primary pose
	var primaryPose Pose
	if m.currentMove != nil {
		elapsed := time.Since(m.moveStartTime)
		if m.currentMove.IsComplete(elapsed) {
			// Move finished - will clear on next lock upgrade
			primaryPose = m.lastPrimaryPose
		} else {
			primaryPose = m.currentMove.Evaluate(elapsed)
		}
	} else {
		primaryPose = m.lastPrimaryPose
	}

	// 2. Get secondary offsets
	secondaryHead := m.secondaryOffset.Combined()

	m.mu.RUnlock()

	// 3. Compose: primary + secondary
	combined := Pose{
		Head: robot.Offset{
			Roll:  primaryPose.Head.Roll + secondaryHead.Roll,
			Pitch: primaryPose.Head.Pitch + secondaryHead.Pitch,
			Yaw:   primaryPose.Head.Yaw + secondaryHead.Yaw,
		},
		Antennas: primaryPose.Antennas,
		BodyYaw:  primaryPose.BodyYaw,
	}

	// 4. Clamp to safe limits
	combined = combined.Clamp()

	// 5. Rate-limit large movements
	combined = m.rateLimitPose(combined)

	// 6. Dead-zone filtering
	if !m.needsSend(combined) {
		m.skippedTicks++
		return
	}

	// 7. Send to robot
	m.tickCount++
	err := m.robot.SetPose(&combined.Head, &combined.Antennas, &combined.BodyYaw)
	if err != nil {
		m.errorCount++
		if m.errorCount%100 == 1 {
			fmt.Printf("⚠️  MovementManager error: %v\n", err)
		}
	} else {
		m.lastSentPose = combined
	}

	// 8. Sync body yaw to world model
	if m.bodyNotify != nil && abs(combined.BodyYaw-m.lastSentPose.BodyYaw) > 0.01 {
		m.bodyNotify.SetBodyYaw(combined.BodyYaw)
	}

	// 9. Check if move completed
	m.mu.Lock()
	if m.currentMove != nil {
		elapsed := time.Since(m.moveStartTime)
		if m.currentMove.IsComplete(elapsed) {
			fmt.Printf("🎬 Move '%s' completed\n", m.currentMove.Name())
			m.lastPrimaryPose = m.currentMove.Evaluate(elapsed)
			m.currentMove = nil
		}
	}
	m.mu.Unlock()

	// 10. Periodic heartbeat
	if m.tickCount%100 == 0 {
		fmt.Printf("💓 MovementManager: %d ticks, %d skipped, head=(%.2f,%.2f,%.2f)\n",
			m.tickCount, m.skippedTicks, combined.Head.Roll, combined.Head.Pitch, combined.Head.Yaw)
	}
}

// rateLimitPose clamps movement deltas to prevent overwhelming the robot.
func (m *Manager) rateLimitPose(target Pose) Pose {
	// Only rate-limit head for now
	deltaRoll := target.Head.Roll - m.lastSentPose.Head.Roll
	deltaPitch := target.Head.Pitch - m.lastSentPose.Head.Pitch
	deltaYaw := target.Head.Yaw - m.lastSentPose.Head.Yaw

	maxDelta := max3(abs(deltaRoll), abs(deltaPitch), abs(deltaYaw))
	if maxDelta <= m.maxStepRad {
		return target // No rate limiting needed
	}

	// Clamp each axis
	result := target
	result.Head.Roll = m.lastSentPose.Head.Roll + clampStep(deltaRoll, m.maxStepRad)
	result.Head.Pitch = m.lastSentPose.Head.Pitch + clampStep(deltaPitch, m.maxStepRad)
	result.Head.Yaw = m.lastSentPose.Head.Yaw + clampStep(deltaYaw, m.maxStepRad)

	fmt.Printf("⚡ Rate-limited: %.1f° → %.1f° step\n", maxDelta*57.3, m.maxStepRad*57.3)

	return result
}

// needsSend returns true if the pose differs enough from last sent.
func (m *Manager) needsSend(p Pose) bool {
	const (
		headThreshold    = 0.005 // ~0.3 degrees
		antennaThreshold = 0.009 // ~0.5 degrees
		bodyThreshold    = 0.009 // ~0.5 degrees
	)

	headDiff := max3(
		abs(p.Head.Roll-m.lastSentPose.Head.Roll),
		abs(p.Head.Pitch-m.lastSentPose.Head.Pitch),
		abs(p.Head.Yaw-m.lastSentPose.Head.Yaw),
	)
	antennaDiff := max(
		abs(p.Antennas[0]-m.lastSentPose.Antennas[0]),
		abs(p.Antennas[1]-m.lastSentPose.Antennas[1]),
	)
	bodyDiff := abs(p.BodyYaw - m.lastSentPose.BodyYaw)

	return headDiff >= headThreshold || antennaDiff >= antennaThreshold || bodyDiff >= bodyThreshold
}

// ============================================================
// Primary Move API
// ============================================================

// QueueMove sets the current primary move.
// The move will start immediately, replacing any current move.
func (m *Manager) QueueMove(move Move) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If there's a current move, cache its last pose
	if m.currentMove != nil {
		elapsed := time.Since(m.moveStartTime)
		m.lastPrimaryPose = m.currentMove.Evaluate(elapsed)
	}

	m.currentMove = move
	m.moveStartTime = time.Now()
	fmt.Printf("🎬 Move queued: %s (duration: %v)\n", move.Name(), move.Duration())
}

// StopMove stops the current primary move immediately.
func (m *Manager) StopMove() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentMove != nil {
		elapsed := time.Since(m.moveStartTime)
		m.lastPrimaryPose = m.currentMove.Evaluate(elapsed)
		fmt.Printf("🎬 Move '%s' stopped\n", m.currentMove.Name())
		m.currentMove = nil
	}
}

// IsMovePlaying returns true if a primary move is currently playing.
func (m *Manager) IsMovePlaying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentMove != nil
}

// CurrentMoveName returns the name of the current move, or empty if idle.
func (m *Manager) CurrentMoveName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.currentMove != nil {
		return m.currentMove.Name()
	}
	return ""
}

// ============================================================
// Secondary Offset API
// ============================================================

// SetFaceTrackingOffset sets the face tracking offset (secondary).
// This is continuously composed with the primary pose.
func (m *Manager) SetFaceTrackingOffset(offset robot.Offset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secondaryOffset.FaceTracking = offset
}

// SetSpeechOffset sets the speech wobble offset (secondary).
func (m *Manager) SetSpeechOffset(offset robot.Offset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secondaryOffset.Speech = offset
}

// SetAudioOffset sets the audio DOA offset (secondary).
func (m *Manager) SetAudioOffset(offset robot.Offset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secondaryOffset.Audio = offset
}

// ClearSecondaryOffsets resets all secondary offsets to zero.
func (m *Manager) ClearSecondaryOffsets() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secondaryOffset = SecondaryOffset{}
}

// ============================================================
// RateController API Compatibility
// (Allows gradual migration from existing RateController)
// ============================================================

// SetBaseHead sets the primary head pose (for compatibility with RateController).
func (m *Manager) SetBaseHead(offset robot.Offset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPrimaryPose.Head = offset
}

// SetTrackingOffset sets the face tracking offset (for compatibility).
func (m *Manager) SetTrackingOffset(offset robot.Offset) {
	m.SetFaceTrackingOffset(offset)
}

// SetAntennas sets the antenna positions (for compatibility).
func (m *Manager) SetAntennas(left, right float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPrimaryPose.Antennas = [2]float64{left, right}
}

// SetBodyYaw sets the body rotation (for compatibility).
func (m *Manager) SetBodyYaw(yaw float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPrimaryPose.BodyYaw = yaw
}

// ============================================================
// Helper functions
// ============================================================

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func max3(a, b, c float64) float64 {
	return max(max(a, b), c)
}

func clampStep(delta, maxStep float64) float64 {
	if delta > maxStep {
		return maxStep
	}
	if delta < -maxStep {
		return -maxStep
	}
	return delta
}

