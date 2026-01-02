# pkg/realtime

Audio streaming, tools, and helpers for OpenAI Realtime API integration.

## Components

### AudioPlayer

Streams audio to the robot via SSH + GStreamer.

```go
player := realtime.NewAudioPlayer(robotIP, sshUser, sshPass)

// Set TTS provider for announcements
player.SetTTSProvider(ttsProvider)

// Stream audio from conversation
player.AppendAudioBytes(pcmData)
player.FlushAndPlay()

// Text-to-speech announcement
player.SpeakText(ctx, "Timer is done!")

// Check state
if player.IsSpeaking() {
    player.Cancel()
}
```

### Tools (EvaTools)

Eva's tool definitions and handlers for LLM function calling.

```go
cfg := realtime.EvaToolsConfig{
    Robot:           robot,
    Memory:          memory,
    Vision:          videoClient,
    InferenceClient: inferenceProvider,
    AudioPlayer:     audioPlayer,
    Tracker:         headTracker,
}

tools := realtime.EvaTools(cfg)
```

#### Available Tools

| Tool | Description |
|------|-------------|
| `move_head` | Look left/right/up/down/center |
| `rotate_body` | Turn body left/right/center |
| `express_emotion` | Happy, curious, excited, etc. |
| `wave_hello` | Antenna wave greeting |
| `describe_scene` | Vision: describe what Eva sees |
| `find_person` | Vision: look for a person |
| `remember_person` | Store memory about someone |
| `recall_person` | Retrieve stored memory |
| `get_time` | Current time and date |
| `set_timer` | Countdown timer |
| `set_volume` | Adjust speaker volume |
| `web_search` | Search the internet |
| `search_flights` | Find flight prices |

### Memory

Persistent key-value memory for people and facts.

```go
memory := realtime.NewMemoryWithFile("~/.eva/memory.json")
memory.Remember("Alice", "loves hiking")
fact := memory.Recall("Alice") // "loves hiking"
```

### Helpers

```go
// Resample audio
resampled := realtime.Resample(pcmData, 48000, 24000)

// Convert int16 to bytes
bytes := realtime.ConvertInt16ToPCM16(samples)
```



