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

- **IP:** `192.168.68.80` (may change on reboot - check router DHCP)
- **SSH User:** `pollen`
- **SSH Password:** `root`

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

