package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/automation"
	"dr-charm/internal/presentation"
	"dr-charm/internal/telemetry"
	"dr-charm/internal/terminaltext"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

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

	width, height            int
	dimensionsReceived       bool
	quitting, sourceDone     bool
	viewMode                 ViewMode
	mapNavigation            bool
	mapPanLine, mapPanColumn int
	mapToken                 string
	modalOffset              int
	pendingTranscript        []string
	drainScheduled           bool

	input        textarea.Model
	inputLabel   string
	history      []string
	historyIndex int

	themes         *themeCatalog
	logger         transcriptLogger
	loggingAllowed bool
	logState       logState
	logMessage     string

	mapOutput []string
	agent     agentState
}

type Options struct {
	Character string
	LogDir    string
	ThemeDir  string
	Logging   bool
	Agent     *agent.Client
	Context   context.Context
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
	ViewModeHelp
	ViewModeTheme
)

// InitialEnhancedModel constructs the UI with explicit runtime options.
func InitialEnhancedModel(session gameSession, options Options) EnhancedModel {
	if options.Context == nil {
		options.Context = context.Background()
	}
	input := textarea.New()
	input.ShowLineNumbers = false
	input.CharLimit = 4096
	input.MaxWidth = 0
	input.MaxHeight = 1
	input.DynamicHeight = true
	input.MinHeight = 1
	input.Prompt = ""
	input.SetWidth(1)
	_ = input.Focus()
	m := EnhancedModel{
		session:        session,
		character:      options.Character,
		snapshot:       presentation.Update{Connection: presentation.Connecting, Character: options.Character},
		triggers:       automation.NewTriggerManager(),
		now:            time.Now,
		width:          80,
		height:         24,
		viewMode:       ViewModeSingle,
		input:          input,
		themes:         newThemeCatalog(options.ThemeDir),
		logger:         telemetry.NewLogger(options.LogDir),
		loggingAllowed: options.Logging,
		logState:       logOff,
		agent:          agentState{ctx: options.Context},
	}
	if options.Agent != nil {
		m.agent.client = options.Agent
	}
	m.syncInputPresentation()
	for _, warning := range m.themes.warnings {
		m.appendSystem("theme warning: " + terminaltext.Sanitize(warning.Error()))
	}
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

type transcriptDrainMsg struct{}

type inputPasteMsg struct {
	content string
	err     error
}

var readInputClipboard = clipboard.ReadAll

func nextTranscriptDrain() tea.Cmd { return func() tea.Msg { return transcriptDrainMsg{} } }

// Update applies terminal input or one detached Session update.
func (m EnhancedModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyPress(message)
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		m.dimensionsReceived = true
		m.syncInputPresentation()
		return m, m.scheduleTranscript()
	case transcriptDrainMsg:
		if len(m.pendingTranscript) == 0 {
			m.drainScheduled = false
			if m.quitting {
				return m, tea.Quit
			}
			return m, nil
		}
		record := m.pendingTranscript[0]
		m.pendingTranscript = m.pendingTranscript[1:]
		return m, tea.Sequence(tea.Println(printableRecord(record)), nextTranscriptDrain())
	case presentation.Update:
		wasReady := m.snapshot.Connection == presentation.Ready
		m.applySessionUpdate(message)
		m.syncInputPresentation()
		if wasReady && message.Connection != presentation.Ready {
			m.cancelAgent()
		}
		wait := waitForSessionUpdate(m.session)
		emit := m.scheduleTranscript()
		if m.sourceDone {
			return m, emit
		}
		if message.Prompted && message.Connection == presentation.Ready {
			return m, tea.Sequence(emit, m.wakeAgent(), wait)
		}
		return m, tea.Sequence(emit, wait)
	case agentResultMsg:
		follow := m.handleAgentResult(message)
		return m, tea.Sequence(m.scheduleTranscript(), follow)
	case editorFinishedMsg:
		m.finishEditor(message)
		return m, m.scheduleTranscript()
	case sessionClosedMsg:
		m.cancelAgent()
		m.sourceDone = true
		m.snapshot.Connection = presentation.Disconnected
		m.appendSystem("disconnected")
		if err := m.stopLogging(); err != nil {
			m.appendSystem("logging failed: " + terminaltext.Sanitize(err.Error()))
		}
		return m, m.scheduleTranscript()
	}
	if m.viewMode == ViewModeHelp || m.viewMode == ViewModeTheme || m.mapNavigation {
		return m, nil
	}
	if message, ok := message.(inputPasteMsg); ok {
		if message.err != nil {
			m.input.Err = message.err
			return m, nil
		}
		return m.updateInput(tea.PasteMsg{Content: normalizeInputValue(message.content)})
	}
	if message, ok := message.(tea.PasteMsg); ok {
		message.Content = normalizeInputValue(message.Content)
		return m.updateInput(message)
	}
	return m.updateInput(message)
}

func (m *EnhancedModel) applySessionUpdate(update presentation.Update) {
	previousConnection := m.snapshot.Connection
	m.snapshot = update
	if update.Connection != previousConnection {
		m.appendSystem("connection: " + connectionText(update.Connection))
	}
	if len(update.Map.Lines) > 0 {
		m.mapOutput = append([]string(nil), update.Map.Lines...)
		if update.Map.CurrentToken != "" && update.Map.CurrentToken != m.mapToken {
			m.mapToken = update.Map.CurrentToken
			m.mapPanLine, m.mapPanColumn = update.Map.CurrentLine, update.Map.CurrentColumn
		}
	}
	for _, entry := range update.Entries {
		switch entry.Operation {
		case presentation.Clear:
			continue
		case presentation.Replace:
			if strings.TrimSpace(entry.Text) == "" {
				continue
			}
			fallthrough
		default:
			text := entry.Text
			if entry.Pane == presentation.Game || entry.Pane == presentation.Familiar {
				m.addRecent(entry.Text)
				m.writeLog(entry.Text)
				text = m.highlightText(text)
			}
			if entry.Pane == presentation.Familiar {
				text = "[familiar] " + text
			}
			m.enqueueTranscript(text)
		}
	}
	for _, notice := range update.Notices {
		m.appendSystem(notice.Text)
	}
}

func (m EnhancedModel) handleKeyPress(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case message.Code == 'c' && message.Mod == tea.ModCtrl:
		m.cancelAgent()
		m.quitting = true
		if err := m.stopLogging(); err != nil {
			m.appendSystem("logging failed: " + terminaltext.Sanitize(err.Error()))
		}
		if len(m.pendingTranscript) == 0 && !m.drainScheduled {
			return m, tea.Quit
		}
		return m, m.scheduleTranscript()
	case message.Code == 'g' && message.Mod == tea.ModCtrl:
		return m, m.openEditor()
	}

	switch message.Code {
	case tea.KeyF1:
		m.viewMode = ViewModeHelp
		m.modalOffset = 0
		return m, nil
	case tea.KeyF3:
		m.viewMode = ViewModeTheme
		m.modalOffset = 0
		return m, nil
	case tea.KeyF4:
		m.toggleLogging()
		return m, nil
	case tea.KeyF6:
		m.toggleAgent()
		m.syncInputPresentation()
		return m, nil
	case tea.KeyTab:
		m.mapNavigation = false
		return m, nil
	}

	if m.viewMode == ViewModeHelp {
		code := modalNavigationCode(message)
		if code == tea.KeyUp || code == 'k' {
			m.modalOffset = max(0, m.modalOffset-1)
			return m, nil
		}
		if code == tea.KeyDown || code == 'j' {
			m.modalOffset++
			return m, nil
		}
		if message.Code == 'u' && message.Mod == tea.ModCtrl {
			m.modalOffset = max(0, m.modalOffset-m.modalRows()/2)
			return m, nil
		}
		if message.Code == 'd' && message.Mod == tea.ModCtrl {
			m.modalOffset += m.modalRows() / 2
			return m, nil
		}
		if code == 'g' {
			m.modalOffset = 0
			return m, nil
		}
		if code == 'G' {
			m.modalOffset = 1 << 30
			return m, nil
		}
		if message.Code == tea.KeyEscape {
			m.viewMode = ViewModeSingle
		}
		return m, nil
	}
	if m.viewMode == ViewModeTheme {
		return m.handleThemeKeys(message), nil
	}
	if message.Code == tea.KeyEscape && m.mapVisible() {
		m.mapNavigation = true
		return m, nil
	}
	if m.mapNavigation {
		m.panMap(message)
		return m, nil
	}
	if message.Code == 'v' && message.Mod == tea.ModCtrl {
		return m, readNormalizedInputClipboard
	}

	switch message.Code {
	case tea.KeyEnter:
		if m.agent.enabled {
			return m, m.whisper()
		}
		return m.sendInput()
	case tea.KeyUp:
		m.previousHistory()
		return m, nil
	case tea.KeyDown:
		m.nextHistory()
		return m, nil
	}

	return m.updateInput(message)
}

func (m EnhancedModel) mapVisible() bool {
	return calculateDashboardGeometry(m.width, m.height, len(m.mapOutput) > 0, m.themes.current(), m.inputLabel).mapVisible
}

func (m *EnhancedModel) panMap(message tea.KeyPressMsg) {
	switch modalNavigationCode(message) {
	case tea.KeyEscape, tea.KeyTab:
		m.mapNavigation = false
	case 'h':
		m.mapPanColumn = max(0, m.mapPanColumn-1)
	case 'l':
		m.mapPanColumn++
	case 'j':
		m.mapPanLine = min(len(m.mapOutput)-1, m.mapPanLine+1)
	case 'k':
		m.mapPanLine = max(0, m.mapPanLine-1)
	case 'u':
		if message.Mod == tea.ModCtrl {
			m.mapPanLine = max(0, m.mapPanLine-max(1, m.height/4))
		}
	case 'd':
		if message.Mod == tea.ModCtrl {
			m.mapPanLine = min(len(m.mapOutput)-1, m.mapPanLine+max(1, m.height/4))
		}
	case 'g':
		m.mapPanLine = 0
	case 'G':
		m.mapPanLine = max(0, len(m.mapOutput)-1)
	}
}

func (m EnhancedModel) sendInput() (tea.Model, tea.Cmd) {
	original := m.input.Value()
	if strings.TrimSpace(original) == "" {
		return m, nil
	}
	m.sendCommand(original, "> ", true)
	return m, nil
}

func (m EnhancedModel) updateInput(message tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(message)
	m.repositionInput()
	return m, cmd
}

func readNormalizedInputClipboard() tea.Msg {
	content, err := readInputClipboard()
	return inputPasteMsg{content: content, err: err}
}

func normalizeInputValue(value string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(value)
}

func (m *EnhancedModel) setInputValue(value string) {
	m.input.SetValue(normalizeInputValue(value))
	m.repositionInput()
}

func (m *EnhancedModel) repositionInput() {
	_ = m.input.View()
	m.input.SetHeight(m.input.Height())
}

func (m *EnhancedModel) sendCommand(original, prefix string, remember bool) bool {
	command := m.triggers.ProcessCommand(original)
	if err := m.session.Send(command); err != nil {
		m.appendSystem("send failed: " + terminaltext.Sanitize(err.Error()))
		return false
	}
	m.writeLog("> " + original)
	m.enqueueTranscript(prefix + original)
	if remember {
		m.history = append(m.history, original)
		m.historyIndex = len(m.history)
		m.input.Reset()
	}
	return true
}

func (m *EnhancedModel) highlightText(text string) string {
	lines := splitLines(text)
	for index := range lines {
		lines[index] = m.triggers.ProcessLine(lines[index])
	}
	return strings.Join(lines, "\n")
}

func splitLines(text string) []string {
	return strings.Split(text, "\n")
}

func (m *EnhancedModel) previousHistory() {
	if len(m.history) == 0 || m.historyIndex <= 0 {
		return
	}
	m.historyIndex--
	m.setInputValue(m.history[m.historyIndex])
}

func (m *EnhancedModel) nextHistory() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.setInputValue(m.history[m.historyIndex])
		return
	}
	m.historyIndex = len(m.history)
	m.input.Reset()
}

func (m EnhancedModel) handleThemeKeys(message tea.KeyPressMsg) EnhancedModel {
	switch modalNavigationCode(message) {
	case tea.KeyUp, 'k':
		m.themes.previous()
	case tea.KeyDown, 'j':
		m.themes.next()
	case 'u':
		if message.Mod == tea.ModCtrl {
			for range max(1, m.modalRows()/2) {
				m.themes.previous()
			}
		}
	case 'd':
		if message.Mod == tea.ModCtrl {
			for range max(1, m.modalRows()/2) {
				m.themes.next()
			}
		}
	case 'g':
		m.themes.currentIndex = 0
	case 'G':
		m.themes.currentIndex = len(m.themes.themes) - 1
	case tea.KeyEnter, tea.KeyEscape:
		m.viewMode = ViewModeSingle
	}
	m.syncInputPresentation()
	return m
}

func modalNavigationCode(message tea.KeyPressMsg) rune {
	if message.Code == 'g' && message.Mod.Contains(tea.ModShift) {
		return 'G'
	}
	return message.Code
}

func (m *EnhancedModel) appendSystem(text string) {
	now := m.now
	if now == nil {
		now = time.Now
	}
	m.enqueueTranscript(fmt.Sprintf("[system %s] %s", now().Format("15:04:05"), text))
}

func printableRecord(record string) string {
	if record == "" {
		return " "
	}
	return record
}

func (m *EnhancedModel) enqueueTranscript(record string) {
	m.pendingTranscript = append(m.pendingTranscript, record)
}

func (m *EnhancedModel) scheduleTranscript() tea.Cmd {
	if m.drainScheduled || len(m.pendingTranscript) == 0 || (!m.dimensionsReceived && !m.quitting) {
		return nil
	}
	m.drainScheduled = true
	return nextTranscriptDrain()
}

// View renders the current terminal screen.
func (m EnhancedModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	m.syncInputPresentation()
	var content string
	switch m.viewMode {
	case ViewModeHelp:
		content = m.renderHelp()
	case ViewModeTheme:
		content = m.renderThemeSelector()
	default:
		content = renderDashboard(m.width, m.height, m.snapshot, m.buildStatusBar(), m.buildInput(), m.mapOutput, m.mapPanLine, m.mapPanColumn, m.mapNavigation, m.themes.current())
	}
	return tea.NewView(content)
}

func (m EnhancedModel) renderHelp() string {
	return m.renderModal("Help", []string{"F1  help", "F3  theme", "F4  logging", "F6  auto mode", "Esc map navigation", "Tab/Esc input focus", "h/j/k/l map pan", "Ctrl-U/D map pan", "g/G map top/bottom", "Up/Down command history", "Ctrl-G edit input", "Ctrl-C quit", "Esc close"}, m.modalOffset)
}

func (m EnhancedModel) renderThemeSelector() string {
	lines := make([]string, 0, len(m.themes.themes))
	current := m.themes.current().Name
	for _, name := range m.themes.names() {
		prefix := "    "
		if name == current {
			prefix = "  > "
		}
		lines = append(lines, prefix+name)
	}
	return m.renderModal("Theme", lines, themeOffset(lines, current, m.modalBodyRows()))
}

func (m EnhancedModel) modalRows() int {
	return calculateDashboardGeometry(m.width, m.height, len(m.mapOutput) > 0, m.themes.current(), m.inputLabel).rows
}

func (m EnhancedModel) modalBodyRows() int {
	geometry := calculateDashboardGeometry(m.width, m.height, len(m.mapOutput) > 0, m.themes.current(), m.inputLabel)
	if geometry.chrome {
		return max(0, geometry.rows-4)
	}
	return max(0, geometry.rows-1)
}

func (m EnhancedModel) renderModal(title string, lines []string, offset int) string {
	return renderModal(m.width, m.height, len(m.mapOutput) > 0, title, lines, offset, m.themes.current())
}

func themeOffset(lines []string, current string, rows int) int {
	for index, line := range lines {
		if strings.Contains(line, "> "+current) {
			return min(max(0, index-rows+1), max(0, len(lines)-rows))
		}
	}
	return 0
}

func (m EnhancedModel) buildStatusBar() string {
	parts := []string{connectionText(m.snapshot.Connection), "LOG " + m.logText()}
	if m.agent.client != nil {
		status := "off"
		if m.agent.enabled {
			status = m.agent.status
		}
		parts = append(parts, "AGENT "+status)
	}
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
	rows := strings.Split(strings.TrimSuffix(m.input.View(), "\n"), "\n")
	indent := strings.Repeat(" ", ansi.StringWidth(m.inputLabel))
	for index := range rows {
		if index == 0 {
			rows[index] = m.inputLabel + rows[index]
			continue
		}
		rows[index] = indent + rows[index]
	}
	return strings.Join(rows, "\n")
}

func (m *EnhancedModel) syncInputPresentation() {
	resolved := m.themes.current().presentation()
	label := "Whisper > "
	if !m.agent.enabled {
		prompt := m.snapshot.Prompt
		if prompt == "" {
			prompt = ">"
		}
		label = "Command " + prompt + " "
	}
	geometry := calculateDashboardGeometry(m.width, m.height, len(m.mapOutput) > 0, resolved, label)
	label = truncate(label, max(0, geometry.inputInterior-2))
	m.inputLabel = label
	m.input.MaxHeight = geometry.inputHeight
	m.input.SetWidth(geometry.inputWidth)
	m.repositionInput()

	fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(resolved.StatusBar))
	if resolved.StatusBarBg != "" {
		fieldStyle = fieldStyle.Background(lipgloss.Color(resolved.StatusBarBg))
	}
	styles := m.input.Styles()
	styles.Focused.Text = fieldStyle
	styles.Focused.Prompt = fieldStyle
	styles.Focused.Placeholder = fieldStyle
	styles.Focused.CursorLine = fieldStyle
	styles.Focused.EndOfBuffer = fieldStyle
	styles.Blurred = styles.Focused
	styles.Cursor.Color = lipgloss.Color(resolved.StatusBar)
	m.input.SetStyles(styles)
}

// Close flushes UI-owned resources after Bubble Tea restores the terminal.
func (m EnhancedModel) Close() error {
	m.cancelAgent()
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
		m.setInputValue(message.draft)
		m.appendSystem("editor failed: " + terminaltext.Sanitize(combineErrors(message.err, message.removeErr)))
		return
	}
	if readErr != nil {
		m.setInputValue(message.draft)
		m.appendSystem("editor failed: " + terminaltext.Sanitize(combineErrors(readErr, message.removeErr)))
		return
	}
	if message.removeErr != nil {
		m.setInputValue(message.draft)
		m.appendSystem("editor failed: " + terminaltext.Sanitize(message.removeErr.Error()))
		return
	}
	value := strings.TrimRight(string(data), "\r\n")
	if strings.ContainsAny(value, "\r\n") {
		m.setInputValue(message.draft)
		m.appendSystem("editor returned more than one line")
		return
	}
	m.setInputValue(value)
}

func combineErrors(primary, cleanup error) string {
	if cleanup == nil {
		return primary.Error()
	}
	return primary.Error() + " (cleanup failed: " + cleanup.Error() + ")"
}
