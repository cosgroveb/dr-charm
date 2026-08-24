package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dr-charm/internal/automation"
	"dr-charm/internal/dragonrealms"
	"dr-charm/internal/telemetry"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type gameSession interface {
	Send(string) error
	Updates() <-chan dragonrealms.Update
}

// EnhancedModel renders DragonRealms Session updates.
type EnhancedModel struct {
	session   gameSession
	character string
	snapshot  dragonrealms.Snapshot

	width        int
	height       int
	quitting     bool
	err          error
	viewMode     ViewMode
	scrollOffset int
	autoScroll   bool

	input        string
	history      []string
	historyIndex int

	layout         layout
	triggerManager *automation.TriggerManager
	themes         *themeCatalog
	logger         *telemetry.Logger
	mainOutput     []string
	familiarOutput []string
	activePane     string
}

// ViewMode selects the visible terminal layout.
type ViewMode int

const (
	ViewModeSingle ViewMode = iota
	ViewModeMulti
	ViewModeHelp
	ViewModeTheme
)

// InitialEnhancedModel constructs the UI around a Session consumer boundary.
func InitialEnhancedModel(session gameSession, character string) EnhancedModel {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".dr-charm")
	logDir := filepath.Join(configDir, "logs")
	themeDir := filepath.Join(configDir, "themes")
	return EnhancedModel{
		session:        session,
		character:      character,
		snapshot:       dragonrealms.Snapshot{Connection: dragonrealms.ConnectionConnected, Character: character},
		width:          80,
		height:         24,
		viewMode:       ViewModeMulti,
		autoScroll:     true,
		mainOutput:     []string{"Connected to DragonRealms"},
		layout:         newLayout(),
		triggerManager: automation.NewTriggerManager(),
		themes:         newThemeCatalog(themeDir),
		logger:         telemetry.NewLogger(logDir),
		activePane:     "main",
	}
}

// Init starts session logging and waits for the first update.
func (m EnhancedModel) Init() tea.Cmd {
	_ = m.logger.Start(m.character)
	return waitForSessionUpdate(m.session)
}

// Update applies terminal input or one detached Session update.
func (m EnhancedModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(message)
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		return m, nil
	case dragonrealms.Update:
		m.applySessionUpdate(message)
		return m, waitForSessionUpdate(m.session)
	case sessionClosedMsg:
		m.quitting = true
		m.logger.Stop()
		return m, tea.Quit
	}
	return m, nil
}

func (m *EnhancedModel) applySessionUpdate(update dragonrealms.Update) {
	m.snapshot = update.Snapshot
	for _, display := range update.Display {
		switch display.Kind {
		case dragonrealms.DisplayText:
			if display.DuplicateEcho {
				continue
			}
			if display.Stream == "familiar" {
				m.addFamiliarOutput(display.Text)
				continue
			}
			processed := m.triggerManager.ProcessLine(display.Text)
			m.addOutput(processed)
			m.logger.LogGameOutput(display.Text)
		case dragonrealms.DisplayClear:
			pane := display.Stream
			if pane == "" {
				pane = display.ID
			}
			if pane == "familiar" || pane == "main" {
				if pane == "main" {
					m.mainOutput = nil
				} else {
					m.familiarOutput = nil
					if m.activePane == "familiar" {
						m.activePane = "main"
					}
				}
			}
		}
	}
	for _, diagnostic := range update.Diagnostics {
		m.addOutput("[protocol] " + diagnostic.Text)
	}
	if update.Err != nil {
		m.err = update.Err
	}
	if m.autoScroll {
		m.scrollOffset = 0
	}
}

func (m EnhancedModel) handleKeyPress(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.Type {
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
		if m.logger.IsEnabled() {
			m.logger.Stop()
		} else {
			_ = m.logger.Start(m.character)
		}
		return m, nil
	case tea.KeyTab:
		if m.viewMode == ViewModeMulti {
			m.cyclePane()
		}
		return m, nil
	}

	if m.viewMode == ViewModeHelp {
		if message.Type == tea.KeyEsc {
			m.viewMode = ViewModeSingle
		}
		return m, nil
	}
	if m.viewMode == ViewModeTheme {
		return m.handleThemeKeys(message), nil
	}

	switch message.Type {
	case tea.KeyEnter:
		if m.input == "" {
			return m, nil
		}
		original := m.input
		command := m.triggerManager.ProcessCommand(original)
		if err := m.session.Send(command); err != nil {
			m.err = err
			return m, nil
		}
		m.logger.LogCommand(original)
		m.addOutput("> " + original)
		m.history = append(m.history, original)
		m.historyIndex = len(m.history)
		m.input = ""
		m.scrollOffset = 0
		m.autoScroll = true
	case tea.KeyUp:
		if m.historyIndex > 0 {
			m.historyIndex--
			m.input = m.history[m.historyIndex]
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
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
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
	case tea.KeyRunes:
		m.input += string(message.Runes)
	}
	return m, nil
}

func (m EnhancedModel) handleThemeKeys(message tea.KeyMsg) EnhancedModel {
	switch message.Type {
	case tea.KeyUp:
		m.themes.previous()
	case tea.KeyDown:
		m.themes.next()
	case tea.KeyEnter, tea.KeyEsc:
		m.viewMode = ViewModeSingle
	}
	return m
}

func (m *EnhancedModel) addOutput(line string) {
	m.mainOutput = append(m.mainOutput, line)
	if len(m.mainOutput) > 500 {
		m.mainOutput = append([]string(nil), m.mainOutput[len(m.mainOutput)-500:]...)
	}
}

func (m *EnhancedModel) addFamiliarOutput(line string) {
	m.familiarOutput = append(m.familiarOutput, line)
	if len(m.familiarOutput) > 100 {
		m.familiarOutput = append([]string(nil), m.familiarOutput[len(m.familiarOutput)-100:]...)
	}
	if !familiarAvailable(m.familiarOutput) {
		m.activePane = "main"
	}
}

func (m *EnhancedModel) cyclePane() {
	limit := len(paneOrder)
	if !familiarAvailable(m.familiarOutput) {
		limit--
	}
	for index := range limit {
		if paneOrder[index] == m.activePane {
			m.activePane = paneOrder[(index+1)%limit]
			return
		}
	}
}

func (m *EnhancedModel) scrollUp() {
	visible := max(m.height-10, 1)
	m.scrollOffset += visible
	maximum := max(len(m.mainOutput)-visible, 0)
	if m.scrollOffset > maximum {
		m.scrollOffset = maximum
	}
	m.autoScroll = false
}

func (m *EnhancedModel) scrollDown() {
	m.scrollOffset -= max(m.height-10, 1)
	if m.scrollOffset <= 0 {
		m.scrollOffset = 0
		m.autoScroll = true
	}
}

func (m *EnhancedModel) scrollToTop() {
	m.scrollOffset = max(len(m.mainOutput)-max(m.height-10, 1), 0)
	m.autoScroll = false
}

func (m *EnhancedModel) scrollToBottom() {
	m.scrollOffset = 0
	m.autoScroll = true
}

// View renders the current terminal screen.
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

func (m EnhancedModel) renderSinglePane() string {
	theme := m.themes.current()
	outputHeight := max(m.height-7, 3)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.TitleBar)).Align(lipgloss.Center).Width(m.width).Render(m.buildTitle())
	status := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.StatusBar)).Background(lipgloss.Color(theme.StatusBarBg)).Padding(0, 1).Width(m.width).Render(m.buildStatusBar())
	border := m.themes.borderStyle().Width(m.width - 2)
	result := strings.Join([]string{title, status, border.Render(m.buildOutput(outputHeight - 2)), border.Render(m.buildInput())}, "\n")
	if m.err != nil {
		result += "\n\nError: " + m.err.Error()
	}
	return result
}

func (m EnhancedModel) renderMultiPane() string {
	theme := m.themes.current()
	layoutHeight := max(m.height-7, 10)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.TitleBar)).Align(lipgloss.Center).Width(m.width).Render(m.buildTitle())
	status := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.StatusBar)).Background(lipgloss.Color(theme.StatusBarBg)).Padding(0, 1).Width(m.width).Render(m.buildStatusBar())
	input := m.themes.borderStyle().Width(m.width - 2).Render(m.buildInput())
	layout := lipgloss.NewStyle().MaxHeight(layoutHeight).Height(layoutHeight).Render(m.layout.render(m.width, layoutHeight, m.snapshot, m.mainOutput, m.familiarOutput, m.activePane))
	result := strings.Join([]string{title, status, layout, input}, "\n")
	if m.err != nil {
		result += "\n\nError: " + m.err.Error()
	}
	return result
}

func (m EnhancedModel) renderHelp() string {
	text := `DragonRealms Charm - Help

F1          Show this help
F2          Toggle multi/single-pane view
F3          Select theme
F4          Toggle logging
Tab         Cycle panes
PgUp/PgDn   Scroll output
Home/End    Jump to top/bottom
Up/Down     Command history
Ctrl+C      Quit

Press ESC to return`
	return lipgloss.NewStyle().Padding(2).Width(m.width).Height(m.height).Foreground(lipgloss.Color(m.themes.current().Foreground)).Render(text)
}

func (m EnhancedModel) renderThemeSelector() string {
	var content strings.Builder
	content.WriteString("Select Theme (Up/Down, Enter, ESC)\n\n")
	current := m.themes.current().Name
	for _, name := range m.themes.names() {
		prefix := "    "
		if name == current {
			prefix = "  > "
		}
		fmt.Fprintf(&content, "%s%s\n", prefix, name)
	}
	return lipgloss.NewStyle().Padding(2).Width(m.width).Height(m.height).Foreground(lipgloss.Color(m.themes.current().Foreground)).Render(content.String())
}

func (m EnhancedModel) buildTitle() string {
	title := "DragonRealms"
	if m.snapshot.Room.Title != "" {
		title += " " + m.snapshot.Room.Title
	}
	if m.logger.IsEnabled() {
		title += " *"
	}
	return title
}

func (m EnhancedModel) buildStatusBar() string {
	vitals := m.snapshot.Vitals
	posture := map[dragonrealms.Posture]string{
		dragonrealms.PostureStanding: "Standing",
		dragonrealms.PostureKneeling: "Kneeling",
		dragonrealms.PostureSitting:  "Sitting",
		dragonrealms.PostureProne:    "Prone",
	}[m.snapshot.Posture]
	if posture == "" {
		posture = "Unknown"
	}
	return fmt.Sprintf("H:%d%% | M:%d%% | F:%d%% | C:%d%% | Sp:%d%% | %s", vitals.Health, vitals.Mana, 100-vitals.Stamina, vitals.Concentration, vitals.Spirit, posture)
}

func (m EnhancedModel) buildOutput(maxLines int) string {
	visible := max(maxLines, 1)
	end := len(m.mainOutput) - m.scrollOffset
	start := max(end-visible, 0)
	if end < start {
		end = start
	}
	output := strings.Join(m.mainOutput[start:end], "\n")
	if m.scrollOffset > 0 {
		output += fmt.Sprintf("\n[Scrolled up %d lines]", m.scrollOffset)
	}
	return output
}

func (m EnhancedModel) buildInput() string {
	prompt := m.snapshot.Prompt
	if prompt == "" {
		prompt = ">"
	}
	return prompt + " " + m.input
}

type sessionClosedMsg struct{}

func waitForSessionUpdate(session gameSession) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-session.Updates()
		if !ok {
			return sessionClosedMsg{}
		}
		return update
	}
}
