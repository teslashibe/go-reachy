package worldmodel

import (
	"testing"
	"time"
)

func TestUpdateObject(t *testing.T) {
	w := New()

	w.UpdateObject("dog", 0.9, 50, 60, 0.3, 0.4, true)

	objects := w.GetObjects()
	if len(objects) != 1 {
		t.Fatalf("GetObjects() len = %d, want 1", len(objects))
	}

	dog := objects[0]
	if dog.ClassName != "dog" {
		t.Errorf("ClassName = %q, want %q", dog.ClassName, "dog")
	}
	if !dog.IsAnimal {
		t.Error("IsAnimal = false, want true")
	}
	if dog.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", dog.Confidence)
	}
}

func TestUpdateObjects_ReplacesAll(t *testing.T) {
	w := New()

	// Add initial object
	w.UpdateObject("dog", 0.9, 50, 60, 0.3, 0.4, true)

	// Replace with new set
	w.UpdateObjects([]*DetectedObject{
		{ClassName: "cat", Confidence: 0.8, IsAnimal: true},
		{ClassName: "cup", Confidence: 0.7, IsAnimal: false},
	})

	objects := w.GetObjects()
	if len(objects) != 2 {
		t.Fatalf("GetObjects() len = %d, want 2", len(objects))
	}

	// Dog should be gone
	if w.HasObject("dog") {
		t.Error("HasObject(dog) = true, want false (should be replaced)")
	}
}

func TestGetAnimals(t *testing.T) {
	w := New()

	w.UpdateObject("dog", 0.9, 50, 60, 0.3, 0.4, true)
	w.UpdateObject("cat", 0.8, 30, 40, 0.2, 0.3, true)
	w.UpdateObject("cup", 0.7, 70, 50, 0.1, 0.1, false)

	animals := w.GetAnimals()
	if len(animals) != 2 {
		t.Errorf("GetAnimals() len = %d, want 2", len(animals))
	}
}

func TestHasObject(t *testing.T) {
	w := New()

	if w.HasObject("dog") {
		t.Error("HasObject(dog) = true before adding")
	}

	w.UpdateObject("dog", 0.9, 50, 60, 0.3, 0.4, true)

	if !w.HasObject("dog") {
		t.Error("HasObject(dog) = false after adding")
	}

	if w.HasObject("cat") {
		t.Error("HasObject(cat) = true, not added")
	}
}

func TestGetObjects_ExpiresStale(t *testing.T) {
	w := New()

	w.UpdateObject("dog", 0.9, 50, 60, 0.3, 0.4, true)

	// Manually expire the object
	w.mu.Lock()
	w.objects["dog"].LastSeen = time.Now().Add(-3 * time.Second)
	w.mu.Unlock()

	objects := w.GetObjects()
	if len(objects) != 0 {
		t.Errorf("GetObjects() len = %d, want 0 (expired)", len(objects))
	}
}

func TestGetObjectsSummary(t *testing.T) {
	w := New()

	// Empty summary
	if summary := w.GetObjectsSummary(); summary != "" {
		t.Errorf("GetObjectsSummary() = %q, want empty", summary)
	}

	w.UpdateObject("dog", 0.9, 50, 60, 0.3, 0.4, true)
	w.UpdateObject("cup", 0.7, 70, 50, 0.1, 0.1, false)

	summary := w.GetObjectsSummary()
	// Order may vary, but should contain both
	if summary == "" {
		t.Error("GetObjectsSummary() = empty, want objects")
	}
}

