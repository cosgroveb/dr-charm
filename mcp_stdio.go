package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// MCPStdioServer handles Model Context Protocol over stdio with IPC to main process
type MCPStdioServer struct {
	input       *json.Decoder
	output      *json.Encoder
	outputMutex sync.Mutex
	ipcConn     net.Conn
	eventChan   chan *SocialEvent
	done        chan bool
}

// NewMCPStdioServer creates a new MCP stdio server that connects to main process via IPC
func NewMCPStdioServer(ipcSocket string) (*MCPStdioServer, error) {
	// Connect to the main process via Unix socket with retries
	var conn net.Conn
	var err error
	
	for i := 0; i < 10; i++ {
		conn, err = net.Dial("unix", ipcSocket)
		if err == nil {
			break
		}
		if i < 9 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IPC socket after retries: %w", err)
	}
	
	return &MCPStdioServer{
		input:     json.NewDecoder(os.Stdin),
		output:    json.NewEncoder(os.Stdout),
		ipcConn:   conn,
		eventChan: make(chan *SocialEvent, 100),
		done:      make(chan bool),
	}, nil
}

// Start begins the MCP stdio server operation
func (s *MCPStdioServer) Start() {
	log.SetOutput(os.Stderr) // Log to stderr to not interfere with JSON-RPC
	log.Println("MCP stdio server started")
	
	// Start IPC event receiver
	go s.receiveIPCEvents()
	
	// Start event pump
	go s.eventPump()
	
	// Main request loop
	for {
		var req MCPRequest
		if err := s.input.Decode(&req); err != nil {
			if err == io.EOF {
				log.Println("MCP client disconnected")
			} else {
				log.Printf("Failed to decode request: %v", err)
			}
			break
		}
		
		log.Printf("Received request: method=%s id=%v", req.Method, req.ID)
		
		// Handle the request
		s.handleRequest(req)
	}
	
	close(s.done)
	s.ipcConn.Close()
}

// IPCMessage represents a message sent between processes
type IPCMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// IPCRequest represents a request from MCP to main process
type IPCRequest struct {
	ID     string                 `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// IPCResponse represents a response from main process to MCP
type IPCResponse struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *MCPError   `json:"error,omitempty"`
}

// handleRequest processes an incoming MCP request
func (s *MCPStdioServer) handleRequest(req MCPRequest) {
	var resp MCPResponse
	resp.ID = req.ID
	
	switch req.Method {
	case "initialize":
		// Handle initialization request
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name": "dr-charm-mcp",
				"version": "1.0.0",
			},
		}
		s.sendResponse(resp)
		
	case "notifications/initialized":
		// Client has acknowledged initialization
		log.Println("MCP client initialized")
		return // No response needed for notifications
		
	case "tools/list":
		// Return list of available tools
		resp.Result = s.getToolsList()
		s.sendResponse(resp)
		
	case "tools/call":
		// Forward tool call to main process via IPC
		if toolName, ok := req.Params["name"].(string); ok {
			if args, ok := req.Params["arguments"].(map[string]interface{}); ok {
				// Send IPC request to main process
				ipcReq := IPCRequest{
					ID:     fmt.Sprintf("%v", req.ID),
					Method: toolName,
					Params: args,
				}
				
				result, err := s.sendIPCRequest(ipcReq)
				if err != nil {
					resp.Error = &MCPError{
						Code:    InternalError,
						Message: err.Error(),
					}
				} else {
					resp.Result = result
				}
			} else {
				resp.Error = &MCPError{
					Code:    InvalidParams,
					Message: "Missing or invalid 'arguments' parameter",
				}
			}
		} else {
			resp.Error = &MCPError{
				Code:    InvalidParams,
				Message: "Missing 'name' parameter",
			}
		}
		s.sendResponse(resp)
		
	default:
		resp.Error = &MCPError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Unknown method: %s", req.Method),
		}
		s.sendResponse(resp)
	}
}

// sendIPCRequest sends a request to the main process and waits for response
func (s *MCPStdioServer) sendIPCRequest(req IPCRequest) (interface{}, error) {
	// Encode request
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal IPC request: %w", err)
	}
	
	msg := IPCMessage{
		Type:    "request",
		Payload: reqData,
	}
	
	// Send to main process
	encoder := json.NewEncoder(s.ipcConn)
	if err := encoder.Encode(msg); err != nil {
		return nil, fmt.Errorf("failed to send IPC request: %w", err)
	}
	
	// Wait for response
	decoder := json.NewDecoder(s.ipcConn)
	var respMsg IPCMessage
	if err := decoder.Decode(&respMsg); err != nil {
		return nil, fmt.Errorf("failed to receive IPC response: %w", err)
	}
	
	if respMsg.Type != "response" {
		return nil, fmt.Errorf("unexpected IPC message type: %s", respMsg.Type)
	}
	
	var resp IPCResponse
	if err := json.Unmarshal(respMsg.Payload, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal IPC response: %w", err)
	}
	
	if resp.Error != nil {
		return nil, fmt.Errorf("%s", resp.Error.Message)
	}
	
	return resp.Result, nil
}

// receiveIPCEvents receives social events from the main process
func (s *MCPStdioServer) receiveIPCEvents() {
	decoder := json.NewDecoder(s.ipcConn)
	for {
		var msg IPCMessage
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				log.Printf("Failed to receive IPC message: %v", err)
			}
			return
		}
		
		if msg.Type == "event" {
			var event SocialEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				log.Printf("Failed to unmarshal social event: %v", err)
				continue
			}
			
			select {
			case s.eventChan <- &event:
			case <-s.done:
				return
			}
		}
	}
}

// sendResponse sends a response to the MCP client
func (s *MCPStdioServer) sendResponse(resp MCPResponse) {
	s.outputMutex.Lock()
	defer s.outputMutex.Unlock()
	
	// Ensure JSON-RPC version is set
	resp.JSONRPC = "2.0"
	
	if err := s.output.Encode(&resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// eventPump handles incoming social events
func (s *MCPStdioServer) eventPump() {
	for {
		select {
		case event := <-s.eventChan:
			log.Printf("MCP received social event: %s from %s", event.Subtype, event.From)
			// In Phase 2 we'll send these as notifications to Claude
		case <-s.done:
			return
		}
	}
}

// getToolsList returns the list of available tools
func (s *MCPStdioServer) getToolsList() map[string]interface{} {
	return map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "send_command",
				"description": "Send a command to DragonRealms",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The command to send to the game",
						},
					},
					"required": []string{"command"},
				},
			},
			{
				"name":        "social_say",
				"description": "Say something in the game",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "What to say",
						},
						"target": map[string]interface{}{
							"type":        "string",
							"description": "Optional target to speak to",
						},
					},
					"required": []string{"message"},
				},
			},
			{
				"name":        "social_whisper",
				"description": "Whisper to someone in the game",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"target": map[string]interface{}{
							"type":        "string",
							"description": "Who to whisper to",
						},
						"message": map[string]interface{}{
							"type":        "string",
							"description": "What to whisper",
						},
					},
					"required": []string{"target", "message"},
				},
			},
			{
				"name":        "get_social_events",
				"description": "Get recent social interactions",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"count": map[string]interface{}{
							"type":        "integer",
							"description": "Number of recent events to retrieve (default: 10)",
							"minimum":     1,
							"maximum":     100,
						},
					},
				},
			},
		},
	}
}

// RunMCPStdioServer is the entry point when running as a separate process
func RunMCPStdioServer() {
	if len(os.Args) < 3 || os.Args[1] != "--mcp-stdio-child" {
		log.Fatal("Invalid invocation of MCP stdio server")
	}
	
	ipcSocket := os.Args[2]
	
	server, err := NewMCPStdioServer(ipcSocket)
	if err != nil {
		log.Fatalf("Failed to create MCP stdio server: %v", err)
	}
	
	server.Start()
}