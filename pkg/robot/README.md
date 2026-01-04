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

Rate-limited motion controller that fuses multiple input sources (tracking + tools) at a fixed update rate.

```go
rateCtrl := robot.NewRateController(httpCtrl, 10*time.Millisecond)
go rateCtrl.Run()
defer rateCtrl.Stop()

// Set base pose from tools
rateCtrl.SetBaseHead(robot.Offset{Yaw: 0.3})

// Add tracking offset (fused with base)
rateCtrl.SetTrackingOffset(robot.Offset{Yaw: 0.1})
```

## Types

### Offset

Represents head position in radians:

```go
type Offset struct {
    Roll, Pitch, Yaw float64
}
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


