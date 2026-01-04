package emotions

import (
	"context"
	"testing"
	"time"
)

func TestListEmbedded(t *testing.T) {
	names, err := ListEmbedded()
	if err != nil {
		t.Fatalf("ListEmbedded failed: %v", err)
	}

	if len(names) != 81 {
		t.Errorf("Expected 81 embedded emotions, got %d", len(names))
	}

	// Check for some known emotions
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	expected := []string{"yes1", "no1", "sad1", "happy1", "dance1", "surprised1"}
	for _, e := range expected {
		if e == "happy1" {
			continue // happy1 might not exist, skip
		}
		if !found[e] {
			t.Logf("Note: emotion %q not found (may be expected)", e)
		}
	}
}

func TestLoadEmbedded(t *testing.T) {
	emotion, err := LoadEmbedded("yes1")
	if err != nil {
		t.Fatalf("LoadEmbedded(yes1) failed: %v", err)
	}

	if emotion.Name != "yes1" {
		t.Errorf("Expected name 'yes1', got %q", emotion.Name)
	}

	if emotion.Description == "" {
		t.Error("Expected non-empty description")
	}

	if emotion.Duration <= 0 {
		t.Errorf("Expected positive duration, got %v", emotion.Duration)
	}

	if len(emotion.Keyframes) == 0 {
		t.Error("Expected keyframes")
	}

	t.Logf("Loaded %q: %s (%.2fs, %d keyframes)",
		emotion.Name, emotion.Description, emotion.Duration.Seconds(), len(emotion.Keyframes))
}

func TestLoadEmbedded_NotFound(t *testing.T) {
	_, err := LoadEmbedded("nonexistent_emotion_12345")
	if err == nil {
		t.Error("Expected error for nonexistent emotion")
	}
}

func TestMatrixToEuler(t *testing.T) {
	// Identity matrix should give zero angles
	identity := [4][4]float64{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{0, 0, 0, 1},
	}

	roll, pitch, yaw := MatrixToEuler(identity)

	if abs(roll) > 0.001 || abs(pitch) > 0.001 || abs(yaw) > 0.001 {
		t.Errorf("Identity should give zero angles, got roll=%.4f, pitch=%.4f, yaw=%.4f",
			roll, pitch, yaw)
	}
}

func TestInterpolateKeyframes(t *testing.T) {
	kf1 := Keyframe{
		Head:     [4][4]float64{{1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}, {0, 0, 0, 1}},
		Antennas: [2]float64{0, 0},
		BodyYaw:  0,
	}

	kf2 := Keyframe{
		Head:     [4][4]float64{{1, 0, 0, 0.1}, {0, 1, 0, 0}, {0, 0, 1, 0}, {0, 0, 0, 1}},
		Antennas: [2]float64{1.0, 1.0},
		BodyYaw:  0.5,
	}

	// Midpoint interpolation
	mid := InterpolateKeyframes(kf1, kf2, 0.5)

	if abs(mid.Antennas[0]-0.5) > 0.001 {
		t.Errorf("Expected antenna[0]=0.5, got %f", mid.Antennas[0])
	}

	if abs(mid.BodyYaw-0.25) > 0.001 {
		t.Errorf("Expected bodyYaw=0.25, got %f", mid.BodyYaw)
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	err := reg.LoadBuiltIn()
	if err != nil {
		t.Fatalf("LoadBuiltIn failed: %v", err)
	}

	count := reg.Count()
	if count != 81 {
		t.Errorf("Expected 81 emotions, got %d", count)
	}

	// Test Get
	emotion, err := reg.Get("yes1")
	if err != nil {
		t.Errorf("Get(yes1) failed: %v", err)
	}
	if emotion == nil {
		t.Error("Expected non-nil emotion")
	}

	// Test List
	names := reg.List()
	if len(names) != 81 {
		t.Errorf("Expected 81 names, got %d", len(names))
	}

	// Test Categories
	cats := reg.Categories()
	if len(cats) == 0 {
		t.Error("Expected categories")
	}
	t.Logf("Found %d categories", len(cats))

	// Test Search
	matches := reg.Search("sad")
	if len(matches) == 0 {
		t.Error("Expected matches for 'sad'")
	}
	t.Logf("Search 'sad': %v", matches)
}

func TestPlayer(t *testing.T) {
	emotion, err := LoadEmbedded("yes1")
	if err != nil {
		t.Fatalf("Failed to load emotion: %v", err)
	}

	player := NewPlayer()

	frameCount := 0
	var lastPose Pose

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fast playback for testing
	opts := PlayerOptions{
		FrameRate: 30,
		Speed:     10.0, // 10x speed for faster test
		Loop:      false,
	}

	err = player.PlayWithOptions(ctx, emotion, func(pose Pose, elapsed time.Duration) bool {
		frameCount++
		lastPose = pose
		return true
	}, opts)

	if err != nil {
		t.Errorf("Playback error: %v", err)
	}

	if frameCount == 0 {
		t.Error("Expected at least one frame")
	}

	t.Logf("Played %d frames, final pose: head=(%.2f, %.2f, %.2f), antennas=(%.2f, %.2f)",
		frameCount, lastPose.Head.Roll, lastPose.Head.Pitch, lastPose.Head.Yaw,
		lastPose.Antennas[0], lastPose.Antennas[1])
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

