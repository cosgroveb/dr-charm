package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dr-charm/internal/automation"
	"dr-charm/internal/presentation"
	"dr-charm/internal/telemetry"
	"dr-charm/internal/terminaltext"
)

const (
	paneInput    = "input"
	paneMain     = "main"
	paneRoom     = "room"
	paneHands    = "hands"
	paneFamiliar = "familiar"
)

var paneOrder = [...]string{paneMain, paneRoom, paneHands, paneFamiliar}

type gameSession interface {
	Send(string) error
	Next() (presentation.Update, bool)
}

type transcriptLogger interface {
	Start(string) (telemetry.StartResult, error)
	Stop() error
	Write(string) error
	IsEnabled() bool
	Path() string
}

// EnhancedModel renders DragonRealms through the presentation boundary.
type EnhancedModel struct {
	session   gameSession
	character string
	snapshot  presentation.Update
	triggers  *automation.TriggerManager
	now       func() time.Time

	width      int
	height     int
	quitting   bool
	viewMode   ViewMode
	sourceDone bool

	input        textinput.Model
	history      []string
	historyIndex int

	layout         layout
	themes         *themeCatalog
	logger         transcriptLogger
	loggingAllowed bool
	logState       logState
	logMessage     string

	mainOutput     []string
	roomOutput     []string
	mapOutput      []string
	handsOutput    []string
	familiarOutput []string
	mainViewport   viewport.Model
	roomViewport   viewport.Model
	handsViewport  viewport.Model
	familiarView   viewport.Model
	activePane     string
	unread         map[string]bool
	showMap        bool
}

type Options struct {
	Character string
	LogDir    string
	ThemeDir  string
	Logging   bool
}

type logState uint8

const (
	logOff logState = iota
	logOn
	logFailed
)

// ViewMode selects the visible terminal layout.
type ViewMode int

const (
	ViewModeSingle ViewMode = iota
	ViewModeMulti
	ViewModeHelp
	ViewModeTheme
)

// InitialEnhancedModel constructs the UI with explicit runtime options.
func InitialEnhancedModel(session gameSession, options Options) EnhancedModel {
	input := textinput.New()
	input.Prompt = "> "
	input.CharLimit = 4096
	input.SetWidth(70)
	_ = input.Focus()
	main := viewport.New(viewport.WithWidth(50), viewport.WithHeight(10))
	main.SoftWrap = true
	room := viewport.New(viewport.WithWidth(20), viewport.WithHeight(5))
	room.SoftWrap = true
	hands := viewport.New(viewport.WithWidth(20), viewport.WithHeight(3))
	hands.SoftWrap = true
	familiar := viewport.New(viewport.WithWidth(20), viewport.WithHeight(3))
	familiar.SoftWrap = true

	m := EnhancedModel{
		session:        session,
		character:      options.Character,
		snapshot:       presentation.Update{Connection: presentation.Connecting, Character: options.Character},
		triggers:       automation.NewTriggerManager(),
		now:            time.Now,
		width:          80,
		height:         24,
		viewMode:       ViewModeMulti,
		input:          input,
		layout:         newLayout(),
		themes:         newThemeCatalog(options.ThemeDir),
		logger:         telemetry.NewLogger(options.LogDir),
		loggingAllowed: options.Logging,
		logState:       logOff,
		mainOutput:     []string{"Connecting to DragonRealms"},
		mainViewport:   main,
		roomViewport:   room,
		handsViewport:  hands,
		familiarView:   familiar,
		activePane:     paneInput,
		unread:         map[string]bool{},
	}
	for _, warning := range m.themes.warnings {
		m.appendSystem("theme warning: " + terminaltext.Sanitize(warning.Error()))
	}
	m.resizePanes()
	m.refreshAllPanes(true)
	if m.loggingAllowed {
		m.startLogging()
	}
	return m
}

// Init starts optional logging and waits for Session updates.
func (m EnhancedModel) Init() tea.Cmd {
	cmd := m.input.Focus()
	return tea.Batch(cmd, waitForSessionUpdate(m.session))
}

// Update applies terminal input or one detached Session update.
func (m EnhancedModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyPress(message)
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		m.resizePanes()
		m.refreshAllPanes(false)
		return m, nil
	case tea.MouseWheelMsg:
		if message.Button == tea.MouseWheelUp {
			m.scrollActivePageUp()
		}
		if message.Button == tea.MouseWheelDown {
			m.scrollActivePageDown()
		}
		return m, nil
	case presentation.Update:
		m.applySessionUpdate(message)
		if m.sourceDone {
			return m, nil
		}
		return m, waitForSessionUpdate(m.session)
	case editorFinishedMsg:
		m.finishEditor(message)
		return m, nil
	case sessionClosedMsg:
		m.sourceDone = true
		m.snapshot.Connection = presentation.Disconnected
		m.appendSystem("disconnected")
		if err := m.stopLogging(); err != nil {
			m.appendSystem("logging failed: " + terminaltext.Sanitize(err.Error()))
		}
		return m, nil
	}
	return m, nil
}

func (m *EnhancedModel) applySessionUpdate(update presentation.Update) {
	previousConnection := m.snapshot.Connection
	m.snapshot = update
	if update.Connection != previousConnection {
		m.appendSystem("connection: " + connectionText(update.Connection))
	}
	if update.Map != "" {
		m.replaceMap(splitLines(update.Map))
	}
	for _, entry := range update.Entries {
		switch entry.Operation {
		case presentation.Clear:
			m.replacePane(paneName(entry.Pane), nil)
		case presentation.Replace:
			m.replacePane(paneName(entry.Pane), splitLines(entry.Text))
		default:
			text := entry.Text
			if entry.Pane == presentation.Game || entry.Pane == presentation.Familiar {
				m.writeLog(entry.Text)
				text = m.highlightText(text)
			}
			m.appendPane(paneName(entry.Pane), text)
		}
	}
	for _, notice := range update.Notices {
		m.appendSystem(notice.Text)
	}
}

func (m EnhancedModel) handleKeyPress(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case message.Code == 'c' && message.Mod == tea.ModCtrl:
		m.quitting = true
		if err := m.stopLogging(); err != nil {
			m.appendSystem("logging failed: " + terminaltext.Sanitize(err.Error()))
		}
		return m, tea.Quit
	case message.Code == 'g' && message.Mod == tea.ModCtrl:
		return m, m.openEditor()
	}

	switch message.Code {
	case tea.KeyF1:
		m.viewMode = ViewModeHelp
		return m, nil
	case tea.KeyF2:
		if m.viewMode == ViewModeSingle {
			m.viewMode = ViewModeMulti
		} else {
			m.viewMode = ViewModeSingle
			m.activePane = paneInput
		}
		return m, nil
	case tea.KeyF3:
		m.viewMode = ViewModeTheme
		return m, nil
	case tea.KeyF4:
		m.toggleLogging()
		return m, nil
	case tea.KeyF5:
		m.toggleMap()
		return m, nil
	case tea.KeyTab:
		if m.viewMode == ViewModeMulti {
			m.cyclePane(!message.Mod.Contains(tea.ModShift))
		}
		return m, nil
	}

	if m.viewMode == ViewModeHelp {
		if message.Code == tea.KeyEscape {
			m.viewMode = ViewModeSingle
		}
		return m, nil
	}
	if m.viewMode == ViewModeTheme {
		return m.handleThemeKeys(message), nil
	}

	switch message.Code {
	case tea.KeyEnter:
		return m.sendInput()
	case tea.KeyUp:
		m.previousHistory()
		return m, nil
	case tea.KeyDown:
		m.nextHistory()
		return m, nil
	case tea.KeyPgUp:
		m.scrollActivePageUp()
		return m, nil
	case tea.KeyPgDown:
		m.scrollActivePageDown()
		return m, nil
	case tea.KeyHome:
		if m.activePane == paneInput {
			break
		}
		m.activeViewport().GotoTop()
		return m, nil
	case tea.KeyEnd:
		if m.activePane == paneInput {
			break
		}
		m.activeViewport().GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(message)
	return m, cmd
}

func (m EnhancedModel) sendInput() (tea.Model, tea.Cmd) {
	original := m.input.Value()
	if strings.TrimSpace(original) == "" {
		return m, nil
	}
	command := m.triggers.ProcessCommand(original)
	if err := m.session.Send(command); err != nil {
		m.appendSystem("send failed: " + terminaltext.Sanitize(err.Error()))
		return m, nil
	}
	m.writeLog("> " + original)
	m.appendPane(paneMain, "> "+original)
	m.history = append(m.history, original)
	m.historyIndex = len(m.history)
	m.input.Reset()
	return m, nil
}

func (m *EnhancedModel) highlightText(text string) string {
	lines := splitLines(text)
	for index := range lines {
		lines[index] = m.triggers.ProcessLine(lines[index])
	}
	return strings.Join(lines, "\n")
}

func (m *EnhancedModel) previousHistory() {
	if len(m.history) == 0 || m.historyIndex <= 0 {
		return
	}
	m.historyIndex--
	m.input.SetValue(m.history[m.historyIndex])
}

func (m *EnhancedModel) nextHistory() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.input.SetValue(m.history[m.historyIndex])
		return
	}
	m.historyIndex = len(m.history)
	m.input.Reset()
}

func (m EnhancedModel) handleThemeKeys(message tea.KeyPressMsg) EnhancedModel {
	switch message.Code {
	case tea.KeyUp:
		m.themes.previous()
	case tea.KeyDown:
		m.themes.next()
	case tea.KeyEnter, tea.KeyEscape:
		m.viewMode = ViewModeSingle
	}
	return m
}

func (m *EnhancedModel) appendSystem(text string) {
	now := m.now
	if now == nil {
		now = time.Now
	}
	m.appendPane(paneMain, fmt.Sprintf("[system %s] %s", now().Format("15:04:05"), text))
}

func (m *EnhancedModel) appendPane(pane, text string) {
	if pane == "" {
		pane = paneMain
	}
	wasActive := pane == m.activePane
	wasBottom := m.viewportFor(pane).AtBottom()
	lines := splitLines(text)
	switch pane {
	case paneFamiliar:
		m.familiarOutput = appendCapped(m.familiarOutput, lines, 100)
	default:
		pane = paneMain
		m.mainOutput = appendCapped(m.mainOutput, lines, 500)
	}
	m.refreshPanePreservingOffset(pane, wasBottom)
	if !wasActive {
		m.unread[pane] = true
	}
}

func (m *EnhancedModel) replacePane(pane string, lines []string) {
	current := m.linesFor(pane)
	if equalLines(current, lines) {
		return
	}
	wasActive := pane == m.activePane
	wasBottom := m.viewportFor(pane).AtBottom()
	switch pane {
	case paneFamiliar:
		m.familiarOutput = append([]string(nil), lines...)
	case paneRoom:
		m.roomOutput = append([]string(nil), lines...)
	case paneHands:
		m.handsOutput = append([]string(nil), lines...)
	default:
		m.mainOutput = append([]string(nil), lines...)
		pane = paneMain
	}
	m.refreshPanePreservingOffset(pane, wasBottom)
	if !wasActive {
		m.unread[pane] = true
	}
	if pane == paneFamiliar && !familiarAvailable(m.familiarOutput) && m.activePane == paneFamiliar {
		m.activePane = paneMain
	}
	if !containsPane(m.focusablePanes(), m.activePane) {
		m.activePane = paneInput
	}
}

func (m *EnhancedModel) refreshPane(pane string, forceBottom bool) {
	v := m.viewportFor(pane)
	atBottom := v.AtBottom()
	v.SetContentLines(m.linesFor(pane))
	if forceBottom || atBottom {
		v.GotoBottom()
	}
}

func (m *EnhancedModel) refreshPanePreservingOffset(pane string, forceBottom bool) {
	v := m.viewportFor(pane)
	atBottom := v.AtBottom()
	v.SetContentLines(m.linesFor(pane))
	if forceBottom || atBottom {
		v.GotoBottom()
	}
}

func (m *EnhancedModel) refreshAllPanes(forceBottom bool) {
	for _, pane := range paneOrder {
		if pane == paneFamiliar && !familiarAvailable(m.familiarOutput) {
			continue
		}
		m.refreshPane(pane, forceBottom)
	}
}

func (m *EnhancedModel) resizePanes() {
	layoutHeight := max(m.height-7, 10)
	leftWidth := int(float64(m.width) * 0.7)
	rightWidth := max(m.width-leftWidth-1, 10)
	mainWidth := max(leftWidth-4, 1)
	rightContentWidth := max(rightWidth-4, 1)
	roomHeight, handsHeight, familiarHeight := paneHeights(layoutHeight, familiarAvailable(m.familiarOutput))

	m.mainViewport.SetWidth(mainWidth)
	m.mainViewport.SetHeight(max(layoutHeight-4, 1))
	m.roomViewport.SetWidth(rightContentWidth)
	m.roomViewport.SetHeight(max(roomHeight-4, 1))
	m.handsViewport.SetWidth(rightContentWidth)
	m.handsViewport.SetHeight(max(handsHeight-4, 1))
	m.familiarView.SetWidth(rightContentWidth)
	m.familiarView.SetHeight(max(familiarHeight-4, 1))
	m.input.SetWidth(max(m.width-8, 1))
}

func (m *EnhancedModel) cyclePane(forward bool) {
	order := m.focusablePanes()
	for index, pane := range order {
		if pane == m.activePane {
			next := index + 1
			if !forward {
				next = index - 1
			}
			if next < 0 {
				next = len(order) - 1
			}
			m.activePane = order[next%len(order)]
			m.unread[m.activePane] = false
			return
		}
	}
	m.activePane = paneMain
}

func (m EnhancedModel) focusablePanes() []string {
	order := []string{paneInput, paneMain, paneRoom, paneHands}
	if familiarAvailable(m.familiarOutput) {
		order = append(order, paneFamiliar)
	}
	return order
}

func containsPane(panes []string, pane string) bool {
	for _, candidate := range panes {
		if candidate == pane {
			return true
		}
	}
	return false
}

func (m *EnhancedModel) scrollActivePageUp() {
	if m.activePane == paneInput {
		return
	}
	m.activeViewport().PageUp()
}

func (m *EnhancedModel) scrollActivePageDown() {
	if m.activePane == paneInput {
		return
	}
	m.activeViewport().PageDown()
	if m.activeViewport().AtBottom() {
		m.unread[m.activePane] = false
	}
}

func (m *EnhancedModel) activeViewport() *viewport.Model {
	return m.viewportFor(m.activePane)
}

func (m *EnhancedModel) viewportFor(pane string) *viewport.Model {
	switch pane {
	case paneRoom:
		return &m.roomViewport
	case paneHands:
		return &m.handsViewport
	case paneFamiliar:
		return &m.familiarView
	default:
		return &m.mainViewport
	}
}

func (m EnhancedModel) linesFor(pane string) []string {
	switch pane {
	case paneRoom:
		if m.showMap {
			return m.mapOutput
		}
		return m.roomOutput
	case paneHands:
		return m.handsOutput
	case paneFamiliar:
		return m.familiarOutput
	default:
		return m.mainOutput
	}
}

// View renders the current terminal screen.
func (m EnhancedModel) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}
	var content string
	switch m.viewMode {
	case ViewModeHelp:
		content = m.renderHelp()
	case ViewModeTheme:
		content = m.renderThemeSelector()
	case ViewModeMulti:
		content = m.renderMultiPane()
	default:
		content = m.renderSinglePane()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.OnMouse = func(msg tea.MouseMsg) tea.Cmd {
		return func() tea.Msg { return msg }
	}
	return v
}

func (m EnhancedModel) renderSinglePane() string {
	theme := m.themes.current()
	outputHeight := max(m.height-8, 3)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.TitleBar)).Align(lipgloss.Center).Width(m.width).Render(m.buildTitle())
	status := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.StatusBar)).Background(lipgloss.Color(theme.StatusBarBg)).Padding(0, 1).Width(m.width).Render(m.buildStatusBar())
	border := m.themes.borderStyle().Width(m.width - 2)
	main := m.mainViewport
	main.SetHeight(max(outputHeight-2, 1))
	body := main.View()
	return strings.Join([]string{title, status, border.Render(body), m.renderInputPane()}, "\n")
}

func (m EnhancedModel) renderMultiPane() string {
	theme := m.themes.current()
	layoutHeight := max(m.height-8, 10)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.TitleBar)).Align(lipgloss.Center).Width(m.width).Render(m.buildTitle())
	status := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.StatusBar)).Background(lipgloss.Color(theme.StatusBarBg)).Padding(0, 1).Width(m.width).Render(m.buildStatusBar())
	layout := lipgloss.NewStyle().MaxHeight(layoutHeight).Height(layoutHeight).Render(m.layout.render(m.width, layoutHeight, m.paneViews()))
	return strings.Join([]string{title, status, layout, m.renderInputPane()}, "\n")
}

func (m EnhancedModel) renderHelp() string {
	text := `DragonRealms Charm - Help

F1          Show this help
F2          Toggle multi/single-pane view
F3          Select theme
F4          Toggle logging
F5          Toggle Room/Map pane
Tab         Cycle input and visible panes
PgUp/PgDn   Scroll active pane
Home/End    Jump active pane
Up/Down     Command history
Ctrl-G      Edit command in $VISUAL or $EDITOR
Ctrl-C      Quit

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
	if m.snapshot.Title != "" {
		title += " " + m.snapshot.Title
	}
	return title
}

func (m EnhancedModel) buildStatusBar() string {
	parts := []string{connectionText(m.snapshot.Connection), "LOG " + m.logText()}
	for _, field := range m.snapshot.Status {
		if field.Label == "" {
			parts = append(parts, field.Value)
			continue
		}
		parts = append(parts, field.Label+":"+field.Value)
	}
	return strings.Join(parts, " | ")
}

func (m EnhancedModel) buildInput() string {
	prompt := m.snapshot.Prompt
	if prompt == "" {
		prompt = ">"
	}
	m.input.Prompt = prompt + " "
	return m.input.View()
}

func (m EnhancedModel) renderInputPane() string {
	title := "Input"
	style := m.themes.borderStyle().Width(m.width - 2)
	if m.activePane == paneInput {
		title = "> " + title
	}
	return style.Render(title + "\n" + m.buildInput())
}

func (m EnhancedModel) paneViews() paneViews {
	roomTitle := "Room"
	if m.showMap {
		roomTitle = "Map"
	}
	return paneViews{
		main:     paneView{title: "Game", body: m.mainViewport.View(), active: m.activePane == paneMain, unread: m.unread[paneMain]},
		room:     paneView{title: roomTitle, body: m.roomViewport.View(), active: m.activePane == paneRoom, unread: m.unread[paneRoom]},
		hands:    paneView{title: "Hands", body: m.handsViewport.View(), active: m.activePane == paneHands, unread: m.unread[paneHands]},
		familiar: paneView{title: "Familiar", body: m.familiarView.View(), active: m.activePane == paneFamiliar, unread: m.unread[paneFamiliar], visible: familiarAvailable(m.familiarOutput)},
	}
}

// Close flushes UI-owned resources after Bubble Tea restores the terminal.
func (m EnhancedModel) Close() error {
	return m.stopLogging()
}

func (m EnhancedModel) logText() string {
	switch m.logState {
	case logOn:
		return "on"
	case logFailed:
		return "failed"
	default:
		return "off"
	}
}

func (m *EnhancedModel) startLogging() {
	if m.logger == nil || m.logger.IsEnabled() {
		return
	}
	result, err := m.logger.Start(m.character)
	if err != nil {
		m.logState = logFailed
		m.logMessage = err.Error()
		m.appendSystem("logging failed: " + terminaltext.Sanitize(err.Error()))
		return
	}
	m.logState = logOn
	m.logMessage = ""
	m.appendSystem("logging started: " + result.Path)
	if result.Warning != nil {
		m.appendSystem("logging warning: " + terminaltext.Sanitize(result.Warning.Error()))
	}
}

func (m *EnhancedModel) stopLogging() error {
	if m.logger == nil {
		m.logState = logOff
		return nil
	}
	err := m.logger.Stop()
	if err != nil {
		m.logState = logFailed
		m.logMessage = err.Error()
		return err
	}
	m.logState = logOff
	return nil
}

func (m *EnhancedModel) writeLog(line string) {
	if m.logger == nil || !m.logger.IsEnabled() {
		return
	}
	if err := m.logger.Write(line); err != nil {
		message := err.Error()
		if stopErr := m.logger.Stop(); stopErr != nil {
			message += " (close failed: " + stopErr.Error() + ")"
		}
		m.logState = logFailed
		m.logMessage = message
		m.appendSystem("logging failed: " + terminaltext.Sanitize(message))
	}
}

func (m *EnhancedModel) toggleLogging() {
	if m.logger != nil && m.logger.IsEnabled() {
		if err := m.stopLogging(); err != nil {
			m.appendSystem("logging failed: " + terminaltext.Sanitize(err.Error()))
			return
		}
		m.appendSystem("logging stopped")
		return
	}
	m.startLogging()
}

func (m *EnhancedModel) toggleMap() {
	m.showMap = !m.showMap
	m.refreshPane(paneRoom, true)
	m.unread[paneRoom] = false
}

func (m *EnhancedModel) replaceMap(lines []string) {
	if equalLines(m.mapOutput, lines) {
		return
	}
	wasActive := m.activePane == paneRoom
	wasBottom := m.roomViewport.AtBottom()
	m.mapOutput = append([]string(nil), lines...)
	if m.showMap {
		m.refreshPanePreservingOffset(paneRoom, wasBottom)
		if !wasActive {
			m.unread[paneRoom] = true
		}
	}
}

func connectionText(state presentation.ConnectionState) string {
	switch state {
	case presentation.Ready:
		return "READY"
	case presentation.Reconnecting:
		return "RECONNECTING"
	case presentation.Disconnected:
		return "DISCONNECTED"
	default:
		return "CONNECTING"
	}
}

func paneName(pane presentation.PaneID) string {
	switch pane {
	case presentation.Familiar:
		return paneFamiliar
	case presentation.RoomPane:
		return paneRoom
	case presentation.HandsPane:
		return paneHands
	default:
		return paneMain
	}
}

func appendCapped(existing, lines []string, limit int) []string {
	out := append(existing, lines...)
	if len(out) > limit {
		out = append([]string(nil), out[len(out)-limit:]...)
	}
	return out
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

type sessionClosedMsg struct{}

func waitForSessionUpdate(session gameSession) tea.Cmd {
	return func() tea.Msg {
		update, ok := session.Next()
		if !ok {
			return sessionClosedMsg{}
		}
		return update
	}
}

type editorFinishedMsg struct {
	path      string
	draft     string
	removeErr error
	err       error
}

var removeEditorFile = os.Remove
var readEditorFile = os.ReadFile

func (m EnhancedModel) openEditor() tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return func() tea.Msg {
			return editorFinishedMsg{draft: m.input.Value(), err: fmt.Errorf("VISUAL or EDITOR is required")}
		}
	}
	file, err := os.CreateTemp("", "dr-charm-editor-*.txt")
	if err != nil {
		return func() tea.Msg { return editorFinishedMsg{draft: m.input.Value(), err: err} }
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return func() tea.Msg { return editorFinishedMsg{draft: m.input.Value(), err: err} }
	}
	if _, err := file.WriteString(m.input.Value()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return func() tea.Msg { return editorFinishedMsg{draft: m.input.Value(), err: err} }
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return func() tea.Msg { return editorFinishedMsg{draft: m.input.Value(), err: err} }
	}
	command := exec.Command("/bin/sh", "-c", "exec "+editor+" \"$1\"", "dr-charm-editor", path)
	draft := m.input.Value()
	return tea.ExecProcess(command, func(err error) tea.Msg {
		if err != nil {
			return editorFinishedMsg{path: path, draft: draft, err: err}
		}
		return editorFinishedMsg{path: path, draft: draft}
	})
}

func (m *EnhancedModel) finishEditor(message editorFinishedMsg) {
	var data []byte
	var readErr error
	if message.path != "" {
		data, readErr = readEditorFile(message.path)
		message.removeErr = removeEditorFile(message.path)
	}
	if message.err != nil {
		m.input.SetValue(message.draft)
		m.appendSystem("editor failed: " + terminaltext.Sanitize(combineErrors(message.err, message.removeErr)))
		return
	}
	if readErr != nil {
		m.input.SetValue(message.draft)
		m.appendSystem("editor failed: " + terminaltext.Sanitize(combineErrors(readErr, message.removeErr)))
		return
	}
	if message.removeErr != nil {
		m.input.SetValue(message.draft)
		m.appendSystem("editor failed: " + terminaltext.Sanitize(message.removeErr.Error()))
		return
	}
	value := strings.TrimRight(string(data), "\r\n")
	if strings.ContainsAny(value, "\r\n") {
		m.input.SetValue(message.draft)
		m.appendSystem("editor returned more than one command")
		return
	}
	m.input.SetValue(value)
}

func combineErrors(primary, cleanup error) string {
	if cleanup == nil {
		return primary.Error()
	}
	return primary.Error() + " (cleanup failed: " + cleanup.Error() + ")"
}
