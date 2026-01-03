# memory

Persistent knowledge storage for Eva.

## Overview

The memory package provides persistent storage for everything Eva needs to remember across sessions. Memory is organized into four categories:

- **Context**: Situational key-value facts ("owner_name" → "Brendan")
- **Spatial**: Known locations and places ("kitchen" → direction, description)
- **People**: Information about individuals (names, facts, last seen)
- **Knowledge**: Dynamic agent-created topic collections (recipes, tasks, preferences)

## Architecture

```
pkg/memory/
├── store.go       # Store interface + JSONStore implementation
├── memory.go      # Memory struct, New(), Save(), Load()
├── person.go      # PersonMemory struct + people methods
├── context.go     # Context key-value methods
├── spatial.go     # Location struct + spatial methods
├── knowledge.go   # Dynamic knowledge collections
└── README.md
```

## Storage Backend

The package uses a `Store` interface for persistence, making it easy to swap backends:

```go
type Store interface {
    Save(data []byte) error
    Load() ([]byte, error)
    Close() error
}
```

Current implementation: `JSONStore` (file-based JSON)

Future options: SQLite, PostgreSQL, Redis, etc.

## Usage

### Basic Setup

```go
// In-memory only (no persistence)
mem := memory.New()

// With JSON file persistence
mem := memory.NewWithFile("eva_memory.json")
defer mem.Close()

// With custom store
store := memory.NewJSONStore("/path/to/memory.json")
mem := memory.NewWithStore(store)
```

### Context (Situational Facts)

```go
// Store situational facts
mem.SetContext("owner_name", "Brendan")
mem.SetContext("time_of_day", "morning")
mem.SetContext("mood", "cheerful")

// Retrieve facts
name, ok := mem.GetContext("owner_name")  // "Brendan", true

// List all context
keys := mem.GetContextKeys()  // ["owner_name", "time_of_day", "mood"]
all := mem.GetAllContext()    // map[string]string

// Search context
results := mem.SearchContext("name")  // finds "owner_name"

// Delete
mem.DeleteContext("mood")
```

### People (Individual Information)

```go
// Remember facts about people
mem.RememberPerson("brendan", "owns the robot")
mem.RememberPerson("brendan", "likes coffee")
mem.RememberPerson("alice", "visits on weekends")

// Recall facts
facts := mem.RecallPerson("brendan")  // ["owns the robot", "likes coffee"]

// Find by partial name
person := mem.FindPerson("bren")  // finds "brendan"

// List all people
names := mem.GetAllPeople()  // ["brendan", "alice"]

// Forget someone
mem.ForgetPerson("alice")
```

### Spatial (Locations)

```go
// Remember locations
mem.RememberLocation("kitchen", "left", "where we make food")
mem.RememberLocation("living_room", "forward", "main area with couch")

// Or with full Location struct
loc := memory.NewLocation("right", "guest bedroom")
loc.Distance = "far"
mem.SetLocation("guest_room", loc)

// Retrieve locations
kitchen := mem.GetLocation("kitchen")
fmt.Printf("Kitchen is %s: %s\n", kitchen.Direction, kitchen.Description)

// Find by partial name
name, loc := mem.FindLocation("living")  // "living_room", *Location

// Find by direction
leftRooms := mem.GetLocationsByDirection("left")  // map[string]*Location

// List all locations
locations := mem.GetAllLocations()  // ["kitchen", "living_room", "guest_room"]
```

### Knowledge (Dynamic Collections)

Eva can create her own knowledge topics:

```go
// Create a topic
mem.CreateKnowledge("recipes")

// Or auto-create when storing
mem.RememberKnowledge("recipes", "pancakes", "flour, eggs, milk")
mem.RememberKnowledge("recipes", "coffee", "pour over method")

// Store structured data
mem.SetKnowledgeItem("tasks", "water_plants", map[string]any{
    "frequency": "weekly",
    "last_done": "2024-01-01",
})

// Retrieve
ingredients, ok := mem.RecallKnowledge("recipes", "pancakes")
task, ok := mem.GetKnowledgeItem("tasks", "water_plants")

// List topics and items
topics := mem.ListKnowledge()              // ["recipes", "tasks"]
items := mem.ListKnowledgeItems("recipes") // ["pancakes", "coffee"]

// Get entire topic
recipes := mem.GetKnowledgeTopic("recipes")  // map[string]any

// Search across all knowledge
results := mem.SearchKnowledge("coffee")  // finds in recipes

// Delete
mem.DeleteKnowledgeItem("recipes", "pancakes")
mem.DeleteKnowledge("recipes")  // delete entire topic
```

## JSON Structure

```json
{
  "context": {
    "owner_name": "Brendan",
    "time_of_day": "morning"
  },
  "spatial": {
    "kitchen": {
      "direction": "left",
      "distance": "nearby",
      "description": "where we make food",
      "last_mentioned": "2024-01-02T12:00:00Z"
    }
  },
  "people": {
    "brendan": {
      "name": "brendan",
      "facts": ["owns the robot", "likes coffee"],
      "last_seen": "2024-01-02T12:00:00Z"
    }
  },
  "knowledge": {
    "recipes": {
      "pancakes": "flour, eggs, milk",
      "coffee": "pour over method"
    },
    "tasks": {
      "water_plants": {
        "frequency": "weekly",
        "last_done": "2024-01-01"
      }
    }
  }
}
```

## AI Tool Integration

The memory system is exposed to Eva as tools:

| Tool | Method | Description |
|------|--------|-------------|
| `set_context` | `SetContext()` | Store situational fact |
| `get_context` | `GetContext()` | Retrieve situational fact |
| `remember_person` | `RememberPerson()` | Store fact about person |
| `recall_person` | `RecallPerson()` | Get facts about person |
| `remember_location` | `RememberLocation()` | Store spatial info |
| `recall_location` | `GetLocation()` | Get spatial info |
| `create_knowledge` | `CreateKnowledge()` | Create new topic |
| `remember_knowledge` | `RememberKnowledge()` | Store in any topic |
| `recall_knowledge` | `RecallKnowledge()` | Get from any topic |
| `list_knowledge` | `ListKnowledge()` | List topics |

## Thread Safety

All Memory methods are thread-safe using a read-write mutex. Reads can happen concurrently, writes are exclusive.

## Statistics

```go
stats := mem.Stats()
// {
//   "context": 3,
//   "spatial": 2,
//   "people": 1,
//   "knowledge_topics": 2,
//   "knowledge_items": 4
// }
```

## Clear Memory

```go
mem.Clear()  // Resets all memory to empty state
```
