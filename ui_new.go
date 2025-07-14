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
	vitalsParser *VitalsParser
	gameState    *GameState
}

// InitialModelV2 creates the enhanced model
func InitialModelV2(conn net.Conn, api *GameAPI) ModelV2 {
	parser := NewXMLParser(false) // Disable debug
	gameState := parser.GetState()
	// Initialize with default values so we don't show 0/0
	gameState.Health = 100
	gameState.MaxHealth = 100
	gameState.Mana = 0
	gameState.MaxMana = 0
	gameState.Stamina = 100
	gameState.MaxStamina = 100
	gameState.Concentration = 100
	gameState.MaxConcentration = 100
	gameState.Spirit = 100
	gameState.MaxSpirit = 100
	gameState.Stance = 3 // Standing
	
	return ModelV2{
		conn:         conn,
		api:          api,
		output:       []string{"Connected to DragonRealms"},
		input:        "",
		history:      []string{},
		parser:       parser,
		vitalsParser: NewVitalsParser(),
		gameState:    gameState,
		width:        80,  // Default width
		height:       24,  // Default height
	}
}

// Init initializes the model
func (m ModelV2) Init() tea.Cmd {
	return readGameOutputV2(m.conn, m.parser, m.vitalsParser)
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
		return m, readGameOutputV2(m.conn, m.parser, m.vitalsParser)

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
		Width(m.width)

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
	
	// Determine if we need compact mode based on terminal width
	// Estimate ~50 chars for full display
	compactMode := m.width < 60
	
	// Build status components
	var components []string
	
	// Health - always show
	healthColor := "196" // red
	if gs.Health > 66 {
		healthColor = "46" // green
	} else if gs.Health > 33 {
		healthColor = "226" // yellow
	}
	healthStr := fmt.Sprintf("H:%d%%", gs.Health)
	if compactMode {
		healthStr = fmt.Sprintf("H:%d", gs.Health)
	}
	health := lipgloss.NewStyle().Foreground(lipgloss.Color(healthColor)).Render(healthStr)
	components = append(components, health)
	
	// Mana - only show if player has mana
	if gs.MaxMana > 0 && gs.Mana > 0 {
		manaColor := "33" // blue
		if gs.Mana < 33 {
			manaColor = "196" // red
		} else if gs.Mana < 66 {
			manaColor = "226" // yellow
		}
		manaStr := fmt.Sprintf("M:%d%%", gs.Mana)
		if compactMode {
			manaStr = fmt.Sprintf("M:%d", gs.Mana)
		}
		mana := lipgloss.NewStyle().Foreground(lipgloss.Color(manaColor)).Render(manaStr)
		components = append(components, mana)
	}
	
	// Stamina/Fatigue - always show (inverted - 100% = fully rested)
	fatigueUsed := 100 - gs.Stamina
	staminaColor := "214" // orange
	if fatigueUsed > 66 {
		staminaColor = "196" // red (very tired)
	} else if fatigueUsed > 33 {
		staminaColor = "226" // yellow (somewhat tired)
	}
	staminaStr := fmt.Sprintf("F:%d%%", fatigueUsed)
	if compactMode {
		staminaStr = fmt.Sprintf("F:%d", fatigueUsed)
	}
	stamina := lipgloss.NewStyle().Foreground(lipgloss.Color(staminaColor)).Render(staminaStr)
	components = append(components, stamina)
	
	// In compact mode, skip concentration and spirit if really tight
	if !compactMode || m.width > 45 {
		// Concentration
		concColor := "51" // cyan
		if gs.Concentration < 33 {
			concColor = "196" // red
		} else if gs.Concentration < 66 {
			concColor = "226" // yellow
		}
		concStr := fmt.Sprintf("C:%d%%", gs.Concentration)
		if compactMode {
			concStr = fmt.Sprintf("C:%d", gs.Concentration)
		}
		conc := lipgloss.NewStyle().Foreground(lipgloss.Color(concColor)).Render(concStr)
		components = append(components, conc)
		
		// Spirit
		spiritColor := "135" // purple
		if gs.Spirit < 33 {
			spiritColor = "196" // red
		} else if gs.Spirit < 66 {
			spiritColor = "226" // yellow
		}
		spiritStr := fmt.Sprintf("Sp:%d%%", gs.Spirit)
		if compactMode {
			spiritStr = fmt.Sprintf("S:%d", gs.Spirit)
		}
		spirit := lipgloss.NewStyle().Foreground(lipgloss.Color(spiritColor)).Render(spiritStr)
		components = append(components, spirit)
	}
	
	// Stance - abbreviate in compact mode
	stanceStr := getStanceName(gs.Stance)
	if compactMode && len(stanceStr) > 3 {
		stanceStr = stanceStr[:3]
	}
	stance := fmt.Sprintf("St:%s", stanceStr)
	if compactMode {
		stance = stanceStr
	}
	components = append(components, stance)
	
	// Build status line
	separator := " | "
	if compactMode {
		separator = " "
	}
	status := strings.Join(components, separator)
	
	// Add RT if present
	if gs.Roundtime > 0 {
		rt := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(
			fmt.Sprintf("RT:%d", gs.Roundtime))
		status += separator + rt
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
func readGameOutputV2(conn net.Conn, parser *XMLParser, vitalsParser *VitalsParser) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return errMsg(err)
		}

		rawData := string(buf[:n])

		// Parse XML and extract display text
		reader := strings.NewReader(rawData)
		text, err := parser.ParseStream(reader)
		if err != nil && err != io.EOF {
			// Log parsing errors but don't fail
			text = rawData // Fallback to raw text
		}

		// Get current state
		state := parser.GetState()
		
		// Try additional vitals parsing
		vitalsParser.ParsePromptXML(rawData, state)
		vitalsParser.ParseFromText(text, state)

		// Decode HTML entities
		text = html.UnescapeString(text)

		return gameOutputMsgV2{
			text:  text,
			state: state,
		}
	}
}