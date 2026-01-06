# Reachy Mini Troubleshooting Guide

## Quick Reference

### Check Robot Status
```bash
# Check API
curl -s "http://192.168.68.80:8000/api/daemon/status" | jq -r '.state'

# Check WebRTC
nc -z 192.168.68.80 8443 && echo "WebRTC: ✅" || echo "WebRTC: ❌"

# Check if daemon is running
sshpass -p "root" ssh pollen@192.168.68.80 "ps aux | grep reachy_mini.daemon | grep -v grep"
```

### Start Daemon via Systemd (Recommended)
```bash
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S systemctl start reachy-mini-daemon"
```

### Restart Daemon
```bash
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S systemctl restart reachy-mini-daemon"
```

### Check Daemon Logs
```bash
sshpass -p "root" ssh pollen@192.168.68.80 "journalctl -u reachy-mini-daemon -n 30 --no-pager"
```

---

## Common Issues and Fixes

### 1. API Not Responding (Port 8000 Closed)

**Symptoms:**
- `curl` to port 8000 times out
- `nc -z 192.168.68.80 8000` fails

**Cause:** Daemon isn't running or crashed.

**Fix:**
```bash
# Start via systemd (keeps it running after SSH disconnects)
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S systemctl start reachy-mini-daemon"
```

**Why this happens:** Running the daemon manually via SSH kills it when SSH disconnects. Always use systemd.

---

### 2. WebRTC Not Available (Port 8443 Closed)

**Symptoms:**
- API works (port 8000) but WebRTC fails (port 8443)
- Go app fails with "signalling connect failed"

**Cause:** The launcher script may not have WebRTC enabled, OR there's a camera conflict.

**Check launcher script:**
```bash
sshpass -p "root" ssh pollen@192.168.68.80 "cat /venvs/mini_daemon/lib/python3.12/site-packages/reachy_mini/daemon/app/services/wireless/launcher.sh"
```

**Working launcher.sh (without --stream-media):**
```bash
#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/generate_asoundrc.sh"
source /venvs/mini_daemon/bin/activate
export GST_PLUGIN_PATH=/opt/gst-plugins-rs/lib/aarch64-linux-gnu/gstreamer-1.0:$GST_PLUGIN_PATH
python -u -m reachy_mini.daemon.app.main --wireless-version --autostart
```

**Note:** The `--autostart` flag is enough to get WebRTC working. The `--stream-media` flag causes camera conflicts.

---

### 3. "Camera not found" Error

**Symptoms:**
```
RuntimeError: Camera not found
ERROR:uvicorn.error:Application startup failed. Exiting.
```

**Cause:** The `--stream-media` flag triggers OpenCV camera detection which fails for CSI cameras (IMX708).

**Fix:** Remove `--stream-media` from launcher.sh:
```bash
sshpass -p "root" ssh pollen@192.168.68.80 "sed -i 's/ --websocket-uri ws:\/\/localhost:8443 --stream-media//' /venvs/mini_daemon/lib/python3.12/site-packages/reachy_mini/daemon/app/services/wireless/launcher.sh"
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S systemctl restart reachy-mini-daemon"
```

---

### 4. "Camera is already in use" Error

**Symptoms:**
```
ERROR:reachy_mini.media.webrtc_daemon:Error: Camera '/base/soc/i2c0mux/i2c@0/imx708@1a' is already in use.
```

**Cause:** Two pipelines trying to access the camera simultaneously (MediaManager + WebRTC daemon).

**Fix:** Don't use `--stream-media` flag. The WebRTC daemon handles camera access internally.

---

### 5. "Device or resource busy" (Serial Port)

**Symptoms:**
```
OSError: Device or resource busy
```

**Cause:** Previous daemon process still holding the serial port.

**Fix:**
```bash
# Kill all Python processes
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S pkill -9 -f python"

# Wait and restart
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S systemctl restart reachy-mini-daemon"
```

---

### 6. UnixFdSink/UnixFdSrc Errors

**Symptoms:**
```
ERROR: GstUnixFdSink:unixfdsink0: Failed to start
ERROR: GstUnixFdSrc:unixfdsrc0: Failed to start
```

**Cause:** Stale camera socket or pipeline initialization order issues.

**Fix:**
```bash
# Remove stale socket
sshpass -p "root" ssh pollen@192.168.68.80 "rm -f /tmp/reachymini_camera_socket"

# Restart daemon
sshpass -p "root" ssh pollen@192.168.68.80 "echo 'root' | sudo -S systemctl restart reachy-mini-daemon"
```

---

### 7. Daemon Keeps Dying After SSH Disconnect

**Cause:** Running daemon manually without proper daemonization.

**Wrong way:**
```bash
ssh pollen@192.168.68.80 "python -m reachy_mini.daemon.app.main ..."  # Dies when SSH ends
```

**Right way:**
```bash
ssh pollen@192.168.68.80 "sudo systemctl start reachy-mini-daemon"  # Survives SSH disconnect
```

---

## Key Files on Robot

| File | Purpose |
|------|---------|
| `/etc/systemd/system/reachy-mini-daemon.service` | Systemd service definition |
| `/venvs/mini_daemon/lib/python3.12/site-packages/reachy_mini/daemon/app/services/wireless/launcher.sh` | Startup script with flags |
| `/tmp/reachymini_camera_socket` | Unix socket for camera data |
| `/opt/gst-plugins-rs/lib/aarch64-linux-gnu/gstreamer-1.0/libgstrswebrtc.so` | WebRTC GStreamer plugin |

---

## GST_PLUGIN_PATH

The GStreamer WebRTC plugin requires this path:
```bash
export GST_PLUGIN_PATH=/opt/gst-plugins-rs/lib/aarch64-linux-gnu/gstreamer-1.0:$GST_PLUGIN_PATH
```

This is set in `launcher.sh`. If WebRTC fails to create `webrtcsink`, check this path.

---

## Robot Credentials

- **IP:** `192.168.68.80` (may change on reboot - see below)
- **SSH User:** `pollen`
- **SSH Password:** `root`

---

## Finding Robot IP (After Reboot)

The robot's IP can change after a reboot. Use this to scan for it:

```bash
# Quick scan for Reachy API on local network
for ip in 192.168.68.{50..100}; do 
  (curl -s --connect-timeout 1 "http://$ip:8000/api/daemon/status" >/dev/null 2>&1 && echo "Found Reachy at $ip") & 
done; wait
```

Or check the ARP cache after pinging:
```bash
arp -a | grep "192.168.68"
```

---

## Full Recovery Procedure

If everything is broken:

```bash
# 1. Power cycle the robot (physically)

# 2. Wait 60 seconds for boot

# 3. Find robot IP
ping 192.168.68.80  # or check router

# 4. Start daemon via systemd
sshpass -p "root" ssh -o StrictHostKeyChecking=no pollen@192.168.68.80 "echo 'root' | sudo -S systemctl start reachy-mini-daemon"

# 5. Wait 30 seconds for initialization

# 6. Verify
curl -s "http://192.168.68.80:8000/api/daemon/status" | jq -r '.state'
nc -z 192.168.68.80 8443 && echo "WebRTC: ✅"

# 7. Run your app
cd /Users/brendanplayford/teslashibe/go-reachy
export GEMINI_API_KEY="..."
export OPENAI_API_KEY="..."
./travis
```

---

## Debug Commands for Crash Investigation

### Check Temperature and Throttling Status
```bash
sshpass -p "root" ssh pollen@192.168.68.83 "vcgencmd measure_temp && vcgencmd get_throttled"
```

**Throttle flags interpretation:**
| Bit | Meaning |
|-----|---------|
| 0 | Under-voltage detected |
| 1 | Arm frequency capped |
| 2 | Currently throttled |
| 3 | Soft temperature limit active |
| 16 | Under-voltage has occurred |
| 17 | Arm frequency capping has occurred |
| 18 | Throttling has occurred |
| 19 | Soft temperature limit has occurred |

`throttled=0x0` = No issues. `throttled=0x50005` = Under-voltage + throttling occurred.

### Check Kernel Messages (dmesg)
```bash
# Look for OOM kills, panics, thermal issues
sshpass -p "root" ssh pollen@192.168.68.83 "dmesg | grep -iE 'killed|oom|error|fail|panic|crash|voltage|throttl' | tail -30"
```

### Check Daemon Logs
```bash
# Recent daemon logs
sshpass -p "root" ssh pollen@192.168.68.83 "journalctl -u reachy-mini-daemon --since '10 minutes ago' --no-pager | tail -50"

# Check previous boot (if persistent journaling enabled)
sshpass -p "root" ssh pollen@192.168.68.83 "journalctl -b -1 -u reachy-mini-daemon | tail -50"
```

### Check Memory and System Load
```bash
sshpass -p "root" ssh pollen@192.168.68.83 "free -m && echo '---' && top -bn1 | head -15"
```

### Check Port Status (API, WebRTC, Zenoh)
```bash
sshpass -p "root" ssh pollen@192.168.68.83 "netstat -tlnp 2>/dev/null | grep -E '7447|8000|8443'"
```

Expected output:
```
tcp  0  0  0.0.0.0:7447  0.0.0.0:*  LISTEN  950/python   # Zenoh
tcp  0  0  0.0.0.0:8000  0.0.0.0:*  LISTEN  950/python   # HTTP API
tcp  0  0  0.0.0.0:8443  0.0.0.0:*  LISTEN  950/python   # WebRTC signaling
```

### Check Voltage Levels
```bash
sshpass -p "root" ssh pollen@192.168.68.83 "vcgencmd measure_volts core && vcgencmd measure_volts sdram_c && vcgencmd measure_volts sdram_i && vcgencmd measure_volts sdram_p"
```

---

## Enable Persistent Journaling

By default, the robot doesn't persist logs across reboots. Enable it to debug crashes:

```bash
sshpass -p "root" ssh pollen@192.168.68.83 "echo 'root' | sudo -S mkdir -p /var/log/journal && echo 'root' | sudo -S systemd-tmpfiles --create --prefix /var/log/journal && echo 'root' | sudo -S systemctl restart systemd-journald"
```

After enabling, you can view previous boot logs:
```bash
# List available boots
sshpass -p "root" ssh pollen@192.168.68.83 "journalctl --list-boots"

# View previous boot
sshpass -p "root" ssh pollen@192.168.68.83 "journalctl -b -1 | tail -100"
```

---

## Known Issues

### 8. Camera Autofocus I2C Errors (dw9807)

**Symptoms (in dmesg):**
```
dw9807 10-000c: I2C write CTL fail ret = -5
dw9807 10-000c: dw9807_ramp I2C failure: -5
imx708 10-001a: probe with driver imx708 failed with error -5
```

**Cause:** The IMX708 camera's autofocus driver (dw9807) has intermittent I2C communication failures.

**Impact:** Usually non-critical - the camera still works but autofocus may not function properly.

**Status:** Known hardware/driver issue. No fix available. Monitor but don't panic.

---

### 9. Robot Freezes During Heavy Movement (WebRTC Disconnection)

**Symptoms:**
- Robot stops responding during conversation
- go-reachy logs show: `Connection state: disconnected` then `Connection state: failed`
- No fan spin-up (not thermal)
- Temperature remains low (~38-45°C)

**Root Cause:** WebRTC connection becomes unstable under load. When the video/audio pipeline fails, the Python daemon's async event loop may stall.

**Mitigation:**
1. Reduce camera resolution (if supported)
2. Use Zenoh for motor control (bypasses HTTP bottleneck)
3. Enable dead-zone filtering to reduce command frequency

**go-reachy command with optimizations:**
```bash
./eva --transport=zenoh --voice=charlotte --tts=elevenlabs-ws
```

---

### 10. Robot Unresponsive But No Thermal Issues

**Investigation checklist:**
1. Check temperature: `vcgencmd measure_temp` (should be <80°C)
2. Check throttling: `vcgencmd get_throttled` (should be 0x0)
3. Check memory: `free -m` (should have >500MB available)
4. Check daemon: `systemctl status reachy-mini-daemon`
5. Check WebRTC port: `nc -z localhost 8443`

**If daemon is running but WebRTC port is closed:**
```bash
sshpass -p "root" ssh pollen@192.168.68.83 "echo 'root' | sudo -S systemctl restart reachy-mini-daemon"
```

---

## Performance Tuning

### Reduce WebRTC Video Resolution (RECOMMENDED)

The default 1080p video encoding consumes ~54% CPU. Reducing to 720p saves ~40% CPU:

```bash
# SSH to robot
ssh pollen@192.168.68.83

# Change default resolution from 1080p to 720p
sudo sed -i 's/R1920x1080at30fps/R1280x720at30fps/g' \
  /venvs/mini_daemon/lib/python3.12/site-packages/reachy_mini/media/camera_constants.py

# Restart daemon
sudo systemctl restart reachy-mini-daemon
```

| Resolution | Pixels | Encoding CPU | Status |
|------------|--------|--------------|--------|
| 1920x1080 @ 30fps | 2,073,600 | ~54% | ❌ Unstable |
| **1536x864 @ 40fps** | 1,327,104 | **~30-35%** | ✅ **Recommended** |
| 1280x720 @ 30fps | 921,600 | ~25% | ✅ Stable (fallback) |

To set 1536x864 (recommended balance of quality/performance):
```bash
sudo sed -i 's/default_resolution = CameraResolution.R[^/]*/default_resolution = CameraResolution.R1536x864at40fps/' \
  /venvs/mini_daemon/lib/python3.12/site-packages/reachy_mini/media/camera_constants.py
sudo systemctl restart reachy-mini-daemon
```

### Reduce go-reachy Control Loop Frequency
The default control loop runs at 20Hz. If experiencing issues, this can be reduced by modifying `cmd/eva/main.go`:
```go
rateCtrl = robot.NewRateController(robotCtrl, 100*time.Millisecond)  // 10Hz instead of 20Hz
```

### Reduce Tracking Detection Rate
The face tracking runs at 10Hz by default. Can be further reduced in `pkg/tracking/config.go`:
```go
DetectionInterval: 200 * time.Millisecond,  // 5Hz instead of 10Hz
```

---

## Robot Hardware Info

| Component | Details |
|-----------|---------|
| Board | Raspberry Pi 5 (4GB) |
| Camera | IMX708 Wide (CSI) |
| OS | Raspberry Pi OS (Bookworm) |
| Python | 3.12 |
| Daemon | reachy_mini v1.x |
| Motor Control | Zenoh (port 7447) |
| HTTP API | Uvicorn (port 8000) |
| WebRTC Signaling | GStreamer rswebrtc (port 8443) |

