// Package config provides configuration helpers for go-reachy commands.
package config

import (
	"fmt"
	"os"
)

// Default robot configuration.
const (
	DefaultRobotPort = "8000"
	DefaultSSHUser   = "pollen"
	DefaultSSHPass   = "root"
)

// RobotIP returns the robot IP from ROBOT_IP env var.
// Falls back to the provided default if not set.
func RobotIP(defaultIP string) string {
	if ip := os.Getenv("ROBOT_IP"); ip != "" {
		return ip
	}
	return defaultIP
}

// RobotIPRequired returns the robot IP from ROBOT_IP env var.
// Panics if not set.
func RobotIPRequired() string {
	ip := os.Getenv("ROBOT_IP")
	if ip == "" {
		fmt.Fprintln(os.Stderr, "Error: ROBOT_IP environment variable is required")
		fmt.Fprintln(os.Stderr, "Usage: ROBOT_IP=192.168.68.80 go run ./cmd/...")
		os.Exit(1)
	}
	return ip
}

// RobotAPIURL returns the robot HTTP API URL.
func RobotAPIURL(robotIP string) string {
	return fmt.Sprintf("http://%s:%s", robotIP, DefaultRobotPort)
}

// SSHUser returns the SSH username from SSH_USER env var or default.
func SSHUser() string {
	if user := os.Getenv("SSH_USER"); user != "" {
		return user
	}
	return DefaultSSHUser
}

// SSHPass returns the SSH password from SSH_PASS env var or default.
func SSHPass() string {
	if pass := os.Getenv("SSH_PASS"); pass != "" {
		return pass
	}
	return DefaultSSHPass
}


