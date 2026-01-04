package eva

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/teslashibe/go-reachy/pkg/vision"
)

// Tools returns all tools available to Eva.
func Tools(cfg ToolsConfig) []Tool {
	robot := cfg.Robot
	mem := cfg.Memory
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
			Name:        "play_emotion",
			Description: `Play a pre-recorded emotion animation. Available emotions include: yes1, no1, sad1, sad2, surprised1, surprised2, happy (cheerful1), laughing1, laughing2, dance1, dance2, dance3, tired1, confused1, scared1, fear1, angry (rage1, furious1, irritated1), curious1, boredom1, boredom2, welcoming1, welcoming2, proud1, proud2, proud3, grateful1, helpful1, helpful2, loving1, shy1, and 70+ more. Use emotions that match the context of the conversation.`,
			Parameters: map[string]interface{}{
				"emotion": map[string]interface{}{
					"type":        "string",
					"description": "Name of the emotion to play (e.g., 'yes1', 'surprised1', 'dance1', 'laughing1')",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				emotionName, _ := args["emotion"].(string)
				if emotionName == "" {
					return "Please specify an emotion name", nil
				}

				if cfg.Emotions == nil {
					return "Emotion system not available", nil
				}

				// Check if emotion exists
				emotion, err := cfg.Emotions.Get(emotionName)
				if err != nil {
					// Try to find a close match
					matches := cfg.Emotions.Search(emotionName)
					if len(matches) > 0 {
						return fmt.Sprintf("Emotion '%s' not found. Did you mean: %s?", emotionName, strings.Join(matches[:min(5, len(matches))], ", ")), nil
					}
					return fmt.Sprintf("Emotion '%s' not found", emotionName), nil
				}

				fmt.Printf("🎭 Playing emotion: %s (%.1fs)\n", emotionName, emotion.Duration.Seconds())

				// Play asynchronously - callback handles robot movement
				go func() {
					ctx := context.Background()
					if err := cfg.Emotions.PlaySync(ctx, emotionName); err != nil {
						fmt.Printf("🎭 Emotion playback error: %v\n", err)
					}
				}()

				return fmt.Sprintf("Playing emotion: %s - %s", emotionName, emotion.Description), nil
			},
		},
		{
			Name:        "stop_emotion",
			Description: "Stop the currently playing emotion animation.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if cfg.Emotions != nil {
					cfg.Emotions.Stop()
					return "Stopped emotion playback", nil
				}
				return "Emotion system not available", nil
			},
		},
		{
			Name:        "list_emotions",
			Description: "List available emotion categories. Use this to see what emotions you can play.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if cfg.Emotions == nil {
					return "Emotion system not available", nil
				}

				cats := cfg.Emotions.Categories()
				var parts []string
				for cat, items := range cats {
					parts = append(parts, fmt.Sprintf("%s (%d)", cat, len(items)))
				}
				return fmt.Sprintf("Emotion categories: %s. Total: %d emotions available.", strings.Join(parts, ", "), cfg.Emotions.Count()), nil
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

				var wait time.Duration
				if unit == "seconds" {
					wait = time.Duration(duration) * time.Second
				} else {
					wait = time.Duration(duration) * time.Minute
				}

				fmt.Printf("⏱️  Timer set: %d %s (label: %s)\n", duration, unit, label)

				go func() {
					time.Sleep(wait)

					msg := "Timer done!"
					if label != "" {
						msg = fmt.Sprintf("Your %s timer is done!", label)
					}

					fmt.Printf("🔔 Timer finished: %s\n", msg)

					if cfg.AudioPlayer != nil {
						if err := cfg.AudioPlayer.SpeakText(msg); err != nil {
							fmt.Printf("🔔 Timer TTS error: %v\n", err)
						}
					} else {
						fmt.Println("🔔 Error: AudioPlayer is nil, cannot announce timer")
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

				if mem != nil && name != "" && fact != "" {
					mem.RememberPerson(name, fact)
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

				if mem != nil && name != "" {
					facts := mem.RecallPerson(name)
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
					robot.SetHeadPose(0, 0, 0.4)
					time.Sleep(500 * time.Millisecond)
					robot.SetHeadPose(0, 0, -0.4)
					time.Sleep(500 * time.Millisecond)
					robot.SetHeadPose(0, 0, 0)
				}
				return "Looked around the room", nil
			},
		},
		{
			Name:        "rotate_body",
			Description: "Rotate your body left or right. Use this to turn your whole body to face someone or something.",
			Parameters: map[string]interface{}{
				"direction": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"left", "right", "center"},
					"description": "Direction to rotate body",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				dir, _ := args["direction"].(string)
				var yaw float64

				switch dir {
				case "left":
					yaw = 0.5
				case "right":
					yaw = -0.5
				case "center":
					yaw = 0
				}

				if robot != nil {
					robot.SetBodyYaw(yaw)
				}

				if cfg.Tracker != nil {
					cfg.Tracker.SetBodyYaw(yaw)
				}

				return fmt.Sprintf("Rotated body %s", dir), nil
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
				level := 100
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

				fmt.Println("👁️  Capturing frame...")
				imageData, err := cfg.Vision.CaptureFrame()
				if err != nil {
					fmt.Printf("👁️  Frame capture error: %v\n", err)
					return fmt.Sprintf("Could not capture image: %v", err), nil
				}
				fmt.Printf("👁️  Captured %d bytes\n", len(imageData))

				var prompt string
				switch focus {
				case "people":
					prompt = "Describe any people you see in this image. How many people are there? What are they doing? Where are they positioned (left, center, right)? Be concise."
				case "general":
					prompt = "Briefly describe what you see in this image. Mention the setting, any people, and notable objects. Keep it to 2-3 sentences."
				default:
					prompt = fmt.Sprintf("Look at this image and tell me if you can see: %s. Describe what you find. Be concise.", focus)
				}

				fmt.Println("👁️  Calling Gemini Flash...")
				description, err := vision.GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
				if err != nil {
					fmt.Printf("👁️  Gemini error: %v\n", err)
					return fmt.Sprintf("Vision error: %v", err), nil
				}
				fmt.Printf("👁️  Gemini response: %s\n", description)

				return description, nil
			},
		},
		{
			Name:        "detect_objects",
			Description: "Quickly detect objects, animals, or people in your camera view. Use this when you want to know if there's a cat, dog, person, or other object nearby. Faster than describe_scene.",
			Parameters: map[string]interface{}{
				"target": map[string]interface{}{
					"type":        "string",
					"description": "What to look for: 'all' for everything, 'animals' for cats/dogs/birds, 'people', or a specific object like 'cat' or 'cup'",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				target, _ := args["target"].(string)
				if target == "" {
					target = "all"
				}

				fmt.Printf("🔍 detect_objects called (target: %s)\n", target)

				if cfg.Vision == nil {
					return "I cannot see right now - camera not connected", nil
				}

				if cfg.ObjectDetector == nil {
					return "Object detection not available", nil
				}

				imageData, err := cfg.Vision.CaptureFrame()
				if err != nil {
					return fmt.Sprintf("Could not capture image: %v", err), nil
				}

				detections, err := cfg.ObjectDetector.Detect(imageData)
				if err != nil {
					return fmt.Sprintf("Detection error: %v", err), nil
				}

				if len(detections) == 0 {
					return "I don't see any objects in my view right now", nil
				}

				var filtered []ObjectDetectionResult
				for _, det := range detections {
					switch target {
					case "all":
						filtered = append(filtered, det)
					case "animals":
						if isAnimal(det.ClassName) {
							filtered = append(filtered, det)
						}
					case "people":
						if det.ClassName == "person" {
							filtered = append(filtered, det)
						}
					default:
						if det.ClassName == target {
							filtered = append(filtered, det)
						}
					}
				}

				if len(filtered) == 0 {
					return fmt.Sprintf("I don't see any %s in my view", target), nil
				}

				counts := make(map[string]int)
				for _, det := range filtered {
					counts[det.ClassName]++
				}

				var parts []string
				for name, count := range counts {
					if count == 1 {
						parts = append(parts, fmt.Sprintf("a %s", name))
					} else {
						parts = append(parts, fmt.Sprintf("%d %ss", count, name))
					}
				}

				var result string
				if len(parts) == 1 {
					result = fmt.Sprintf("I can see %s", parts[0])
				} else {
					result = fmt.Sprintf("I can see %s", strings.Join(parts, ", "))
				}

				if len(filtered) == 1 {
					det := filtered[0]
					cx := det.X + det.W/2
					if cx < 0.33 {
						result += " on my left"
					} else if cx > 0.66 {
						result += " on my right"
					} else {
						result += " in front of me"
					}
				}

				fmt.Printf("🔍 Detected: %s\n", result)
				return result, nil
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

				result, err := vision.WebSearch(cfg.GoogleAPIKey, query)
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

				query := fmt.Sprintf(
					"Find specific flights from %s to %s on %s in %s class. "+
						"I need: airline names, departure times, arrival times, flight numbers, and prices in USD. "+
						"Search Google Flights or airline websites for real current availability.",
					origin, destination, date, cabinClass)

				result, err := vision.WebSearch(cfg.GoogleAPIKey, query)
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

				if robot != nil {
					robot.SetHeadPose(0, 0, 0.3)
					time.Sleep(400 * time.Millisecond)
				}

				imageData, err := cfg.Vision.CaptureFrame()
				if err == nil {
					prompt := fmt.Sprintf("Is there a person in this image who might be %s? Answer briefly.", person)
					desc, _ := vision.GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
					if strings.Contains(strings.ToLower(desc), "yes") {
						return fmt.Sprintf("I see someone on my left who might be %s. %s", person, desc), nil
					}
				}

				if robot != nil {
					robot.SetHeadPose(0, 0, -0.3)
					time.Sleep(400 * time.Millisecond)
				}

				imageData, err = cfg.Vision.CaptureFrame()
				if err == nil {
					prompt := fmt.Sprintf("Is there a person in this image who might be %s? Answer briefly.", person)
					desc, _ := vision.GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
					if strings.Contains(strings.ToLower(desc), "yes") {
						return fmt.Sprintf("I see someone on my right who might be %s. %s", person, desc), nil
					}
				}

				if robot != nil {
					robot.SetHeadPose(0, 0, 0)
					time.Sleep(300 * time.Millisecond)
				}

				imageData, err = cfg.Vision.CaptureFrame()
				if err == nil {
					prompt := fmt.Sprintf("Is there a person in this image who might be %s? Answer briefly.", person)
					desc, _ := vision.GeminiVision(cfg.GoogleAPIKey, imageData, prompt)
					if strings.Contains(strings.ToLower(desc), "yes") {
						return fmt.Sprintf("I see someone in front of me who might be %s. %s", person, desc), nil
					}
				}

				return fmt.Sprintf("I looked around but I don't see %s right now.", person), nil
			},
		},
		// =============================================================
		// Context Memory Tools (situational key-value facts)
		// =============================================================
		{
			Name:        "set_context",
			Description: "Store a situational fact about the current context. Use this for things like owner's name, current location, time of day, or any key-value information you want to remember.",
			Parameters: map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The key or name for this fact (e.g., 'owner_name', 'current_room', 'mood')",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "The value to store",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				key, _ := args["key"].(string)
				value, _ := args["value"].(string)

				if mem != nil && key != "" && value != "" {
					mem.SetContext(key, value)
					return fmt.Sprintf("Stored context: %s = %s", key, value), nil
				}
				return "Could not store context", nil
			},
		},
		{
			Name:        "get_context",
			Description: "Retrieve a stored situational fact. Use this to recall context information you previously stored.",
			Parameters: map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The key to look up",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				key, _ := args["key"].(string)

				if mem != nil && key != "" {
					value, found := mem.GetContext(key)
					if found {
						return fmt.Sprintf("%s: %s", key, value), nil
					}
					return fmt.Sprintf("I don't have any context stored for '%s'", key), nil
				}
				return "No memory available", nil
			},
		},
		{
			Name:        "list_context",
			Description: "List all stored context keys. Use this to see what situational facts you have stored.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if mem != nil {
					keys := mem.GetContextKeys()
					if len(keys) == 0 {
						return "No context stored yet", nil
					}
					return fmt.Sprintf("Stored context keys: %s", strings.Join(keys, ", ")), nil
				}
				return "No memory available", nil
			},
		},
		// =============================================================
		// Spatial Memory Tools (location knowledge)
		// =============================================================
		{
			Name:        "remember_location",
			Description: "Remember a location or place in your environment. Use this to store where things are relative to you.",
			Parameters: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the location (e.g., 'kitchen', 'front_door', 'brendans_desk')",
				},
				"direction": map[string]interface{}{
					"type":        "string",
					"description": "Direction relative to you (e.g., 'left', 'right', 'forward', 'behind', 'forward-left')",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Optional description of the location (e.g., 'where we make food')",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				name, _ := args["name"].(string)
				direction, _ := args["direction"].(string)
				description, _ := args["description"].(string)

				if mem != nil && name != "" && direction != "" {
					mem.RememberLocation(name, direction, description)
					if description != "" {
						return fmt.Sprintf("Remembered: %s is to my %s (%s)", name, direction, description), nil
					}
					return fmt.Sprintf("Remembered: %s is to my %s", name, direction), nil
				}
				return "Could not remember location", nil
			},
		},
		{
			Name:        "recall_location",
			Description: "Recall where a location is. Use this to remember where places are in your environment.",
			Parameters: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the location to find",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				name, _ := args["name"].(string)

				if mem != nil && name != "" {
					loc := mem.GetLocation(name)
					if loc != nil {
						if loc.Description != "" {
							return fmt.Sprintf("%s is to my %s - %s", name, loc.Direction, loc.Description), nil
						}
						return fmt.Sprintf("%s is to my %s", name, loc.Direction), nil
					}
					// Try partial match
					foundName, loc := mem.FindLocation(name)
					if loc != nil {
						if loc.Description != "" {
							return fmt.Sprintf("%s is to my %s - %s", foundName, loc.Direction, loc.Description), nil
						}
						return fmt.Sprintf("%s is to my %s", foundName, loc.Direction), nil
					}
					return fmt.Sprintf("I don't know where %s is", name), nil
				}
				return "No memory available", nil
			},
		},
		{
			Name:        "list_locations",
			Description: "List all known locations. Use this to see what places you remember.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if mem != nil {
					locations := mem.GetAllLocations()
					if len(locations) == 0 {
						return "I don't know any locations yet", nil
					}
					return fmt.Sprintf("Known locations: %s", strings.Join(locations, ", ")), nil
				}
				return "No memory available", nil
			},
		},
		// =============================================================
		// Knowledge Memory Tools (dynamic agent-created topics)
		// =============================================================
		{
			Name:        "create_knowledge_topic",
			Description: "Create a new knowledge topic to organize information. Use this to create categories for things you want to remember, like 'recipes', 'tasks', 'jokes', 'preferences'.",
			Parameters: map[string]interface{}{
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "The name of the knowledge topic to create",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				topic, _ := args["topic"].(string)

				if mem != nil && topic != "" {
					err := mem.CreateKnowledge(topic)
					if err != nil {
						// Topic might already exist, which is fine
						if mem.HasKnowledge(topic) {
							return fmt.Sprintf("Knowledge topic '%s' already exists", topic), nil
						}
						return fmt.Sprintf("Could not create topic: %v", err), nil
					}
					return fmt.Sprintf("Created knowledge topic: %s", topic), nil
				}
				return "Could not create topic", nil
			},
		},
		{
			Name:        "remember_knowledge",
			Description: "Store information in a knowledge topic. Use this to remember specific facts within a category you've created.",
			Parameters: map[string]interface{}{
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "The knowledge topic (e.g., 'recipes', 'tasks')",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "A key or name for this piece of knowledge",
				},
				"value": map[string]interface{}{
					"type":        "string",
					"description": "The information to store",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				topic, _ := args["topic"].(string)
				key, _ := args["key"].(string)
				value, _ := args["value"].(string)

				if mem != nil && topic != "" && key != "" && value != "" {
					err := mem.RememberKnowledge(topic, key, value)
					if err != nil {
						return fmt.Sprintf("Could not store knowledge: %v", err), nil
					}
					return fmt.Sprintf("Stored in %s: %s = %s", topic, key, value), nil
				}
				return "Could not store knowledge", nil
			},
		},
		{
			Name:        "recall_knowledge",
			Description: "Retrieve information from a knowledge topic. Use this to remember facts you've stored in a category.",
			Parameters: map[string]interface{}{
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "The knowledge topic to search in",
				},
				"key": map[string]interface{}{
					"type":        "string",
					"description": "The key to look up",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				topic, _ := args["topic"].(string)
				key, _ := args["key"].(string)

				if mem != nil && topic != "" && key != "" {
					value, found := mem.RecallKnowledge(topic, key)
					if found {
						return fmt.Sprintf("%s/%s: %s", topic, key, value), nil
					}
					if !mem.HasKnowledge(topic) {
						return fmt.Sprintf("I don't have a knowledge topic called '%s'", topic), nil
					}
					return fmt.Sprintf("I don't have '%s' stored in %s", key, topic), nil
				}
				return "No memory available", nil
			},
		},
		{
			Name:        "list_knowledge_topics",
			Description: "List all knowledge topics you've created. Use this to see what categories of information you have.",
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if mem != nil {
					topics := mem.ListKnowledge()
					if len(topics) == 0 {
						return "I haven't created any knowledge topics yet", nil
					}
					return fmt.Sprintf("Knowledge topics: %s", strings.Join(topics, ", ")), nil
				}
				return "No memory available", nil
			},
		},
		{
			Name:        "list_knowledge_items",
			Description: "List all items in a knowledge topic. Use this to see what you've stored in a specific category.",
			Parameters: map[string]interface{}{
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "The knowledge topic to list items from",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				topic, _ := args["topic"].(string)

				if mem != nil && topic != "" {
					if !mem.HasKnowledge(topic) {
						return fmt.Sprintf("I don't have a knowledge topic called '%s'", topic), nil
					}
					items := mem.ListKnowledgeItems(topic)
					if len(items) == 0 {
						return fmt.Sprintf("No items stored in %s yet", topic), nil
					}
					return fmt.Sprintf("Items in %s: %s", topic, strings.Join(items, ", ")), nil
				}
				return "No memory available", nil
			},
		},
	}
}
