package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EnhancedModel is the full-featured UI model
type EnhancedModel struct {
	// Core
	conn      net.Conn
	api       *GameAPI
	gameState *GameState

	// UI State
	width        int
	height       int
	quitting     bool
	err          error
	viewMode     ViewMode
	scrollOffset int
	autoScroll   bool

	// Input
	input        string
	history      []string
	historyIndex int

	// Systems
	xmlParser      *XMLStreamParser
	layout         *Layout
	triggerManager *TriggerManager
	themeManager   *ThemeManager
	logger         *Logger

	// Output buffer
	mainOutput []string
}

// ViewMode represents different UI modes
type ViewMode int

const (
	ViewModeSingle ViewMode = iota // Single pane (main output only)
	ViewModeMulti                  // Multi-pane layout
	ViewModeHelp                   // Help screen
	ViewModeTheme                  // Theme selector
)

// InitialEnhancedModel creates the enhanced model
func InitialEnhancedModel(conn net.Conn, api *GameAPI) EnhancedModel {
	// Get user home directory for config
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".dr-charm")

	// Create config directories
	os.MkdirAll(filepath.Join(configDir, "logs"), 0755)
	os.MkdirAll(filepath.Join(configDir, "themes"), 0755)

	// Initialize systems
	debug := os.Getenv("DR_CHARM_DEBUG") == "true"
	if debug {
		home, _ := os.UserHomeDir()
		debugPath := filepath.Join(home, ".dr-charm", "logs", "debug", fmt.Sprintf("raw-xml-%s.log", time.Now().Format("2006-01-02")))
		fmt.Printf("DEBUG mode enabled - raw XML will be logged to %s\n", debugPath)
	}
	xmlParser := NewXMLStreamParser(debug)
	gameState := xmlParser.GetState()

	// Set initial game state
	gameState.Health = 100
	gameState.MaxHealth = 100
	gameState.Stamina = 100
	gameState.MaxStamina = 100
	gameState.Concentration = 100
	gameState.MaxConcentration = 100
	gameState.Spirit = 100
	gameState.MaxSpirit = 100
	gameState.Stance = 3
	gameState.Aliases = make(map[string]string)

	return EnhancedModel{
		conn:           conn,
		api:            api,
		gameState:      gameState,
		width:          80,
		height:         24,
		viewMode:       ViewModeSingle,
		scrollOffset:   0,
		autoScroll:     true,
		mainOutput:     []string{"Connected to DragonRealms"},
		history:        []string{},
		xmlParser:      xmlParser,
		layout:         NewLayout(80, 24, debug),
		triggerManager: NewTriggerManager(),
		themeManager:   NewThemeManager(filepath.Join(configDir, "themes")),
		logger:         NewLogger(filepath.Join(configDir, "logs")),
	}
}

// Init initializes the model
func (m EnhancedModel) Init() tea.Cmd {
	// Start logging
	m.logger.Start("Cennedig")

	return readEnhancedGameOutput(m.conn, m.xmlParser)
}

// Update handles messages
func (m EnhancedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Calculate available height for layout (accounting for UI elements)
		// titleHeight(1) + statusHeight(1) + inputHeight(3) + gaps(2) = 7
		layoutHeight := msg.Height - 7
		if layoutHeight < 10 {
			layoutHeight = 10
		}
		m.layout.Resize(msg.Width, layoutHeight)
		return m, nil

	case enhancedGameMsg:
		return m.handleGameMessage(msg)

	case errMsg:
		m.err = msg
		m.quitting = true
		m.logger.Stop()
		if m.api != nil {
			m.api.connected.Store(false)
		}
		return m, tea.Quit
	}

	return m, nil
}

// handleKeyPress processes keyboard input
func (m EnhancedModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys work in any mode
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		m.logger.Stop()
		return m, tea.Quit

	case tea.KeyF1:
		m.viewMode = ViewModeHelp
		return m, nil

	case tea.KeyF2:
		if m.viewMode == ViewModeSingle {
			m.viewMode = ViewModeMulti
		} else {
			m.viewMode = ViewModeSingle
		}
		return m, nil

	case tea.KeyF3:
		m.viewMode = ViewModeTheme
		return m, nil

	case tea.KeyF4:
		// Toggle logging
		if m.logger.IsEnabled() {
			m.logger.Stop()
		} else {
			m.logger.Start("Cennedig")
		}
		return m, nil

	case tea.KeyTab:
		// Cycle through panes in multi-pane mode
		if m.viewMode == ViewModeMulti {
			m.layout.NextPane()
		}
		return m, nil
	}

	// Mode-specific handling
	switch m.viewMode {
	case ViewModeHelp:
		if msg.Type == tea.KeyEsc {
			m.viewMode = ViewModeSingle
		}

	case ViewModeTheme:
		m = m.handleThemeKeys(msg)

	default:
		// Normal input handling
		switch msg.Type {
		case tea.KeyEnter:
			if m.input != "" {
				// Process aliases
				cmd := m.triggerManager.ProcessCommand(m.input)

				// Send command
				_, err := m.conn.Write([]byte(cmd + "\n"))
				if err != nil {
					m.err = err
					return m, nil
				}

				// Log command
				m.logger.LogCommand(m.input)

				// Add to display and history
				m.addOutput("> " + m.input)
				m.history = append(m.history, m.input)
				m.historyIndex = len(m.history)
				m.input = ""

				// Auto-scroll to bottom
				m.scrollOffset = 0
				m.autoScroll = true
			}

		case tea.KeyUp:
			if m.historyIndex > 0 {
				m.historyIndex--
				if m.historyIndex < len(m.history) {
					m.input = m.history[m.historyIndex]
				}
			}

		case tea.KeyDown:
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

		case tea.KeyPgUp:
			m.scrollUp()

		case tea.KeyPgDown:
			m.scrollDown()

		case tea.KeyHome:
			m.scrollToTop()

		case tea.KeyEnd:
			m.scrollToBottom()

		default:
			if msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			}
		}
	}

	return m, nil
}

// handleGameMessage processes game output
func (m EnhancedModel) handleGameMessage(msg enhancedGameMsg) (tea.Model, tea.Cmd) {
	// Update game state
	m.gameState = msg.state
	m.layout.UpdateFromGameState(m.gameState)

	// Process and display text
	if msg.text != "" {
		lines := strings.Split(msg.text, "\n")
		inRoomDesc := false
		roomDescLines := []string{}
		
		for _, line := range lines {
			if line != "" {
				// Filter out empty "Obvious paths: , , ." lines
				if strings.Contains(line, "Obvious paths:") {
					// Check if it only contains commas, spaces, and periods after the colon
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						exits := strings.TrimSpace(parts[1])
						// Remove all commas, spaces, and periods
						cleaned := strings.Trim(exits, ", .")
						if cleaned == "" {
							continue
						}
					}
				}
				
				// Filter out XML fragments that leaked through
				if strings.Contains(line, "='") || strings.Contains(line, "=\"") ||
					strings.Contains(line, "/>") || strings.Contains(line, "</") ||
					(strings.Contains(line, "<") && strings.Contains(line, ">")) {
					continue
				}
				
				// Apply triggers for highlighting
				processed := m.triggerManager.ProcessLine(line)
				m.addOutput(processed)

				// Log the output
				m.logger.LogGameOutput(line)

				// Check for auto-look trigger
				if strings.Contains(line, "Please wait for connection") {
					go func() {
						time.Sleep(500 * time.Millisecond)
						m.conn.Write([]byte("look\n"))
					}()
				}
				
				// Check for room name in square brackets at the start of a line
				if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
					roomName := strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[")
					// Make sure it's not a system message (those usually have more text)
					if !strings.Contains(roomName, "login") && !strings.Contains(roomName, "You are") &&
					   !strings.Contains(roomName, "You are standing") {
						// Only update if it's a different room
						if m.gameState.Room.Title != roomName {
							m.gameState.Room.Title = roomName
							if m.xmlParser.debug {
								fmt.Printf("[DEBUG] Set room title: %s\n", roomName)
								fmt.Printf("[DEBUG] Current room desc: %s\n", m.gameState.Room.Description)
							}
							// Clear room data when entering a NEW room
							m.gameState.Room.Objects = []string{}
							m.gameState.Room.Description = ""
						}
						// Start capturing room description
						inRoomDesc = true
						roomDescLines = []string{}
						// Update the layout immediately
						m.layout.UpdateFromGameState(m.gameState)
					}
				}
				
				// Capture room description lines
				if inRoomDesc {
					// Stop capturing when we hit "Obvious paths:"
					if strings.Contains(line, "Obvious paths:") {
						// Save the accumulated description
						if len(roomDescLines) > 0 {
							m.gameState.Room.Description = strings.Join(roomDescLines, " ")
							m.layout.UpdateFromGameState(m.gameState)
						}
						inRoomDesc = false
					} else if strings.HasPrefix(strings.TrimSpace(line), "You also see") {
						// This is objects, not description, but save any description we have so far
						if len(roomDescLines) > 0 {
							m.gameState.Room.Description = strings.Join(roomDescLines, " ")
							m.layout.UpdateFromGameState(m.gameState)
						}
						inRoomDesc = false
						// Continue to process this line as objects below
					} else if !strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "Also here:") {
						// Add to room description (skip the room name line and "Also here:" lines)
						trimmed := strings.TrimSpace(line)
						if trimmed != "" && trimmed != "[You are standing up.]" {
							roomDescLines = append(roomDescLines, trimmed)
						}
					}
				}
				
				// Check for "You also see" lines
				trimmedLine := strings.TrimSpace(line)
				if strings.HasPrefix(trimmedLine, "You also see") {
					// Extract objects from the line
					parts := strings.SplitN(trimmedLine, "see", 2)
					if len(parts) == 2 {
						objectsText := strings.TrimSpace(parts[1])
						// Remove trailing period
						objectsText = strings.TrimSuffix(objectsText, ".")
						// Split by "and" and commas
						objectsText = strings.ReplaceAll(objectsText, " and ", ", ")
						objects := strings.Split(objectsText, ", ")
						
						// Clean up and add objects
						var cleanedObjects []string
						for _, obj := range objects {
							obj = strings.TrimSpace(obj)
							if obj != "" {
								// Handle "a" and "an" articles
								if strings.HasPrefix(obj, "a ") {
									obj = strings.TrimPrefix(obj, "a ")
								} else if strings.HasPrefix(obj, "an ") {
									obj = strings.TrimPrefix(obj, "an ")
								}
								cleanedObjects = append(cleanedObjects, obj)
							}
						}
						
						if len(cleanedObjects) > 0 {
							m.gameState.Room.Objects = cleanedObjects
							if m.xmlParser.debug {
								fmt.Printf("[DEBUG] Parsed room objects: %v\n", cleanedObjects)
							}
							// Update the layout immediately
							m.layout.UpdateFromGameState(m.gameState)
						}
					}
				}
			}
		}
	}

	// Reset scroll if auto-scroll enabled
	if m.autoScroll {
		m.scrollOffset = 0
	}

	return m, readEnhancedGameOutput(m.conn, m.xmlParser)
}

// handleThemeKeys handles theme selection
func (m EnhancedModel) handleThemeKeys(msg tea.KeyMsg) EnhancedModel {
	themes := m.themeManager.GetThemeNames()
	currentIdx := 0

	for i, name := range themes {
		if name == m.themeManager.currentTheme {
			currentIdx = i
			break
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		if currentIdx > 0 {
			m.themeManager.SetTheme(themes[currentIdx-1])
		}
	case tea.KeyDown:
		if currentIdx < len(themes)-1 {
			m.themeManager.SetTheme(themes[currentIdx+1])
		}
	case tea.KeyEnter, tea.KeyEsc:
		m.viewMode = ViewModeSingle
	}

	return m
}

// Scrolling methods
func (m *EnhancedModel) scrollUp() {
	visibleLines := m.height - 10
	if visibleLines < 1 {
		visibleLines = 1
	}
	m.scrollOffset += visibleLines
	maxOffset := len(m.mainOutput) - visibleLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	m.autoScroll = false
}

func (m *EnhancedModel) scrollDown() {
	visibleLines := m.height - 10
	if visibleLines < 1 {
		visibleLines = 1
	}
	m.scrollOffset -= visibleLines
	if m.scrollOffset <= 0 {
		m.scrollOffset = 0
		m.autoScroll = true
	}
}

func (m *EnhancedModel) scrollToTop() {
	visibleLines := m.height - 10
	if visibleLines < 1 {
		visibleLines = 1
	}
	m.scrollOffset = len(m.mainOutput) - visibleLines
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	m.autoScroll = false
}

func (m *EnhancedModel) scrollToBottom() {
	m.scrollOffset = 0
	m.autoScroll = true
}

// addOutput adds a line to the main output
func (m *EnhancedModel) addOutput(line string) {
	m.mainOutput = append(m.mainOutput, line)
	if m.api != nil {
		m.api.AddOutput(line)
	}

	// Update layout main pane
	m.layout.AddLineToPane("main", line)

	// Keep reasonable buffer
	if len(m.mainOutput) > 2000 {
		m.mainOutput = m.mainOutput[len(m.mainOutput)-2000:]
	}
}

// View renders the UI
func (m EnhancedModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	switch m.viewMode {
	case ViewModeHelp:
		return m.renderHelp()
	case ViewModeTheme:
		return m.renderThemeSelector()
	case ViewModeMulti:
		return m.renderMultiPane()
	default:
		return m.renderSinglePane()
	}
}

// renderSinglePane renders the single-pane view
func (m EnhancedModel) renderSinglePane() string {
	theme := m.themeManager.GetTheme()

	// Calculate component heights
	titleHeight := 1
	statusHeight := 1
	inputHeight := 3
	errorHeight := 0
	if m.err != nil {
		errorHeight = 2
	}

	totalFixedHeight := titleHeight + statusHeight + inputHeight + errorHeight + 2
	outputHeight := m.height - totalFixedHeight
	if outputHeight < 3 {
		outputHeight = 3
	}

	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Colors.TitleBar)).
		Align(lipgloss.Center).
		Width(m.width)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Colors.StatusBar)).
		Background(lipgloss.Color(theme.Colors.StatusBarBg)).
		Padding(0, 1).
		Width(m.width)

	borderStyle := m.themeManager.CreateBorderStyle()
	outputStyle := borderStyle.
		Width(m.width - 2).
		Height(outputHeight).
		MaxHeight(outputHeight)

	inputStyle := borderStyle.
		Width(m.width - 2)

	// Build content
	title := m.buildTitle()
	statusBar := m.buildStatusBar()
	output := m.buildOutput(outputHeight - 2)
	input := m.buildInput()

	// Combine components
	components := []string{
		titleStyle.Render(title),
		statusStyle.Render(statusBar),
		outputStyle.Render(output),
		inputStyle.Render(input),
	}

	result := strings.Join(components, "\n")

	if m.err != nil {
		result += "\n\nError: " + m.err.Error()
	}

	return result
}

// renderMultiPane renders the multi-pane layout
func (m EnhancedModel) renderMultiPane() string {
	theme := m.themeManager.GetTheme()
	
	// Calculate component heights
	titleHeight := 1
	statusHeight := 1
	inputHeight := 3
	errorHeight := 0
	if m.err != nil {
		errorHeight = 2
	}
	
	// Reserve space for fixed UI elements
	totalFixedHeight := titleHeight + statusHeight + inputHeight + errorHeight + 2
	layoutHeight := m.height - totalFixedHeight
	if layoutHeight < 10 {
		layoutHeight = 10
	}
	
	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Colors.TitleBar)).
		Align(lipgloss.Center).
		Width(m.width)
	
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Colors.StatusBar)).
		Background(lipgloss.Color(theme.Colors.StatusBarBg)).
		Padding(0, 1).
		Width(m.width)
	
	borderStyle := m.themeManager.CreateBorderStyle()
	inputStyle := borderStyle.
		Width(m.width - 2)
	
	// Build content
	title := m.buildTitle()
	statusBar := m.buildStatusBar()
	layoutContent := m.layout.RenderWithHeight(layoutHeight)
	input := m.buildInput()
	
	// Wrap layout content to ensure it doesn't exceed allocated height
	layoutStyle := lipgloss.NewStyle().
		MaxHeight(layoutHeight).
		Height(layoutHeight)
	
	// Combine components
	components := []string{
		titleStyle.Render(title),
		statusStyle.Render(statusBar),
		layoutStyle.Render(layoutContent),
		inputStyle.Render(input),
	}
	
	result := strings.Join(components, "\n")
	
	if m.err != nil {
		result += "\n\nError: " + m.err.Error()
	}
	
	return result
}

// renderHelp renders the help screen
func (m EnhancedModel) renderHelp() string {
	theme := m.themeManager.GetTheme()

	helpText := `DragonRealms Charm CLI - Help

KEYBOARD SHORTCUTS:
  F1          - Show this help
  F2          - Toggle single/multi-pane view
  F3          - Theme selector
  F4          - Toggle logging
  Tab         - Cycle through panes (multi-pane mode)
  
  PgUp/PgDn   - Scroll output
  Home/End    - Jump to top/bottom
  Up/Down     - Command history
  
  Ctrl+C      - Quit
  Esc         - Close dialogs

COMMAND ALIASES:
  Movement: n, s, e, w, ne, nw, se, sw, u, d, o
  Actions: l (look), i (inventory), sta (stand), sit, kne (kneel)
  Combat: att (attack), ki (kill), sk (skin), loot (loot all)

FEATURES:
  • Triggers highlight important text (combat, whispers, etc.)
  • Auto-look on connection
  • Session logging to ~/.dr-charm/logs/
  • Custom themes in ~/.dr-charm/themes/
  • Room tracking with exits and objects
  • Hands and spell tracking
  • Familiar window for companion messages

Press ESC to return`

	style := lipgloss.NewStyle().
		Padding(2).
		Width(m.width).
		Height(m.height).
		Foreground(lipgloss.Color(theme.Colors.Foreground))

	return style.Render(helpText)
}

// renderThemeSelector renders the theme selection screen
func (m EnhancedModel) renderThemeSelector() string {
	theme := m.themeManager.GetTheme()
	themes := m.themeManager.GetThemeNames()

	var content strings.Builder
	content.WriteString("Select Theme (Up/Down to navigate, Enter to select, ESC to cancel)\n\n")

	for _, name := range themes {
		if name == m.themeManager.currentTheme {
			content.WriteString(fmt.Sprintf("  > %s (current)\n", name))
		} else {
			content.WriteString(fmt.Sprintf("    %s\n", name))
		}
	}

	style := lipgloss.NewStyle().
		Padding(2).
		Width(m.width).
		Height(m.height).
		Foreground(lipgloss.Color(theme.Colors.Foreground))

	return style.Render(content.String())
}

// Helper methods for building UI components
func (m EnhancedModel) buildTitle() string {
	title := "DragonRealms"
	if m.gameState.Room.Title != "" {
		title = fmt.Sprintf("DragonRealms [%s]", m.gameState.Room.Title)
	}

	// Add logging indicator if enabled (small indicator at the end)
	if m.logger.IsEnabled() {
		title += " •"
	}

	return title
}

func (m EnhancedModel) buildStatusBar() string {
	gs := m.gameState
	theme := m.themeManager.GetTheme()
	compactMode := m.width < 60

	var components []string

	// Health
	healthColor := theme.Colors.HealthGood
	if gs.Health <= 33 {
		healthColor = theme.Colors.HealthCrit
	} else if gs.Health <= 66 {
		healthColor = theme.Colors.HealthWarn
	}
	healthStr := fmt.Sprintf("H:%d%%", gs.Health)
	if compactMode {
		healthStr = fmt.Sprintf("H:%d", gs.Health)
	}
	health := lipgloss.NewStyle().Foreground(lipgloss.Color(healthColor)).Render(healthStr)
	components = append(components, health)

	// Mana (if applicable)
	if gs.MaxMana > 0 && gs.Mana > 0 {
		manaColor := theme.Colors.ManaGood
		if gs.Mana < 33 {
			manaColor = theme.Colors.ManaLow
		} else if gs.Mana < 66 {
			manaColor = theme.Colors.ManaWarn
		}
		manaStr := fmt.Sprintf("M:%d%%", gs.Mana)
		if compactMode {
			manaStr = fmt.Sprintf("M:%d", gs.Mana)
		}
		mana := lipgloss.NewStyle().Foreground(lipgloss.Color(manaColor)).Render(manaStr)
		components = append(components, mana)
	}

	// Stamina/Fatigue
	fatigueUsed := 100 - gs.Stamina
	staminaColor := theme.Colors.StaminaGood
	if fatigueUsed > 66 {
		staminaColor = theme.Colors.StaminaLow
	} else if fatigueUsed > 33 {
		staminaColor = theme.Colors.StaminaWarn
	}
	staminaStr := fmt.Sprintf("F:%d%%", fatigueUsed)
	if compactMode {
		staminaStr = fmt.Sprintf("F:%d", fatigueUsed)
	}
	stamina := lipgloss.NewStyle().Foreground(lipgloss.Color(staminaColor)).Render(staminaStr)
	components = append(components, stamina)

	// Add other vitals if space allows
	if !compactMode || m.width > 45 {
		// Concentration
		concStr := fmt.Sprintf("C:%d%%", gs.Concentration)
		if compactMode {
			concStr = fmt.Sprintf("C:%d", gs.Concentration)
		}
		components = append(components, concStr)

		// Spirit
		spiritStr := fmt.Sprintf("Sp:%d%%", gs.Spirit)
		if compactMode {
			spiritStr = fmt.Sprintf("S:%d", gs.Spirit)
		}
		components = append(components, spiritStr)
	}

	// Stance
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
		rt := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Error)).
			Bold(true).
			Render(fmt.Sprintf("RT:%d", gs.Roundtime))
		status += separator + rt
	}

	return status
}

func (m EnhancedModel) buildOutput(maxLines int) string {
	theme := m.themeManager.GetTheme()

	// Calculate visible lines
	visibleLines := maxLines
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Get lines to display with scroll offset
	totalLines := len(m.mainOutput)
	endIdx := totalLines - m.scrollOffset
	startIdx := endIdx - visibleLines

	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > totalLines {
		endIdx = totalLines
	}

	var output strings.Builder
	for i := startIdx; i < endIdx; i++ {
		if i > startIdx {
			output.WriteString("\n")
		}
		output.WriteString(m.mainOutput[i])
	}

	// Add scroll indicator
	if m.scrollOffset > 0 {
		scrollInfo := fmt.Sprintf("\n[Scrolled up %d lines - PgDn/End to return]", m.scrollOffset)
		output.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.ScrollIndicator)).
			Italic(true).
			Render(scrollInfo))
	}

	return output.String()
}

func (m EnhancedModel) buildInput() string {
	return "> " + m.input
}

// Message types
type enhancedGameMsg struct {
	text  string
	state *GameState
}

// readEnhancedGameOutput reads and parses game output
func readEnhancedGameOutput(conn net.Conn, parser *XMLStreamParser) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return errMsg(err)
		}

		text, err := parser.ParseChunk(buf[:n])
		if err != nil {
			// Log parsing errors but don't fail
			text = string(buf[:n])
		}

		return enhancedGameMsg{
			text:  text,
			state: parser.GetState(),
		}
	}
}
