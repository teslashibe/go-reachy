package tracking

import (
	"math"
	"testing"
	"time"
)

func TestPDController_InterpolateToNeutral(t *testing.T) {
	cfg := DefaultConfig()
	c := NewPDController(cfg)

	// Start at yaw 0.5
	c.SetCurrentYaw(0.5)

	// Start interpolation to neutral over 100ms
	c.InterpolateToNeutral(100 * time.Millisecond)

	if !c.IsInterpolating() {
		t.Error("Expected IsInterpolating to be true")
	}

	// Immediately after start, should still be near 0.5
	yaw, shouldMove := c.Update()
	if !shouldMove {
		t.Error("Expected shouldMove to be true during interpolation")
	}
	if yaw < 0.4 || yaw > 0.5 {
		t.Errorf("Expected yaw near 0.5 at start, got %v", yaw)
	}

	// Wait halfway
	time.Sleep(50 * time.Millisecond)
	yaw, _ = c.Update()
	// Should be around 0.25
	if yaw < 0.15 || yaw > 0.35 {
		t.Errorf("Expected yaw near 0.25 at midpoint, got %v", yaw)
	}

	// Wait for completion
	time.Sleep(60 * time.Millisecond)
	yaw, _ = c.Update()

	// Should be at 0 (neutral)
	if math.Abs(yaw) > 0.01 {
		t.Errorf("Expected yaw at 0 after interpolation, got %v", yaw)
	}

	if c.IsInterpolating() {
		t.Error("Expected IsInterpolating to be false after completion")
	}
}

func TestPDController_InterpolationComplete(t *testing.T) {
	cfg := DefaultConfig()
	c := NewPDController(cfg)

	// Start at yaw 0.8
	c.SetCurrentYaw(0.8)

	// Interpolate to 0.2 over 50ms
	c.InterpolateTo(0.2, 50*time.Millisecond)

	// Wait for completion plus some margin
	time.Sleep(60 * time.Millisecond)

	yaw, _ := c.Update()

	// Should end at target
	if math.Abs(yaw-0.2) > 0.01 {
		t.Errorf("Expected yaw at 0.2, got %v", yaw)
	}

	// Current yaw should be updated
	if math.Abs(c.GetCurrentYaw()-0.2) > 0.01 {
		t.Errorf("Expected GetCurrentYaw at 0.2, got %v", c.GetCurrentYaw())
	}
}

func TestPDController_InterpolationInterrupted(t *testing.T) {
	cfg := DefaultConfig()
	c := NewPDController(cfg)

	// Start at yaw 0.5
	c.SetCurrentYaw(0.5)

	// Start interpolation
	c.InterpolateToNeutral(100 * time.Millisecond)

	// Update a couple times
	time.Sleep(20 * time.Millisecond)
	c.Update()

	if !c.IsInterpolating() {
		t.Error("Expected interpolation to be active")
	}

	// Set a new target - should cancel interpolation
	c.SetTarget(0.8)

	if c.IsInterpolating() {
		t.Error("Expected interpolation to be cancelled by SetTarget")
	}

	// Update should now use PD control, not interpolation
	yaw, _ := c.Update()

	// Should be moving toward 0.8, not toward 0
	// Current yaw was somewhere between 0 and 0.5, so should increase
	time.Sleep(10 * time.Millisecond)
	yaw2, _ := c.Update()

	if yaw2 <= yaw {
		t.Errorf("Expected yaw to increase toward target 0.8, got %v -> %v", yaw, yaw2)
	}
}

func TestPDController_InterpolateToNeutral_AlreadyAtNeutral(t *testing.T) {
	cfg := DefaultConfig()
	c := NewPDController(cfg)

	// Already at neutral
	c.SetCurrentYaw(0)

	// Interpolate to neutral
	c.InterpolateToNeutral(50 * time.Millisecond)

	// Should complete immediately (or nearly so)
	yaw, _ := c.Update()

	if math.Abs(yaw) > 0.01 {
		t.Errorf("Expected yaw at 0, got %v", yaw)
	}
}

func TestPDController_PDControl_StillWorks(t *testing.T) {
	cfg := DefaultConfig()
	c := NewPDController(cfg)

	// Set target
	c.SetTarget(0.5)

	// Should not be interpolating
	if c.IsInterpolating() {
		t.Error("Should not be interpolating when using SetTarget")
	}

	// Update should work with PD control
	yaw, shouldMove := c.Update()
	if !shouldMove {
		t.Error("Expected shouldMove for PD control")
	}

	// Yaw should be moving toward target
	if yaw <= 0 {
		t.Errorf("Expected positive yaw movement toward 0.5, got %v", yaw)
	}
}

