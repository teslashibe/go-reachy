#!/bin/bash
# Stream logs from Reachy Mini robot to local terminal
# Usage: ./scripts/stream-robot-logs.sh [robot-ip]

ROBOT_IP="${1:-192.168.68.83}"
SSH_PASS="root"

echo "🤖 Streaming logs from robot at $ROBOT_IP..."
echo "   Press Ctrl+C to stop"
echo ""

# Use sshpass if available, otherwise prompt for password
if command -v sshpass &> /dev/null; then
    sshpass -p "$SSH_PASS" ssh -o StrictHostKeyChecking=no pollen@$ROBOT_IP \
        "journalctl -f -u reachy-mini-daemon --no-pager"
else
    echo "Note: Install sshpass for passwordless connection"
    ssh -o StrictHostKeyChecking=no pollen@$ROBOT_IP \
        "journalctl -f -u reachy-mini-daemon --no-pager"
fi

