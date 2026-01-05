// Package eva provides AI tool definitions and orchestration for the Eva robot assistant.
package eva

import (
	"os"

	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/emotions"
	"github.com/teslashibe/go-reachy/pkg/memory"
	"github.com/teslashibe/go-reachy/pkg/robot"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/tts"
	"github.com/teslashibe/go-reachy/pkg/vision"
)

// Default configuration values.
const (
	DefaultRobotIP = "192.168.68.77"
	DefaultSSHUser = "pollen"
	DefaultSSHPass = "root"
)

// Config holds all configuration for the Eva application.
// Flag parsing is done in cmd/eva/main.go; this struct is data only.
type Config struct {
	// Debug enables verbose debug logging.
	Debug bool

	// RobotIP is the IP address of the Reachy robot.
	RobotIP string

	// SSH credentials for robot access.
	SSHUser string
	SSHPass string

	// TTS configuration.
	TTSMode  string // "realtime", "elevenlabs", "elevenlabs-streaming", "openai-tts"
	TTSVoice string // Voice ID or preset name

	// Feature flags.
	SparkEnabled bool // Enable Spark idea collection
	NoBody       bool // Disable body rotation (head-only tracking)

	// API Keys (typically from environment variables).
	OpenAIKey     string
	ElevenLabsKey string
	GoogleAPIKey  string

	// Google OAuth (for Spark Google Docs integration).
	GoogleClientID     string
	GoogleClientSecret string
}

// DefaultConfig returns sensible defaults for Eva configuration.
func DefaultConfig() Config {
	return Config{
		RobotIP:      DefaultRobotIP,
		SSHUser:      DefaultSSHUser,
		SSHPass:      DefaultSSHPass,
		TTSMode:      "realtime",
		TTSVoice:     tts.DefaultElevenLabsVoice,
		SparkEnabled: true,
	}
}

// LoadEnvConfig loads configuration values from environment variables.
// Call this after flag parsing to apply environment overrides.
func (c *Config) LoadEnvConfig() {
	if ip := os.Getenv("ROBOT_IP"); ip != "" {
		c.RobotIP = ip
	}
	c.OpenAIKey = os.Getenv("OPENAI_API_KEY")
	c.ElevenLabsKey = os.Getenv("ELEVENLABS_API_KEY")
	c.GoogleAPIKey = os.Getenv("GOOGLE_API_KEY")
	c.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	c.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")

	// Voice can come from env if not set by flag
	if c.TTSVoice == "" || c.TTSVoice == tts.DefaultElevenLabsVoice {
		if voice := os.Getenv("ELEVENLABS_VOICE_ID"); voice != "" {
			c.TTSVoice = voice
		}
	}
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.OpenAIKey == "" {
		return &ConfigError{Field: "OpenAIKey", Message: "OPENAI_API_KEY environment variable is required"}
	}
	if (c.TTSMode == "elevenlabs" || c.TTSMode == "elevenlabs-streaming") && c.ElevenLabsKey == "" {
		return &ConfigError{Field: "ElevenLabsKey", Message: "ELEVENLABS_API_KEY environment variable is required for ElevenLabs TTS"}
	}
	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// BodyYawNotifier is called when body yaw changes (for tracker coordination).
type BodyYawNotifier interface {
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
	Robot           robot.Controller
	Memory          *memory.Memory
	Vision          vision.Provider
	ObjectDetector  ObjectDetector
	GoogleAPIKey    string
	AudioPlayer     *audio.Player
	Tracker         BodyYawNotifier
	Emotions        *emotions.Registry
	SparkStore      *spark.JSONStore        // Spark idea storage
	SparkGemini     *spark.GeminiClient     // Spark Gemini for title/tag generation
	SparkGoogleDocs *spark.GoogleDocsClient // Spark Google Docs for syncing
}

// isAnimal returns true if the class name is an animal.
func isAnimal(className string) bool {
	animals := map[string]bool{
		"bird": true, "cat": true, "dog": true, "horse": true, "sheep": true,
		"cow": true, "elephant": true, "bear": true, "zebra": true, "giraffe": true,
	}
	return animals[className]
}
