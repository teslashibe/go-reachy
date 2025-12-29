# Reachy Mini HTTP API Reference

The Reachy Mini robot exposes an HTTP REST API on port 8000.

## Base URL

```
http://<ROBOT_IP>:8000
```

## Endpoints

### Daemon Management

#### GET /api/daemon/status

Get the current state of the robot daemon.

**Response:**
```json
{
  "robot_name": "reachy_mini",
  "state": "running",
  "motors_on": true,
  "battery_level": 85.5
}
```

**States:**
- `not_initialized` - Daemon not started
- `running` - Normal operation
- `error` - Error state

---

#### POST /api/daemon/start

Start the robot daemon.

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `wake_up` | bool | false | Enable motors immediately |

**Example:**
```bash
curl -X POST "http://192.168.68.80:8000/api/daemon/start?wake_up=true"
```

---

#### POST /api/daemon/stop

Stop the robot daemon and disable motors.

**Example:**
```bash
curl -X POST "http://192.168.68.80:8000/api/daemon/stop"
```

---

### Movement Control

#### POST /api/move/set_target

Set the target position for head, antennas, and body.

**Request Body:**
```json
{
  "target_head_pose": {
    "x": 0.0,
    "y": 0.0,
    "z": 0.0,
    "roll": 0.0,
    "pitch": 0.0,
    "yaw": 0.0
  },
  "target_antennas": [0.0, 0.0],
  "target_body_yaw": 0.0,
  "duration": 1.0
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `target_head_pose.x` | float | Head X position (forward/back) |
| `target_head_pose.y` | float | Head Y position (left/right) |
| `target_head_pose.z` | float | Head Z position (up/down), typically 0.0-0.2 |
| `target_head_pose.roll` | float | Head roll in radians |
| `target_head_pose.pitch` | float | Head pitch in radians |
| `target_head_pose.yaw` | float | Head yaw in radians |
| `target_antennas` | [float, float] | Left and right antenna angles in radians |
| `target_body_yaw` | float | Body rotation in radians |
| `duration` | float | Time to reach position in seconds |

**Example:**
```bash
curl -X POST "http://192.168.68.80:8000/api/move/set_target" \
  -H "Content-Type: application/json" \
  -d '{
    "target_head_pose": {"x": 0, "y": 0, "z": 0.1, "roll": 0, "pitch": 0, "yaw": 0.3},
    "target_antennas": [0.5, -0.5],
    "target_body_yaw": 0.2,
    "duration": 0.5
  }'
```

---

### Emotions

#### POST /api/emotions/{emotion}

Play a predefined emotion animation.

**Available Emotions:**
- `happy`
- `sad`
- `surprised`
- `angry`
- `confused`
- `thinking`

**Example:**
```bash
curl -X POST "http://192.168.68.80:8000/api/emotions/happy"
```

---

### Audio

#### GET /api/volume/get

Get the current speaker volume.

**Response:**
```json
{
  "volume": 75
}
```

---

#### POST /api/volume/set

Set the speaker volume.

**Request Body:**
```json
{
  "volume": 80
}
```

---

#### POST /api/volume/test-sound

Play a test sound through the robot's speaker.

**Example:**
```bash
curl -X POST "http://192.168.68.80:8000/api/volume/test-sound"
```

---

## WebSocket Endpoints

### /ws/state

Real-time robot state updates.

**Connection:**
```javascript
const ws = new WebSocket('ws://192.168.68.80:8000/ws/state');
ws.onmessage = (event) => {
  const state = JSON.parse(event.data);
  console.log(state);
};
```

**Message Format:**
```json
{
  "timestamp": 1703849123.456,
  "head_pose": {
    "x": 0.0,
    "y": 0.0,
    "z": 0.1,
    "roll": 0.0,
    "pitch": 0.0,
    "yaw": 0.0
  },
  "antennas": [0.0, 0.0],
  "body_yaw": 0.0,
  "battery": {
    "level": 85.5,
    "charging": false
  }
}
```

---

## Go Usage Examples

### Basic Movement

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

func main() {
    robotIP := "192.168.68.80"
    
    // Create movement payload
    payload := map[string]interface{}{
        "target_head_pose": map[string]float64{
            "x": 0, "y": 0, "z": 0.1,
            "roll": 0, "pitch": 0, "yaw": 0.3,
        },
        "target_antennas":  []float64{0.5, -0.5},
        "target_body_yaw":  0.2,
        "duration":         0.5,
    }
    
    body, _ := json.Marshal(payload)
    
    resp, err := http.Post(
        "http://"+robotIP+":8000/api/move/set_target",
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
}
```

### WebSocket State Streaming

```go
package main

import (
    "log"
    "github.com/gorilla/websocket"
)

func main() {
    robotIP := "192.168.68.80"
    
    conn, _, err := websocket.DefaultDialer.Dial(
        "ws://"+robotIP+":8000/ws/state",
        nil,
    )
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    for {
        _, message, err := conn.ReadMessage()
        if err != nil {
            log.Fatal(err)
        }
        log.Printf("State: %s", message)
    }
}
```

