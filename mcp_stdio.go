package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// MCPStdioServer handles Model Context Protocol over stdio
type MCPStdioServer struct {
	input          *json.Decoder
	output         *json.Encoder
	outputMutex    sync.Mutex
	eventChan      chan *SocialEvent
	done           chan bool
	subscribed     bool
	subscribeMutex sync.RWMutex
}

// NewMCPStdioServer creates a new MCP stdio server
func NewMCPStdioServer() *MCPStdioServer {
	return &MCPStdioServer{
		input:     json.NewDecoder(os.Stdin),
		output:    json.NewEncoder(os.Stdout),
		eventChan: make(chan *SocialEvent, 100),
		done:      make(chan bool),
	}
}

// Start begins the MCP stdio server operation
func (s *MCPStdioServer) Start(gameClient *GameClient) {
	log.SetOutput(os.Stderr) // Log to stderr to not interfere with JSON-RPC
	log.Println("MCP stdio server started")
	
	// Connect to game client
	gameClient.SetMCPEventChannel(s.eventChan)
	
	// Start event notifier
	go s.eventNotifier()
	
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
		s.handleRequest(req, gameClient)
	}
	
	close(s.done)
}

// handleRequest processes an incoming MCP request
func (s *MCPStdioServer) handleRequest(req MCPRequest, gameClient *GameClient) {
	var resp MCPResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID
	
	switch req.Method {
	case "initialize":
		// Handle initialization request
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
				"notifications": map[string]interface{}{
					"social": map[string]interface{}{
						"description": "Real-time social events from DragonRealms",
					},
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "dr-charm-mcp",
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
		// Handle tool invocation
		if toolName, ok := req.Params["name"].(string); ok {
			if args, ok := req.Params["arguments"].(map[string]interface{}); ok {
				result, err := s.callTool(toolName, args, gameClient)
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

// sendResponse sends a response to the MCP client
func (s *MCPStdioServer) sendResponse(resp MCPResponse) {
	s.outputMutex.Lock()
	defer s.outputMutex.Unlock()
	
	if err := s.output.Encode(&resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// sendNotification sends a notification to the MCP client
func (s *MCPStdioServer) sendNotification(method string, params interface{}) {
	s.outputMutex.Lock()
	defer s.outputMutex.Unlock()
	
	notif := MCPNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params: map[string]interface{}{
			"data": params,
		},
	}
	
	if err := s.output.Encode(&notif); err != nil {
		log.Printf("Failed to encode notification: %v", err)
	}
}

// eventNotifier sends social events as notifications
func (s *MCPStdioServer) eventNotifier() {
	for {
		select {
		case event := <-s.eventChan:
			// Check if subscribed
			s.subscribeMutex.RLock()
			subscribed := s.subscribed
			s.subscribeMutex.RUnlock()
			
			if subscribed {
				log.Printf("Sending social event notification: %s from %s", event.Subtype, event.From)
				s.sendNotification("social_event", event)
			}
			
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
			{
				"name":        "subscribe_social_events",
				"description": "Subscribe to real-time social event notifications",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "Enable or disable social event notifications",
						},
					},
					"required": []string{"enabled"},
				},
			},
		},
	}
}

// callTool executes a tool and returns the result
func (s *MCPStdioServer) callTool(name string, args map[string]interface{}, gameClient *GameClient) (interface{}, error) {
	switch name {
	case "send_command":
		command, ok := args["command"].(string)
		if !ok {
			return nil, fmt.Errorf("command must be a string")
		}
		err := gameClient.SendCommand(command)
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
		
		err := gameClient.SendCommand(command)
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
		err := gameClient.SendCommand(command)
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
		
		events := gameClient.GetSocialBuffer().GetRecent(count)
		return map[string]interface{}{
			"events": events,
			"count":  len(events),
		}, nil
		
	case "subscribe_social_events":
		enabled, ok := args["enabled"].(bool)
		if !ok {
			return nil, fmt.Errorf("enabled must be a boolean")
		}
		
		s.subscribeMutex.Lock()
		s.subscribed = enabled
		s.subscribeMutex.Unlock()
		
		return map[string]interface{}{
			"subscribed": enabled,
			"message":    fmt.Sprintf("Social event notifications %s", map[bool]string{true: "enabled", false: "disabled"}[enabled]),
		}, nil
		
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// GetEventChannel returns the channel for sending events to MCP
func (s *MCPStdioServer) GetEventChannel() chan<- *SocialEvent {
	return s.eventChan
}