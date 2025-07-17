package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
)

// MCPServer handles Model Context Protocol connections
type MCPServer struct {
	gameClient  *GameClient
	input       *json.Decoder
	output      *json.Encoder
	outputMutex sync.Mutex
	eventChan   chan *SocialEvent
	done        chan bool
}

// MCPRequest represents a JSON-RPC request
type MCPRequest struct {
	ID     interface{}            `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC response
type MCPResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// NewMCPServer creates a new MCP server instance
func NewMCPServer(gameClient *GameClient) *MCPServer {
	return &MCPServer{
		gameClient: gameClient,
		input:      json.NewDecoder(os.Stdin),
		output:     json.NewEncoder(os.Stdout),
		eventChan:  make(chan *SocialEvent, 100),
		done:       make(chan bool),
	}
}

// Start begins the MCP server operation
func (s *MCPServer) Start() {
	log.Println("MCP server started")
	
	// Start event pump in background
	go s.eventPump()
	
	// For now, just read one message and exit
	var req MCPRequest
	if err := s.input.Decode(&req); err != nil {
		log.Printf("Failed to decode request: %v", err)
		return
	}
	
	log.Printf("Received request: method=%s id=%v", req.Method, req.ID)
	
	// Send a simple response
	resp := MCPResponse{
		ID: req.ID,
		Result: map[string]string{
			"status": "MCP server is running",
		},
	}
	
	s.outputMutex.Lock()
	if err := s.output.Encode(&resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
	s.outputMutex.Unlock()
	
	fmt.Println("MCP server shutting down")
}

// eventPump handles incoming social events
func (s *MCPServer) eventPump() {
	for {
		select {
		case event := <-s.eventChan:
			log.Printf("MCP received social event: %s from %s", event.Subtype, event.From)
			// In Phase 2 we'll send these to subscribed clients
		case <-s.done:
			return
		}
	}
}

// GetEventChannel returns the channel for sending events to MCP
func (s *MCPServer) GetEventChannel() chan<- *SocialEvent {
	return s.eventChan
}