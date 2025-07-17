package main

import (
	"container/ring"
	"net"
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
