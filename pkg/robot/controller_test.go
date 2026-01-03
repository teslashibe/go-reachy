package robot

import (
	"math"
	"sync"
	"testing"
	"time"
)

const floatTolerance = 1e-9

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

// mockRobot records all commands for testing
type mockRobot struct {
	mu       sync.Mutex
	headCalls []struct{ roll, pitch, yaw float64 }
	antCalls  []struct{ left, right float64 }
	bodyCalls []float64
}

func (m *mockRobot) SetHeadPose(roll, pitch, yaw float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.headCalls = append(m.headCalls, struct{ roll, pitch, yaw float64 }{roll, pitch, yaw})
	return nil
}

func (m *mockRobot) SetAntennas(left, right float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.antCalls = append(m.antCalls, struct{ left, right float64 }{left, right})
	return nil
}

func (m *mockRobot) SetBodyYaw(yaw float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bodyCalls = append(m.bodyCalls, yaw)
	return nil
}

func (m *mockRobot) lastHead() (roll, pitch, yaw float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.headCalls) == 0 {
		return 0, 0, 0
	}
	last := m.headCalls[len(m.headCalls)-1]
	return last.roll, last.pitch, last.yaw
}

func (m *mockRobot) headCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.headCalls)
}

func TestOffset_Add(t *testing.T) {
	a := Offset{Roll: 0.1, Pitch: 0.2, Yaw: 0.3}
	b := Offset{Roll: 0.05, Pitch: -0.1, Yaw: 0.2}
	
	result := a.Add(b)
	
	if !floatEquals(result.Roll, 0.15) {
		t.Errorf("Roll: got %v, want 0.15", result.Roll)
	}
	if !floatEquals(result.Pitch, 0.1) {
		t.Errorf("Pitch: got %v, want 0.1", result.Pitch)
	}
	if !floatEquals(result.Yaw, 0.5) {
		t.Errorf("Yaw: got %v, want 0.5", result.Yaw)
	}
}

func TestController_FusesPoses(t *testing.T) {
	mock := &mockRobot{}
	ctrl := NewRateController(mock, 10*time.Millisecond)
	
	// Set base and tracking offsets
	ctrl.SetBaseHead(Offset{Roll: 0, Pitch: 0, Yaw: 0.3})
	ctrl.SetTrackingOffset(Offset{Roll: 0, Pitch: 0, Yaw: 0.2})
	
	// Run one tick
	ctrl.tick()
	
	// Check combined output
	roll, pitch, yaw := mock.lastHead()
	if roll != 0 {
		t.Errorf("Roll: got %v, want 0", roll)
	}
	if pitch != 0 {
		t.Errorf("Pitch: got %v, want 0", pitch)
	}
	if yaw != 0.5 {
		t.Errorf("Yaw: got %v, want 0.5 (0.3 + 0.2)", yaw)
	}
}

func TestController_BodyYaw(t *testing.T) {
	ctrl := NewRateController(nil, 10*time.Millisecond)
	
	// Initial value should be 0
	if ctrl.BodyYaw() != 0 {
		t.Errorf("Initial BodyYaw: got %v, want 0", ctrl.BodyYaw())
	}
	
	// Set and get
	ctrl.SetBodyYaw(0.5)
	if ctrl.BodyYaw() != 0.5 {
		t.Errorf("BodyYaw after set: got %v, want 0.5", ctrl.BodyYaw())
	}
}

func TestController_BodyYaw_ThreadSafe(t *testing.T) {
	ctrl := NewRateController(nil, 10*time.Millisecond)
	
	var wg sync.WaitGroup
	
	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ctrl.SetBodyYaw(val)
			}
		}(float64(i) * 0.1)
	}
	
	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = ctrl.BodyYaw()
			}
		}()
	}
	
	wg.Wait()
	// If we get here without deadlock/race, test passes
}

func TestController_RunStop(t *testing.T) {
	mock := &mockRobot{}
	ctrl := NewRateController(mock, 5*time.Millisecond)
	
	// Start controller in goroutine
	done := make(chan struct{})
	go func() {
		ctrl.Run()
		close(done)
	}()
	
	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)
	
	// Stop it
	ctrl.Stop()
	
	// Should exit promptly
	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Controller did not stop within timeout")
	}
	
	// Should have made some calls
	if mock.headCallCount() < 5 {
		t.Errorf("Expected at least 5 head calls, got %d", mock.headCallCount())
	}
}

func TestController_Rate(t *testing.T) {
	mock := &mockRobot{}
	ctrl := NewRateController(mock, 10*time.Millisecond) // 100Hz
	
	go func() {
		ctrl.Run()
	}()
	
	// Run for 100ms
	time.Sleep(100 * time.Millisecond)
	ctrl.Stop()
	
	// At 100Hz for 100ms, expect ~10 calls (with some tolerance)
	count := mock.headCallCount()
	if count < 8 || count > 15 {
		t.Errorf("Expected ~10 calls at 100Hz over 100ms, got %d", count)
	}
}

func TestController_CombinedHead(t *testing.T) {
	ctrl := NewRateController(nil, 10*time.Millisecond)
	
	ctrl.SetBaseHead(Offset{Roll: 0.1, Pitch: 0.2, Yaw: 0.3})
	ctrl.SetTrackingOffset(Offset{Roll: 0.05, Pitch: -0.1, Yaw: 0.1})
	
	combined := ctrl.CombinedHead()
	
	if !floatEquals(combined.Roll, 0.15) {
		t.Errorf("Combined Roll: got %v, want 0.15", combined.Roll)
	}
	if !floatEquals(combined.Pitch, 0.1) {
		t.Errorf("Combined Pitch: got %v, want 0.1", combined.Pitch)
	}
	if !floatEquals(combined.Yaw, 0.4) {
		t.Errorf("Combined Yaw: got %v, want 0.4", combined.Yaw)
	}
}

func TestController_NilRobot(t *testing.T) {
	ctrl := NewRateController(nil, 10*time.Millisecond)
	
	// Should not panic with nil robot
	ctrl.SetBaseHead(Offset{Yaw: 0.5})
	ctrl.tick()
	// If we get here, test passes
}

