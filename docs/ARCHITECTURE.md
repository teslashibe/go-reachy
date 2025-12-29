# Architecture Overview

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         DEPLOYMENT OPTIONS                               │
├─────────────────────────────────┬───────────────────────────────────────┤
│        TETHERED MODE            │          STANDALONE MODE               │
│   (Go runs on laptop/desktop)   │       (Go runs ON the robot)          │
├─────────────────────────────────┼───────────────────────────────────────┤
│                                 │                                        │
│  ┌──────────────────────────┐   │   ┌──────────────────────────────────┐│
│  │      Your Computer       │   │   │        REACHY MINI               ││
│  │  ┌────────────────────┐  │   │   │  ┌────────────────────────────┐ ││
│  │  │   Go Binary        │  │   │   │  │      Go Binary             │ ││
│  │  │  - Robot Control   │  │   │   │  │   - Robot Control          │ ││
│  │  │  - OpenAI API      │  │   │   │  │   - OpenAI API             │ ││
│  │  │  - Audio I/O       │  │   │   │  │   - Local Mic/Speaker      │ ││
│  │  └─────────┬──────────┘  │   │   │  └──────────┬─────────────────┘ ││
│  └────────────┼─────────────┘   │   │             │ localhost:8000     ││
│               │ WiFi            │   │  ┌──────────▼─────────────────┐ ││
│               │ HTTP/WebSocket  │   │  │    Python Daemon           │ ││
│               ▼                 │   │  │    (motor control)         │ ││
│  ┌──────────────────────────┐   │   │  └────────────────────────────┘ ││
│  │      REACHY MINI         │   │   └──────────────────────────────────┘│
│  │  ┌────────────────────┐  │   │                                        │
│  │  │   Python Daemon    │  │   │                                        │
│  │  │   (motor control)  │  │   │                                        │
│  │  └────────────────────┘  │   │                                        │
│  └──────────────────────────┘   │                                        │
│                                 │                                        │
└─────────────────────────────────┴───────────────────────────────────────┘
```

## Communication Layers

### HTTP REST API (Port 8000)

The robot exposes a REST API for control:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/daemon/status` | GET | Get robot state (initialized, motors on, etc.) |
| `/api/daemon/start` | POST | Start robot backend, optionally wake up motors |
| `/api/daemon/stop` | POST | Stop robot backend |
| `/api/move/set_target` | POST | Set head, antennas, body position |
| `/api/emotions/{emotion}` | POST | Play emotion animation |
| `/api/volume/get` | GET | Get current volume level |
| `/api/volume/set` | POST | Set volume level |
| `/api/volume/test-sound` | POST | Play test sound through speaker |

### WebSocket (Port 8000)

Real-time streaming data:

| Endpoint | Description |
|----------|-------------|
| `/ws/state` | Live robot state (joint positions, battery, etc.) |

### Zenoh (Port 7447)

Low-level robotics communication protocol used by the Python daemon internally.
We interface via HTTP/WebSocket which wraps Zenoh.

## Go Package Structure

```
go-reachy/
├── cmd/
│   ├── reachy/          # Main CLI application
│   │   └── main.go
│   ├── poc/             # Proof of concept demo
│   │   └── main.go
│   └── dance/           # Dance demo
│       └── main.go
├── pkg/
│   ├── robot/
│   │   ├── robot.go     # High-level robot abstraction
│   │   └── zenoh.go     # HTTP/WebSocket client
│   └── speech/
│       └── handler.go   # OpenAI integration (WIP)
├── docs/                # Documentation
└── go.mod
```

## Data Flow

### Movement Command Flow

```
User Input → Go Binary → HTTP POST /api/move/set_target
                                    │
                                    ▼
                         Python Daemon (port 8000)
                                    │
                                    ▼
                              Zenoh Publish
                                    │
                                    ▼
                            Motor Controllers
                                    │
                                    ▼
                           Physical Movement
```

### State Feedback Flow

```
Motor Encoders → Motor Controllers → Zenoh Subscribe
                                           │
                                           ▼
                                   Python Daemon
                                           │
                                           ▼
                            WebSocket /ws/state
                                           │
                                           ▼
                                    Go Binary
                                           │
                                           ▼
                              Update Internal State
```

## Cross-Compilation

Go makes it trivial to compile for the robot's ARM64 architecture:

```bash
# Build for the robot (Raspberry Pi 4, ARM64)
GOOS=linux GOARCH=arm64 go build -o app-arm64 ./cmd/reachy

# Deploy to robot
scp app-arm64 pollen@reachy-mini.local:~/

# Run on robot
ssh pollen@reachy-mini.local "./app-arm64"
```

## Why Go over Python?

| Metric | Python | Go |
|--------|--------|-----|
| Binary Size | N/A (needs interpreter + deps) | ~8MB self-contained |
| Control Loop Latency | 10-50ms | <1ms |
| Memory Usage | ~200MB | ~10MB |
| Concurrency | asyncio/threading complexity | Native goroutines |
| Deployment | venv + pip install | Single binary copy |
| Cross-compile | Painful | One command |
| Type Safety | Runtime errors | Compile-time checks |

## Future Architecture (Voice Assistant)

```
┌─────────────────────────────────────────────────────────────────┐
│                       REACHY MINI (Standalone)                   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Go Binary                              │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────────┐  │   │
│  │  │  Mic    │  │ OpenAI  │  │ Speaker │  │   Robot     │  │   │
│  │  │ Capture │→ │Realtime │→ │Playback │  │  Control    │  │   │
│  │  │ (ALSA)  │  │  API    │  │ (ALSA)  │  │ (Movement)  │  │   │
│  │  └─────────┘  └────┬────┘  └─────────┘  └──────┬──────┘  │   │
│  │                    │                           │          │   │
│  │                    │   Function Calls          │          │   │
│  │                    └───────────────────────────┘          │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              │ localhost:8000                    │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   Python Daemon                           │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
              │
              │ WiFi (only for OpenAI API)
              ▼
        ┌───────────┐
        │  OpenAI   │
        │  Cloud    │
        └───────────┘
```

