package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"dr-charm/internal/game"
)

// MCPHTTPServer handles Model Context Protocol over HTTP for commands
type MCPHTTPServer struct {
	gameClient  *game.GameClient
	port        int
	server      *http.Server
	outputMutex sync.Mutex
}

// NewMCPHTTPServer creates a new MCP HTTP server instance
func NewMCPHTTPServer(gameClient *game.GameClient, port int) *MCPHTTPServer {
	return &MCPHTTPServer{
		gameClient: gameClient,
		port:       port,
	}
}

// Start begins the MCP HTTP server operation
func (s *MCPHTTPServer) Start() error {
	mux := http.NewServeMux()

	// Handle MCP requests
	mux.HandleFunc("/mcp", s.handleMCPRequest)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	log.Printf("MCP HTTP server starting on port %d", s.port)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *MCPHTTPServer) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleMCPRequest processes incoming MCP requests over HTTP
func (s *MCPHTTPServer) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers for streaming response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendErrorResponse(w, nil, ParseError, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse the JSON-RPC request
	var req MCPRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.sendErrorResponse(w, nil, ParseError, "Invalid JSON")
		return
	}

	// Debug logging disabled to avoid UI clutter
	// log.Printf("MCP HTTP received request: method=%s id=%v", req.Method, req.ID)

	// Handle the request
	resp := s.processRequest(req)

	// Send the response
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}

	// Flush the response writer if it supports flushing
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// processRequest handles the actual MCP request processing
func (s *MCPHTTPServer) processRequest(req MCPRequest) MCPResponse {
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
				"name":    "dr-charm-mcp",
				"version": "1.0.0",
			},
		}

	case "notifications/initialized":
		// Client has acknowledged initialization - no log needed
		return resp // Return empty response for notifications

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

	resp.JSONRPC = "2.0"
	return resp
}

// sendErrorResponse sends an error response
func (s *MCPHTTPServer) sendErrorResponse(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

// getToolsList returns the list of available tools (reuse from mcp.go)
func (s *MCPHTTPServer) getToolsList() map[string]interface{} {
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
				"name":        "get_connection_status",
				"description": "Get current connection status and health information",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

// callTool executes a tool and returns the result (reuse from mcp.go)
func (s *MCPHTTPServer) callTool(name string, args map[string]interface{}) (interface{}, error) {
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
		// Debug: log to stderr to avoid UI clutter
		fmt.Fprintf(os.Stderr, "[MCP] get_social_events: returning %d events\n", len(events))
		return map[string]interface{}{
			"events": events,
			"count":  len(events),
		}, nil

	case "get_connection_status":
		return s.gameClient.GetConnectionInfo(), nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}
