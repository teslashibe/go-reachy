# robot

Robot control interfaces and implementations for Reachy Mini.

## Overview

This package provides interfaces and implementations for controlling the Reachy Mini robot. It follows the **Interface Segregation Principle (ISP)** by defining small, focused interfaces that can be composed as needed.

## Interfaces

### Segregated Interfaces

| Interface | Methods | Use Case |
|-----------|---------|----------|
| `HeadController` | `SetHeadPose(roll, pitch, yaw)` | Head tracking, looking at faces |
| `AntennaController` | `SetAntennas(left, right)` | Antenna animations |
| `BodyController` | `SetBodyYaw(yaw)` | Body rotation |
| `PoseController` | `SetPose(head, antennas, bodyYaw)` | Batched control (prevents daemon flooding) |
| `StatusController` | `GetDaemonStatus()` | Health checks |
| `VolumeController` | `SetVolume(level)` | Audio control |

### Composite Interface

```go
type Controller interface {
    HeadController
    AntennaController
    BodyController
    StatusController
    VolumeController
}
```

## Implementations

### HTTPController

HTTP-based robot control via the Reachy Mini daemon REST API.

```go
ctrl := robot.NewHTTPController("192.168.68.80")
err := ctrl.SetHeadPose(0, 0, 0.5) // Look right
```

### RateController

Rate-limited motion controller that fuses multiple input sources (tracking + tools) at a fixed update rate. **This is the recommended way to control the robot** as it centralizes all commands and prevents daemon flooding (Issue #135).

```go
rateCtrl := robot.NewRateController(httpCtrl, 50*time.Millisecond) // 20Hz
go rateCtrl.Run()
defer rateCtrl.Stop()

// Set base pose from tools
rateCtrl.SetBaseHead(robot.Offset{Yaw: 0.3})

// Add tracking offset (fused with base)
rateCtrl.SetTrackingOffset(robot.Offset{Yaw: 0.1})

// Set antennas and body (all batched into one HTTP call per tick)
rateCtrl.SetAntennas(0.1, -0.1)
rateCtrl.SetBodyYaw(0.5)
```

## Centralized Architecture (Issue #135 Fix)

The robot daemon can be overwhelmed by too many HTTP requests. The centralized architecture solves this:

```
┌─────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   Tracker   │────▶│                  │     │                  │
└─────────────┘     │                  │     │                  │
                    │  RateController  │────▶│  HTTPController  │───▶ Robot
┌─────────────┐     │  (sets state)    │     │  (20 HTTP/s)     │     Daemon
│  Emotions   │────▶│                  │     │                  │
└─────────────┘     │                  │     │                  │
                    └──────────────────┘     └──────────────────┘
┌─────────────┐            ▲
│Body Align   │────────────┘                 ONE batched call
└─────────────┘                              per tick (SetPose)
```

**Before:** Multiple systems making independent HTTP calls = 50-100+ requests/second → daemon crash

**After:** All systems set state on RateController, which batches into ONE HTTP call per tick = 20 requests/second → stable

## Types

### Offset

Represents head position in radians:

```go
type Offset struct {
    Roll, Pitch, Yaw float64
}
```

## Batched Control (SetPose)

The `SetPose` method combines head, antennas, and body yaw into a single HTTP call:

```go
// Individual calls (3 HTTP requests - NOT recommended in loops)
ctrl.SetHeadPose(0, 0, 0.5)
ctrl.SetAntennas(0.1, -0.1)
ctrl.SetBodyYaw(0.3)

// Batched call (1 HTTP request - recommended)
head := &robot.Offset{Roll: 0, Pitch: 0, Yaw: 0.5}
antennas := [2]float64{0.1, -0.1}
bodyYaw := 0.3
ctrl.SetPose(head, &antennas, &bodyYaw)

// Pass nil to leave values unchanged
ctrl.SetPose(head, nil, nil) // Only update head
```

## Usage Examples

### Minimal Interface Dependency

If you only need head control (e.g., for tracking), depend on `HeadController`:

```go
func NewTracker(head robot.HeadController) *Tracker {
    // Only requires SetHeadPose, not the full Controller
}
```

### Full Robot Control

For complete robot control, use `Controller`:

```go
func NewEva(ctrl robot.Controller) *Eva {
    // Has access to all robot capabilities
}
```



