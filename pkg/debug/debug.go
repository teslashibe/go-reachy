// Package debug provides a global debug logging flag
package debug

import "fmt"

// Enabled controls whether debug logging is active
var Enabled bool

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
