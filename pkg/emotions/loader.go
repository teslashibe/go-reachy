package emotions

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed data/*.json
var embeddedEmotions embed.FS

// LoadEmbedded loads an emotion from the embedded data.
func LoadEmbedded(name string) (*Emotion, error) {
	filename := fmt.Sprintf("data/%s.json", name)
	data, err := embeddedEmotions.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("emotion %q not found: %w", name, err)
	}

	return parseEmotionJSON(name, data, "")
}

// LoadFromFile loads an emotion from a JSON file on disk.
// This allows users to add custom emotions.
func LoadFromFile(path string) (*Emotion, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read emotion file: %w", err)
	}

	// Extract name from filename (without extension)
	name := strings.TrimSuffix(filepath.Base(path), ".json")

	// Check for associated sound file
	soundPath := strings.TrimSuffix(path, ".json") + ".wav"
	if _, err := os.Stat(soundPath); os.IsNotExist(err) {
		soundPath = ""
	}

	return parseEmotionJSON(name, data, soundPath)
}

// LoadFromDirectory loads all emotions from a directory.
// Useful for loading custom emotion packs.
func LoadFromDirectory(dir string) ([]*Emotion, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list emotion files: %w", err)
	}

	var emotions []*Emotion
	for _, file := range files {
		emotion, err := LoadFromFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", file, err)
		}
		emotions = append(emotions, emotion)
	}

	return emotions, nil
}

// ListEmbedded returns the names of all embedded emotions.
func ListEmbedded() ([]string, error) {
	entries, err := embeddedEmotions.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded emotions: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			names = append(names, name)
		}
	}

	return names, nil
}

// parseEmotionJSON parses JSON data into an Emotion.
func parseEmotionJSON(name string, data []byte, soundPath string) (*Emotion, error) {
	var raw EmotionData
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse emotion JSON: %w", err)
	}

	if len(raw.Time) == 0 || len(raw.SetTargetData) == 0 {
		return nil, fmt.Errorf("emotion %q has no keyframe data", name)
	}

	if len(raw.Time) != len(raw.SetTargetData) {
		return nil, fmt.Errorf("emotion %q has mismatched timestamps and keyframes", name)
	}

	// Calculate duration from timestamps
	duration := raw.Time[len(raw.Time)-1] - raw.Time[0]

	return &Emotion{
		Name:        name,
		Description: raw.Description,
		Duration:    time.Duration(duration * float64(time.Second)),
		Keyframes:   raw.SetTargetData,
		Timestamps:  raw.Time,
		HasSound:    soundPath != "",
		SoundPath:   soundPath,
	}, nil
}

// GetDescription returns the description for an embedded emotion without fully loading it.
func GetDescription(name string) (string, error) {
	emotion, err := LoadEmbedded(name)
	if err != nil {
		return "", err
	}
	return emotion.Description, nil
}

