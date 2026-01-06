// Package eva provides AI tool definitions and orchestration for the Eva robot assistant.
package eva

import (
	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/emotions"
	"github.com/teslashibe/go-reachy/pkg/memory"
	"github.com/teslashibe/go-reachy/pkg/robot"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/vision"
)

// BodyYawNotifier is called when body yaw changes (for tracker coordination).
type BodyYawNotifier interface {
	SetBodyYaw(yaw float64)
}

// TrackingController allows pausing/resuming tracking during animations.
type TrackingController interface {
	SetEnabled(enabled bool)
}

// MotionController provides rate-limited motion control.
// All motion commands should go through this interface to prevent
// HTTP racing with the RateController (Issue #139).
type MotionController interface {
	SetBaseHead(offset robot.Offset)
	SetAntennas(left, right float64)
	SetBodyYaw(yaw float64)
}

// ObjectDetector interface for YOLO-style object detection.
type ObjectDetector interface {
	Detect(jpeg []byte) ([]ObjectDetectionResult, error)
}

// ObjectDetectionResult represents a detected object.
type ObjectDetectionResult struct {
	ClassName  string
	Confidence float64
	X, Y, W, H float64 // Normalized 0-1
}

// Tool represents an AI tool that Eva can use.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     func(args map[string]interface{}) (string, error)
}

// ToolsConfig holds dependencies for Eva's tools.
type ToolsConfig struct {
	Robot              robot.Controller   // For non-motion ops (volume, status)
	Motion             MotionController   // For all motion (head, antennas, body) - Issue #139
	Memory             *memory.Memory
	Vision             vision.Provider
	ObjectDetector     ObjectDetector
	GoogleAPIKey       string
	AudioPlayer        *audio.Player
	Tracker            BodyYawNotifier
	TrackingController TrackingController // For pausing tracking during emotions
	Emotions           *emotions.Registry
	SparkStore         *spark.JSONStore        // Spark idea storage
	SparkGemini        *spark.GeminiClient     // Spark Gemini for title/tag generation
	SparkGoogleDocs    *spark.GoogleDocsClient // Spark Google Docs for syncing
}

// isAnimal returns true if the class name is an animal.
func isAnimal(className string) bool {
	animals := map[string]bool{
		"bird": true, "cat": true, "dog": true, "horse": true, "sheep": true,
		"cow": true, "elephant": true, "bear": true, "zebra": true, "giraffe": true,
	}
	return animals[className]
}
