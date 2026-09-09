package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"dr-charm/internal/telemetry"
	"github.com/charmbracelet/x/ansi"
)

type fakeSession struct {
	updates chan presentation.Update
	sent    []string
	err     error
}

func (s *fakeSession) Send(value string) error           { s.sent = append(s.sent, value); return s.err }
func (s *fakeSession) Next() (presentation.Update, bool) { value, ok := <-s.updates; return value, ok }

func newTestModel(t *testing.T, session *fakeSession) EnhancedModel {
	t.Helper()
	model := InitialEnhancedModel(session, Options{})
	model.now = func() time.Time { return time.Date(2026, 1, 1, 1, 2, 3, 0, time.UTC) }
	return model
}

func TestEnhancedModelCommandHistoryAndFailedSend(t *testing.T) {
	session := &fakeSession{updates: make(chan presentation.Update)}
	model := newTestModel(t, session)
	logger := &recordingLogger{enabled: true}
	model.logger = logger
	model.input.SetValue("n")
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if len(session.sent) != 1 || session.sent[0] != "north" || len(model.history) != 1 || model.history[0] != "n" || model.pendingTranscript[len(model.pendingTranscript)-1] != "> n" || len(logger.writes) != 1 || logger.writes[0] != "> n" {
		t.Fatalf("successful command state: sent=%v history=%v queue=%v", session.sent, model.history, model.pendingTranscript)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model = updated.(EnhancedModel)
	if got := model.input.Value(); got != "n" {
		t.Fatalf("history up=%q, want n", got)
	}
	session.err = errors.New("closed")
	model.input.SetValue("north")
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if len(session.sent) != 2 || len(model.history) != 1 || strings.Count(strings.Join(model.pendingTranscript, "\n"), "> north") != 0 || len(logger.writes) != 1 || !contains(model.pendingTranscript[len(model.pendingTranscript)-1], "send failed") {
		t.Fatalf("failed command state: history=%v queue=%v", model.history, model.pendingTranscript)
	}
}

func TestEnhancedModelPasteInsertsFlattenedRunesAndHonorsModalGuards(t *testing.T) {
	for _, test := range []struct {
		name, initial, paste, want string
		cursor                     int
		mode                       ViewMode
	}{
		{name: "at Unicode cursor", initial: "say  hello", cursor: 4, paste: "café", want: "say café hello"},
		{name: "newlines and tabs", paste: "look\nnorth\twest\n", want: "look north west "},
		{name: "rune cap", paste: strings.Repeat("界", 4097), want: strings.Repeat("界", 4096)},
		{name: "help", initial: "draft", paste: "look", want: "draft", mode: ViewModeHelp},
		{name: "theme", initial: "draft", paste: "look", want: "draft", mode: ViewModeTheme},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{updates: make(chan presentation.Update)}
			model := newTestModel(t, session)
			model.viewMode = test.mode
			model.input.SetValue(test.initial)
			model.input.SetCursorColumn(test.cursor)

			updated, _ := model.Update(tea.PasteMsg{Content: test.paste})
			model = updated.(EnhancedModel)
			if got := model.input.Value(); got != test.want {
				t.Fatalf("pasted input=%q, want %q", got, test.want)
			}
			if len(session.sent) != 0 || len(model.history) != 0 {
				t.Fatalf("paste submitted command: sent=%v history=%v", session.sent, model.history)
			}
		})
	}
}

func TestEnhancedModelQueuesGameFamiliarDuplicateAndBlankRecords(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Entries: []presentation.Entry{{Pane: presentation.Game, Text: "one", Operation: presentation.Append}, {Pane: presentation.Game, Text: "one", Operation: presentation.Append}, {Pane: presentation.Game, Text: "", Operation: presentation.Append}, {Pane: presentation.Familiar, Text: "friend", Operation: presentation.Append}, {Pane: presentation.Game, Text: "ignored", Operation: presentation.Clear}}})
	want := []string{"[system 01:02:03] connection: READY", "one", "one", "", "[familiar] friend"}
	if len(model.pendingTranscript) != len(want) {
		t.Fatalf("queue=%#v", model.pendingTranscript)
	}
	for index := range want {
		if model.pendingTranscript[index] != want[index] {
			t.Fatalf("queue[%d]=%q want %q", index, model.pendingTranscript[index], want[index])
		}
	}
}

func TestEnhancedModelReplaceUsesGameAndFamiliarSourcePathOnce(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	logger := &recordingLogger{enabled: true}
	model.logger = logger
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Entries: []presentation.Entry{
		{Pane: presentation.Game, Text: "You are dead", Operation: presentation.Replace},
		{Pane: presentation.Familiar, Text: "You are dead", Operation: presentation.Replace},
		{Pane: presentation.Game, Text: " \t", Operation: presentation.Replace},
	}})
	highlighted := model.highlightText("You are dead")
	if got, want := model.pendingTranscript[1:], []string{highlighted, "[familiar] " + highlighted}; !equalStrings(got, want) {
		t.Fatalf("transcript=%q want %q", got, want)
	}
	if got, want := logger.writes, []string{"You are dead", "You are dead"}; !equalStrings(got, want) {
		t.Fatalf("log=%q want %q", got, want)
	}
	if !contains(model.agent.recent, "You are dead\nYou are dead\n") {
		t.Fatalf("recent=%q", model.agent.recent)
	}
}

func TestEnhancedModelResizeDoesNotClearTranscript(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	updated, command := model.Update(tea.WindowSizeMsg{Width: 44, Height: 20})
	model = updated.(EnhancedModel)
	if command != nil || !model.dimensionsReceived || model.width != 44 || model.height != 20 {
		t.Fatalf("resize command=%v dimensionsReceived=%v dimensions=%dx%d", command, model.dimensionsReceived, model.width, model.height)
	}
}

func TestEnhancedModelSynchronizesInputPresentationWithoutChangingDraft(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	initialGeometry := calculateDashboardGeometry(model.width, model.height, false, model.themes.current(), model.inputLabel)
	if model.inputLabel != "Command > " || model.input.Width() != initialGeometry.inputWidth || !model.input.Focused() {
		t.Fatalf("initial input prompt=%q width=%d geometry=%+v", model.inputLabel, model.input.Width(), initialGeometry)
	}
	model.agent.client = nativeTerminalAgent{}
	model.input.SetValue("say 你好")
	model.input.SetCursorColumn(3)
	model.width, model.height = 168, 24
	model.mapOutput = []string{"@"}
	model.syncInputPresentation()
	mapGeometry := calculateDashboardGeometry(model.width, model.height, true, model.themes.current(), model.inputLabel)
	if mapGeometry.inputInterior != 123 || model.input.Width() != mapGeometry.inputWidth || ansi.StringWidth(model.inputLabel)+model.input.Width() > mapGeometry.inputInterior || !model.input.Focused() {
		t.Fatalf("mapped input prompt=%q width=%d geometry=%+v", model.inputLabel, model.input.Width(), mapGeometry)
	}

	updated, _ := model.Update(presentation.Update{Connection: presentation.Ready, Prompt: strings.Repeat("very-long-prompt", 8) + ">"})
	model = updated.(EnhancedModel)
	if !strings.HasPrefix(model.inputLabel, "Command ") || model.input.Value() != "say 你好" || model.input.Column() != 3 || !model.input.Focused() {
		t.Fatalf("prompt update changed input prompt=%q value=%q cursor=%d", model.inputLabel, model.input.Value(), model.input.Column())
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	model = updated.(EnhancedModel)
	if model.inputLabel != "Whisper > " || model.input.Value() != "say 你好" || model.input.Column() != 3 || !model.input.Focused() {
		t.Fatalf("agent toggle changed input prompt=%q value=%q cursor=%d", model.inputLabel, model.input.Value(), model.input.Column())
	}
	model.themes.add(theme{Name: "wide-padding", Padding: 2})
	model.viewMode = ViewModeTheme
	model = model.handleThemeKeys(tea.KeyPressMsg{Code: 'G'})
	if model.input.Value() != "say 你好" || model.input.Column() != 3 {
		t.Fatalf("theme change changed input value=%q cursor=%d", model.input.Value(), model.input.Column())
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 10, Height: 24})
	model = updated.(EnhancedModel)
	if model.input.Width() < 1 || ansi.StringWidth(model.inputLabel)+model.input.Width() > 10 || model.input.Value() != "say 你好" || model.input.Column() != 3 {
		t.Fatalf("narrow input prompt=%q width=%d value=%q cursor=%d", model.inputLabel, model.input.Width(), model.input.Value(), model.input.Column())
	}
	for _, row := range strings.Split(model.View().Content, "\n") {
		if got := ansi.StringWidth(row); got > 10 {
			t.Fatalf("narrow input wrapped at width %d: %q", got, row)
		}
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	model = updated.(EnhancedModel)
	if !strings.HasPrefix(model.inputLabel, "Command") || model.input.Value() != "say 你好" || model.input.Column() != 3 {
		t.Fatalf("command restore prompt=%q value=%q cursor=%d", model.inputLabel, model.input.Value(), model.input.Column())
	}
}

func TestEnhancedModelWrapsDraftWithoutChangingSubmittedCommand(t *testing.T) {
	session := &fakeSession{updates: make(chan presentation.Update)}
	model := newTestModel(t, session)
	model.width, model.height = 49, 24
	model.mapOutput = []string{"@"}
	draft := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda"
	model.input.SetValue(draft)
	model.syncInputPresentation()

	inputRows := strings.Split(ansi.Strip(model.buildInput()), "\n")
	if len(inputRows) < 2 {
		t.Fatalf("draft did not wrap: %q", inputRows)
	}
	if strings.Count(strings.Join(inputRows, "\n"), "Command >") != 1 {
		t.Fatalf("prompt count=%d rows=%q", strings.Count(strings.Join(inputRows, "\n"), "Command >"), inputRows)
	}
	if model.input.Value() != draft {
		t.Fatalf("wrapped value=%q want %q", model.input.Value(), draft)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if len(session.sent) != 1 || session.sent[0] != draft || model.input.Value() != "" {
		t.Fatalf("submission sent=%q remaining=%q", session.sent, model.input.Value())
	}
}

func TestEnhancedModelWrappedDraftFollowsCursorWithinInputRows(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.width, model.height = 49, 24
	model.mapOutput = []string{"@", "|", "o", "|", "o", "|", "o", "|", "o"}
	draft := "HEAD " + strings.Repeat("alpha ", 40) + "TAIL"
	model.input.SetValue(draft)
	model.syncInputPresentation()
	geometry := calculateDashboardGeometry(model.width, model.height, true, model.themes.current(), model.inputLabel)

	endRows := strings.Split(ansi.Strip(model.buildInput()), "\n")
	if model.input.Height() != geometry.inputHeight || len(endRows) != geometry.inputHeight || model.input.ScrollYOffset() == 0 || !strings.Contains(strings.Join(endRows, ""), "TAIL") || strings.Contains(strings.Join(endRows, ""), "HEAD") {
		t.Fatalf("end viewport height=%d offset=%d rows=%q geometry=%+v", model.input.Height(), model.input.ScrollYOffset(), endRows, geometry)
	}
	for _, row := range endRows {
		if got := ansi.StringWidth(row); got != geometry.inputInterior {
			t.Fatalf("wrapped row width=%d want %d: %q", got, geometry.inputInterior, row)
		}
	}
	if strings.Count(strings.Join(endRows, "\n"), "Command >") != 1 {
		t.Fatalf("scrolled prompt rows=%q", endRows)
	}

	model.input.SetCursorColumn(0)
	model.input.SetHeight(model.input.Height())
	startRows := strings.Split(ansi.Strip(model.buildInput()), "\n")
	if model.input.ScrollYOffset() != 0 || !strings.Contains(strings.Join(startRows, ""), "HEAD") || strings.Contains(strings.Join(startRows, ""), "TAIL") {
		t.Fatalf("start viewport offset=%d rows=%q", model.input.ScrollYOffset(), startRows)
	}

	model.input.SetValue(strings.Repeat("x", 40))
	model.input.SetCursorColumn(model.input.Width() - 1)
	before := model.input.LineInfo().RowOffset
	model.input.SetCursorColumn(model.input.Width())
	after := model.input.LineInfo().RowOffset
	if before != 0 || after != 1 {
		t.Fatalf("hard-wrap cursor rows before=%d after=%d width=%d", before, after, model.input.Width())
	}
}

func TestEnhancedModelWrappedDraftSurvivesResizeAndModeChange(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.agent.client = nativeTerminalAgent{}
	model.width, model.height = 49, 24
	model.mapOutput = []string{"@"}
	draft := strings.Repeat("wide 界 text ", 12)
	model.input.SetValue(draft)
	model.input.SetCursorColumn(17)
	model.syncInputPresentation()

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	model = updated.(EnhancedModel)
	if model.inputLabel != "Whisper > " || model.input.Value() != draft || model.input.Column() != 17 || len(strings.Split(ansi.Strip(model.buildInput()), "\n")) < 2 {
		t.Fatalf("F6 prompt=%q value=%q cursor=%d view=%q", model.inputLabel, model.input.Value(), model.input.Column(), ansi.Strip(model.buildInput()))
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 19})
	model = updated.(EnhancedModel)
	geometry := calculateDashboardGeometry(100, 19, true, model.themes.current(), model.inputLabel)
	if model.input.Value() != draft || model.input.Column() != 17 || model.input.Height() > geometry.inputHeight {
		t.Fatalf("resize value=%q cursor=%d height=%d geometry=%+v", model.input.Value(), model.input.Column(), model.input.Height(), geometry)
	}
}

func TestEnhancedModelNormalizesCtrlVPasteIntoOneCommand(t *testing.T) {
	original := readInputClipboard
	readInputClipboard = func() (string, error) { return "look\r\nnorth\twest\n", nil }
	t.Cleanup(func() { readInputClipboard = original })
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.input.SetValue("say  now")
	model.input.SetCursorColumn(4)

	updated, command := model.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	model = updated.(EnhancedModel)
	if command == nil {
		t.Fatal("Ctrl-V returned no clipboard command")
	}
	updated, _ = model.Update(command())
	model = updated.(EnhancedModel)
	if got, want := model.input.Value(), "say look north west  now"; got != want || model.input.LineCount() != 1 || model.input.Column() != len([]rune("say look north west ")) {
		t.Fatalf("Ctrl-V value=%q lines=%d want %q", got, model.input.LineCount(), want)
	}
}

func TestEnhancedModelWrappedInputStaysWithinDashboardAtNarrowWidths(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height int
		mapLines      []string
		draft         string
		wantTail      bool
	}{
		{name: "zero", height: 24, draft: "界a"},
		{name: "one", width: 1, height: 24, draft: "界a"},
		{name: "two", width: 2, height: 24, draft: "界a"},
		{name: "no-map scroll", width: 60, height: 24, draft: "HEAD " + strings.Repeat("combining e\u0301 and 界 ", 20) + "TAIL", wantTail: true},
		{name: "compact map", width: 49, height: 24, mapLines: []string{"@"}, draft: strings.Repeat("combining e\u0301 and 界 ", 20)},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
			model.width, model.height = test.width, test.height
			model.mapOutput = test.mapLines
			model.input.SetValue(test.draft)
			model.syncInputPresentation()
			view := model.View().Content
			geometry := calculateDashboardGeometry(test.width, test.height, len(test.mapLines) > 0, model.themes.current(), model.inputLabel)
			if got := lipgloss.Height(view); got != geometry.rows {
				t.Fatalf("height=%d want %d: %q", got, geometry.rows, ansi.Strip(view))
			}
			for _, row := range strings.Split(view, "\n") {
				if got := ansi.StringWidth(row); got > max(0, test.width) {
					t.Fatalf("row width=%d want <=%d: %q", got, test.width, ansi.Strip(row))
				}
			}
			if model.input.Value() != test.draft {
				t.Fatalf("value=%q want %q", model.input.Value(), test.draft)
			}
			input := ansi.Strip(model.buildInput())
			if test.wantTail && !strings.Contains(input, "TAIL") {
				t.Fatalf("visible input=%q", input)
			}
		})
	}
}

func TestEnhancedModelDefersTranscriptUntilWindowSize(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.enqueueTranscript("startup record")
	if command := model.scheduleTranscript(); command != nil {
		t.Fatalf("default dimensions scheduled startup transcript: %T", command())
	}

	updated, command := model.Update(tea.WindowSizeMsg{Width: 44, Height: 20})
	model = updated.(EnhancedModel)
	if !model.dimensionsReceived || command == nil {
		t.Fatalf("first WindowSizeMsg dimensionsReceived=%v command=%v", model.dimensionsReceived, command)
	}
	if _, ok := command().(transcriptDrainMsg); !ok {
		t.Fatalf("first WindowSizeMsg command=%T, want transcriptDrainMsg", command())
	}
}

func TestEnhancedModelCtrlCQuitsImmediatelyOrAfterDrain(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(EnhancedModel)
	if !model.quitting {
		t.Fatal("Ctrl-C did not begin quitting")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("empty Ctrl-C command=%T, want QuitMsg", command())
	}

	model = newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.enqueueTranscript("last record")
	updated, command = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(EnhancedModel)
	if _, ok := command().(transcriptDrainMsg); !ok || !model.quitting {
		t.Fatalf("draining Ctrl-C command=%T quitting=%v", command(), model.quitting)
	}
}

func TestEnhancedModelCtrlCWaitsForInFlightAndPreSizeDrain(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.enqueueTranscript("startup record")
	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(EnhancedModel)
	if command == nil || !model.quitting {
		t.Fatalf("pre-size Ctrl-C command=%v quitting=%v", command, model.quitting)
	}
	if _, ok := command().(transcriptDrainMsg); !ok {
		t.Fatalf("pre-size Ctrl-C command=%T, want transcriptDrainMsg", command())
	}

	model = newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.dimensionsReceived = true
	model.drainScheduled = true
	model.enqueueTranscript("last record")
	updated, printCommand := model.Update(transcriptDrainMsg{})
	model = updated.(EnhancedModel)
	if printCommand == nil || len(model.pendingTranscript) != 0 || !model.drainScheduled {
		t.Fatalf("drain did not leave final print in flight: command=%v pending=%d scheduled=%v", printCommand, len(model.pendingTranscript), model.drainScheduled)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(EnhancedModel)
	if command != nil || !model.quitting || !model.drainScheduled {
		t.Fatalf("Ctrl-C during final print command=%v quitting=%v scheduled=%v", command, model.quitting, model.drainScheduled)
	}
	_, command = model.Update(transcriptDrainMsg{})
	if command == nil {
		t.Fatal("final drain did not schedule quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("final drain command=%T, want QuitMsg", command())
	}
}

func TestEnhancedModelSourceCloseDrainsInFlightRecordAndStaysDisconnected(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.dimensionsReceived = true
	model.drainScheduled = true
	model.enqueueTranscript("last record")
	updated, printCommand := model.Update(transcriptDrainMsg{})
	model = updated.(EnhancedModel)
	if printCommand == nil || len(model.pendingTranscript) != 0 || !model.drainScheduled {
		t.Fatalf("drain did not leave final print in flight: command=%v pending=%d scheduled=%v", printCommand, len(model.pendingTranscript), model.drainScheduled)
	}

	updated, command := model.Update(sessionClosedMsg{})
	model = updated.(EnhancedModel)
	if command != nil || !model.sourceDone || model.snapshot.Connection != presentation.Disconnected || len(model.pendingTranscript) != 1 {
		t.Fatalf("source close command=%v sourceDone=%v connection=%v pending=%v", command, model.sourceDone, model.snapshot.Connection, model.pendingTranscript)
	}
	if !contains(model.pendingTranscript[0], "disconnected") {
		t.Fatalf("source close did not enqueue disconnect: %v", model.pendingTranscript)
	}

	updated, command = model.Update(transcriptDrainMsg{})
	model = updated.(EnhancedModel)
	if command == nil || len(model.pendingTranscript) != 0 || !model.drainScheduled {
		t.Fatalf("disconnect did not drain: command=%v pending=%d scheduled=%v", command, len(model.pendingTranscript), model.drainScheduled)
	}
	updated, command = model.Update(transcriptDrainMsg{})
	model = updated.(EnhancedModel)
	if command != nil || model.quitting || model.snapshot.Connection != presentation.Disconnected {
		t.Fatalf("source close quit or changed state: command=%v quitting=%v connection=%v", command, model.quitting, model.snapshot.Connection)
	}
}

type recordingLogger struct {
	enabled bool
	writes  []string
}

func (logger *recordingLogger) Start(string) (telemetry.StartResult, error) {
	logger.enabled = true
	return telemetry.StartResult{}, nil
}
func (logger *recordingLogger) Stop() error { logger.enabled = false; return nil }
func (logger *recordingLogger) Write(line string) error {
	logger.writes = append(logger.writes, line)
	return nil
}
func (logger *recordingLogger) IsEnabled() bool { return logger.enabled }
func (logger *recordingLogger) Path() string    { return "" }

func TestEnhancedModelUsesInlineViewAndMapNavigation(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.width, model.height = 80, 24
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Location: presentation.Location{Title: "[Town]", Exits: []string{"north"}}, Hands: presentation.Hands{Left: "shield", Right: "sword", PreparedSpell: "Fire"}, Map: presentation.Map{Lines: []string{"o─@", "  │", "  o"}, CurrentToken: "1", CurrentLine: 0, CurrentColumn: 2}})
	view := model.View()
	if view.AltScreen || view.MouseMode != tea.MouseModeNone {
		t.Fatalf("inline view=%#v", view)
	}
	if got := view.Content; !contains(got, "L: shield") || !contains(got, "R: sword") || !contains(got, "Sp: Fire") || !contains(got, "Exits: north") || contains(got, "DragonRealms") {
		t.Fatalf("dashboard=%q", got)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(EnhancedModel)
	if !model.mapNavigation {
		t.Fatal("escape did not enter map navigation")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated.(EnhancedModel)
	if model.mapNavigation {
		t.Fatal("tab did not return input focus")
	}
}

func TestEnhancedModelEscapeKeepsMapVisibleAtMinimumHeight(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.width, model.height = 60, 19
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Map: presentation.Map{Lines: []string{"@"}}})
	if !model.mapVisible() {
		t.Fatal("map hidden before Escape")
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(EnhancedModel)
	if !model.mapNavigation || !model.mapVisible() {
		t.Fatalf("Escape navigation=%v mapVisible=%v", model.mapNavigation, model.mapVisible())
	}
}

func TestEnhancedModelPlacesHandsBesideVisibleMap(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.width, model.height = 60, 22
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Location: presentation.Location{Title: "[Town]"}, Hands: presentation.Hands{Left: "shield", Right: "sword"}, Map: presentation.Map{Lines: []string{"o---@", "    |", "    o", "    |", "    o"}, CurrentToken: "1", CurrentLine: 0, CurrentColumn: 4}})
	for _, row := range strings.Split(model.View().Content, "\n") {
		if contains(row, "L: shield") && contains(row, "│") {
			return
		}
	}
	t.Fatalf("hands were not beside visible map: %q", model.View().Content)
}

func TestEnhancedModelModalsKeepDashboardHeight(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.width, model.height = 100, 30
	model.mapOutput = []string{"@", "|", "o", "|", "o", "|", "o", "|"}
	want := lipgloss.Height(model.View().Content)
	for _, mode := range []ViewMode{ViewModeHelp, ViewModeTheme} {
		model.viewMode = mode
		if got := lipgloss.Height(model.View().Content); got != want {
			t.Errorf("mode %v height = %d, want dashboard height %d", mode, got, want)
		}
	}
}

func TestEnhancedModelModesPreviewThemeAndPreserveInteractiveState(t *testing.T) {
	useANSI256(t)
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.width, model.height = 100, 30
	model.mapOutput = []string{"@", "|", "o", "|", "o", "|", "o", "|"}
	model.mapPanLine, model.mapPanColumn = 4, 7
	model.mapNavigation = true
	model.input.SetValue("draft command")
	model.input.SetCursorColumn(5)
	model.syncInputPresentation()

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	model = updated.(EnhancedModel)
	if !strings.Contains(model.View().Content, "Help") {
		t.Fatalf("help frame missing: %q", model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	model = updated.(EnhancedModel)
	before := model.View().Content
	updated, _ = model.Update(keyDown())
	model = updated.(EnhancedModel)
	after := model.View().Content
	if strings.Contains(before, "38;5;237") || !strings.Contains(after, "38;5;237") || !strings.Contains(after, "> dark") {
		t.Fatalf("theme did not preview before close\nbefore=%q\nafter=%q", before, after)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 60, Height: 22})
	model = updated.(EnhancedModel)
	if model.input.Value() != "draft command" || model.input.Column() != 5 || model.mapPanLine != 4 || model.mapPanColumn != 7 || !model.mapNavigation {
		t.Fatalf("mode state changed: input=%q cursor=%d pan=%d,%d navigation=%v", model.input.Value(), model.input.Column(), model.mapPanLine, model.mapPanColumn, model.mapNavigation)
	}
	for _, size := range [][2]int{{100, 30}, {100, 19}, {60, 15}, {5, 24}} {
		model.width, model.height = size[0], size[1]
		model.syncInputPresentation()
		dashboardRows := lipgloss.Height(renderDashboard(model.width, model.height, model.snapshot, model.buildStatusBar(), model.buildInput(), model.mapOutput, model.mapPanLine, model.mapPanColumn, model.mapNavigation, model.themes.current()))
		if got := lipgloss.Height(model.renderThemeSelector()); got != dashboardRows {
			t.Fatalf("size %dx%d modal rows=%d dashboard rows=%d", size[0], size[1], got, dashboardRows)
		}
	}
}

func TestEnhancedModelHelpShiftGMovesToBottom(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.viewMode = ViewModeHelp
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'g', Mod: tea.ModShift}))
	model = updated.(EnhancedModel)
	if model.modalOffset != 1<<30 {
		t.Fatalf("Shift-G help offset=%d, want bottom sentinel", model.modalOffset)
	}
}

func TestEnhancedModelMapShiftGMovesToBottomAndPreservesDraft(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.mapOutput = []string{"@", "|", "o", "|", "o"}
	model.mapNavigation = true
	model.input.SetValue("draft command")
	model.input.SetCursorColumn(5)

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'g', Mod: tea.ModShift}))
	model = updated.(EnhancedModel)
	if model.mapPanLine != len(model.mapOutput)-1 || model.input.Value() != "draft command" || model.input.Column() != 5 {
		t.Fatalf("Shift-G map pan=%d draft=%q cursor=%d", model.mapPanLine, model.input.Value(), model.input.Column())
	}
}

func TestEnhancedModelEditorPreservesDraftAndRemovesTemporaryFile(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	path := filepath.Join(t.TempDir(), "draft")
	if err := os.WriteFile(path, []byte("dance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model.finishEditor(editorFinishedMsg{path: path, draft: "look"})
	if model.input.Value() != "dance" {
		t.Fatalf("editor result=%q", model.input.Value())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("editor file remains: %v", err)
	}
	path = filepath.Join(t.TempDir(), "tabbed")
	if err := os.WriteFile(path, []byte("dance\tquickly\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model.finishEditor(editorFinishedMsg{path: path, draft: "look"})
	if model.input.Value() != "dance quickly" || model.input.LineCount() != 1 {
		t.Fatalf("tabbed editor value=%q lines=%d", model.input.Value(), model.input.LineCount())
	}
	path = filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model.finishEditor(editorFinishedMsg{path: path, draft: "dance"})
	if model.input.Value() != "dance" || !contains(model.pendingTranscript[len(model.pendingTranscript)-1], "editor returned more than one line") {
		t.Fatalf("multiline editor: input=%q queue=%v", model.input.Value(), model.pendingTranscript)
	}
	path = filepath.Join(t.TempDir(), "process-failure")
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model.finishEditor(editorFinishedMsg{path: path, draft: "dance", err: errors.New("editor exited 1")})
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || model.input.Value() != "dance" || !contains(strings.Join(model.pendingTranscript, "\n"), "editor failed: editor exited 1") {
		t.Fatalf("process failure: file=%v input=%q queue=%v", err, model.input.Value(), model.pendingTranscript)
	}
	path = filepath.Join(t.TempDir(), "read-failure")
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	readEditorFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
	t.Cleanup(func() { readEditorFile = os.ReadFile })
	model.finishEditor(editorFinishedMsg{path: path, draft: "dance"})
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || model.input.Value() != "dance" || !contains(strings.Join(model.pendingTranscript, "\n"), "editor failed: read failed") {
		t.Fatalf("read failure: file=%v input=%q queue=%v", err, model.input.Value(), model.pendingTranscript)
	}
	readEditorFile = os.ReadFile
	removeEditorFile = func(string) error { return errors.New("remove failed") }
	t.Cleanup(func() { removeEditorFile = os.Remove })
	path = filepath.Join(t.TempDir(), "remove-failure")
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model.finishEditor(editorFinishedMsg{path: path, draft: "dance"})
	if model.input.Value() != "dance" || !contains(strings.Join(model.pendingTranscript, "\n"), "editor failed: remove failed") {
		t.Fatalf("remove failure: input=%q queue=%v", model.input.Value(), model.pendingTranscript)
	}
}

func TestEnhancedModelLoggingToggleAndConnectionNotices(t *testing.T) {
	logDir := t.TempDir()
	model := InitialEnhancedModel(&fakeSession{updates: make(chan presentation.Update)}, Options{Character: "Hero", LogDir: logDir})
	if model.logger.IsEnabled() {
		t.Fatal("logging enabled initially")
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyF4})
	model = updated.(EnhancedModel)
	if !model.logger.IsEnabled() || model.logState != logOn {
		t.Fatalf("logging start: %v", model.logState)
	}
	model.input.SetValue("look")
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(EnhancedModel)
	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("log entries=%d err=%v", len(entries), err)
	}
	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil || !contains(string(data), "> look") {
		t.Fatalf("command log=%q err=%v", data, err)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF4})
	model = updated.(EnhancedModel)
	if model.logger.IsEnabled() || model.logState != logOff {
		t.Fatalf("logging stop: %v", model.logState)
	}
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready})
	model.applySessionUpdate(presentation.Update{Connection: presentation.Reconnecting})
	if len(model.pendingTranscript) < 3 {
		t.Fatalf("connection notices=%v", model.pendingTranscript)
	}
}

func contains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
