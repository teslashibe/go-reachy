package detection

import (
	"fmt"
	"image"
	"os"
	"sync"

	"github.com/teslashibe/go-reachy/pkg/debug"
	"gocv.io/x/gocv"
)

// YuNetDetector uses OpenCV's FaceDetectorYN for face detection
type YuNetDetector struct {
	detector gocv.FaceDetectorYN
	config   Config
	mu       sync.Mutex // Protects inference
}

// NewYuNet creates a new YuNet face detector using GoCV's built-in FaceDetectorYN
func NewYuNet(cfg Config) (*YuNetDetector, error) {
	// Check if model file exists first
	if _, err := os.Stat(cfg.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", cfg.ModelPath)
	}

	// Create FaceDetectorYN with initial size (will be updated per-image)
	detector := gocv.NewFaceDetectorYNWithParams(
		cfg.ModelPath,
		"",                                                      // No config file needed for ONNX
		image.Pt(cfg.InputWidth, cfg.InputHeight),               // Initial input size
		float32(cfg.ConfidenceThresh),                           // Score threshold
		0.3,                                                     // NMS threshold
		5000,                                                    // Top K
		int(gocv.NetBackendDefault),                             // Backend
		int(gocv.NetTargetCPU),                                  // Target
	)

	return &YuNetDetector{
		detector: detector,
		config:   cfg,
	}, nil
}

// Detect finds faces in the JPEG image
func (d *YuNetDetector) Detect(jpeg []byte) ([]Detection, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Decode JPEG to Mat
	img, err := gocv.IMDecode(jpeg, gocv.IMReadColor)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	defer img.Close()

	if img.Empty() {
		return nil, fmt.Errorf("empty image")
	}

	imgW := float64(img.Cols())
	imgH := float64(img.Rows())

	// Update detector input size to match image
	d.detector.SetInputSize(image.Pt(img.Cols(), img.Rows()))

	// Prepare output matrix for faces
	faces := gocv.NewMat()
	defer faces.Close()

	// Run detection
	d.detector.Detect(img, &faces)

	// Parse results
	var detections []Detection
	for r := 0; r < faces.Rows(); r++ {
		// YuNet output format (15 columns):
		// 0-3: x, y, w, h (bounding box in pixels)
		// 4-13: 5 facial landmarks (x,y pairs)
		// 14: face score
		x := float64(faces.GetFloatAt(r, 0))
		y := float64(faces.GetFloatAt(r, 1))
		w := float64(faces.GetFloatAt(r, 2))
		h := float64(faces.GetFloatAt(r, 3))
		score := float64(faces.GetFloatAt(r, 14))

		// Normalize to 0-1 range
		detections = append(detections, Detection{
			X:          x / imgW,
			Y:          y / imgH,
			W:          w / imgW,
			H:          h / imgH,
			Confidence: score,
		})
	}

	if len(detections) > 0 {
		debug.TrackLog("👁️  YuNet found %d face(s)\n", len(detections))
	}

	return detections, nil
}

// Close releases the detector resources
func (d *YuNetDetector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.detector.Close()
	return nil
}
