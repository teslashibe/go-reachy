// Package debug provides global debug logging flags
package debug

import "fmt"

// Enabled controls whether debug logging is active
var Enabled bool

// Tracking controls whether verbose tracking logs are shown (face, body, audio DOA)
// Use --debug-tracking flag to enable these very verbose logs
var Tracking bool

// Log prints a message only if debug mode is enabled
func Log(format string, args ...interface{}) {
	if Enabled {
		fmt.Printf(format, args...)
	}
}

// Logln prints a message with newline only if debug mode is enabled
func Logln(msg string) {
	if Enabled {
		fmt.Println(msg)
	}
}

// TrackLog prints a message only if tracking debug mode is enabled
func TrackLog(format string, args ...interface{}) {
	if Tracking {
		fmt.Printf(format, args...)
	}
}

// TrackLogln prints a message with newline only if tracking debug mode is enabled
func TrackLogln(msg string) {
	if Tracking {
		fmt.Println(msg)
	}
}

