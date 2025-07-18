package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"dr-charm/internal/game"
	"dr-charm/internal/social"
)

// MCPSSEServer handles Model Context Protocol Server-Sent Events
type MCPSSEServer struct {
	gameClient *game.GameClient
	port       int
	server     *http.Server
	clients    map[chan *SSEMessage]bool
	clientsMux sync.RWMutex
	eventChan  chan *social.SocialEvent
	done       chan bool
}

// SSEMessage represents a server-sent event
type SSEMessage struct {
	Event string      `json:"event,omitempty"`
	Data  interface{} `json:"data"`
	ID    string      `json:"id,omitempty"`
}

// NewMCPSSEServer creates a new MCP SSE server instance
func NewMCPSSEServer(gameClient *game.GameClient, port int) *MCPSSEServer {
	return &MCPSSEServer{
		gameClient: gameClient,
		port:       port,
		clients:    make(map[chan *SSEMessage]bool),
		eventChan:  make(chan *social.SocialEvent, 100),
		done:       make(chan bool),
	}
}

// Start begins the MCP SSE server operation
func (s *MCPSSEServer) Start() error {
	mux := http.NewServeMux()

	// SSE endpoint for events
	mux.HandleFunc("/events", s.handleSSE)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	// Start event broadcaster
	go s.eventBroadcaster()

	log.Printf("MCP SSE server starting on port %d", s.port)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *MCPSSEServer) Stop() error {
	close(s.done)
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleSSE handles SSE connections
func (s *MCPSSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for this client
	clientChan := make(chan *SSEMessage, 100)

	// Register the client
	s.clientsMux.Lock()
	s.clients[clientChan] = true
	s.clientsMux.Unlock()

	// Remove client on disconnect
	defer func() {
		s.clientsMux.Lock()
		delete(s.clients, clientChan)
		s.clientsMux.Unlock()
		close(clientChan)
	}()

	// Send initial connection event
	s.sendSSEMessage(w, &SSEMessage{
		Event: "connected",
		Data: map[string]interface{}{
			"message": "Connected to DragonRealms MCP SSE server",
			"version": "1.0.0",
		},
	})

	// Send recent social events
	recentEvents := s.gameClient.GetSocialBuffer().GetRecent(10)
	if len(recentEvents) > 0 {
		s.sendSSEMessage(w, &SSEMessage{
			Event: "social_history",
			Data:  recentEvents,
		})
	}

	// Create a ticker for keepalive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Listen for events to send to this client
	for {
		select {
		case msg := <-clientChan:
			if err := s.sendSSEMessage(w, msg); err != nil {
				log.Printf("Error sending SSE message: %v", err)
				return
			}

		case <-ticker.C:
			// Send keepalive comment
			fmt.Fprintf(w, ":keepalive\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case <-r.Context().Done():
			// Client disconnected
			return
		}
	}
}

// sendSSEMessage sends a message to a specific client
func (s *MCPSSEServer) sendSSEMessage(w http.ResponseWriter, msg *SSEMessage) error {
	// Format the SSE message
	if msg.Event != "" {
		fmt.Fprintf(w, "event: %s\n", msg.Event)
	}

	data, err := json.Marshal(msg.Data)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "data: %s\n", string(data))

	if msg.ID != "" {
		fmt.Fprintf(w, "id: %s\n", msg.ID)
	}

	fmt.Fprintf(w, "\n")

	// Flush the data immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// eventBroadcaster broadcasts events to all connected clients
func (s *MCPSSEServer) eventBroadcaster() {
	for {
		select {
		case event := <-s.eventChan:
			// Create SSE message for social event
			msg := &SSEMessage{
				Event: "social",
				Data:  event,
				ID:    fmt.Sprintf("%d", event.Timestamp),
			}

			// Broadcast to all clients
			s.clientsMux.RLock()
			for clientChan := range s.clients {
				select {
				case clientChan <- msg:
				default:
					// Client channel is full, skip
				}
			}
			s.clientsMux.RUnlock()

			log.Printf("Broadcasted social event: %s from %s to %d clients",
				event.Subtype, event.From, len(s.clients))

		case <-s.done:
			return
		}
	}
}

// GetEventChannel returns the channel for sending events to SSE
func (s *MCPSSEServer) GetEventChannel() chan<- *social.SocialEvent {
	return s.eventChan
}

// BroadcastGameState sends game state updates to all clients
func (s *MCPSSEServer) BroadcastGameState(state interface{}) {
	msg := &SSEMessage{
		Event: "game_state",
		Data:  state,
	}

	s.clientsMux.RLock()
	defer s.clientsMux.RUnlock()

	for clientChan := range s.clients {
		select {
		case clientChan <- msg:
		default:
			// Client channel is full, skip
		}
	}
}
