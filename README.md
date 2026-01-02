# go-reachy 🤖

A high-performance Go controller for the [Reachy Mini](https://www.pollen-robotics.com/reachy-mini/) robot with real-time conversational AI.

## ✨ Features

- **Eva** — Conversational AI robot agent with voice, vision, and tool use
- **ElevenLabs Integration** — Custom voice cloning with Gemini/Claude/GPT-4o
- **OpenAI Realtime** — Alternative voice provider with built-in voices
- **Face Tracking** — YuNet-based face detection with smooth head following
- **Single Binary** — No Python, no dependencies, ~15MB
- **Cross-compile** — Runs directly on the robot's Raspberry Pi 4

## 🚀 Quick Start

### Run Eva (Conversational Agent)

```bash
# Set up environment
export ELEVENLABS_API_KEY=your-key
export ELEVENLABS_VOICE_ID=your-voice-id  # From ElevenLabs dashboard
export ROBOT_IP=192.168.68.80             # Your robot's IP

# Run
go run ./cmd/eva
```

### Demo Commands

```bash
# Dance demo
go run ./cmd/dance

# Vision demo (describe what Eva sees)
go run ./cmd/vision

# Face tracking
go run ./cmd/explore
```

## 📁 Project Structure

```
go-reachy/
├── cmd/
│   ├── eva/              # 🌟 Main conversational agent
│   ├── test-elevenlabs/  # ElevenLabs integration test
│   ├── dance/            # Dance demo
│   ├── explore/          # Look around and describe
│   ├── vision/           # Vision-only demo
│   └── travis/           # Special demo
├── pkg/
│   ├── conversation/     # Voice conversation (ElevenLabs, OpenAI)
│   ├── inference/        # LLM/Vision (Gemini, OpenAI, Ollama)
│   ├── tts/              # Text-to-speech providers
│   ├── tracking/         # Face detection and head tracking
│   ├── robot/            # Robot control (head, antennas)
│   ├── realtime/         # Audio processing and tools
│   ├── hub/              # WebSocket message hub
│   └── web/              # Web dashboard
├── docs/
│   ├── ARCHITECTURE.md   # System design
│   ├── EVA-2.0.md        # Eva architecture
│   └── SETUP.md          # Installation guide
└── go.mod
```

## 🎤 Voice Providers

| Provider | Voice | LLM Choice | Latency |
|----------|-------|------------|---------|
| **ElevenLabs** ⭐ | Custom cloned | Gemini/Claude/GPT-4o | ~400-600ms |
| OpenAI Realtime | Built-in (shimmer) | GPT-4o only | ~500-800ms |

### ElevenLabs Setup (Recommended)

```bash
export ELEVENLABS_API_KEY=sk_...
export ELEVENLABS_VOICE_ID=...           # Your voice ID
export ELEVENLABS_LLM=gemini-2.0-flash   # Optional (default)
```

### OpenAI Setup

```bash
export OPENAI_API_KEY=sk_...
export CONVERSATION_PROVIDER=openai
```

## 🤖 Deploy to Robot

```bash
# Cross-compile for ARM64
GOOS=linux GOARCH=arm64 go build -o eva-arm64 ./cmd/eva

# Deploy
scp eva-arm64 pollen@192.168.68.80:~/eva

# Run on robot
ssh pollen@192.168.68.80 "./eva"
```

## 🔧 Robot Configuration

| Setting | Value |
|---------|-------|
| SSH User | `pollen` |
| SSH Password | `root` |
| HTTP API | `http://<IP>:8000` |
| WebRTC | `ws://<IP>:8443` |

## 📖 Documentation

- [Architecture Overview](docs/ARCHITECTURE.md) — System design and provider flow
- [Eva 2.0](docs/EVA-2.0.md) — Conversational agent architecture
- [Setup Guide](docs/SETUP.md) — Installation and deployment
- [Troubleshooting](docs/TROUBLESHOOTING.md) — Common issues and fixes

## 🎯 Status

- [x] ElevenLabs programmatic agent configuration
- [x] OpenAI Realtime API integration
- [x] Face detection and tracking (YuNet)
- [x] Vision with Gemini Flash
- [x] Tool calling (describe scene, dance, etc.)
- [x] Web dashboard with live camera
- [x] Cross-compilation for ARM64
- [ ] Wake word detection
- [ ] Multi-turn memory persistence

## 📦 Why Go?

| Metric | Python | Go |
|--------|--------|-----|
| Deployment | venv + 142 packages | Single 15MB binary |
| Control latency | 10-50ms | <1ms |
| Memory | ~200MB | ~20MB |
| Cross-compile | Complex | `GOOS=linux GOARCH=arm64` |

## 🤝 Contributing

Contributions welcome! Please read the [Architecture](docs/ARCHITECTURE.md) doc first.

## 📄 License

MIT

---

Made with ❤️ for robotics
