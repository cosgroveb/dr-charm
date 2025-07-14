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
		Width(m.width)

	outputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1).
		Width(m.width - 2).
		Height(m.height - 8) // Adjusted for status bar

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Width(m.width - 2)

	// Build status bar
	statusBar := m.buildStatusBar()

	// Build output
	var output strings.Builder
	boxHeight := m.height - 10 // Adjusted for status bar
	startIdx := 0
	if len(m.output) > boxHeight {
		startIdx = len(m.output) - boxHeight
	}

	for i := startIdx; i < len(m.output); i++ {
		output.WriteString(m.output[i] + "\n")
	}

	// Build view
	s := titleStyle.Render("DragonRealms") + "\n"
	s += statusStyle.Render(statusBar) + "\n"
	s += outputStyle.Render(output.String()) + "\n"
	s += inputStyle.Render("> " + m.input)

	if m.err != nil {
		s += "\n\nError: " + m.err.Error()
	}

	return s
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
	
	// Calculate percentages for future use (e.g., bar visualization)
	// manaPct := 0
	// if gs.MaxMana > 0 {
	// 	manaPct = gs.Mana * 100 / gs.MaxMana
	// }
	
	// stamPct := 0
	// if gs.MaxStamina > 0 {
	// 	stamPct = gs.Stamina * 100 / gs.MaxStamina
	// }
	
	// Stance
	stanceStr := getStanceName(gs.Stance)
	
	// Build status line
	health := lipgloss.NewStyle().Foreground(lipgloss.Color(healthColor)).Render(
		fmt.Sprintf("H:%d/%d", gs.Health, gs.MaxHealth))
	mana := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(
		fmt.Sprintf("M:%d/%d", gs.Mana, gs.MaxMana))
	stamina := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(
		fmt.Sprintf("S:%d/%d", gs.Stamina, gs.MaxStamina))
	stance := fmt.Sprintf("Stance:%s", stanceStr)
	
	rt := ""
	if gs.Roundtime > 0 {
		rt = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(
			fmt.Sprintf(" RT:%d", gs.Roundtime))
	}
	
	hands := ""
	if gs.RightHand != "" || gs.LeftHand != "" {
		r := gs.RightHand
		if r == "" {
			r = "Empty"
		}
		l := gs.LeftHand
		if l == "" {
			l = "Empty"
		}
		hands = fmt.Sprintf(" | R:%s L:%s", r, l)
	}
	
	return fmt.Sprintf("%s | %s | %s | %s%s%s", health, mana, stamina, stance, rt, hands)
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