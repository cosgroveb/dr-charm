package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Simple CLI mode for testing performance
func RunCLIMode(gameConn net.Conn, api *GameAPI) {
	fmt.Println("\n=== CLI Mode - Performance Testing ===")
	fmt.Println("Commands: 'quit' to exit, 'stats' for performance stats")
	fmt.Println("Press Enter to send commands to the game\n")

	// Create performance tracker
	debug := os.Getenv("DR_CHARM_DEBUG") == "true"
	perfTracker := NewPerformanceTracker(debug)
	xmlParser := NewXMLStreamParser(debug)
	defer xmlParser.Close()

	// Start reading from game
	go func() {
		for {
			// Start event tracking
			event := perfTracker.StartEvent()

			buf := make([]byte, 4096)
			n, err := gameConn.Read(buf)
			if err != nil {
				fmt.Printf("\nConnection lost: %v\n", err)
				return
			}

			// Parse XML
			text, err := xmlParser.ParseChunk(buf[:n])
			event.ParseEndTime = time.Now()
			event.StateUpdTime = time.Now() // State updated during parsing

			if err != nil {
				text = string(buf[:n])
			}

			// Update API
			if text != "" {
				lines := strings.Split(text, "\n")
				for _, line := range lines {
					if line != "" {
						api.AddOutput(line)
						fmt.Println(line)
					}
				}
			}

			// UI update time (console print)
			event.UIUpdateTime = time.Now()
			event.RenderTime = time.Now()

			// Record the event
			perfTracker.RecordEvent(event)
		}
	}()

	// Read user input
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := scanner.Text()

		if input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		if input == "stats" {
			// Display performance stats
			stats := perfTracker.GetStats()
			if stats != nil {
				fmt.Println("\n=== Performance Stats ===")
				if eventCount, ok := stats["event_count"].(int); ok {
					fmt.Printf("Events tracked: %d\n", eventCount)
				}

				stages := []string{"parse", "state", "ui_update", "render", "total"}
				for _, stage := range stages {
					if stageStats, ok := stats[stage].(map[string]float64); ok {
						fmt.Printf("%s: avg=%.1fms p95=%.1fms max=%.1fms\n",
							stage,
							stageStats["avg_ms"],
							stageStats["p95_ms"],
							stageStats["max_ms"])
					}
				}
				fmt.Println("=========================")
			}
			continue
		}

		// Send command to game
		_, err := gameConn.Write([]byte(input + "\n"))
		if err != nil {
			fmt.Printf("Failed to send command: %v\n", err)
			return
		}
		api.AddOutput("> " + input)
	}
}
