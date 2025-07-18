package game

import (
	"container/ring"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"dr-charm/internal/social"
)

// GameClient manages the game connection and output buffer
// (renamed from GameAPI to better reflect its purpose)
type GameClient struct {
	gameConn       net.Conn
	outputBuffer   *ring.Ring
	mu             sync.RWMutex
	lastID         atomic.Uint64
	connected      atomic.Bool
	character      string
	socialBuffer   *social.SocialEventBuffer
	mcpEventChan   chan<- *social.SocialEvent // Channel to send events to MCP server
	lastActivity   time.Time
	activityMu     sync.RWMutex
	reconnectChan  chan bool // Channel to trigger reconnection attempts
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
		gameConn:      gameConn,
		outputBuffer:  ring.New(1000), // Default buffer size
		character:     character,
		socialBuffer:  social.NewSocialEventBuffer(1000), // 1000 social events
		lastActivity:  time.Now(),
		reconnectChan: make(chan bool, 1),
	}
	client.connected.Store(true)
	
	// Start connection monitor
	go client.monitorConnection()
	
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
		return fmt.Errorf("not connected to game server")
	}

	_, err := gc.gameConn.Write([]byte(command + "\n"))
	if err != nil {
		gc.connected.Store(false)
		// Notify MCP about disconnection
		if gc.mcpEventChan != nil {
			event := &social.SocialEvent{
				Type:      "system",
				Subtype:   "connection_lost",
				Timestamp: time.Now().Unix(),
				Raw:       fmt.Sprintf("Connection lost: %v", err),
			}
			select {
			case gc.mcpEventChan <- event:
			default:
			}
		}
		return err
	}

	// Update activity on successful send
	gc.UpdateActivity()
	// Record the command in output
	gc.AddOutput("> " + command)
	return nil
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
func (gc *GameClient) GetSocialBuffer() *social.SocialEventBuffer {
	return gc.socialBuffer
}

// SetMCPEventChannel sets the channel for sending events to MCP
func (gc *GameClient) SetMCPEventChannel(ch chan<- *social.SocialEvent) {
	gc.mcpEventChan = ch
}

// AddSocialEvent adds a social event to the buffer and sends to MCP if connected
func (gc *GameClient) AddSocialEvent(event *social.SocialEvent) {
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

// monitorConnection monitors the connection health
func (gc *GameClient) monitorConnection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Check if we've received data recently
			gc.activityMu.RLock()
			lastActivity := gc.lastActivity
			gc.activityMu.RUnlock()
			
			if time.Since(lastActivity) > 60*time.Second {
				// No activity for 60 seconds, connection might be dead
				if gc.connected.Load() {
					gc.connected.Store(false)
					fmt.Fprintf(os.Stderr, "[GameClient] Connection appears to be dead (no activity for 60s)\n")
					
					// Notify MCP about disconnection
					if gc.mcpEventChan != nil {
						event := &social.SocialEvent{
							Type:      "system",
							Subtype:   "connection_lost",
							Timestamp: time.Now().Unix(),
							Raw:       "Connection to game server lost",
						}
						select {
						case gc.mcpEventChan <- event:
						default:
						}
					}
				}
			}
		case <-gc.reconnectChan:
			// Reconnection requested
			fmt.Fprintf(os.Stderr, "[GameClient] Reconnection requested\n")
			// Note: Actual reconnection would require storing credentials
			// and re-authenticating, which is beyond current scope
		}
	}
}

// UpdateActivity updates the last activity timestamp
func (gc *GameClient) UpdateActivity() {
	gc.activityMu.Lock()
	gc.lastActivity = time.Now()
	gc.activityMu.Unlock()
}

// IsConnected returns the current connection status
func (gc *GameClient) IsConnected() bool {
	return gc.connected.Load()
}

// SetConnected updates the connection status
func (gc *GameClient) SetConnected(connected bool) {
	gc.connected.Store(connected)
}

// GetConnectionInfo returns connection status information
func (gc *GameClient) GetConnectionInfo() map[string]interface{} {
	gc.activityMu.RLock()
	lastActivity := gc.lastActivity
	gc.activityMu.RUnlock()
	
	return map[string]interface{}{
		"connected":      gc.connected.Load(),
		"character":      gc.character,
		"last_activity":  lastActivity.Format(time.RFC3339),
		"idle_seconds":   int(time.Since(lastActivity).Seconds()),
	}
}
