# eva

AI tool definitions and configuration for Eva.

## Overview

This package contains all the AI-callable tools that give Eva her capabilities. Tools are functions that the AI can invoke during conversation.

## Tool Categories

### Movement Tools

| Tool | Description |
|------|-------------|
| `move_head` | Point head in a direction (left, right, up, down, center) |
| `rotate_body` | Rotate body to face direction |
| `look_around` | Look around the room |
| `nod_yes` | Nod head to agree |
| `shake_head_no` | Shake head to disagree |

### Expression Tools

| Tool | Description |
|------|-------------|
| `express_emotion` | Express emotion (happy, curious, excited, confused, sad, surprised) |
| `wave_hello` | Wave antennas to greet |

### Perception Tools

| Tool | Description |
|------|-------------|
| `describe_scene` | Describe what Eva sees (uses Gemini Vision) |
| `detect_objects` | Detect and locate objects in view (YOLO) |
| `find_person` | Look for a specific person in the room |

### Memory Tools - People

| Tool | Description |
|------|-------------|
| `remember_person` | Store a fact about a person |
| `recall_person` | Retrieve facts about a person |

### Memory Tools - Context

| Tool | Description |
|------|-------------|
| `set_context` | Store a situational key-value fact (e.g., owner_name, mood) |
| `get_context` | Retrieve a stored context fact |
| `list_context` | List all stored context keys |

### Memory Tools - Spatial

| Tool | Description |
|------|-------------|
| `remember_location` | Store a location (name, direction, description) |
| `recall_location` | Retrieve where a location is |
| `list_locations` | List all known locations |

### Memory Tools - Knowledge

| Tool | Description |
|------|-------------|
| `create_knowledge_topic` | Create a new knowledge category (e.g., recipes, tasks) |
| `remember_knowledge` | Store information in a topic |
| `recall_knowledge` | Retrieve information from a topic |
| `list_knowledge_topics` | List all knowledge topics |
| `list_knowledge_items` | List items in a knowledge topic |

### Communication Tools

| Tool | Description |
|------|-------------|
| `web_search` | Search the internet for information |
| `search_flights` | Search for flight prices and availability |

### System Tools

| Tool | Description |
|------|-------------|
| `get_time` | Get current time and date |
| `set_timer` | Set a timer with optional label |
| `set_volume` | Adjust speaker volume |

## Configuration

```go
config := eva.ToolsConfig{
    Robot:          robotCtrl,     // robot.Controller
    Memory:         memoryStore,   // *memory.Memory
    Vision:         visionClient,  // vision.Provider
    ObjectDetector: detector,      // ObjectDetector
    GoogleAPIKey:   apiKey,        // For Gemini/web search
    AudioPlayer:    audioPlayer,   // *audio.Player
    Tracker:        tracker,       // BodyYawNotifier
}

tools := eva.Tools(config)
```

## Tool Definition

Each tool has:

```go
type Tool struct {
    Name        string                 // Unique identifier
    Description string                 // Shown to AI
    Parameters  map[string]any         // JSON Schema for args
    Handler     func(map[string]any) (string, error)
}
```

## Memory System Overview

Eva's memory is organized into four categories:

1. **People** - Facts about individuals (`remember_person`, `recall_person`)
2. **Context** - Situational key-value facts (`set_context`, `get_context`)
3. **Spatial** - Location knowledge (`remember_location`, `recall_location`)
4. **Knowledge** - Dynamic agent-created topics (`create_knowledge_topic`, `remember_knowledge`)

All memory persists to a JSON file and survives restarts.

## Adding New Tools

1. Define the tool in `tools.go`
2. Add handler function
3. Register in `Tools()` function
4. Document in this README
