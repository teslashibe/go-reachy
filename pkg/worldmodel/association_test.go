package worldmodel

import (
	"testing"
	"time"
)

func TestAssociateAudio_ExactMatch(t *testing.T) {
	w := New()

	// Add a face entity at angle 0.5 rad
	w.UpdateEntity("face1", 0.5, 50)

	// Audio at same angle should match
	entityID := w.AssociateAudio(0.5, true, 0.8)
	if entityID != "face1" {
		t.Errorf("AssociateAudio() = %q, want %q", entityID, "face1")
	}

	// Entity should have audio confidence updated
	entity := w.GetFocusTarget()
	if entity.AudioConfidence < 0.5 {
		t.Errorf("AudioConfidence = %v, want > 0.5", entity.AudioConfidence)
	}
}

func TestAssociateAudio_CloseMatch(t *testing.T) {
	w := New()

	// Add a face entity at angle 0.5 rad
	w.UpdateEntity("face1", 0.5, 50)

	// Audio within threshold (~15 degrees = 0.26 rad) should match
	entityID := w.AssociateAudio(0.6, true, 0.8)
	if entityID != "face1" {
		t.Errorf("AssociateAudio() = %q, want %q", entityID, "face1")
	}
}

func TestAssociateAudio_NoMatch(t *testing.T) {
	w := New()

	// Add a face entity at angle 0.5 rad
	w.UpdateEntity("face1", 0.5, 50)

	// Audio far from face (> 0.26 rad threshold) should not match
	entityID := w.AssociateAudio(1.0, true, 0.8)
	if entityID != "" {
		t.Errorf("AssociateAudio() = %q, want empty string", entityID)
	}
}

func TestAssociateAudio_NotSpeaking(t *testing.T) {
	w := New()

	// Add a face entity
	w.UpdateEntity("face1", 0.5, 50)

	// Audio not speaking should not match
	entityID := w.AssociateAudio(0.5, false, 0.8)
	if entityID != "" {
		t.Errorf("AssociateAudio() with speaking=false = %q, want empty", entityID)
	}
}

func TestAssociateAudio_LowConfidence(t *testing.T) {
	w := New()

	// Add a face entity
	w.UpdateEntity("face1", 0.5, 50)

	// Audio with low confidence should not match
	entityID := w.AssociateAudio(0.5, true, 0.2)
	if entityID != "" {
		t.Errorf("AssociateAudio() with low confidence = %q, want empty", entityID)
	}
}

func TestGetSpeakingEntity(t *testing.T) {
	w := New()

	// Add a face entity and associate audio
	w.UpdateEntity("face1", 0.5, 50)
	w.AssociateAudio(0.5, true, 0.8)

	// Should return the speaking entity
	speaking := w.GetSpeakingEntity()
	if speaking == nil {
		t.Error("GetSpeakingEntity() = nil, want entity")
	} else if speaking.ID != "face1" {
		t.Errorf("GetSpeakingEntity().ID = %q, want %q", speaking.ID, "face1")
	}
}

func TestGetTarget_PrioritizesSpeakingFace(t *testing.T) {
	w := New()

	// Add a face entity and associate audio
	w.UpdateEntity("face1", 0.5, 50)
	w.AssociateAudio(0.5, true, 0.8)

	angle, source, ok := w.GetTarget()
	if !ok {
		t.Error("GetTarget() ok = false, want true")
	}
	if source != "face+audio" {
		t.Errorf("GetTarget() source = %q, want %q", source, "face+audio")
	}
	if angle < 0.4 || angle > 0.6 {
		t.Errorf("GetTarget() angle = %v, want ~0.5", angle)
	}
}

func TestAudioAssociationDecays(t *testing.T) {
	w := New()

	// Add entity and associate audio
	w.UpdateEntity("face1", 0.5, 50)
	w.AssociateAudio(0.5, true, 0.8)

	// Manually expire the audio match
	w.mu.Lock()
	w.entities["face1"].LastAudioMatch = time.Now().Add(-2 * time.Second)
	w.mu.Unlock()

	// Speaking entity should now be nil (audio association expired)
	speaking := w.GetSpeakingEntity()
	if speaking != nil {
		t.Error("GetSpeakingEntity() should be nil after expiry")
	}
}

