package main

import (
	"fmt"
	"html"
	"io"
	"net"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Enhanced Model with game state
type ModelV2 struct {
	conn         net.Conn
	api          *GameAPI
	output       []string
	input        string
	history      []string
	historyIndex int
	err          error
	quitting     bool
	width        int
	height       int
	parser       *XMLParser
	gameState    *GameState
}

// InitialModelV2 creates the enhanced model
func InitialModelV2(conn net.Conn, api *GameAPI) ModelV2 {
	parser := NewXMLParser(false) // Set to true for debug
	return ModelV2{
		conn:      conn,
		api:       api,
		output:    []string{"Connected to DragonRealms"},
		input:     "",
		history:   []string{},
		parser:    parser,
		gameState: parser.GetState(),
	}
}

// Init initializes the model
func (m ModelV2) Init() tea.Cmd {
	return readGameOutputV2(m.conn, m.parser)
}

// Update handles messages
func (m ModelV2) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
			
		case tea.KeyEnter:
			if m.input != "" {
				// Send command to game
				_, err := m.conn.Write([]byte(m.input + "\n"))
				if err != nil {
					m.err = err
					return m, nil
				}
				// Add to output and history
				m.output = append(m.output, "> "+m.input)
				if m.api != nil {
					m.api.AddOutput("> " + m.input)
				}
				m.history = append(m.history, m.input)
				m.historyIndex = len(m.history)
				// Keep only last 1000 lines
				if len(m.output) > 1000 {
					m.output = m.output[len(m.output)-1000:]
				}
				m.input = ""
			}
			
		case tea.KeyUp:
			// Navigate command history
			if m.historyIndex > 0 {
				m.historyIndex--
				if m.historyIndex < len(m.history) {
					m.input = m.history[m.historyIndex]
				}
			}
			
		case tea.KeyDown:
			// Navigate command history
			if m.historyIndex < len(m.history)-1 {
				m.historyIndex++
				m.input = m.history[m.historyIndex]
			} else if m.historyIndex == len(m.history)-1 {
				m.historyIndex = len(m.history)
				m.input = ""
			}
			
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			
		case tea.KeySpace:
			m.input += " "
			
		default:
			if msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case gameOutputMsgV2:
		// Add parsed output
		lines := strings.Split(msg.text, "\n")
		for _, line := range lines {
			if line != "" {
				m.output = append(m.output, line)
				if m.api != nil {
					m.api.AddOutput(line)
				}
			}
		}
		// Keep only last 1000 lines
		if len(m.output) > 1000 {
			m.output = m.output[len(m.output)-1000:]
		}
		// Update game state
		m.gameState = msg.state
		// Continue reading
		return m, readGameOutputV2(m.conn, m.parser)

	case errMsg:
		m.err = msg
		m.quitting = true
		if m.api != nil {
			m.api.connected.Store(false)
		}
		return m, tea.Quit
	}

	return m, nil
}

// View renders the UI
func (m ModelV2) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	// Calculate component heights
	titleHeight := 1
	statusHeight := 1
	inputHeight := 3 // border + content + border
	errorHeight := 0
	if m.err != nil {
		errorHeight = 2
	}
	
	// Calculate output window height
	totalFixedHeight := titleHeight + statusHeight + inputHeight + errorHeight + 2 // +2 for margins
	outputHeight := m.height - totalFixedHeight
	if outputHeight < 3 {
		outputHeight = 3
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Align(lipgloss.Center).
		Width(m.width)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Background(lipgloss.Color("235")).
		Padding(0, 1).
		Width(m.width).
		MaxWidth(m.width)

	outputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1).
		Width(m.width - 2).
		Height(outputHeight).
		MaxHeight(outputHeight)

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Width(m.width - 2)

	// Build status bar
	statusBar := m.buildStatusBar()

	// Build output content
	var output strings.Builder
	
	// Calculate visible lines (account for padding)
	visibleLines := outputHeight - 2 // -2 for padding
	if visibleLines < 1 {
		visibleLines = 1
	}
	
	// Get the lines to display
	startIdx := 0
	if len(m.output) > visibleLines {
		startIdx = len(m.output) - visibleLines
	}

	for i := startIdx; i < len(m.output); i++ {
		if i > startIdx {
			output.WriteString("\n")
		}
		output.WriteString(m.output[i])
	}

	// Build view - order matters!
	components := []string{
		titleStyle.Render("DragonRealms"),
		statusStyle.Render(statusBar),
		outputStyle.Render(output.String()),
		inputStyle.Render("> " + m.input),
	}

	result := strings.Join(components, "\n")

	if m.err != nil {
		result += "\n\nError: " + m.err.Error()
	}

	return result
}

func (m ModelV2) buildStatusBar() string {
	gs := m.gameState
	
	// Health bar
	healthPct := 0
	if gs.MaxHealth > 0 {
		healthPct = gs.Health * 100 / gs.MaxHealth
	}
	healthColor := "196" // red
	if healthPct > 66 {
		healthColor = "46" // green
	} else if healthPct > 33 {
		healthColor = "226" // yellow
	}
	
	// Stance
	stanceStr := getStanceName(gs.Stance)
	
	// Build status components
	health := lipgloss.NewStyle().Foreground(lipgloss.Color(healthColor)).Render(
		fmt.Sprintf("H:%d/%d", gs.Health, gs.MaxHealth))
	mana := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(
		fmt.Sprintf("M:%d/%d", gs.Mana, gs.MaxMana))
	stamina := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(
		fmt.Sprintf("S:%d/%d", gs.Stamina, gs.MaxStamina))
	stance := fmt.Sprintf("St:%s", stanceStr)
	
	// Build basic status
	status := fmt.Sprintf("%s | %s | %s | %s", health, mana, stamina, stance)
	
	// Add RT if present
	if gs.Roundtime > 0 {
		rt := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(
			fmt.Sprintf("RT:%d", gs.Roundtime))
		status += " | " + rt
	}
	
	// Truncate status if too long for screen width
	// Account for padding in status style (2 chars)
	maxLen := m.width - 2
	if len(status) > maxLen && maxLen > 20 {
		status = status[:maxLen-3] + "..."
	}
	
	return status
}

func getStanceName(stance int) string {
	stances := []string{"Prone", "Sitting", "Kneeling", "Standing", "Hiding"}
	if stance >= 0 && stance < len(stances) {
		return stances[stance]
	}
	return fmt.Sprintf("%d", stance)
}

// Message types
type gameOutputMsgV2 struct {
	text  string
	state *GameState
}

// readGameOutputV2 reads and parses game output
func readGameOutputV2(conn net.Conn, parser *XMLParser) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return errMsg(err)
		}

		// Parse XML and extract display text
		reader := strings.NewReader(string(buf[:n]))
		text, err := parser.ParseStream(reader)
		if err != nil && err != io.EOF {
			// Log parsing errors but don't fail
			text = string(buf[:n]) // Fallback to raw text
		}

		// Decode HTML entities
		text = html.UnescapeString(text)

		return gameOutputMsgV2{
			text:  text,
			state: parser.GetState(),
		}
	}
}