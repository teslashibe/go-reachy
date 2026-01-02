package detection

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// TestYuNetNew tests detector initialization
func TestYuNetNew(t *testing.T) {
	// Find model path relative to test location
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("YuNet model not found, skipping test")
	}

	cfg := Config{
		ModelPath:        modelPath,
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	detector, err := NewYuNet(cfg)
	if err != nil {
		t.Fatalf("NewYuNet failed: %v", err)
	}
	defer detector.Close()

	// If we got here without error, detector is valid
	// FaceDetectorYN doesn't have an Empty() method like Net
}

// TestYuNetNewInvalidPath tests error handling for missing model
func TestYuNetNewInvalidPath(t *testing.T) {
	cfg := Config{
		ModelPath:        "/nonexistent/path/model.onnx",
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	_, err := NewYuNet(cfg)
	if err == nil {
		t.Error("Expected error for invalid model path")
	}
}

// TestYuNetDetect_EmptyImage tests detection on empty/invalid image
func TestYuNetDetect_EmptyImage(t *testing.T) {
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("YuNet model not found, skipping test")
	}

	cfg := Config{
		ModelPath:        modelPath,
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	detector, err := NewYuNet(cfg)
	if err != nil {
		t.Fatalf("NewYuNet failed: %v", err)
	}
	defer detector.Close()

	// Empty bytes should fail
	_, err = detector.Detect([]byte{})
	if err == nil {
		t.Error("Expected error for empty image")
	}

	// Invalid JPEG should fail
	_, err = detector.Detect([]byte("not a jpeg"))
	if err == nil {
		t.Error("Expected error for invalid JPEG")
	}
}

// TestYuNetDetect_SolidImage tests detection on solid color image (no faces)
func TestYuNetDetect_SolidImage(t *testing.T) {
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("YuNet model not found, skipping test")
	}

	cfg := Config{
		ModelPath:        modelPath,
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	detector, err := NewYuNet(cfg)
	if err != nil {
		t.Fatalf("NewYuNet failed: %v", err)
	}
	defer detector.Close()

	// Create solid blue image (no face)
	jpeg := createSolidJPEG(320, 240, color.RGBA{0, 0, 255, 255})

	detections, err := detector.Detect(jpeg)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should find no faces in solid color image
	if len(detections) > 0 {
		t.Errorf("Expected no detections in solid color image, got %d", len(detections))
	}
}

// TestYuNetClose tests proper resource cleanup
func TestYuNetClose(t *testing.T) {
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("YuNet model not found, skipping test")
	}

	cfg := Config{
		ModelPath:        modelPath,
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	detector, err := NewYuNet(cfg)
	if err != nil {
		t.Fatalf("NewYuNet failed: %v", err)
	}

	err = detector.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestYuNetConcurrency tests thread safety
func TestYuNetConcurrency(t *testing.T) {
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("YuNet model not found, skipping test")
	}

	cfg := Config{
		ModelPath:        modelPath,
		ConfidenceThresh: 0.5,
		InputWidth:       320,
		InputHeight:      320,
	}

	detector, err := NewYuNet(cfg)
	if err != nil {
		t.Fatalf("NewYuNet failed: %v", err)
	}
	defer detector.Close()

	jpeg := createSolidJPEG(320, 240, color.RGBA{100, 100, 100, 255})

	// Run concurrent detections
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := detector.Detect(jpeg)
			if err != nil {
				t.Errorf("Concurrent detection failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Helper functions

func findModelPath() string {
	// Try different relative paths from test location
	paths := []string{
		"../../../models/face_detection_yunet.onnx",
		"../../models/face_detection_yunet.onnx",
		"models/face_detection_yunet.onnx",
	}

	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	// Try from workspace root
	if cwd, err := os.Getwd(); err == nil {
		// Walk up to find models directory
		for dir := cwd; dir != "/"; dir = filepath.Dir(dir) {
			modelPath := filepath.Join(dir, "models", "face_detection_yunet.onnx")
			if _, err := os.Stat(modelPath); err == nil {
				return modelPath
			}
		}
	}

	return ""
}

func createSolidJPEG(width, height int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}
