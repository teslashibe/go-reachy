package tracking

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/teslashibe/go-reachy/pkg/worldmodel"
)

// mockRobotController records head poses for testing
type mockRobotController struct {
	mu        sync.Mutex
	headCalls []struct{ roll, pitch, yaw float64 }
}

func (m *mockRobotController) SetHeadPose(roll, pitch, yaw float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.headCalls = append(m.headCalls, struct{ roll, pitch, yaw float64 }{roll, pitch, yaw})
	return nil
}

func (m *mockRobotController) lastYaw() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.headCalls) == 0 {
		return 0
	}
	return m.headCalls[len(m.headCalls)-1].yaw
}

func (m *mockRobotController) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.headCalls)
}

func TestTracker_OffsetMode(t *testing.T) {
	// Create a mock offset handler
	var receivedOffsets []Offset
	var mu sync.Mutex

	handler := func(offset Offset) {
		mu.Lock()
		receivedOffsets = append(receivedOffsets, offset)
		mu.Unlock()
	}

	// Create tracker with mock robot (direct mode)
	cfg := DefaultConfig()
	cfg.MovementInterval = 10 * time.Millisecond
	robot := &mockRobotController{}

	// Create tracker without video/detector (manual testing)
	tracker := &Tracker{
		config:        cfg,
		robot:         robot,
		world:         worldmodel.New(),
		controller:    NewPDController(cfg),
		lastLoggedYaw: 999.0,
	}

	// Set offset handler - should switch to offset mode
	tracker.SetOffsetHandler(handler)

	// Add a target to the world model
	tracker.world.UpdateEntity("test", 0.5, 50.0)

	// Update movement manually
	tracker.updateMovement()

	// Wait a bit for processing
	time.Sleep(20 * time.Millisecond)

	// Should have received offset, not called robot directly
	mu.Lock()
	offsetCount := len(receivedOffsets)
	mu.Unlock()

	if offsetCount == 0 {
		t.Error("Expected to receive offset in offset mode")
	}

	if robot.callCount() > 0 {
		t.Error("Expected no direct robot calls in offset mode")
	}
}

func TestTracker_DirectMode(t *testing.T) {
	// Create tracker without offset handler (direct mode)
	cfg := DefaultConfig()
	cfg.MovementInterval = 10 * time.Millisecond
	robot := &mockRobotController{}

	tracker := &Tracker{
		config:        cfg,
		robot:         robot,
		world:         worldmodel.New(),
		controller:    NewPDController(cfg),
		lastLoggedYaw: 999.0,
	}

	// No offset handler - should use direct mode
	// Add a target
	tracker.world.UpdateEntity("test", 0.5, 50.0)

	// Update movement
	tracker.updateMovement()

	// Should have called robot directly
	if robot.callCount() == 0 {
		t.Error("Expected direct robot call in direct mode")
	}
}

func TestTracker_BodyRotation(t *testing.T) {
	// Test that body rotation maintains accurate tracking
	cfg := DefaultConfig()
	tracker := &Tracker{
		config:        cfg,
		world:         worldmodel.New(),
		controller:    NewPDController(cfg),
		lastLoggedYaw: 999.0,
	}

	// Entity at room angle 0.5
	tracker.world.UpdateEntity("person", 0.5, 50.0)

	// With body at 0, target should be at 0.5 (body-relative)
	angle1, ok := tracker.world.GetTargetWorldAngle()
	if !ok {
		t.Fatal("Expected target")
	}
	if !floatEq(angle1, 0.5) {
		t.Errorf("Before body rotate: expected 0.5, got %v", angle1)
	}

	// Rotate body to 0.3
	tracker.SetBodyYaw(0.3)

	// Now target should be at 0.2 (0.5 - 0.3)
	angle2, ok := tracker.world.GetTargetWorldAngle()
	if !ok {
		t.Fatal("Expected target")
	}
	if !floatEq(angle2, 0.2) {
		t.Errorf("After body rotate: expected 0.2, got %v", angle2)
	}
}

func TestTracker_SmoothInterpolation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ScanStartDelay = 50 * time.Millisecond // Short delay for testing

	var receivedOffsets []Offset
	var mu sync.Mutex

	tracker := &Tracker{
		config:        cfg,
		world:         worldmodel.New(),
		controller:    NewPDController(cfg),
		lastLoggedYaw: 999.0,
	}

	tracker.SetOffsetHandler(func(offset Offset) {
		mu.Lock()
		receivedOffsets = append(receivedOffsets, offset)
		mu.Unlock()
	})

	// Start with head at 0.5
	tracker.controller.SetCurrentYaw(0.5)
	tracker.lastFaceSeenAt = time.Now().Add(-100 * time.Millisecond) // Face lost 100ms ago

	// No target - should trigger interpolation
	tracker.updateNoTarget()

	// Should start interpolating
	if !tracker.isInterpolating {
		t.Error("Expected interpolation to start")
	}

	// Wait for some interpolation
	time.Sleep(50 * time.Millisecond)

	// Update a few times
	for i := 0; i < 5; i++ {
		tracker.updateNoTarget()
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	offsets := receivedOffsets
	mu.Unlock()

	// Should have received multiple offsets
	if len(offsets) < 2 {
		t.Errorf("Expected multiple offsets during interpolation, got %d", len(offsets))
	}

	// First offset should be close to 0.5, later ones closer to 0
	if len(offsets) >= 2 {
		firstYaw := offsets[0].Yaw
		lastYaw := offsets[len(offsets)-1].Yaw

		if lastYaw >= firstYaw {
			t.Errorf("Expected decreasing yaw during interpolation: first=%v, last=%v",
				firstYaw, lastYaw)
		}
	}
}

func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

