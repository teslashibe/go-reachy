# Epic: Cloud Architecture - Thin Client Migration

## Overview

Migrate Eva to a cloud-native architecture where `go-eva` runs as a thin client on the Raspberry Pi, streaming all data to `go-reachy` running in the cloud. This enables:

- **Scalability**: One cloud service can manage multiple robots
- **Performance**: Heavy processing (AI, face detection) runs on powerful cloud hardware
- **Simplicity**: Pi only handles hardware I/O, minimal CPU usage
- **Flexibility**: Easy updates, no robot-side deployments for AI changes

## Architecture

```
Phase 1 (v1): Hybrid Proxy
┌─────────────────────────────────────────────────────────────┐
│                    Raspberry Pi (Robot)                      │
│  ┌─────────────────┐    ┌───────────────────────────────┐   │
│  │  Pollen Daemon  │◄───│         go-eva                │   │
│  │     :8000       │    │          :9000                │   │
│  │  Motors/Camera  │    │  DOA + Camera + Motor Proxy   │   │
│  └─────────────────┘    └───────────────┬───────────────┘   │
└─────────────────────────────────────────┼───────────────────┘
                                          │ WebSocket
                                          ▼
┌─────────────────────────────────────────────────────────────┐
│              go-reachy (Cloud) - BRAIN                       │
│  Face Detection │ OpenAI GPT-4o │ TTS │ Tracking            │
└─────────────────────────────────────────────────────────────┘

Phase 2 (Future): Full Go Replacement
┌─────────────────────────────────────────────────────────────┐
│                    Raspberry Pi (Robot)                      │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              go-eva (Full Daemon)                    │    │
│  │  Direct Motor Control │ Camera │ Audio │ DOA        │    │
│  │              NO PYTHON DEPENDENCY                    │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Sub-Tickets

### Phase 1: Hybrid Proxy (v1)

#### EVA-101: WebSocket Protocol Design
**Priority**: P0 | **Estimate**: 1 day | **Repo**: go-eva, go-reachy

Define the WebSocket message protocol for robot↔cloud communication:
- Frame messages (video)
- DOA messages (audio direction)
- Motor command messages
- Audio messages (mic/speaker)
- State/health messages
- Config messages

**Acceptance Criteria**:
- [ ] Protocol documented in shared types package
- [ ] Message types defined in Go structs
- [ ] JSON schema for each message type

---

#### EVA-102: go-eva Camera Proxy
**Priority**: P0 | **Estimate**: 2-3 days | **Repo**: go-eva

Add camera frame forwarding from Pollen WebRTC to WebSocket.

**Tasks**:
- [ ] Connect to Pollen WebRTC stream (port 8000)
- [ ] Decode H264 frames to JPEG
- [ ] Forward frames over WebSocket to cloud
- [ ] Configurable frame rate (default 10 FPS for bandwidth)
- [ ] Add `--enable-camera-proxy` flag

**Acceptance Criteria**:
- [ ] Camera frames visible in cloud go-reachy
- [ ] Configurable resolution/framerate
- [ ] <100ms latency

---

#### EVA-103: go-eva Motor Command Handler
**Priority**: P0 | **Estimate**: 1 day | **Repo**: go-eva

Receive motor commands from cloud WebSocket, forward to Pollen HTTP API.

**Tasks**:
- [ ] Add WebSocket command handler for "motor" type
- [ ] POST to `http://localhost:8000/api/move/set_target`
- [ ] Handle connection errors gracefully
- [ ] Add command rate limiting (30 Hz max)

**Acceptance Criteria**:
- [ ] Cloud can control robot head/antennas via WebSocket
- [ ] Smooth movement, no jitter
- [ ] Error handling for Pollen daemon down

---

#### EVA-104: go-eva Audio Bridge
**Priority**: P1 | **Estimate**: 2 days | **Repo**: go-eva

Bidirectional audio: microphone to cloud, cloud TTS to speaker.

**Tasks**:
- [ ] Capture microphone audio (ALSA or PulseAudio)
- [ ] Stream mic audio to cloud as PCM chunks
- [ ] Receive TTS audio from cloud
- [ ] Play TTS through robot speaker
- [ ] Handle audio format conversion if needed

**Acceptance Criteria**:
- [ ] Voice commands work through cloud
- [ ] TTS plays on robot speaker
- [ ] No audio glitches/dropouts

---

#### EVA-105: go-reachy WebSocket Client Mode
**Priority**: P0 | **Estimate**: 2 days | **Repo**: go-reachy

Refactor go-reachy to accept WebSocket connections instead of direct Pollen connection.

**Tasks**:
- [ ] Add `--mode=cloud` flag (vs `--mode=direct`)
- [ ] Implement WebSocket server for robot connections
- [ ] Route incoming frames to face detection
- [ ] Send motor commands over WebSocket
- [ ] Handle multiple robot connections (future)

**Acceptance Criteria**:
- [ ] go-reachy works with go-eva WebSocket connection
- [ ] Face tracking works over WebSocket
- [ ] Dashboard shows video from WebSocket

---

#### EVA-106: go-reachy Cloud Deployment
**Priority**: P1 | **Estimate**: 1 day | **Repo**: go-reachy

Prepare go-reachy for cloud deployment.

**Tasks**:
- [ ] Dockerfile for cloud deployment
- [ ] Environment variable configuration
- [ ] Health check endpoint
- [ ] Graceful shutdown handling
- [ ] Document deployment to fly.io/Railway/etc

**Acceptance Criteria**:
- [ ] Can deploy go-reachy to cloud provider
- [ ] Robot connects to cloud instance
- [ ] End-to-end face tracking works

---

#### EVA-107: Connection Management & Reconnection
**Priority**: P1 | **Estimate**: 1 day | **Repo**: go-eva

Robust connection handling for unreliable networks.

**Tasks**:
- [ ] Automatic reconnection with exponential backoff
- [ ] Connection health monitoring
- [ ] Offline mode (queue commands)
- [ ] Connection status reporting

**Acceptance Criteria**:
- [ ] Survives network interruptions
- [ ] Auto-reconnects within 30s
- [ ] No crash on disconnect

---

### Phase 2: Full Go Replacement (Future)

#### EVA-201: Research Pollen Motor Protocol
**Priority**: P2 | **Estimate**: 1 week | **Repo**: go-eva

Reverse-engineer how Pollen daemon controls motors.

**Tasks**:
- [ ] Analyze Pollen Python source code
- [ ] Document I2C/SPI protocol to motor drivers
- [ ] Identify motor driver chips
- [ ] Create protocol documentation

---

#### EVA-202: Go Motor Driver
**Priority**: P2 | **Estimate**: 2-3 weeks | **Repo**: go-eva

Direct motor control in Go, bypassing Python.

**Tasks**:
- [ ] Implement I2C/SPI communication
- [ ] Motor position control
- [ ] Smooth interpolation
- [ ] Safety limits

---

#### EVA-203: Go Camera Capture
**Priority**: P2 | **Estimate**: 1 week | **Repo**: go-eva

Direct camera access via V4L2/libcamera.

**Tasks**:
- [ ] V4L2 or libcamera bindings
- [ ] MJPEG/H264 encoding
- [ ] Resolution/framerate control

---

#### EVA-204: Go Audio I/O
**Priority**: P2 | **Estimate**: 1 week | **Repo**: go-eva

Direct audio via ALSA.

**Tasks**:
- [ ] ALSA capture (microphone)
- [ ] ALSA playback (speaker)
- [ ] Audio format handling

---

## Timeline

| Phase | Duration | Outcome |
|-------|----------|---------|
| Phase 1 | 1-2 weeks | Cloud architecture working with Pollen proxy |
| Phase 2 | 2-3 months | Full Go replacement, no Python dependency |

## Success Metrics

- [ ] Robot CPU usage <20% (vs current ~100%+)
- [ ] End-to-end latency <200ms
- [ ] 99.9% uptime for cloud service
- [ ] Support 10+ concurrent robots (Phase 2)

## Dependencies

- go-eva v1.1.0+ (current)
- go-reachy camera-config-api branch (merging now)
- Cloud provider account (fly.io, Railway, or AWS)

## Risks

| Risk | Mitigation |
|------|------------|
| WebSocket latency | Optimize protocol, use binary encoding |
| Pollen API changes | Pin Pollen version, abstract API calls |
| Network reliability | Robust reconnection, offline queuing |
| Audio sync | Timestamp-based synchronization |



