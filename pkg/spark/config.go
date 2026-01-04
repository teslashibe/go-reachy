package spark

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all Spark configuration.
type Config struct {
	// Enabled controls whether Spark is active
	Enabled bool `json:"enabled"`

	// AutoSync automatically syncs sparks to Google Docs when connected
	AutoSync bool `json:"auto_sync"`

	// PlanningEnabled allows AI plan generation
	PlanningEnabled bool `json:"planning_enabled"`

	// GeminiModel is the Gemini model to use (e.g., "gemini-2.0-flash")
	GeminiModel string `json:"gemini_model,omitempty"`
}

// DefaultConfig returns the default Spark configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:         true,
		AutoSync:        true,
		PlanningEnabled: true,
		GeminiModel:     DefaultGeminiModel,
	}
}

// EvaConfig is the root configuration file structure.
type EvaConfig struct {
	Spark Config `json:"spark"`
}

// LoadConfig loads Spark configuration from ~/.eva/config.json.
// Returns defaults if file doesn't exist.
func LoadConfig() Config {
	cfg := DefaultConfig()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	configPath := filepath.Join(homeDir, ".eva", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// File doesn't exist, return defaults
		return cfg
	}

	var evaConfig EvaConfig
	if err := json.Unmarshal(data, &evaConfig); err != nil {
		// Invalid JSON, return defaults
		return cfg
	}

	// Merge loaded config with defaults (only override non-zero values)
	if evaConfig.Spark.GeminiModel != "" {
		cfg.GeminiModel = evaConfig.Spark.GeminiModel
	}

	// Boolean fields are tricky - we check if the whole spark section was present
	// by checking if GeminiModel was set (since it defaults to empty in JSON)
	// For a cleaner approach, we just use the loaded values directly
	cfg.Enabled = evaConfig.Spark.Enabled
	cfg.AutoSync = evaConfig.Spark.AutoSync
	cfg.PlanningEnabled = evaConfig.Spark.PlanningEnabled

	return cfg
}

// LoadConfigWithOverrides loads config and applies environment/CLI overrides.
// Priority (highest to lowest): CLI flags > Environment variables > Config file > Defaults
func LoadConfigWithOverrides(envEnabled, cliEnabled *bool) Config {
	cfg := LoadConfig()

	// Check environment variable
	if envVal := os.Getenv("SPARK_ENABLED"); envVal != "" {
		cfg.Enabled = envVal != "false" && envVal != "0"
	}

	// Check environment variable for Gemini model
	if model := os.Getenv("GEMINI_MODEL"); model != "" {
		cfg.GeminiModel = model
	}

	// CLI flag has highest priority
	if cliEnabled != nil {
		cfg.Enabled = *cliEnabled
	}

	return cfg
}

// SaveConfig saves the configuration to ~/.eva/config.json.
func SaveConfig(cfg Config) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(homeDir, ".eva")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.json")

	// Load existing config to preserve other settings
	var evaConfig EvaConfig
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &evaConfig)
	}

	evaConfig.Spark = cfg

	data, err := json.MarshalIndent(evaConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// ConfigPath returns the path to the Eva config file.
func ConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".eva", "config.json")
}

