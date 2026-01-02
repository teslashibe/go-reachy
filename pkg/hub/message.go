// Package hub provides a thread-safe websocket broadcast hub
// using the idiomatic Go channel-based fan-out pattern.
package hub

// MessageType indicates the websocket message format
type MessageType int

const (
	// JSONMessage is a JSON-encoded message
	JSONMessage MessageType = iota
	// BinaryMessage is raw binary data (e.g., JPEG frames)
	BinaryMessage
)

// Message represents a message to be broadcast to clients
type Message struct {
	Type MessageType
	Data []byte
}

// NewJSONMessage creates a JSON message from pre-encoded bytes
func NewJSONMessage(data []byte) Message {
	return Message{Type: JSONMessage, Data: data}
}

// NewBinaryMessage creates a binary message
func NewBinaryMessage(data []byte) Message {
	return Message{Type: BinaryMessage, Data: data}
}
