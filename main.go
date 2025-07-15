package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Hardcoded credentials - UPDATE THESE
const (
	account   = "cosgroveb4" // Outlander uses lowercase
	password  = "rupture-ella"
	character = "Cennedig"
)

func main() {
	fmt.Println("DragonRealms Authentication Test")
	fmt.Println("================================")

	if password == "" {
		panic("Please update the password constant in main.go")
	}

	// Connect to auth server
	conn, err := net.DialTimeout("tcp", "eaccess.play.net:7900", 30*time.Second)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect: %v", err))
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send K to get hash
	fmt.Println("Requesting hash...")
	_, err = conn.Write([]byte("K\n"))
	if err != nil {
		panic(fmt.Sprintf("Failed to send K: %v", err))
	}

	// Read hash
	hash, err := reader.ReadString('\n')
	if err != nil {
		panic(fmt.Sprintf("Failed to read hash: %v", err))
	}
	hash = strings.TrimSpace(hash)
	fmt.Printf("Got hash: %s\n", hash)

	// Encrypt password
	encrypted := encryptPassword(password, hash)
	fmt.Printf("Encrypted password: %s\n", encrypted)

	// Send account and encrypted password - Outlander sends raw bytes, not hex!
	encryptedBytes := encryptPasswordBytes(password, hash)

	// Build auth message
	authMsg := []byte("A\t" + strings.ToUpper(account) + "\t")
	authMsg = append(authMsg, encryptedBytes...)
	authMsg = append(authMsg, '\n')

	fmt.Printf("Sending auth for account: %s\n", strings.ToUpper(account))
	_, err = conn.Write(authMsg)
	if err != nil {
		panic(fmt.Sprintf("Failed to send auth: %v", err))
	}

	// Read auth response
	authResp, err := reader.ReadString('\n')
	if err != nil {
		panic(fmt.Sprintf("Failed to read auth response: %v", err))
	}
	fmt.Printf("Auth response: %s", authResp)

	// Debug: let's see raw bytes
	fmt.Printf("Auth response bytes: %v\n", []byte(authResp))

	if strings.Contains(authResp, "PASSWORD") {
		panic("Invalid password")
	}
	if strings.Contains(authResp, "NORECORD") {
		panic("Account not found")
	}

	// Send game selection
	fmt.Println("Selecting game DR...")
	_, err = conn.Write([]byte("G\tDR\n"))
	if err != nil {
		panic(fmt.Sprintf("Failed to send game: %v", err))
	}

	// Read game response
	gameResp, err := reader.ReadString('\n')
	if err != nil {
		panic(fmt.Sprintf("Failed to read game response: %v", err))
	}
	fmt.Printf("Game response: %s", gameResp)

	// Get character list
	fmt.Println("Getting character list...")
	_, err = conn.Write([]byte("C\n"))
	if err != nil {
		panic(fmt.Sprintf("Failed to send C: %v", err))
	}

	// Read character list response line by line
	fmt.Println("Reading character list...")
	var charLines []string
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout reached, we have all the data
				break
			}
			panic(fmt.Sprintf("Failed to read character list: %v", err))
		}

		line = strings.TrimSpace(line)
		fmt.Printf("Character line: %s\n", line)
		charLines = append(charLines, line)

		// Character list ends with a line containing just 5 tab-separated zeros
		if line == "C\t0\t0\t0\t0" {
			break
		}
	}
	// Reset deadline
	conn.SetReadDeadline(time.Time{})

	fmt.Printf("Total character lines: %d\n", len(charLines))

	// Parse character list to find our character
	var charID string
	for _, line := range charLines {
		// Skip the terminator line
		if line == "C\t0\t0\t0\t0" {
			continue
		}

		// Look for our character
		parts := strings.Split(line, "\t")
		if len(parts) >= 7 {
			// Character name is in the last field (index 6)
			charName := strings.TrimSpace(parts[6])
			if strings.EqualFold(charName, character) {
				// Character ID is in field 5
				charID = parts[5]
				fmt.Printf("Found character: %s with ID: %s\n", character, charID)
				break
			}
		}
	}

	if charID == "" {
		panic(fmt.Sprintf("Character '%s' not found", character))
	}

	fmt.Printf("Found character ID: %s\n", charID)

	// Login with character
	loginCmd := fmt.Sprintf("L\t%s\tPLAY\n", charID)
	_, err = conn.Write([]byte(loginCmd))
	if err != nil {
		panic(fmt.Sprintf("Failed to send login: %v", err))
	}

	// Read login response and extract connection info
	fmt.Println("Reading login response...")
	var gameKey, gameHost, gamePort string
	loginData := ""
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout reached
				break
			}
			panic(fmt.Sprintf("Failed to read login response: %v", err))
		}

		loginData += line
		fmt.Printf("Login response line: %s", line)

		// Extract connection info from the accumulated data
		if strings.Contains(loginData, "KEY=") {
			gameKey = extractValue(loginData, "KEY=(\\w+)")
		}
		if strings.Contains(loginData, "GAMEHOST=") {
			gameHost = extractValue(loginData, "GAMEHOST=(\\S+)")
		}
		if strings.Contains(loginData, "GAMEPORT=") {
			gamePort = extractValue(loginData, "GAMEPORT=(\\d+)")
		}

		// Check if we have all info
		if gameKey != "" && gameHost != "" && gamePort != "" {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})

	fmt.Println("\n=== Game Connection Info ===")
	fmt.Printf("Host: %s\n", gameHost)
	fmt.Printf("Port: %s\n", gamePort)
	fmt.Printf("Key: %s\n", gameKey)
	fmt.Println("============================")

	// Close auth connection
	conn.Close()

	// Connect to game server
	fmt.Printf("\nConnecting to game server %s:%s...\n", gameHost, gamePort)
	gameConn, err := net.DialTimeout("tcp", gameHost+":"+gamePort, 30*time.Second)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to game server: %v", err))
	}
	defer gameConn.Close()

	// Send key and client info
	fmt.Println("Sending authentication key...")
	_, err = gameConn.Write([]byte(gameKey + "\n"))
	if err != nil {
		panic(fmt.Sprintf("Failed to send key: %v", err))
	}

	// Send client identification (StormFront protocol)
	clientID := "/FE:STORMFRONT /VERSION:1.0.26 /P:OSX /XML\n"
	_, err = gameConn.Write([]byte(clientID))
	if err != nil {
		panic(fmt.Sprintf("Failed to send client ID: %v", err))
	}

	fmt.Println("\n=== Connected to DragonRealms ===")

	// Create API instance
	api := NewGameAPI(gameConn, character, 1000) // 1000 line buffer

	// Start API server in background
	go func() {
		if err := StartAPIServer(api, ":8080"); err != nil {
			fmt.Printf("API server error: %v\n", err)
		}
	}()

	fmt.Println("API server starting on http://localhost:8080")

	// Check for CLI mode
	if os.Getenv("DR_CHARM_CLI") == "true" {
		fmt.Println("Running in CLI mode for testing...")
		RunCLIMode(gameConn, api)
		return
	}

	fmt.Println("Launching UI...")

	// Start Bubble Tea UI - Enhanced UI is now the default
	fmt.Println("Starting enhanced UI with multi-pane support...")
	p := tea.NewProgram(InitialEnhancedModel(gameConn, api), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		panic(fmt.Sprintf("Failed to start UI: %v", err))
	}
}

// encryptPassword implements DR's password encryption (hex string version)
func encryptPassword(password, hash string) string {
	maxLen := len(password)
	if len(hash) < maxLen {
		maxLen = len(hash)
	}

	result := make([]byte, maxLen)
	for i := 0; i < maxLen; i++ {
		hashByte := hash[i]
		passByte := password[i]
		encrypted := ((hashByte ^ (passByte - 32)) + 32)
		result[i] = encrypted
	}

	// Convert to hex string
	hexStr := ""
	for _, b := range result {
		hexStr += fmt.Sprintf("%02X", b) // Fixed: use %02X for consistent hex
	}

	return hexStr
}

// encryptPasswordBytes implements DR's password encryption (raw bytes version)
func encryptPasswordBytes(password, hash string) []byte {
	maxLen := len(password)
	if len(hash) < maxLen {
		maxLen = len(hash)
	}

	result := make([]byte, maxLen)
	for i := 0; i < maxLen; i++ {
		hashByte := hash[i]
		passByte := password[i]
		encrypted := ((hashByte ^ (passByte - 32)) + 32)
		result[i] = encrypted
	}

	return result
}

// extractValue extracts a value using regex pattern
func extractValue(data, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(data)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// runGameLoop handles the main game interaction loop
func runGameLoop(conn net.Conn) {
	// Create channels for communication
	userInput := make(chan string)
	done := make(chan bool)

	// Start goroutine to read from game
	go func() {
		for {
			// Read byte by byte to handle partial lines
			b := make([]byte, 1)
			_, err := conn.Read(b)
			if err != nil {
				fmt.Printf("\nConnection lost: %v\n", err)
				done <- true
				return
			}

			// Strip XML tags for now (basic approach)
			char := string(b[0])
			if char == "<" {
				// Skip until we find >
				for {
					_, err := conn.Read(b)
					if err != nil || string(b[0]) == ">" {
						break
					}
				}
				continue
			}

			// Print the character
			fmt.Print(char)
		}
	}()

	// Start goroutine to read user input
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			userInput <- scanner.Text()
		}
	}()

	// Main loop
	for {
		select {
		case <-done:
			return
		case input := <-userInput:
			if input == "quit" {
				fmt.Println("\nGoodbye!")
				return
			}
			// Send command to game
			_, err := conn.Write([]byte(input + "\n"))
			if err != nil {
				fmt.Printf("\nFailed to send command: %v\n", err)
				return
			}
		}
	}
}
