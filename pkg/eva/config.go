// Package eva provides AI tool definitions and orchestration for the Eva robot assistant.
package eva

import (
	"os"
	"time"

	"github.com/teslashibe/go-reachy/pkg/audio"
	"github.com/teslashibe/go-reachy/pkg/emotions"
	"github.com/teslashibe/go-reachy/pkg/memory"
	"github.com/teslashibe/go-reachy/pkg/robot"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/voice"
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

	// Voice pipeline configuration (ElevenLabs)
	VoiceLLM        string        // LLM model (default: gpt-5-mini)
	VoiceTTS        string        // TTS model (default: eleven_flash_v2)
	VoiceSTT        string        // STT model (default: scribe_v2_realtime)
	VoiceChunk      time.Duration // Audio chunk duration (default: 50ms)
	VoiceVADMode    string        // VAD mode (default: server_vad)
	VoiceVADSilence time.Duration // VAD silence duration (default: 500ms)

	// ElevenLabs credentials
	ElevenLabsVoiceID string

	// Feature flags.
	SparkEnabled bool // Enable Spark idea collection
	NoBody       bool // Disable body rotation (head-only tracking)

	// API Keys (from environment variables).
	ElevenLabsKey string

	// Google OAuth (for Spark Google Docs integration).
	GoogleAPIKey       string
	GoogleClientID     string
	GoogleClientSecret string
}

// DefaultConfig returns sensible defaults for Eva configuration.
func DefaultConfig() Config {
	return Config{
		RobotIP:         DefaultRobotIP,
		SSHUser:         DefaultSSHUser,
		SSHPass:         DefaultSSHPass,
		VoiceLLM:        voice.LLMGpt5Mini,
		VoiceTTS:        voice.TTSFlash,
		VoiceSTT:        voice.STTRealtime,
		VoiceChunk:      50 * time.Millisecond,
		VoiceVADMode:    "server_vad",
		VoiceVADSilence: 500 * time.Millisecond,
		SparkEnabled:    true,
	}
}

// LoadEnvConfig loads configuration values from environment variables.
// Call this after flag parsing to apply environment overrides.
func (c *Config) LoadEnvConfig() {
	if ip := os.Getenv("ROBOT_IP"); ip != "" {
		c.RobotIP = ip
	}
	c.ElevenLabsKey = os.Getenv("ELEVENLABS_API_KEY")
	c.GoogleAPIKey = os.Getenv("GOOGLE_API_KEY")
	c.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	c.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")

	// Voice ID from env if not set by flag
	if c.ElevenLabsVoiceID == "" {
		c.ElevenLabsVoiceID = os.Getenv("ELEVENLABS_VOICE_ID")
	}
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.ElevenLabsKey == "" {
		return &ConfigError{Field: "ElevenLabsKey", Message: "ELEVENLABS_API_KEY environment variable is required"}
	}
	if c.ElevenLabsVoiceID == "" {
		return &ConfigError{Field: "ElevenLabsVoiceID", Message: "ELEVENLABS_VOICE_ID is required"}
	}
	return nil
}

// ToVoiceConfig converts Eva config to voice pipeline config.
func (c *Config) ToVoiceConfig() voice.Config {
	return voice.Config{
		ElevenLabsKey:      c.ElevenLabsKey,
		ElevenLabsVoiceID:  c.ElevenLabsVoiceID,
		LLMModel:           c.VoiceLLM,
		TTSModel:           c.VoiceTTS,
		STTModel:           c.VoiceSTT,
		ChunkDuration:      c.VoiceChunk,
		VADMode:            c.VoiceVADMode,
		VADSilenceDuration: c.VoiceVADSilence,
		Debug:              c.Debug,
	}
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
