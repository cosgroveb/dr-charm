package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

// RunPerfTest runs an automated performance test
func RunPerfTest() {
	fmt.Println("DragonRealms Performance Test")
	fmt.Println("=============================")

	// Connect and authenticate
	conn, err := net.DialTimeout("tcp", "eaccess.play.net:7900", 30*time.Second)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect: %v", err))
	}
	defer conn.Close()

	// Authenticate (simplified for testing)
	fmt.Println("Authenticating...")
	// ... (auth code would go here)

	// Create performance tracker
	debug := true
	perfTracker := NewPerformanceTracker(debug)
	xmlParser := NewXMLStreamParser(debug)

	// Simulate receiving game data
	testData := []string{
		`<prompt time="1752546884">`,
		`<roundTime value="3"/>`,
		`<pushStream id="room" /><style id="" />[Via Iltesh, Peddler's Alley]</pushStream>`,
		`<streamwindow id="main" subtitle=" - Via Iltesh, Peddler's Alley" />`,
		`<component id="room objs">You also see a wooden barrel.</component>`,
		`<component id="room exits">Obvious paths: north, south, east.</component>`,
		`<popStream />`,
		`Random game text that should appear.`,
		`<progressBar id="health" value="100" />`,
		`<progressBar id="mana" value="85" />`,
		`<progressBar id="stamina" value="70" />`,
	}

	fmt.Println("\nRunning performance test with sample data...\n")

	// Process each chunk and measure performance
	for i, data := range testData {
		event := perfTracker.StartEvent()
		
		// Simulate network delay
		time.Sleep(10 * time.Millisecond)
		
		// Parse XML
		text, err := xmlParser.ParseChunk([]byte(data))
		event.ParseEndTime = time.Now()
		
		if err != nil {
			fmt.Printf("Parse error on chunk %d: %v\n", i, err)
		}
		
		// Simulate state update
		event.StateUpdTime = time.Now()
		
		// Simulate UI update
		time.Sleep(5 * time.Millisecond)
		event.UIUpdateTime = time.Now()
		
		// Simulate render
		time.Sleep(2 * time.Millisecond)
		event.RenderTime = time.Now()
		
		perfTracker.RecordEvent(event)
		
		if text != "" {
			fmt.Printf("Output: %s\n", text)
		}
	}

	// Display stats
	fmt.Println("\n=== Performance Results ===")
	stats := perfTracker.GetStats()
	if stats != nil {
		if eventCount, ok := stats["event_count"].(int); ok {
			fmt.Printf("Events tracked: %d\n", eventCount)
		}
		
		stages := []string{"parse", "state", "ui_update", "render", "total"}
		for _, stage := range stages {
			if stageStats, ok := stats[stage].(map[string]float64); ok {
				fmt.Printf("%-12s avg=%6.1fms p95=%6.1fms max=%6.1fms\n",
					stage+":",
					stageStats["avg_ms"],
					stageStats["p95_ms"],
					stageStats["max_ms"])
			}
		}
	}
	fmt.Println("===========================")
}

// Add this to main.go temporarily for testing
func init() {
	if os.Getenv("DR_CHARM_PERF_TEST") == "true" {
		RunPerfTest()
		os.Exit(0)
	}
}