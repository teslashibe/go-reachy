# memory

Persistent memory system for Eva's conversations.

## Overview

This package provides a simple key-value memory system that persists to disk. Eva uses this to remember information about people and past conversations.

## Usage

```go
// Create memory with file persistence
mem := memory.NewMemoryWithFile("memory.json")

// Store a memory
mem.Remember("user_name", "Alice")

// Recall a memory
name, ok := mem.Recall("user_name")

// Store person-specific memory
mem.RememberPerson("alice", "favorite_color", "blue")

// Recall person-specific memory
color := mem.RecallPerson("alice", "favorite_color")
```

## Features

- **Persistence**: Memories are saved to a JSON file
- **Per-person memories**: Store information about specific individuals
- **Global memories**: Store general information
- **Atomic writes**: Safe concurrent access

## Memory Structure

```json
{
  "global": {
    "owner_name": "Brendan"
  },
  "people": {
    "alice": {
      "name": "Alice",
      "favorite_color": "blue",
      "last_seen": "2024-01-15"
    }
  }
}
```

## AI Tool Integration

The memory system is exposed to Eva as tools:

- `remember` - Store information for later
- `recall` - Retrieve stored information
- `remember_person` - Store info about a specific person
- `recall_person` - Retrieve info about a person

