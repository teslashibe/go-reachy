package worldmodel

import (
	"testing"
	"time"
)

func TestSelectPriorityTarget_SingleFace(t *testing.T) {
	w := New()
	w.UpdateEntity("face_0", 0.5, 50) // Centered face

	target := w.SelectPriorityTarget()
	if target == nil {
		t.Fatal("SelectPriorityTarget() = nil, want entity")
	}
	if target.ID != "face_0" {
		t.Errorf("SelectPriorityTarget().ID = %q, want %q", target.ID, "face_0")
	}
}

func TestSelectPriorityTarget_MultipleFaces(t *testing.T) {
	w := New()
	w.UpdateEntity("face_0", 0.3, 20) // Left face
	w.UpdateEntity("face_1", 0.5, 50) // Centered face
	w.UpdateEntity("face_2", 0.7, 80) // Right face

	target := w.SelectPriorityTarget()
	if target == nil {
		t.Fatal("SelectPriorityTarget() = nil, want entity")
	}
	// Centered face should win (most centered)
	if target.ID != "face_1" {
		t.Errorf("SelectPriorityTarget().ID = %q, want %q (most centered)", target.ID, "face_1")
	}
}

func TestSelectPriorityTarget_SpeakingWins(t *testing.T) {
	w := New()
	w.UpdateEntity("face_0", 0.5, 50) // Centered face
	w.UpdateEntity("face_1", 0.7, 80) // Right face, will be speaking

	// Mark face_1 as speaking
	w.mu.Lock()
	w.entities["face_1"].AudioConfidence = 0.8
	w.entities["face_1"].LastAudioMatch = time.Now()
	w.mu.Unlock()

	target := w.SelectPriorityTarget()
	if target == nil {
		t.Fatal("SelectPriorityTarget() = nil, want entity")
	}
	// Speaking face should win even though not centered
	if target.ID != "face_1" {
		t.Errorf("SelectPriorityTarget().ID = %q, want %q (speaking)", target.ID, "face_1")
	}
}

func TestSelectPriorityTarget_StaleFacesIgnored(t *testing.T) {
	w := New()
	w.UpdateEntity("face_0", 0.5, 50)

	// Make the entity stale
	w.mu.Lock()
	w.entities["face_0"].Confidence = 0.05 // Below forgetThreshold
	w.mu.Unlock()

	target := w.SelectPriorityTarget()
	if target != nil {
		t.Errorf("SelectPriorityTarget() = %v, want nil (stale entity)", target.ID)
	}
}

func TestGetEntityCount(t *testing.T) {
	w := New()

	if count := w.GetEntityCount(); count != 0 {
		t.Errorf("GetEntityCount() = %d, want 0", count)
	}

	w.UpdateEntity("face_0", 0.5, 50)
	if count := w.GetEntityCount(); count != 1 {
		t.Errorf("GetEntityCount() = %d, want 1", count)
	}

	w.UpdateEntity("face_1", 0.7, 80)
	if count := w.GetEntityCount(); count != 2 {
		t.Errorf("GetEntityCount() = %d, want 2", count)
	}
}

