package main

import (
	"encoding/json"
	"fmt"
	"io"
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
	standalone  bool // true when running without UI
}

// MCPRequest represents a JSON-RPC request
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC response
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
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
		standalone: false,
	}
}

// SetStandalone sets whether the MCP server is running without UI
func (s *MCPServer) SetStandalone(standalone bool) {
	s.standalone = standalone
}

// Start begins the MCP server operation
func (s *MCPServer) Start() {
	if s.standalone {
		// Use stderr for logging in standalone mode
		log.SetOutput(os.Stderr)
		fmt.Fprintf(os.Stderr, "MCP server started in standalone mode\n")
	} else {
		log.Println("MCP server started")
	}
	
	// Start event pump in background
	go s.eventPump()
	
	// Main request loop
	for {
		var req MCPRequest
		if err := s.input.Decode(&req); err != nil {
			if err == io.EOF {
				if s.standalone {
					fmt.Fprintf(os.Stderr, "MCP client disconnected\n")
				} else {
					log.Println("MCP client disconnected")
				}
			} else {
				if s.standalone {
					fmt.Fprintf(os.Stderr, "Failed to decode request: %v\n", err)
				} else {
					log.Printf("Failed to decode request: %v", err)
				}
			}
			break
		}
		
		if s.standalone {
			fmt.Fprintf(os.Stderr, "Received request: method=%s id=%v\n", req.Method, req.ID)
		} else {
			log.Printf("Received request: method=%s id=%v", req.Method, req.ID)
		}
		
		// Handle the request
		s.handleRequest(req)
	}
	
	close(s.done)
	if s.standalone {
		fmt.Fprintf(os.Stderr, "MCP server shutting down\n")
	} else {
		fmt.Println("MCP server shutting down")
	}
}

// eventPump handles incoming social events
func (s *MCPServer) eventPump() {
	for {
		select {
		case event := <-s.eventChan:
			if s.standalone {
				fmt.Fprintf(os.Stderr, "MCP received social event: %s from %s\n", event.Subtype, event.From)
			} else {
				log.Printf("MCP received social event: %s from %s", event.Subtype, event.From)
			}
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

// handleRequest processes an incoming MCP request
func (s *MCPServer) handleRequest(req MCPRequest) {
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
		
	case "notifications/initialized":
		// Client has acknowledged initialization
		if s.standalone {
			fmt.Fprintf(os.Stderr, "MCP client initialized\n")
		} else {
			log.Println("MCP client initialized")
		}
		return // No response needed for notifications
		
	case "tools/list":
		// Return list of available tools
		resp.Result = s.getToolsList()
	
	case "tools/call":
		// Handle tool invocation
		if toolName, ok := req.Params["name"].(string); ok {
			if args, ok := req.Params["arguments"].(map[string]interface{}); ok {
				result, err := s.callTool(toolName, args)
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
	
	default:
		resp.Error = &MCPError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Unknown method: %s", req.Method),
		}
	}
	
	// Send response
	resp.JSONRPC = "2.0"
	s.outputMutex.Lock()
	if err := s.output.Encode(&resp); err != nil {
		if s.standalone {
			fmt.Fprintf(os.Stderr, "Failed to encode response: %v\n", err)
		} else {
			log.Printf("Failed to encode response: %v", err)
		}
	}
	s.outputMutex.Unlock()
}

// getToolsList returns the list of available tools
func (s *MCPServer) getToolsList() map[string]interface{} {
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

// callTool executes a tool and returns the result
func (s *MCPServer) callTool(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "send_command":
		command, ok := args["command"].(string)
		if !ok {
			return nil, fmt.Errorf("command must be a string")
		}
		err := s.gameClient.SendCommand(command)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"success": true,
			"command": command,
		}, nil
	
	case "social_say":
		message, ok := args["message"].(string)
		if !ok {
			return nil, fmt.Errorf("message must be a string")
		}
		target, _ := args["target"].(string)
		
		var command string
		if target != "" {
			command = fmt.Sprintf("say to %s %s", target, message)
		} else {
			command = fmt.Sprintf("say %s", message)
		}
		
		err := s.gameClient.SendCommand(command)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"success": true,
			"command": command,
		}, nil
	
	case "social_whisper":
		target, ok := args["target"].(string)
		if !ok {
			return nil, fmt.Errorf("target must be a string")
		}
		message, ok := args["message"].(string)
		if !ok {
			return nil, fmt.Errorf("message must be a string")
		}
		
		command := fmt.Sprintf("whisper %s %s", target, message)
		err := s.gameClient.SendCommand(command)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"success": true,
			"command": command,
		}, nil
	
	case "get_social_events":
		count := 10
		if c, ok := args["count"].(float64); ok {
			count = int(c)
		}
		
		events := s.gameClient.GetSocialBuffer().GetRecent(count)
		return map[string]interface{}{
			"events": events,
			"count":  len(events),
		}, nil
	
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}