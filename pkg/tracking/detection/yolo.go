package detection

import (
	"fmt"
	"image"
	"os"
	"sync"

	"github.com/teslashibe/go-reachy/pkg/debug"
	"gocv.io/x/gocv"
)

// ObjectDetection represents a detected object with class info
type ObjectDetection struct {
	Detection
	ClassID   int     // COCO class ID
	ClassName string  // Human-readable class name
}

// YOLODetector uses YOLOv8 for general object detection
type YOLODetector struct {
	net       gocv.Net
	config    YOLOConfig
	mu        sync.Mutex
	inputSize image.Point
}

// YOLOConfig holds YOLO detector configuration
type YOLOConfig struct {
	ModelPath        string
	ConfidenceThresh float32
	NMSThresh        float32
	InputWidth       int
	InputHeight      int
}

// DefaultYOLOConfig returns production defaults for YOLOv8n
func DefaultYOLOConfig() YOLOConfig {
	return YOLOConfig{
		ModelPath:        "models/yolov8n.onnx",
		ConfidenceThresh: 0.5,
		NMSThresh:        0.45,
		InputWidth:       640,
		InputHeight:      640,
	}
}

// NewYOLO creates a new YOLO object detector
func NewYOLO(cfg YOLOConfig) (*YOLODetector, error) {
	// Check if model file exists
	if _, err := os.Stat(cfg.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", cfg.ModelPath)
	}

	// Load ONNX model
	net := gocv.ReadNetFromONNX(cfg.ModelPath)
	if net.Empty() {
		return nil, fmt.Errorf("failed to load YOLO model from %s", cfg.ModelPath)
	}

	// Set backend and target
	net.SetPreferableBackend(gocv.NetBackendDefault)
	net.SetPreferableTarget(gocv.NetTargetCPU)

	return &YOLODetector{
		net:       net,
		config:    cfg,
		inputSize: image.Pt(cfg.InputWidth, cfg.InputHeight),
	}, nil
}

// Detect finds objects in the JPEG image
func (d *YOLODetector) Detect(jpeg []byte) ([]ObjectDetection, error) {
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

	imgW := float32(img.Cols())
	imgH := float32(img.Rows())

	// Create blob from image
	blob := gocv.BlobFromImage(img, 1.0/255.0, d.inputSize, gocv.NewScalar(0, 0, 0, 0), true, false)
	defer blob.Close()

	// Set input
	d.net.SetInput(blob, "")

	// Forward pass
	output := d.net.Forward("")
	defer output.Close()

	// Parse YOLOv8 output
	// Output shape: [1, 84, 8400] - 84 = 4 bbox + 80 classes, 8400 detections
	detections := d.parseYOLOv8Output(output, imgW, imgH)

	if len(detections) > 0 {
		debug.Log("🔍 YOLO found %d object(s)\n", len(detections))
	}

	return detections, nil
}

// parseYOLOv8Output parses the YOLOv8 output tensor
func (d *YOLODetector) parseYOLOv8Output(output gocv.Mat, imgW, imgH float32) []ObjectDetection {
	var detections []ObjectDetection
	var boxes []image.Rectangle
	var confidences []float32
	var classIDs []int

	// YOLOv8 output: [1, 84, 8400] - need to transpose to [1, 8400, 84]
	// 84 = 4 (x, y, w, h) + 80 (class scores)
	rows := output.Cols() // 8400 detections
	cols := output.Rows() // 84 (4 bbox + 80 classes)

	// Get data pointer
	data, err := output.DataPtrFloat32()
	if err != nil {
		return nil
	}

	for i := 0; i < rows; i++ {
		// Get class scores (starting at index 4)
		maxScore := float32(0)
		maxClassID := 0

		for c := 4; c < cols; c++ {
			score := data[c*rows+i]
			if score > maxScore {
				maxScore = score
				maxClassID = c - 4
			}
		}

		if maxScore < d.config.ConfidenceThresh {
			continue
		}

		// Get bounding box (center x, center y, width, height)
		cx := data[0*rows+i]
		cy := data[1*rows+i]
		w := data[2*rows+i]
		h := data[3*rows+i]

		// Convert to corner format and scale to image size
		x1 := int((cx - w/2) * imgW / float32(d.config.InputWidth))
		y1 := int((cy - h/2) * imgH / float32(d.config.InputHeight))
		x2 := int((cx + w/2) * imgW / float32(d.config.InputWidth))
		y2 := int((cy + h/2) * imgH / float32(d.config.InputHeight))

		boxes = append(boxes, image.Rect(x1, y1, x2, y2))
		confidences = append(confidences, maxScore)
		classIDs = append(classIDs, maxClassID)
	}

	// Return early if no detections
	if len(boxes) == 0 {
		return detections
	}

	// Apply NMS
	indices := gocv.NMSBoxes(boxes, confidences, d.config.ConfidenceThresh, d.config.NMSThresh)

	for _, idx := range indices {
		box := boxes[idx]
		detections = append(detections, ObjectDetection{
			Detection: Detection{
				X:          float64(box.Min.X) / float64(imgW),
				Y:          float64(box.Min.Y) / float64(imgH),
				W:          float64(box.Dx()) / float64(imgW),
				H:          float64(box.Dy()) / float64(imgH),
				Confidence: float64(confidences[idx]),
			},
			ClassID:   classIDs[idx],
			ClassName: COCOClasses[classIDs[idx]],
		})
	}

	return detections
}

// DetectClass finds objects of a specific class
func (d *YOLODetector) DetectClass(jpeg []byte, targetClass string) ([]ObjectDetection, error) {
	all, err := d.Detect(jpeg)
	if err != nil {
		return nil, err
	}

	var filtered []ObjectDetection
	for _, det := range all {
		if det.ClassName == targetClass {
			filtered = append(filtered, det)
		}
	}
	return filtered, nil
}

// Close releases the detector resources
func (d *YOLODetector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.net.Close()
	return nil
}

// COCOClasses contains the 80 COCO class names
var COCOClasses = []string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
	"traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat",
	"dog", "horse", "sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack",
	"umbrella", "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard", "sports ball",
	"kite", "baseball bat", "baseball glove", "skateboard", "surfboard", "tennis racket",
	"bottle", "wine glass", "cup", "fork", "knife", "spoon", "bowl", "banana", "apple",
	"sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair",
	"couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
	"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink", "refrigerator",
	"book", "clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
}

// IsAnimal returns true if the class is an animal
func IsAnimal(className string) bool {
	animals := map[string]bool{
		"bird": true, "cat": true, "dog": true, "horse": true, "sheep": true,
		"cow": true, "elephant": true, "bear": true, "zebra": true, "giraffe": true,
	}
	return animals[className]
}

// IsPerson returns true if the class is a person
func IsPerson(className string) bool {
	return className == "person"
}

