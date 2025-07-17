package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// MCPIPCServer handles IPC communication with the stdio MCP child process
type MCPIPCServer struct {
	gameClient   *GameClient
	socketPath   string
	listener     net.Listener
	conn         net.Conn
	eventChan    chan *SocialEvent
	done         chan bool
	childProcess *exec.Cmd
	mutex        sync.Mutex
}

// NewMCPIPCServer creates a new IPC server for MCP communication
func NewMCPIPCServer(gameClient *GameClient) (*MCPIPCServer, error) {
	// Create a unique socket path
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("dr-charm-mcp-%d.sock", os.Getpid()))
	
	// Remove any existing socket file
	os.Remove(socketPath)
	
	// Create Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPC socket: %w", err)
	}
	
	return &MCPIPCServer{
		gameClient: gameClient,
		socketPath: socketPath,
		listener:   listener,
		eventChan:  make(chan *SocialEvent, 100),
		done:       make(chan bool),
	}, nil
}

// Start launches the MCP child process and handles IPC
func (s *MCPIPCServer) Start() error {
	// Get the path to the current executable
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	
	// Launch MCP stdio server as child process
	s.childProcess = exec.Command(exePath, "--mcp-stdio-child", s.socketPath)
	s.childProcess.Stderr = os.Stderr // Pass through stderr for logging
	
	// Get pipes for the child's stdin/stdout
	childStdin, err := s.childProcess.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get child stdin pipe: %w", err)
	}
	
	childStdout, err := s.childProcess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get child stdout pipe: %w", err)
	}
	
	// Start the child process
	if err := s.childProcess.Start(); err != nil {
		return fmt.Errorf("failed to start MCP child process: %w", err)
	}
	
	log.Printf("Started MCP stdio child process (PID: %d)", s.childProcess.Process.Pid)
	
	// Start accepting IPC connection
	go s.acceptConnection()
	
	// Start event forwarder
	go s.forwardEvents()
	
	// Clean up when process exits
	go func() {
		s.childProcess.Wait()
		close(s.done)
		s.cleanup()
	}()
	
	// Log that pipes are connected for Claude
	log.Printf("MCP stdio server ready - stdin: %v, stdout: %v", childStdin, childStdout)
	
	return nil
}

// acceptConnection accepts the IPC connection from the child process
func (s *MCPIPCServer) acceptConnection() {
	// Accept with timeout
	s.listener.(*net.UnixListener).SetDeadline(time.Now().Add(5 * time.Second))
	
	conn, err := s.listener.Accept()
	if err != nil {
		log.Printf("Failed to accept IPC connection: %v", err)
		return
	}
	
	s.mutex.Lock()
	s.conn = conn
	s.mutex.Unlock()
	
	log.Println("MCP child process connected via IPC")
	
	// Handle IPC messages
	s.handleIPCMessages()
}

// handleIPCMessages processes messages from the MCP child process
func (s *MCPIPCServer) handleIPCMessages() {
	decoder := json.NewDecoder(s.conn)
	encoder := json.NewEncoder(s.conn)
	
	for {
		var msg IPCMessage
		if err := decoder.Decode(&msg); err != nil {
			log.Printf("Failed to decode IPC message: %v", err)
			return
		}
		
		switch msg.Type {
		case "request":
			// Handle request from MCP child
			var req IPCRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				log.Printf("Failed to unmarshal IPC request: %v", err)
				continue
			}
			
			// Process the request
			result, err := s.processRequest(req)
			
			// Send response
			resp := IPCResponse{
				ID:     req.ID,
				Result: result,
			}
			if err != nil {
				resp.Error = &MCPError{
					Code:    InternalError,
					Message: err.Error(),
				}
			}
			
			respData, _ := json.Marshal(resp)
			respMsg := IPCMessage{
				Type:    "response",
				Payload: respData,
			}
			
			if err := encoder.Encode(respMsg); err != nil {
				log.Printf("Failed to send IPC response: %v", err)
			}
		}
	}
}

// processRequest handles tool calls from the MCP child process
func (s *MCPIPCServer) processRequest(req IPCRequest) (interface{}, error) {
	switch req.Method {
	case "send_command":
		command, ok := req.Params["command"].(string)
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
		message, ok := req.Params["message"].(string)
		if !ok {
			return nil, fmt.Errorf("message must be a string")
		}
		target, _ := req.Params["target"].(string)
		
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
		target, ok := req.Params["target"].(string)
		if !ok {
			return nil, fmt.Errorf("target must be a string")
		}
		message, ok := req.Params["message"].(string)
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
		if c, ok := req.Params["count"].(float64); ok {
			count = int(c)
		}
		
		events := s.gameClient.GetSocialBuffer().GetRecent(count)
		return map[string]interface{}{
			"events": events,
			"count":  len(events),
		}, nil
		
	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

// forwardEvents forwards social events to the MCP child process
func (s *MCPIPCServer) forwardEvents() {
	for {
		select {
		case event := <-s.eventChan:
			s.mutex.Lock()
			conn := s.conn
			s.mutex.Unlock()
			
			if conn != nil {
				eventData, _ := json.Marshal(event)
				msg := IPCMessage{
					Type:    "event",
					Payload: eventData,
				}
				
				encoder := json.NewEncoder(conn)
				if err := encoder.Encode(msg); err != nil {
					log.Printf("Failed to forward event to MCP: %v", err)
				}
			}
			
		case <-s.done:
			return
		}
	}
}

// GetEventChannel returns the channel for sending events to MCP
func (s *MCPIPCServer) GetEventChannel() chan<- *SocialEvent {
	return s.eventChan
}

// Stop gracefully shuts down the IPC server and child process
func (s *MCPIPCServer) Stop() error {
	close(s.done)
	
	if s.childProcess != nil && s.childProcess.Process != nil {
		s.childProcess.Process.Kill()
	}
	
	s.cleanup()
	return nil
}

// cleanup removes the socket file and closes connections
func (s *MCPIPCServer) cleanup() {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
}