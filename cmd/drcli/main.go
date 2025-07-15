package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config for the CLI client
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// GameClient wraps the REST API
type GameClient struct {
	config Config
	client *http.Client
}

// NewGameClient creates a new game client
func NewGameClient(config Config) *GameClient {
	return &GameClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// CommandRequest represents a command to send
type CommandRequest struct {
	Command string `json:"command"`
}

// CommandResponse represents the API response
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

// SendCommand sends a command to the game
func (gc *GameClient) SendCommand(command string) error {
	reqBody, err := json.Marshal(CommandRequest{Command: command})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := gc.client.Post(
		gc.config.BaseURL+"/command",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var cmdResp CommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if cmdResp.Status != "sent" {
		return fmt.Errorf("unexpected status: %s", cmdResp.Status)
	}

	return nil
}

// GetOutput retrieves recent game output
func (gc *GameClient) GetOutput(lines int) (*OutputResponse, error) {
	url := fmt.Sprintf("%s/output?lines=%d", gc.config.BaseURL, lines)
	resp, err := gc.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get output: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var output OutputResponse
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return nil, fmt.Errorf("failed to decode output: %w", err)
	}

	return &output, nil
}

// CheckHealth verifies the API is responsive
func (gc *GameClient) CheckHealth() error {
	resp, err := gc.client.Get(gc.config.BaseURL + "/health")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}

	return nil
}

// Interactive mode
func runInteractive(client *GameClient, showPrompt bool) {
	if showPrompt {
		fmt.Println("DragonRealms CLI - Interactive Mode")
		fmt.Println("Type 'help' for commands, 'quit' to exit")
		fmt.Println()
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		if showPrompt {
			fmt.Print("> ")
		}
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "quit", "exit":
			if showPrompt {
				fmt.Println("Goodbye!")
			}
			return
		case "help":
			printHelp()
		case "health":
			if err := client.CheckHealth(); err != nil {
				fmt.Printf("Health check failed: %v\n", err)
			} else {
				fmt.Println("API is healthy")
			}
		case "output", "o":
			output, err := client.GetOutput(20)
			if err != nil {
				fmt.Printf("Failed to get output: %v\n", err)
			} else {
				for _, line := range output.Lines {
					fmt.Println(line)
				}
			}
		default:
			// Send as game command
			if err := client.SendCommand(input); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else if !showPrompt {
				// In pipe mode, confirm each command
				fmt.Printf("Sent: %s\n", input)
			}
		}
	}
}

func printHelp() {
	fmt.Println(`
Available commands:
  help       - Show this help
  health     - Check API health
  output, o  - Show recent game output
  quit, exit - Exit the CLI
  
Any other input is sent as a game command.`)
}

// Watch mode - continuously display output
func runWatch(client *GameClient, interval time.Duration) {
	fmt.Println("Watching game output... Press Ctrl+C to stop")
	
	var lastID uint64 = 0
	for {
		output, err := client.GetOutputSince(lastID)
		if err != nil {
			fmt.Printf("Error getting output: %v\n", err)
			time.Sleep(interval)
			continue
		}

		// Display new lines
		for _, line := range output.Lines {
			fmt.Println(line)
		}
		
		// Update lastID if we got new content
		if output.LastID > lastID {
			lastID = output.LastID
		}

		time.Sleep(interval)
	}
}

// GetOutputSince retrieves game output since a specific ID
func (gc *GameClient) GetOutputSince(sinceID uint64) (*OutputResponse, error) {
	url := fmt.Sprintf("%s/output?since=%d", gc.config.BaseURL, sinceID)
	resp, err := gc.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get output: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var output OutputResponse
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return nil, fmt.Errorf("failed to decode output: %w", err)
	}

	return &output, nil
}

func main() {
	var (
		baseURL     = flag.String("url", "http://localhost:8080", "API base URL")
		timeout     = flag.Duration("timeout", 10*time.Second, "Request timeout")
		command     = flag.String("c", "", "Send a single command")
		output      = flag.Bool("o", false, "Show recent output")
		outputLines = flag.Int("n", 20, "Number of output lines to show")
		watch       = flag.Bool("w", false, "Watch mode - continuously show output")
		watchInt    = flag.Duration("i", 1*time.Second, "Watch interval")
		health      = flag.Bool("health", false, "Check API health")
	)
	flag.Parse()

	config := Config{
		BaseURL: *baseURL,
		Timeout: *timeout,
	}
	client := NewGameClient(config)

	// Handle single operations
	if *health {
		if err := client.CheckHealth(); err != nil {
			fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("API is healthy")
		return
	}

	if *output {
		resp, err := client.GetOutput(*outputLines)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get output: %v\n", err)
			os.Exit(1)
		}
		for _, line := range resp.Lines {
			fmt.Println(line)
		}
		return
	}

	if *command != "" {
		if err := client.SendCommand(*command); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send command: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Command sent successfully")
		return
	}

	if *watch {
		runWatch(client, *watchInt)
		return
	}

	// Check if stdin is a terminal
	stat, _ := os.Stdin.Stat()
	isPipe := (stat.Mode() & os.ModeCharDevice) == 0
	
	// Default to interactive mode
	runInteractive(client, !isPipe)
}