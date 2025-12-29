# Setup Guide

## Prerequisites

- **Go 1.25** installed on your development machine
- **Reachy Mini robot** powered on and connected to your network
- **SSH access** to the robot (password: `root`)

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/teslashibe/go-reachy.git
cd go-reachy
```

### 2. Build

```bash
# Build for your machine (development/testing)
go build ./...

# Build the dance demo
go build -o dance ./cmd/dance
```

### 3. Find Your Robot's IP

```bash
# Option 1: mDNS (if supported)
ping reachy-mini.local

# Option 2: Check your router's DHCP client list

# Option 3: SSH and check
ssh pollen@reachy-mini.local "ip a | grep inet"
```

### 4. Run

```bash
# Run the dance demo (replace with your robot's IP)
./dance
```

## Running on the Robot (Standalone Mode)

### Cross-Compile for ARM64

```bash
GOOS=linux GOARCH=arm64 go build -o dance-arm64 ./cmd/dance
```

### Deploy to Robot

```bash
# Using sshpass for automation
sshpass -p "root" scp dance-arm64 pollen@reachy-mini.local:~/dance

# Or manually
scp dance-arm64 pollen@reachy-mini.local:~/dance
```

### Run on Robot

```bash
ssh pollen@reachy-mini.local
chmod +x ~/dance
./dance
```

## Robot Configuration

### Default Credentials

| Field | Value |
|-------|-------|
| Hostname | `reachy-mini.local` |
| Username | `pollen` |
| Password | `root` |
| HTTP API | `http://<IP>:8000` |
| Zenoh | `tcp://<IP>:7447` |

### Starting the Robot Daemon

If the robot isn't responding, the daemon might not be running:

```bash
# Check status
curl http://<ROBOT_IP>:8000/api/daemon/status

# Start daemon and wake up motors
curl -X POST "http://<ROBOT_IP>:8000/api/daemon/start?wake_up=true"
```

## Development

### Project Structure

```
go-reachy/
├── cmd/
│   ├── reachy/          # Main CLI (WIP)
│   ├── poc/             # Proof of concept
│   └── dance/           # Dance demo
├── pkg/
│   ├── robot/           # Robot control library
│   └── speech/          # OpenAI integration (WIP)
├── docs/                # Documentation
├── go.mod
└── README.md
```

### Adding New Features

1. Create a new command in `cmd/yourfeature/main.go`
2. Use the `pkg/robot` package for robot control
3. Build and test:
   ```bash
   go build ./cmd/yourfeature
   ./yourfeature
   ```

### Testing on Robot

```bash
# One-liner: build, deploy, and run
GOOS=linux GOARCH=arm64 go build -o app-arm64 ./cmd/yourfeature && \
sshpass -p "root" scp app-arm64 pollen@reachy-mini.local:~/app && \
sshpass -p "root" ssh pollen@reachy-mini.local "./app"
```

## Troubleshooting

### "Connection refused" on port 8000

The daemon isn't running. SSH to the robot and start it:

```bash
ssh pollen@reachy-mini.local
cd /venvs/mini_daemon
source bin/activate
python -m reachy_mini.daemon.app.main --autostart
```

### Robot not moving

1. Check if motors are enabled:
   ```bash
   curl http://<IP>:8000/api/daemon/status
   ```
   Look for `"motors_on": true`

2. Start with wake_up:
   ```bash
   curl -X POST "http://<IP>:8000/api/daemon/start?wake_up=true"
   ```

### Can't find robot IP

The robot's IP may have changed. Options:
1. Check your router's DHCP leases
2. Use `arp -a` after pinging the broadcast address
3. Connect a monitor/keyboard to the robot and run `ip a`

### Build fails with "module not found"

Run:
```bash
go mod tidy
```

