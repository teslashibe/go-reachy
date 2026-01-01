# pkg/robot

Low-level robot control via Zenoh protocol.

## Overview

Direct connection to Reachy Mini's Zenoh-based control system. Handles joint positions, head pose, antennas, and body rotation.

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/robot"

ctx := context.Background()
r, err := robot.Connect(ctx, "192.168.68.77", false)
if err != nil {
    log.Fatal(err)
}
defer r.Disconnect()

// Control head
r.SetHead(0, 0, 0.5, 0) // x, y, z, yaw

// Control antennas
r.SetAntennas(0.3, -0.3) // left, right

// Control body
r.SetBodyYaw(0.5) // radians

// Get current state
joints := r.GetJointPositions()
```

## Joint Positions

```go
type JointPositions struct {
    Neck         [3]float64 // roll, pitch, yaw
    LeftAntenna  float64
    RightAntenna float64
    BodyYaw      float64
}
```

## SimpleRobotController

Wrapper used by Eva's tools for common operations.

```go
robot := realtime.NewSimpleRobotController(robotIP)
robot.SetHeadPose(roll, pitch, yaw)
robot.SetAntennas(left, right)
robot.SetBodyYaw(yaw)
robot.SetVolume(100)
status, _ := robot.GetDaemonStatus()
```

## Requirements

- Reachy Mini daemon running
- Zenoh port 7447 accessible


