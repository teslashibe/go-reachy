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


