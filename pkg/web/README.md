# pkg/web

Real-time web dashboard for Eva.

## Overview

Fiber-based web server providing a real-time dashboard with WebSocket updates for status, logs, and camera streaming.

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/web"

server := web.NewServer("8181")

// Configure callbacks
server.OnToolTrigger = func(name string, args map[string]interface{}) (string, error) {
    return executeTool(name, args)
}

server.OnCaptureFrame = func() ([]byte, error) {
    return videoClient.CaptureJPEG()
}

// Start server
go server.Start()
defer server.Shutdown()

// Update state
server.UpdateState(func(s *web.EvaState) {
    s.RobotConnected = true
    s.Listening = true
})

// Add log entry
server.AddLog("info", "Eva started")

// Add conversation
server.AddConversation("user", "Hello Eva!")

// Stream camera frame
server.SendCameraFrame(jpegData)
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Static dashboard files |
| `/api/status` | GET | Current EvaState |
| `/api/tools` | GET | List available tools |
| `/api/tools/:name` | POST | Trigger a tool |
| `/api/logs` | GET | Recent log entries |
| `/api/conversation` | GET | Conversation history |
| `/ws/status` | WS | Real-time state updates |
| `/ws/logs` | WS | Real-time log streaming |
| `/ws/camera` | WS | JPEG frame streaming |

## EvaState

```go
type EvaState struct {
    RobotConnected  bool    `json:"robot_connected"`
    OpenAIConnected bool    `json:"openai_connected"`
    WebRTCConnected bool    `json:"webrtc_connected"`
    Speaking        bool    `json:"speaking"`
    Listening       bool    `json:"listening"`
    HeadYaw         float64 `json:"head_yaw"`
    FacePosition    float64 `json:"face_position"`
    ActiveTimer     string  `json:"active_timer"`
    LastUserMessage string  `json:"last_user_message"`
    LastEvaMessage  string  `json:"last_eva_message"`
}
```

## WebSocket Hubs

Uses `pkg/hub` for thread-safe broadcasting:
- `statusHub` - State updates
- `logHub` - Log entries
- `cameraHub` - Camera frames (binary)

## Static Files

Serve from `./web/` directory:
- `index.html` - Dashboard UI
- `style.css` - Styles
- `app.js` - Client-side logic



