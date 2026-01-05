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

## 🧠 Running Eva (Conversational AI)

Eva is a low-latency conversational AI agent that runs on the Reachy Mini robot.

### Prerequisites

Set up your API keys:
```bash
export OPENAI_API_KEY="your-openai-key"
export ELEVENLABS_API_KEY="your-elevenlabs-key"
export GOOGLE_API_KEY="your-google-api-key"

# For Spark (Google Docs integration)
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
```

### Find Your Robot

The robot's IP may change after reboot. Scan to find it:
```bash
for ip in 192.168.68.{50..100}; do 
  (curl -s --connect-timeout 1 "http://$ip:8000/api/daemon/status" >/dev/null 2>&1 && echo "Found Reachy at $ip") &
done; wait
```

### Run Eva

```bash
cd go-reachy

go run ./cmd/eva \
  --debug \
  --tts=elevenlabs-streaming \
  --tts-voice=lily \
  --robot-ip=192.168.68.83 \
  --spark=true \
  --no-body
```

### Command Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--debug` | Enable verbose logging | `false` |
| `--robot-ip` | Robot IP address | env `ROBOT_IP` |
| `--tts` | TTS provider: `realtime`, `elevenlabs`, `elevenlabs-streaming` | `realtime` |
| `--tts-voice` | Voice preset (lily, sage, etc.) | - |
| `--spark` | Enable Spark idea capture | `true` |
| `--no-body` | Disable body rotation (head-only tracking) | `false` |

### Web Dashboards

- **Eva Dashboard:** http://localhost:8181
- **Spark Ideas:** http://localhost:8181/spark.html

### Wake Up / Sleep Robot

```bash
# Find robot IP first, then:

# Start daemon (wake up)
sshpass -p "root" ssh pollen@<ROBOT_IP> "echo 'root' | sudo -S systemctl start reachy-mini-daemon"

# Stop daemon (sleep)
sshpass -p "root" ssh pollen@<ROBOT_IP> "echo 'root' | sudo -S systemctl stop reachy-mini-daemon"
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
- [x] Eva Spark - Voice-powered idea capture with Google Docs sync
- [x] Web dashboard for Eva and Spark
- [ ] Latency optimization (sub-200ms response) - [#112](https://github.com/teslashibe/go-reachy/issues/112)
- [ ] Body rotation auto-reset - [#113](https://github.com/teslashibe/go-reachy/issues/113)
- [ ] Gemini Live API integration - [#111](https://github.com/teslashibe/go-reachy/issues/111)
- [ ] Systemd service for auto-start

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
