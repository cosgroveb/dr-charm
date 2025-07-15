package main

import (
	"container/ring"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// GameAPI handles HTTP API requests for game control
type GameAPI struct {
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

// CommandRequest represents a command to send to the game
type CommandRequest struct {
	Command string `json:"command"`
}

// CommandResponse represents the response to a command
type CommandResponse struct {
	Status string `json:"status"`
	ID     uint64 `json:"id"`
}

// OutputResponse represents game output
type OutputResponse struct {
	Lines  []string `json:"lines"`
	LastID uint64   `json:"last_id"`
	Count  int      `json:"count"`
}

// HealthResponse represents API health status
type HealthResponse struct {
	Status    string `json:"status"`
	Character string `json:"character"`
	Uptime    string `json:"uptime"`
}

var startTime = time.Now()

// NewGameAPI creates a new game API instance
func NewGameAPI(gameConn net.Conn, character string, bufferSize int) *GameAPI {
	api := &GameAPI{
		gameConn:     gameConn,
		outputBuffer: ring.New(bufferSize),
		character:    character,
	}
	api.connected.Store(true)
	return api
}

// AddOutput adds a line of output to the buffer
func (api *GameAPI) AddOutput(text string) {
	api.mu.Lock()
	defer api.mu.Unlock()

	id := api.lastID.Add(1)
	line := OutputLine{
		ID:        id,
		Text:      text,
		Timestamp: time.Now(),
	}
	api.outputBuffer.Value = line
	api.outputBuffer = api.outputBuffer.Next()
}

// handleCommand handles POST /command
func (api *GameAPI) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Command == "" {
		http.Error(w, "Command cannot be empty", http.StatusBadRequest)
		return
	}

	if !api.connected.Load() {
		http.Error(w, "Game connection lost", http.StatusServiceUnavailable)
		return
	}

	// Send command to game
	_, err := api.gameConn.Write([]byte(req.Command + "\n"))
	if err != nil {
		api.connected.Store(false)
		http.Error(w, "Failed to send command", http.StatusInternalServerError)
		return
	}

	// Record the command in output
	api.AddOutput("> " + req.Command)

	resp := CommandResponse{
		Status: "sent",
		ID:     api.lastID.Load(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleOutput handles GET /output
func (api *GameAPI) handleOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sinceID := uint64(0)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if parsed, err := strconv.ParseUint(sinceStr, 10, 64); err == nil {
			sinceID = parsed
		}
	}

	api.mu.RLock()
	defer api.mu.RUnlock()

	var lines []string
	count := 0
	api.outputBuffer.Do(func(v interface{}) {
		if v != nil {
			if line, ok := v.(OutputLine); ok && line.ID > sinceID {
				lines = append(lines, line.Text)
				count++
			}
		}
	})

	resp := OutputResponse{
		Lines:  lines,
		LastID: api.lastID.Load(),
		Count:  count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleHealth handles GET /health
func (api *GameAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := "disconnected"
	if api.connected.Load() {
		status = "connected"
	}

	resp := HealthResponse{
		Status:    status,
		Character: api.character,
		Uptime:    time.Since(startTime).Round(time.Second).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// StartAPIServer starts the HTTP API server
func StartAPIServer(api *GameAPI, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/command", api.handleCommand)
	mux.HandleFunc("/output", api.handleOutput)
	mux.HandleFunc("/health", api.handleHealth)

	// Add CORS headers for local development
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		mux.ServeHTTP(w, r)
	})

	fmt.Printf("API server listening on %s\n", addr)
	return http.ListenAndServe(addr, handler)
}
