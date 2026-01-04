# worldmodel

Spatial world model for tracking entities in the environment.

## Overview

This package maintains a spatial representation of entities (faces, audio sources, objects) in world coordinates. It handles coordinate transformations between camera-relative and room-relative positions.

## Features

- Entity tracking with confidence decay
- Coordinate transformation (camera → world)
- Target priority (face > audio > none)
- Body orientation awareness

## Usage

```go
world := worldmodel.New()

// Update body orientation
world.SetBodyYaw(0.5) // Body rotated 0.5 rad right

// Update entity from camera detection
world.UpdateEntity("person1", roomAngle, framePosition)

// Update from audio source
world.UpdateAudioSource(angle, confidence, speaking)

// Get tracking target
angle, source, hasTarget := world.GetTarget()
if hasTarget {
    fmt.Printf("Target at %.2f rad (source: %s)\n", angle, source)
}
```

## Coordinate System

```
        +Y (up)
         │
         │
         │
─────────┼─────────▶ +X (right)
         │
         │
         │

Yaw: 0 = forward, positive = right, negative = left
```

## Entity Types

| Type | Priority | Source |
|------|----------|--------|
| Face | High | Camera + YuNet |
| Audio | Medium | XVF3800 DOA |
| Object | Low | YOLO detection |

## Confidence Decay

Entity confidence decays over time when not updated:

```go
world.DecayConfidence(deltaTime)
```

Entities with confidence below threshold are forgotten.



