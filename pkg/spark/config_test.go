package spark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if !cfg.AutoSync {
		t.Error("expected AutoSync to be true by default")
	}
	if !cfg.PlanningEnabled {
		t.Error("expected PlanningEnabled to be true by default")
	}
	if cfg.GeminiModel != DefaultGeminiModel {
		t.Errorf("expected GeminiModel to be %s, got %s", DefaultGeminiModel, cfg.GeminiModel)
	}
}

func TestLoadConfigWithOverrides(t *testing.T) {
	// Test environment variable override
	t.Run("env var override", func(t *testing.T) {
		os.Setenv("SPARK_ENABLED", "false")
		defer os.Unsetenv("SPARK_ENABLED")

		cfg := LoadConfigWithOverrides(nil, nil)
		if cfg.Enabled {
			t.Error("expected Enabled to be false from env var")
		}
	})

	// Test CLI flag override (highest priority)
	t.Run("CLI flag override", func(t *testing.T) {
		os.Setenv("SPARK_ENABLED", "false")
		defer os.Unsetenv("SPARK_ENABLED")

		enabled := true
		cfg := LoadConfigWithOverrides(nil, &enabled)
		if !cfg.Enabled {
			t.Error("expected CLI flag to override env var")
		}
	})

	// Test Gemini model from env var
	t.Run("GEMINI_MODEL env var", func(t *testing.T) {
		os.Setenv("GEMINI_MODEL", "gemini-1.5-pro")
		defer os.Unsetenv("GEMINI_MODEL")

		cfg := LoadConfigWithOverrides(nil, nil)
		if cfg.GeminiModel != "gemini-1.5-pro" {
			t.Errorf("expected GeminiModel to be gemini-1.5-pro, got %s", cfg.GeminiModel)
		}
	})
}

func TestLoadConfigFromFile(t *testing.T) {
	// Create a temp directory for test config
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create .eva directory
	evaDir := filepath.Join(tempDir, ".eva")
	if err := os.MkdirAll(evaDir, 0755); err != nil {
		t.Fatalf("failed to create .eva dir: %v", err)
	}

	// Write test config
	testConfig := EvaConfig{
		Spark: Config{
			Enabled:         false,
			AutoSync:        false,
			PlanningEnabled: true,
			GeminiModel:     "gemini-1.5-flash",
		},
	}
	data, _ := json.Marshal(testConfig)
	configPath := filepath.Join(evaDir, "config.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load and verify
	cfg := LoadConfig()
	if cfg.Enabled {
		t.Error("expected Enabled to be false from file")
	}
	if cfg.AutoSync {
		t.Error("expected AutoSync to be false from file")
	}
	if cfg.GeminiModel != "gemini-1.5-flash" {
		t.Errorf("expected GeminiModel to be gemini-1.5-flash, got %s", cfg.GeminiModel)
	}
}

func TestSaveConfig(t *testing.T) {
	// Create a temp directory for test config
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Save config
	cfg := Config{
		Enabled:         true,
		AutoSync:        false,
		PlanningEnabled: true,
		GeminiModel:     "gemini-2.0-flash",
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tempDir, ".eva", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("expected config file to exist")
	}

	// Load and verify
	loaded := LoadConfig()
	if loaded.GeminiModel != "gemini-2.0-flash" {
		t.Errorf("expected GeminiModel to be gemini-2.0-flash, got %s", loaded.GeminiModel)
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath()
	if path == "" {
		t.Error("expected ConfigPath to return non-empty path")
	}
	if filepath.Base(path) != "config.json" {
		t.Errorf("expected config.json, got %s", filepath.Base(path))
	}
}

