# go-reachy 🤖

A high-performance Go controller for the [Reachy Mini](https://www.pollen-robotics.com/reachy-mini/) robot.

**Run Go code directly ON the robot** — no laptop tether required!

## ✨ Features

- **Single binary deployment** — no Python, no dependencies
- **Cross-compile for ARM64** — runs directly on the robot's Raspberry Pi 4
- **Sub-millisecond control loops** — real-time performance
- **~8MB binary** — vs ~200MB for Python environment
- **Native concurrency** — goroutines for parallel operations

## 🚀 Quick Start

```bash
# Clone
git clone https://github.com/teslashibe/go-reachy.git
cd go-reachy

# Build
go build ./...

# Run dance demo (replace IP with your robot's)
go run ./cmd/dance
```

## 🤖 Run on the Robot (Standalone)

```bash
# Cross-compile for ARM64
GOOS=linux GOARCH=arm64 go build -o dance-arm64 ./cmd/dance

# Deploy to robot
scp dance-arm64 pollen@reachy-mini.local:~/dance

# SSH and run
ssh pollen@reachy-mini.local "./dance"
```

## 📁 Project Structure

```
go-reachy/
├── cmd/
│   ├── eva/             # Eva conversational AI agent
│   ├── reachy/          # Main CLI
│   ├── poc/             # Proof of concept
│   └── dance/           # Dance demo ← start here!
├── pkg/
│   ├── robot/           # Robot control (HTTP/WebSocket)
│   ├── tracking/        # Head tracking (face + audio DOA)
│   ├── speech/          # Speech-synced head wobble
│   ├── video/           # WebRTC video stream
│   ├── tts/             # Text-to-speech (ElevenLabs, OpenAI)
│   ├── eva/             # Eva AI tools and personality
│   ├── worldmodel/      # Entity tracking and spatial awareness
│   ├── memory/          # Persistent memory storage
│   └── debug/           # Conditional debug logging
├── docs/
│   ├── ARCHITECTURE.md  # System design
│   ├── SETUP.md         # Installation guide
│   └── API.md           # HTTP API reference
└── go.mod
```

## 📖 Documentation

- [Architecture Overview](docs/ARCHITECTURE.md) — system design and data flow
- [Setup Guide](docs/SETUP.md) — installation and deployment
- [API Reference](docs/API.md) — HTTP endpoints and examples

## 🎯 Roadmap

- [x] HTTP API robot control
- [x] Dance/movement demos
- [x] Cross-compilation for ARM64
- [x] Run directly on robot
- [x] OpenAI Realtime API integration
- [x] Head tracking (face detection + audio DOA)
- [x] Speech-synced head wobble animation
- [x] Breathing animation for idle state
- [x] ElevenLabs TTS streaming
- [x] Microphone audio capture
- [x] Speaker audio playback
- [ ] Systemd service for auto-start
- [ ] Web UI control panel

## 🔧 Robot Configuration

| Setting | Value |
|---------|-------|
| Hostname | `reachy-mini.local` |
| SSH User | `pollen` |
| SSH Password | `root` |
| HTTP API | `http://<IP>:8000` |

## 📦 Why Go?

| Metric | Python | Go |
|--------|--------|-----|
| Deployment | venv + 142 packages | Single 8MB binary |
| Control latency | 10-50ms | <1ms |
| Memory | ~200MB | ~10MB |
| Cross-compile | Complex | `GOOS=linux GOARCH=arm64` |

## 🤝 Contributing

Contributions welcome! Please read the [Architecture](docs/ARCHITECTURE.md) doc first.

## 📄 License

MIT

---

Made with ❤️ for robotics
