package realtime

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// RobotController interface for robot control
type RobotController interface {
	SetHeadPose(roll, pitch, yaw float64) error
	SetAntennas(left, right float64) error
	GetDaemonStatus() (string, error)
	SetVolume(level int) error
}

// VisionProvider interface for camera access
type VisionProvider interface {
	CaptureFrame() ([]byte, error) // Returns JPEG image data
}

// GeminiVision calls Gemini Flash to describe an image
func GeminiVision(apiKey string, imageData []byte, prompt string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("GOOGLE_API_KEY not set")
	}

	// Encode image as base64
	b64Image := base64.StdEncoding.EncodeToString(imageData)

	// Build Gemini API request (matching format from working explore code)
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
					{"inline_data": map[string]string{"mime_type": "image/jpeg", "data": b64Image}},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 500, // Increased for better scene descriptions
		},
	}

	jsonData, _ := json.Marshal(payload)

	// Using Gemini 3 Flash - latest model from https://deepmind.google/models/gemini/flash/
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash-preview:generateContent?key=%s", apiKey)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read full response for debugging
	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w (body: %s)", err, string(bodyBytes[:min(200, len(bodyBytes))]))
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("Gemini error: %s", result.Error.Message)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no response from Gemini (raw: %s)", string(bodyBytes[:min(300, len(bodyBytes))]))
}

// WebSearch uses Gemini with Google Search grounding to search the web
func WebSearch(apiKey string, query string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("GOOGLE_API_KEY not set")
	}

	// Build request with search grounding enabled using dynamic retrieval
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": query},
				},
			},
		},
		"tools": []map[string]interface{}{
			{
				"google_search": map[string]interface{}{},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.2, // Lower temp for factual responses
			"maxOutputTokens": 300,
		},
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": "You are a helpful assistant that searches the web for real-time information. Always use Google Search to find current, accurate information. Provide specific details like prices, times, dates, and links when available. Be concise but informative."},
			},
		},
	}

	jsonData, _ := json.Marshal(payload)

	// Use Gemini 2.0 Flash with grounding
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		// Try to parse error
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(bodyBytes, &errResp)
		if errResp.Error.Message != "" {
			return "", fmt.Errorf("Gemini error: %s", errResp.Error.Message)
		}
		return "", fmt.Errorf("Gemini API error (status %d)", resp.StatusCode)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			GroundingMetadata struct {
				WebSearchQueries []string `json:"webSearchQueries"`
				SearchEntryPoint struct {
					RenderedContent string `json:"renderedContent"`
				} `json:"searchEntryPoint"`
				GroundingChunks []struct {
					Web struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web"`
				} `json:"groundingChunks"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		response := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)

		// Add source links if available
		metadata := result.Candidates[0].GroundingMetadata
		if len(metadata.GroundingChunks) > 0 {
			response += "\n\nSources: "
			for i, chunk := range metadata.GroundingChunks {
				if i > 2 {
					break // Limit to 3 sources
				}
				if chunk.Web.Title != "" {
					response += fmt.Sprintf("%s (%s), ", chunk.Web.Title, chunk.Web.URI)
				}
			}
		}

		return response, nil
	}

	return "", fmt.Errorf("no search results")
}

// EvaToolsConfig holds dependencies for Eva's tools
type EvaToolsConfig struct {
	Robot        RobotController
	Memory       *Memory
	Vision       VisionProvider
	GoogleAPIKey string
	AudioPlayer  *AudioPlayer // For timer announcements
}

// EvaTools returns all tools available to Eva
func EvaTools(cfg EvaToolsConfig) []Tool {
	robot := cfg.Robot
	memory := cfg.Memory
	return []Tool{
		{
			Name:        "move_head",
			Description: "Move Eva's head to look in a direction. Use this when you want to look at something or someone.",
			Parameters: map[string]interface{}{
				"direction": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"left", "right", "up", "down", "center"},
					"description": "Direction to look",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				dir, _ := args["direction"].(string)
				var roll, pitch, yaw float64

				switch dir {
				case "left":
					yaw = 0.4
				case "right":
					yaw = -0.4
				case "up":
					pitch = 0.3
				case "down":
					pitch = -0.3
				case "center":
					// All zero
				}

				if robot != nil {
					robot.SetHeadPose(roll, pitch, yaw)
				}
				return fmt.Sprintf("Looking %s", dir), nil
			},
		},
		{
			Name:        "express_emotion",
			Description: "Express an emotion through antenna movements and head gestures. Use this to show how you feel.",
			Parameters: map[string]interface{}{
				"emotion": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"happy", "curious", "excited", "confused", "sad", "surprised"},
					"description": "The emotion to express",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				emotion, _ := args["emotion"].(string)

				if robot != nil {
					switch emotion {
					case "happy":
						// Wiggle antennas happily
						for i := 0; i < 3; i++ {
							robot.SetAntennas(0.3, -0.3)
							time.Sleep(100 * time.Millisecond)
							robot.SetAntennas(-0.3, 0.3)
							time.Sleep(100 * time.Millisecond)
						}
						robot.SetAntennas(0, 0)
					case "curious":
						// Tilt head, one antenna up
						robot.SetHeadPose(0, 0.1, 0.2)
						robot.SetAntennas(0.3, 0)
					case "excited":
						// Fast antenna wiggle
						for i := 0; i < 5; i++ {
							robot.SetAntennas(0.5, -0.5)
							time.Sleep(80 * time.Millisecond)
							robot.SetAntennas(-0.5, 0.5)
							time.Sleep(80 * time.Millisecond)
						}
						robot.SetAntennas(0, 0)
					case "confused":
						// Tilt head, lower antennas
						robot.SetHeadPose(0.2, 0, 0)
						robot.SetAntennas(-0.2, -0.2)
					case "sad":
						// Lower head and antennas
						robot.SetHeadPose(0, -0.2, 0)
						robot.SetAntennas(-0.4, -0.4)
					case "surprised":
						// Quick head back, antennas up
						robot.SetHeadPose(0, 0.15, 0)
						robot.SetAntennas(0.5, 0.5)
					}
				}

				return fmt.Sprintf("Expressing %s", emotion), nil
			},
		},
		{
			Name:        "wave_hello",
			Description: "Wave your antennas to greet someone friendly.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if robot != nil {
					for i := 0; i < 3; i++ {
						robot.SetAntennas(0.4, 0)
						time.Sleep(150 * time.Millisecond)
						robot.SetAntennas(0, 0.4)
						time.Sleep(150 * time.Millisecond)
					}
					robot.SetAntennas(0, 0)
				}
				return "Waved hello with antennas", nil
			},
		},
		{
			Name:        "get_time",
			Description: "Get the current time and date. Use when someone asks what time it is or what day it is.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				now := time.Now()
				return now.Format("It's Monday, January 2 at 3:04 PM"), nil
			},
		},
		{
			Name:        "set_timer",
			Description: "Set a timer for a specified duration. Use when someone asks to set a timer or reminder.",
			Parameters: map[string]interface{}{
				"duration": map[string]interface{}{
					"type":        "integer",
					"description": "Number of minutes or seconds",
				},
				"unit": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"seconds", "minutes"},
					"description": "Time unit: seconds or minutes",
				},
				"label": map[string]interface{}{
					"type":        "string",
					"description": "Optional label for the timer like 'pasta' or 'meeting'",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				duration := 1
				if d, ok := args["duration"].(float64); ok {
					duration = int(d)
				}

				unit := "minutes"
				if u, ok := args["unit"].(string); ok && u != "" {
					unit = u
				}

				label, _ := args["label"].(string)

				// Calculate wait duration
				var wait time.Duration
				if unit == "seconds" {
					wait = time.Duration(duration) * time.Second
				} else {
					wait = time.Duration(duration) * time.Minute
				}

				fmt.Printf("⏱️  Timer set: %d %s (label: %s)\n", duration, unit, label)

				// Spawn timer goroutine
				go func() {
					time.Sleep(wait)

					// Announce timer done
					msg := "Timer done!"
					if label != "" {
						msg = fmt.Sprintf("Your %s timer is done!", label)
					}

					fmt.Printf("🔔 Timer finished: %s\n", msg)

					if cfg.AudioPlayer != nil {
						cfg.AudioPlayer.SpeakText(msg)
					}
				}()

				if label != "" {
					return fmt.Sprintf("Timer set for %d %s - I'll let you know when your %s is ready!", duration, unit, label), nil
				}
				return fmt.Sprintf("Timer set for %d %s!", duration, unit), nil
			},
		},
		{
			Name:        "remember_person",
			Description: "Remember something about a person you're talking to. Use this to store facts about people.",
			Parameters: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The person's name",
				},
				"fact": map[string]interface{}{
					"type":        "string",
					"description": "A fact to remember about them",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				name, _ := args["name"].(string)
				fact, _ := args["fact"].(string)

				if memory != nil && name != "" && fact != "" {
					memory.RememberPerson(name, fact)
					return fmt.Sprintf("Remembered that %s: %s", name, fact), nil
				}
				return "Noted", nil
			},
		},
		{
			Name:        "recall_person",
			Description: "Recall what you know about a person.",
			Parameters: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The person's name to recall",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				name, _ := args["name"].(string)

				if memory != nil && name != "" {
					facts := memory.RecallPerson(name)
					if len(facts) > 0 {
						return fmt.Sprintf("About %s: %s", name, strings.Join(facts, "; ")), nil
					}
					return fmt.Sprintf("I don't know anything about %s yet", name), nil
				}
				return "No memory available", nil
			},
		},
		{
			Name:        "look_around",
			Description: "Look around the room to see who or what is there.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if robot != nil {
					// Look left
					robot.SetHeadPose(0, 0, 0.4)
					time.Sleep(500 * time.Millisecond)
					// Look right
					robot.SetHeadPose(0, 0, -0.4)
					time.Sleep(500 * time.Millisecond)
					// Center
					robot.SetHeadPose(0, 0, 0)
				}
				return "Looked around the room", nil
			},
		},
		{
			Name:        "nod_yes",
			Description: "Nod your head to agree with something.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if robot != nil {
					for i := 0; i < 2; i++ {
						robot.SetHeadPose(0, 0.15, 0)
						time.Sleep(200 * time.Millisecond)
						robot.SetHeadPose(0, -0.1, 0)
						time.Sleep(200 * time.Millisecond)
					}
					robot.SetHeadPose(0, 0, 0)
				}
				return "Nodded yes", nil
			},
		},
		{
			Name:        "shake_head_no",
			Description: "Shake your head to disagree with something.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if robot != nil {
					for i := 0; i < 2; i++ {
						robot.SetHeadPose(0, 0, 0.2)
						time.Sleep(200 * time.Millisecond)
						robot.SetHeadPose(0, 0, -0.2)
						time.Sleep(200 * time.Millisecond)
					}
					robot.SetHeadPose(0, 0, 0)
				}
				return "Shook head no", nil
			},
		},
		{
			Name:        "set_volume",
			Description: "Adjust your speaker volume. Use this if someone asks you to speak louder or quieter.",
			Parameters: map[string]interface{}{
				"level": map[string]interface{}{
					"type":        "integer",
					"description": "Volume level from 0 (silent) to 100 (maximum)",
					"minimum":     0,
					"maximum":     100,
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				level := 100 // default to max
				if l, ok := args["level"].(float64); ok {
					level = int(l)
				}
				if robot != nil {
					robot.SetVolume(level)
				}
				return fmt.Sprintf("Volume set to %d%%", level), nil
			},
		},
		{
			Name:        "describe_scene",
			Description: "Look through your camera and describe what you see. Use this when someone asks what you can see, who is in the room, or to look for something.",
			Parameters: map[string]interface{}{
				"focus": map[string]interface{}{
					"type":        "string",
					"description": "What to focus on: 'general' for overall scene, 'people' to look for people, or a specific thing to look for",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				focus, _ := args["focus"].(string)
				if focus == "" {
					focus = "general"
				}

				fmt.Printf("👁️  describe_scene called (focus: %s)\n", focus)

				if cfg.Vision == nil {
					fmt.Println("👁️  Error: Vision provider is nil")
					return "I cannot see right now - camera not connected", nil
				}

				if cfg.GoogleAPIKey == "" {
					fmt.Println("👁️  Error: Google API key not set")
					return "I cannot see right now - vision not configured", nil
				}

				// Capture frame
				fmt.Println("👁️  Capturing frame...")
				imageData, err := cfg.Vision.CaptureFrame()
				if err != nil {
					fmt.Printf("👁️  Frame capture error: %v\n", err)
					return fmt.Sprintf("Could not capture image: %v", err), nil
				}
				fmt.Printf("👁️  Captured %d bytes\n", len(imageData))

				// Build prompt based on focus
				var prompt string
				switch focus {
				case "people":
					prompt = "Describe any people you see in this image. How many people are there? What are they doing? Where are they positioned (left, center, right)? Be concise."
				case "general":
					prompt = "Briefly describe what you see in this image. Mention the setting, any people, and notable objects. Keep it to 2-3 sentences."
				default:
					prompt = fmt.Sprintf("Look at this image and tell me if you can see: %s. Describe what you find. Be concise.", focus)
				}

				// Call Gemini
				fmt.Println("👁️  Calling Gemini Flash...")
				description, err := GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
				if err != nil {
					fmt.Printf("👁️  Gemini error: %v\n", err)
					return fmt.Sprintf("Vision error: %v", err), nil
				}
				fmt.Printf("👁️  Gemini response: %s\n", description)

				return description, nil
			},
		},
		{
			Name:        "web_search",
			Description: "Search the internet for general information. Use for news, facts, weather, products, etc.",
			Parameters: map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The search query to look up",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				query, _ := args["query"].(string)
				if query == "" {
					return "I need a search query to look up", nil
				}

				fmt.Printf("🌐 web_search called (query: %s)\n", query)

				if cfg.GoogleAPIKey == "" {
					return "Web search not configured", nil
				}

				result, err := WebSearch(cfg.GoogleAPIKey, query)
				if err != nil {
					fmt.Printf("🌐 Search error: %v\n", err)
					return fmt.Sprintf("Search failed: %v", err), nil
				}

				fmt.Printf("🌐 Search result: %s\n", result)
				return result, nil
			},
		},
		{
			Name:        "search_flights",
			Description: "Search for real flight prices and availability. Use this when someone asks about flights, travel, or booking.",
			Parameters: map[string]interface{}{
				"origin": map[string]interface{}{
					"type":        "string",
					"description": "Origin city or airport code (e.g., 'San Francisco' or 'SFO')",
				},
				"destination": map[string]interface{}{
					"type":        "string",
					"description": "Destination city or airport code (e.g., 'Los Angeles' or 'LAX')",
				},
				"date": map[string]interface{}{
					"type":        "string",
					"description": "Travel date (e.g., 'January 6, 2025' or '2025-01-06')",
				},
				"cabin_class": map[string]interface{}{
					"type":        "string",
					"description": "Cabin class: economy, business, or first",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				origin, _ := args["origin"].(string)
				destination, _ := args["destination"].(string)
				date, _ := args["date"].(string)
				cabinClass, _ := args["cabin_class"].(string)
				if cabinClass == "" {
					cabinClass = "economy"
				}

				fmt.Printf("✈️  search_flights called (from: %s, to: %s, date: %s, class: %s)\n",
					origin, destination, date, cabinClass)

				if cfg.GoogleAPIKey == "" {
					return "Flight search not configured", nil
				}

				// Use Gemini with detailed flight search query
				query := fmt.Sprintf(
					"Find specific flights from %s to %s on %s in %s class. "+
						"I need: airline names, departure times, arrival times, flight numbers, and prices in USD. "+
						"Search Google Flights or airline websites for real current availability.",
					origin, destination, date, cabinClass)

				result, err := WebSearch(cfg.GoogleAPIKey, query)
				if err != nil {
					return fmt.Sprintf("Flight search failed: %v", err), nil
				}

				fmt.Printf("✈️  Flight search result: %s\n", result)
				return result, nil
			},
		},
		{
			Name:        "find_person",
			Description: "Look for a specific person in the room by name or description.",
			Parameters: map[string]interface{}{
				"person": map[string]interface{}{
					"type":        "string",
					"description": "Name or description of the person to find",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				person, _ := args["person"].(string)
				if person == "" {
					person = "anyone"
				}

				if cfg.Vision == nil || cfg.GoogleAPIKey == "" {
					return "I cannot see right now", nil
				}

				// Look around first
				if robot != nil {
					// Look left
					robot.SetHeadPose(0, 0, 0.3)
					time.Sleep(400 * time.Millisecond)
				}

				// Capture and check left
				imageData, err := cfg.Vision.CaptureFrame()
				if err == nil {
					prompt := fmt.Sprintf("Is there a person in this image who might be %s? Answer briefly.", person)
					desc, _ := GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
					if strings.Contains(strings.ToLower(desc), "yes") {
						return fmt.Sprintf("I see someone on my left who might be %s. %s", person, desc), nil
					}
				}

				// Look right
				if robot != nil {
					robot.SetHeadPose(0, 0, -0.3)
					time.Sleep(400 * time.Millisecond)
				}

				imageData, err = cfg.Vision.CaptureFrame()
				if err == nil {
					prompt := fmt.Sprintf("Is there a person in this image who might be %s? Answer briefly.", person)
					desc, _ := GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
					if strings.Contains(strings.ToLower(desc), "yes") {
						return fmt.Sprintf("I see someone on my right who might be %s. %s", person, desc), nil
					}
				}

				// Look center
				if robot != nil {
					robot.SetHeadPose(0, 0, 0)
					time.Sleep(300 * time.Millisecond)
				}

				imageData, err = cfg.Vision.CaptureFrame()
				if err == nil {
					prompt := fmt.Sprintf("Is there a person in this image who might be %s? Answer briefly.", person)
					desc, _ := GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
					if strings.Contains(strings.ToLower(desc), "yes") {
						return fmt.Sprintf("I see someone in front of me who might be %s. %s", person, desc), nil
					}
				}

				return fmt.Sprintf("I looked around but I don't see %s right now.", person), nil
			},
		},
	}
}

// Memory stores information about people and conversations
type Memory struct {
	People   map[string]*PersonMemory `json:"people"`
	FilePath string                   `json:"-"`
}

// PersonMemory stores facts about a person
type PersonMemory struct {
	Name     string    `json:"name"`
	Facts    []string  `json:"facts"`
	LastSeen time.Time `json:"last_seen"`
}

// NewMemory creates a new memory store
func NewMemory() *Memory {
	return &Memory{
		People: make(map[string]*PersonMemory),
	}
}

// NewMemoryWithFile creates a memory store that persists to a file
func NewMemoryWithFile(filePath string) *Memory {
	m := &Memory{
		People:   make(map[string]*PersonMemory),
		FilePath: filePath,
	}
	m.Load() // Load existing memory if file exists
	return m
}

// Save persists memory to file
func (m *Memory) Save() error {
	if m.FilePath == "" {
		return nil
	}

	// Ensure directory exists
	dir := m.FilePath[:strings.LastIndex(m.FilePath, "/")]
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.FilePath, data, 0644)
}

// Load reads memory from file
func (m *Memory) Load() error {
	if m.FilePath == "" {
		return nil
	}

	data, err := os.ReadFile(m.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's OK
		}
		return err
	}

	return json.Unmarshal(data, m)
}

// RememberPerson stores a fact about a person and auto-saves
func (m *Memory) RememberPerson(name, fact string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}

	if _, ok := m.People[name]; !ok {
		m.People[name] = &PersonMemory{
			Name:  name,
			Facts: []string{},
		}
	}

	m.People[name].Facts = append(m.People[name].Facts, fact)
	m.People[name].LastSeen = time.Now()

	// Auto-save to file
	m.Save()
}

// RecallPerson retrieves facts about a person
func (m *Memory) RecallPerson(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if person, ok := m.People[name]; ok {
		person.LastSeen = time.Now()
		return person.Facts
	}
	return nil
}

// GetAllPeople returns names of all known people
func (m *Memory) GetAllPeople() []string {
	names := make([]string, 0, len(m.People))
	for name := range m.People {
		names = append(names, name)
	}
	return names
}

// ToJSON serializes memory to JSON
func (m *Memory) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserializes memory from JSON
func (m *Memory) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}

// SimpleRobotController implements RobotController using HTTP API
type SimpleRobotController struct {
	BaseURL string
}

// NewSimpleRobotController creates a new robot controller
func NewSimpleRobotController(robotIP string) *SimpleRobotController {
	return &SimpleRobotController{
		BaseURL: fmt.Sprintf("http://%s:8000", robotIP),
	}
}

// SetHeadPose sets the robot's head position
func (r *SimpleRobotController) SetHeadPose(roll, pitch, yaw float64) error {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{
			"roll":  roll,
			"pitch": pitch,
			"yaw":   yaw,
		},
		"target_antennas": []float64{0, 0},
		"duration":        0.3,
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post(r.BaseURL+"/api/move/set_target", "application/json", strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SetAntennas sets the robot's antenna positions
func (r *SimpleRobotController) SetAntennas(left, right float64) error {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{
			"roll":  0,
			"pitch": 0,
			"yaw":   0,
		},
		"target_antennas": []float64{left, right},
		"duration":        0.15,
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post(r.BaseURL+"/api/move/set_target", "application/json", strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetDaemonStatus returns the robot daemon status
func (r *SimpleRobotController) GetDaemonStatus() (string, error) {
	resp, err := http.Get(r.BaseURL + "/api/daemon/status")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var status struct {
		State string `json:"state"`
	}
	json.NewDecoder(resp.Body).Decode(&status)
	return status.State, nil
}

// SetVolume sets the robot's speaker volume (0-100)
func (r *SimpleRobotController) SetVolume(level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	payload := fmt.Sprintf(`{"volume": %d}`, level)
	resp, err := http.Post(r.BaseURL+"/api/volume/set", "application/json", strings.NewReader(payload))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
