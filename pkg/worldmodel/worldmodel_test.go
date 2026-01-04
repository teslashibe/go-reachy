package worldmodel

import (
	"math"
	"testing"
	"time"
)

const floatTolerance = 1e-9

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

func TestWorldModel_BodyYaw(t *testing.T) {
	w := New()

	// Initial value should be 0
	if w.GetBodyYaw() != 0 {
		t.Errorf("Initial BodyYaw: got %v, want 0", w.GetBodyYaw())
	}

	// Set and get
	w.SetBodyYaw(0.5)
	if !floatEquals(w.GetBodyYaw(), 0.5) {
		t.Errorf("BodyYaw after set: got %v, want 0.5", w.GetBodyYaw())
	}

	// Negative value
	w.SetBodyYaw(-0.3)
	if !floatEquals(w.GetBodyYaw(), -0.3) {
		t.Errorf("BodyYaw after negative set: got %v, want -0.3", w.GetBodyYaw())
	}
}

func TestWorldModel_GetTargetWorldAngle_WithBodyYaw(t *testing.T) {
	w := New()

	// Add an entity at room angle 0.5 rad
	w.UpdateEntity("person", 0.5, 50.0)

	// With body at 0, target should be at 0.5 (body-relative = room)
	angle, ok := w.GetTargetWorldAngle()
	if !ok {
		t.Fatal("Expected target to exist")
	}
	if !floatEquals(angle, 0.5) {
		t.Errorf("With body at 0: got %v, want 0.5", angle)
	}

	// Rotate body to 0.3 rad
	// Entity is at room 0.5, body at 0.3, so body-relative = 0.5 - 0.3 = 0.2
	w.SetBodyYaw(0.3)
	angle, ok = w.GetTargetWorldAngle()
	if !ok {
		t.Fatal("Expected target to exist")
	}
	if !floatEquals(angle, 0.2) {
		t.Errorf("With body at 0.3: got %v, want 0.2", angle)
	}

	// Rotate body to match entity
	// Entity at room 0.5, body at 0.5, body-relative = 0
	w.SetBodyYaw(0.5)
	angle, ok = w.GetTargetWorldAngle()
	if !ok {
		t.Fatal("Expected target to exist")
	}
	if !floatEquals(angle, 0.0) {
		t.Errorf("With body at 0.5: got %v, want 0.0", angle)
	}
}

func TestWorldModel_GetTargetRoomAngle(t *testing.T) {
	w := New()

	// Add an entity at room angle 0.5 rad
	w.UpdateEntity("person", 0.5, 50.0)

	// Room angle should always be 0.5 regardless of body yaw
	roomAngle, ok := w.GetTargetRoomAngle()
	if !ok {
		t.Fatal("Expected target to exist")
	}
	if !floatEquals(roomAngle, 0.5) {
		t.Errorf("Room angle: got %v, want 0.5", roomAngle)
	}

	// Even after body rotation
	w.SetBodyYaw(0.8)
	roomAngle, ok = w.GetTargetRoomAngle()
	if !ok {
		t.Fatal("Expected target to exist")
	}
	if !floatEquals(roomAngle, 0.5) {
		t.Errorf("Room angle after body rotate: got %v, want 0.5", roomAngle)
	}
}

func TestWorldModel_RoomCoordinates(t *testing.T) {
	w := New()

	// Simulate: face detected at center of frame while head at yaw 0.3, body at 0.2
	// The world angle passed to UpdateEntity should be in room coords
	// Room angle = bodyYaw + headYaw + cameraOffset
	// For this test, assume caller computes room angle correctly
	roomAngle := 0.5 // Entity is at 0.5 rad in room
	w.UpdateEntity("person", roomAngle, 50.0)

	// Verify stored in room coordinates
	entities := w.GetAllEntities()
	if len(entities) != 1 {
		t.Fatalf("Expected 1 entity, got %d", len(entities))
	}
	if !floatEquals(entities[0].WorldAngle, 0.5) {
		t.Errorf("Stored WorldAngle: got %v, want 0.5", entities[0].WorldAngle)
	}
}

func TestWorldModel_NoTarget(t *testing.T) {
	w := New()

	// No entities
	_, ok := w.GetTargetWorldAngle()
	if ok {
		t.Error("Expected no target when empty")
	}

	_, ok = w.GetTargetRoomAngle()
	if ok {
		t.Error("Expected no room target when empty")
	}
}

func TestWorldModel_VelocityPrediction_WithBodyYaw(t *testing.T) {
	w := New()

	// Add entity and wait a bit
	w.UpdateEntity("person", 0.5, 50.0)
	time.Sleep(100 * time.Millisecond)

	// Update with new position (moving right in room)
	w.UpdateEntity("person", 0.6, 60.0)

	// Body at 0.2, so expected body-relative ~= 0.6 - 0.2 = 0.4
	// (velocity prediction adds a small amount)
	w.SetBodyYaw(0.2)
	angle, ok := w.GetTargetWorldAngle()
	if !ok {
		t.Fatal("Expected target to exist")
	}

	// Should be close to 0.4 (with small prediction adjustment)
	if angle < 0.35 || angle > 0.5 {
		t.Errorf("Expected body-relative angle ~0.4, got %v", angle)
	}
}

// Issue #79: Tests for body yaw limit functionality

func TestWorldModel_BodyYawLimit_Default(t *testing.T) {
	w := New()

	// Default limit should match Python reachy's 0.9*π ≈ 2.827 rad
	limit := w.GetBodyYawLimit()
	expectedLimit := 0.9 * math.Pi

	if math.Abs(limit-expectedLimit) > 0.001 {
		t.Errorf("Default BodyYawLimit: got %v, want ~%v (0.9*π)", limit, expectedLimit)
	}
}

func TestWorldModel_SetBodyYawLimit(t *testing.T) {
	w := New()

	// Set custom limit
	w.SetBodyYawLimit(1.5)
	if w.GetBodyYawLimit() != 1.5 {
		t.Errorf("After SetBodyYawLimit(1.5): got %v, want 1.5", w.GetBodyYawLimit())
	}

	// Zero and negative values should be ignored
	w.SetBodyYawLimit(0)
	if w.GetBodyYawLimit() != 1.5 {
		t.Errorf("SetBodyYawLimit(0) should be ignored: got %v, want 1.5", w.GetBodyYawLimit())
	}

	w.SetBodyYawLimit(-1.0)
	if w.GetBodyYawLimit() != 1.5 {
		t.Errorf("SetBodyYawLimit(-1.0) should be ignored: got %v, want 1.5", w.GetBodyYawLimit())
	}
}

func TestWorldModel_IsBodyAtLimit(t *testing.T) {
	w := New()
	w.SetBodyYawLimit(1.0) // Set smaller limit for easier testing

	tests := []struct {
		name      string
		bodyYaw   float64
		direction float64
		atLimit   bool
	}{
		{"center, move left", 0.0, 0.1, false},
		{"center, move right", 0.0, -0.1, false},
		{"at positive limit, move left", 1.0, 0.1, true},
		{"at positive limit, move right", 1.0, -0.1, false},
		{"at negative limit, move right", -1.0, -0.1, true},
		{"at negative limit, move left", -1.0, 0.1, false},
		{"near positive limit, move left", 0.99, 0.1, false},
		{"beyond positive limit, move left", 1.1, 0.1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w.SetBodyYaw(tc.bodyYaw)
			result := w.IsBodyAtLimit(tc.direction)
			if result != tc.atLimit {
				t.Errorf("IsBodyAtLimit(%v) with body at %v: got %v, want %v",
					tc.direction, tc.bodyYaw, result, tc.atLimit)
			}
		})
	}
}

func TestWorldModel_CanBodyRotate(t *testing.T) {
	w := New()
	w.SetBodyYawLimit(1.0)

	// CanBodyRotate is the inverse of IsBodyAtLimit
	w.SetBodyYaw(1.0) // At positive limit
	if w.CanBodyRotate(0.1) {
		t.Error("Should not be able to rotate left when at positive limit")
	}
	if !w.CanBodyRotate(-0.1) {
		t.Error("Should be able to rotate right when at positive limit")
	}

	w.SetBodyYaw(0.0) // At center
	if !w.CanBodyRotate(0.1) {
		t.Error("Should be able to rotate left when at center")
	}
	if !w.CanBodyRotate(-0.1) {
		t.Error("Should be able to rotate right when at center")
	}
}

func TestWorldModel_IsBodyAtLimit_ZeroDirection(t *testing.T) {
	w := New()
	w.SetBodyYaw(1.0)

	// Zero direction should return false (no movement = not at limit)
	if w.IsBodyAtLimit(0) {
		t.Error("IsBodyAtLimit(0) should return false")
	}
}

