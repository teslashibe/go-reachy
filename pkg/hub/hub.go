package hub

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Hub maintains the set of active clients and broadcasts messages to them
type Hub struct {
	// Name for logging
	name string

	// Registered clients
	clients map[*Client]bool

	// Inbound messages to broadcast
	broadcast chan Message

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for client count (read-only access from outside)
	mu sync.RWMutex

	// Running state
	running bool
}

// New creates a new Hub
func New(name string) *Hub {
	return &Hub{
		name:       name,
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
// This should be called in a goroutine
func (h *Hub) Run() {
	h.running = true
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			fmt.Printf("🔌 [%s] Client connected (%d total)\n", h.name, count)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			count := len(h.clients)
			h.mu.Unlock()
			fmt.Printf("🔌 [%s] Client disconnected (%d remaining)\n", h.name, count)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
					// Message queued successfully
				default:
					// Client's buffer is full - they're too slow
					// Close and remove them
					close(client.send)
					delete(h.clients, client)
					fmt.Printf("⚠️  [%s] Dropped slow client\n", h.name)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(msg Message) {
	select {
	case h.broadcast <- msg:
	default:
		// Broadcast channel full - drop message
		fmt.Printf("⚠️  [%s] Broadcast channel full, dropping message\n", h.name)
	}
}

// BroadcastJSON encodes and broadcasts a JSON message
func (h *Hub) BroadcastJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.Broadcast(NewJSONMessage(data))
	return nil
}

// BroadcastBinary broadcasts binary data (e.g., camera frames)
func (h *Hub) BroadcastBinary(data []byte) {
	h.Broadcast(NewBinaryMessage(data))
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// IsRunning returns whether the hub is running
func (h *Hub) IsRunning() bool {
	return h.running
}



