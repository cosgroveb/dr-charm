package main

import (
	"container/ring"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// GameClient manages the game connection and output buffer
// (renamed from GameAPI to better reflect its purpose)
type GameClient struct {
	gameConn     net.Conn
	outputBuffer *ring.Ring
	mu           sync.RWMutex
	lastID       atomic.Uint64
	connected    atomic.Bool
	character    string
	socialBuffer *SocialEventBuffer
	mcpEventChan chan<- *SocialEvent // Channel to send events to MCP server
}

// OutputLine represents a line of game output with metadata
type OutputLine struct {
	ID        uint64    `json:"id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// NewGameClient creates a new game client instance
func NewGameClient(gameConn net.Conn, character string) *GameClient {
	client := &GameClient{
		gameConn:     gameConn,
		outputBuffer: ring.New(1000), // Default buffer size
		character:    character,
		socialBuffer: NewSocialEventBuffer(1000), // 1000 social events
	}
	client.connected.Store(true)
	return client
}

// AddOutput adds a line of output to the buffer
func (gc *GameClient) AddOutput(text string) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	id := gc.lastID.Add(1)
	line := OutputLine{
		ID:        id,
		Text:      text,
		Timestamp: time.Now(),
	}
	gc.outputBuffer.Value = line
	gc.outputBuffer = gc.outputBuffer.Next()
}

// SendCommand sends a command to the game
func (gc *GameClient) SendCommand(command string) error {
	if !gc.connected.Load() {
		return net.ErrClosed
	}

	_, err := gc.gameConn.Write([]byte(command + "\n"))
	if err != nil {
		gc.connected.Store(false)
		return err
	}

	// Record the command in output
	gc.AddOutput("> " + command)
	return nil
}

// IsConnected returns whether the game connection is active
func (gc *GameClient) IsConnected() bool {
	return gc.connected.Load()
}

// SetDisconnected marks the connection as disconnected
func (gc *GameClient) SetDisconnected() {
	gc.connected.Store(false)
}

// GetConnection returns the underlying connection
func (gc *GameClient) GetConnection() net.Conn {
	return gc.gameConn
}

// GetCharacter returns the character name
func (gc *GameClient) GetCharacter() string {
	return gc.character
}

// GetSocialBuffer returns the social event buffer
func (gc *GameClient) GetSocialBuffer() *SocialEventBuffer {
	return gc.socialBuffer
}

// SetMCPEventChannel sets the channel for sending events to MCP
func (gc *GameClient) SetMCPEventChannel(ch chan<- *SocialEvent) {
	gc.mcpEventChan = ch
}

// AddSocialEvent adds a social event to the buffer and sends to MCP if connected
func (gc *GameClient) AddSocialEvent(event *SocialEvent) {
	// Add to buffer
	gc.socialBuffer.Add(event)
	
	// Debug log to stderr
	fmt.Fprintf(os.Stderr, "[GameClient] Added social event: %s from %s\n", event.Subtype, event.From)
	
	// Send to MCP if channel is set
	if gc.mcpEventChan != nil {
		select {
		case gc.mcpEventChan <- event:
			// Sent successfully
		default:
			// Channel full or closed, skip
		}
	}
}
