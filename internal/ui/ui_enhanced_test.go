package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"dr-charm/internal/presentation"
	"dr-charm/internal/telemetry"
)

func TestEnhancedModelConsumesPresentationUpdatesAndKeepsClosedSourceVisible(t *testing.T) {
	session := &fakeSession{updates: make(chan presentation.Update, 2)}
	session.updates <- presentation.Update{
		Connection: presentation.Ready,
		Title:      "[First]",
		Prompt:     ">",
		Entries:    []presentation.Entry{{Pane: presentation.Game, Text: "first", Operation: presentation.Append}},
	}
	session.updates <- presentation.Update{
		Connection: presentation.Ready,
		Title:      "[Second]",
		Prompt:     ">",
		Entries:    []presentation.Entry{{Pane: presentation.Game, Text: "second", Operation: presentation.Append}},
	}
	close(session.updates)

	model := newTestModel(t, session, false)
	cmd := waitForSessionUpdate(session)
	for range 2 {
		msg := cmd()
		updated, next := model.Update(msg)
		model = updated.(EnhancedModel)
		cmd = next
	}
	if model.snapshot.Title != "[Second]" || !strings.Contains(model.View().Content, "second") {
		t.Fatalf("model did not apply updates: %#v\n%s", model.snapshot, model.View().Content)
	}
	updated, next := model.Update(cmd())
	model = updated.(EnhancedModel)
	if next != nil || !model.sourceDone || model.quitting {
		t.Fatalf("closed source state: next=%v sourceDone=%v quitting=%v", next, model.sourceDone, model.quitting)
	}
	view := model.View().Content
	if !strings.Contains(view, "DISCONNECTED") || !strings.Contains(view, "[system 01:02:03] disconnected") {
		t.Fatalf("closed source not visible: %q", view)
	}
}

func TestEnhancedModelSendsOriginalCommandAndLogsAfterSuccess(t *testing.T) {
	logDir := t.TempDir()
	session := &fakeSession{updates: make(chan presentation.Update)}
	model := newTestModelWithLogDir(t, session, logDir, true)
	model.Init()
	model.input.SetValue(" l at target ")

	updated, _ := model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)
	if got := session.sent; len(got) != 1 || got[0] != "look at target" {
		t.Fatalf("sent=%#v", got)
	}
	if !strings.Contains(strings.Join(model.mainOutput, "\n"), ">  l at target ") {
		t.Fatalf("command echo missing: %#v", model.mainOutput)
	}
	if err := model.logger.Stop(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("log entries=%d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, ">  l at target ") {
		t.Fatalf("command log=%q", text)
	}

	session.err = errors.New("send failed")
	model.input.SetValue("north")
	updated, _ = model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)
	if got := session.sent; len(got) != 2 {
		t.Fatalf("send error did not call session: %#v", got)
	}
	if strings.Contains(strings.Join(model.mainOutput, "\n"), "> north") {
		t.Fatalf("failed command was echoed: %#v", model.mainOutput)
	}
}

func TestEnhancedModelOwnsAliasesAndHighlights(t *testing.T) {
	useANSI256(t)
	session := &fakeSession{updates: make(chan presentation.Update)}
	model := newTestModel(t, session, false)
	model.input.SetValue("n")

	updated, _ := model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)
	if got := session.sent; len(got) != 1 || got[0] != "north" {
		t.Fatalf("sent=%#v", got)
	}

	model.applySessionUpdate(presentation.Update{
		Connection: presentation.Ready,
		Entries:    []presentation.Entry{{Pane: presentation.Game, Text: "Goblin just arrived.", Operation: presentation.Append}},
	})
	if output := strings.Join(model.mainOutput, "\n"); !strings.Contains(output, "\x1b[") {
		t.Fatalf("highlight not applied: %q", output)
	}
}

func TestEnhancedModelReportsTranscriptWriteFailure(t *testing.T) {
	logger := &fakeLogger{enabled: true, writeErr: errors.New("disk full"), stopErr: errors.New("sync failed")}
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.logger = logger
	model.logState = logOn

	model.applySessionUpdate(presentation.Update{
		Connection: presentation.Ready,
		Entries:    []presentation.Entry{{Pane: presentation.Game, Text: "line", Operation: presentation.Append}},
	})
	if model.logState != logFailed || logger.stopCalls != 1 {
		t.Fatalf("log state=%v stopCalls=%d", model.logState, logger.stopCalls)
	}
	if output := strings.Join(model.mainOutput, "\n"); !strings.Contains(output, "logging failed: disk full (close failed: sync failed)") {
		t.Fatalf("missing logging failure: %q", output)
	}
}

func TestEnhancedModelRoutesPaneUpdatesFocusUnreadAndScroll(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 20
	model.resizePanes()
	lines := make([]string, 0, 30)
	for i := range 30 {
		lines = append(lines, "line-"+string(rune('A'+i%26)))
	}
	roomLines := make([]string, 0, 20)
	for i := range 20 {
		roomLines = append(roomLines, fmt.Sprintf("room-%02d", i))
	}
	model.applySessionUpdate(presentation.Update{
		Connection: presentation.Ready,
		Entries: []presentation.Entry{
			{Pane: presentation.RoomPane, Text: strings.Join(roomLines, "\n"), Operation: presentation.Replace},
			{Pane: presentation.HandsPane, Text: "Right: sword\nLeft: shield", Operation: presentation.Replace},
			{Pane: presentation.Familiar, Text: "familiar", Operation: presentation.Append},
			{Pane: presentation.Game, Text: strings.Join(lines, "\n"), Operation: presentation.Append},
		},
	})
	if !model.unread[paneFamiliar] {
		t.Fatal("inactive familiar pane was not marked unread")
	}
	updated, _ := model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	if model.activePane != paneMain {
		t.Fatalf("active pane=%q, want main", model.activePane)
	}
	updated, _ = model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	if model.activePane != paneRoom {
		t.Fatalf("active pane=%q, want room", model.activePane)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	model = updated.(EnhancedModel)
	if model.activePane != paneMain {
		t.Fatalf("shift-tab active pane=%q", model.activePane)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	model = updated.(EnhancedModel)
	if model.activePane != paneInput {
		t.Fatalf("second shift-tab active pane=%q, want input", model.activePane)
	}
	updated, _ = model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	updated, _ = model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	if !containsPane(model.focusablePanes(), paneHands) {
		t.Fatalf("hands should be focusable when visible: %#v", model.focusablePanes())
	}
	updated, _ = model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	if model.activePane != paneHands {
		t.Fatalf("third tab active pane=%q, want hands", model.activePane)
	}
	model.replacePane(paneHands, strings.Split(strings.Repeat("hand\n", model.handsViewport.Height()+2), "\n"))
	if !containsPane(model.focusablePanes(), paneHands) {
		t.Fatalf("hands should be focusable when overflowing: %#v", model.focusablePanes())
	}
	model.activePane = paneFamiliar
	model.replacePane(paneFamiliar, nil)
	if model.activePane != paneMain {
		t.Fatalf("optional pane disappearance did not restore game focus: %q", model.activePane)
	}
	before := model.roomViewport.YOffset()
	model.activePane = paneRoom
	updated, _ = model.Update(key(tea.KeyPgUp))
	model = updated.(EnhancedModel)
	if model.roomViewport.YOffset() > before {
		t.Fatalf("room viewport moved the wrong way")
	}
	view := model.View()
	if view.MouseMode != tea.MouseModeCellMotion || view.OnMouse == nil {
		t.Fatalf("mouse mode = %v handler nil=%v", view.MouseMode, view.OnMouse == nil)
	}
	msg := view.OnMouse(tea.MouseWheelMsg{Button: tea.MouseWheelUp})()
	if _, ok := msg.(tea.MouseWheelMsg); !ok {
		t.Fatalf("mouse handler returned %T", msg)
	}
}

func TestEnhancedModelFocusIncludesVisiblePanesAndSinglePaneScrollsGame(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 20
	model.resizePanes()
	model.replacePane(paneRoom, []string{"small room"})
	model.appendPane(paneFamiliar, "familiar")
	if !containsPane(model.focusablePanes(), paneInput) {
		t.Fatalf("input should be focusable: %#v", model.focusablePanes())
	}
	if !containsPane(model.focusablePanes(), paneRoom) {
		t.Fatalf("visible room should be focusable: %#v", model.focusablePanes())
	}
	updated, _ := model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	if model.activePane != paneMain {
		t.Fatalf("active pane=%q, want main", model.activePane)
	}
	updated, _ = model.Update(key(tea.KeyTab))
	model = updated.(EnhancedModel)
	if model.activePane != paneRoom {
		t.Fatalf("active pane=%q, want room", model.activePane)
	}
	updated, _ = model.Update(key(tea.KeyF2))
	model = updated.(EnhancedModel)
	if model.viewMode != ViewModeSingle || model.activePane != paneInput {
		t.Fatalf("single mode=%v active=%q", model.viewMode, model.activePane)
	}
}

func TestEnhancedModelTogglesRoomSlotBetweenRoomAndMap(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 20
	model.resizePanes()
	model.applySessionUpdate(presentation.Update{
		Connection: presentation.Ready,
		Map:        "@ #1 [Town Square]\nExits: north",
		Entries: []presentation.Entry{
			{Pane: presentation.RoomPane, Text: "[Town Square]\n\nA room.", Operation: presentation.Replace},
		},
	})
	if !strings.Contains(model.View().Content, "Room") || strings.Contains(model.View().Content, "@ #1 [Town Square]") {
		t.Fatalf("default room view wrong:\n%s", model.View().Content)
	}

	updated, _ := model.Update(key(tea.KeyF5))
	model = updated.(EnhancedModel)
	if !model.showMap || !strings.Contains(model.View().Content, "Map") || !strings.Contains(model.View().Content, "@ #1 [Town Square]") {
		t.Fatalf("map view wrong:\n%s", model.View().Content)
	}

	updated, _ = model.Update(key(tea.KeyF5))
	model = updated.(EnhancedModel)
	if model.showMap || !strings.Contains(model.View().Content, "A room.") {
		t.Fatalf("room view not restored:\n%s", model.View().Content)
	}
}

func TestEnhancedModelInputFocusKeepsCommandEditingKeys(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 20
	model.resizePanes()
	model.activePane = paneInput
	model.input.Focus()
	model.input.SetValue("look north")
	model.input.SetCursor(4)

	updated, _ := model.Update(key(tea.KeyEnd))
	model = updated.(EnhancedModel)
	if got := model.input.Position(); got != len("look north") {
		t.Fatalf("end cursor=%d", got)
	}

	updated, _ = model.Update(key(tea.KeyHome))
	model = updated.(EnhancedModel)
	if got := model.input.Position(); got != 0 {
		t.Fatalf("home cursor=%d", got)
	}
}

func TestEnhancedModelInitialInputAcceptsTyping(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.Init()

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = updated.(EnhancedModel)
	if got := model.input.Value(); got != "n" {
		t.Fatalf("input value=%q, want n", got)
	}
}

func TestEnhancedModelPageUpDoesNotClearUnread(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 18
	model.resizePanes()
	lines := make([]string, 0, 20)
	for i := range 20 {
		lines = append(lines, fmt.Sprintf("room-%02d", i))
	}
	model.replacePane(paneRoom, lines)
	model.unread[paneRoom] = true
	model.activePane = paneRoom
	model.roomViewport.GotoBottom()

	updated, _ := model.Update(key(tea.KeyPgUp))
	model = updated.(EnhancedModel)
	if !model.unread[paneRoom] {
		t.Fatal("page up cleared unread")
	}
	model.roomViewport.GotoBottom()
	updated, _ = model.Update(key(tea.KeyPgDown))
	model = updated.(EnhancedModel)
	if model.unread[paneRoom] {
		t.Fatal("page down at bottom did not clear unread")
	}
}

func TestEnhancedModelResizePreservesScrolledInactivePaneOffset(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 18
	model.resizePanes()
	lines := make([]string, 0, 20)
	for i := range 20 {
		lines = append(lines, fmt.Sprintf("room-%02d", i))
	}
	model.replacePane(paneRoom, lines)
	model.activePane = paneRoom
	model.roomViewport.GotoTop()
	scrolledOffset := model.roomViewport.YOffset()
	model.activePane = paneMain

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 18})
	model = updated.(EnhancedModel)
	if model.roomViewport.YOffset() != scrolledOffset {
		t.Fatalf("room offset after resize = %d, want %d", model.roomViewport.YOffset(), scrolledOffset)
	}
}

func TestEnhancedModelPaneReplacementPreservesScrolledInactiveOffset(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 18
	model.resizePanes()
	roomLines := make([]string, 0, 20)
	for i := range 20 {
		roomLines = append(roomLines, fmt.Sprintf("room-%02d", i))
	}
	model.replacePane(paneRoom, roomLines)
	model.activePane = paneRoom
	model.roomViewport.GotoTop()
	scrolledOffset := model.roomViewport.YOffset()
	model.activePane = paneMain
	model.unread[paneRoom] = false

	updatedLines := append([]string(nil), roomLines...)
	updatedLines[len(updatedLines)-1] = "room-changed"
	model.replacePane(paneRoom, updatedLines)
	if model.roomViewport.YOffset() != scrolledOffset {
		t.Fatalf("room offset = %d, want %d", model.roomViewport.YOffset(), scrolledOffset)
	}
	if !model.unread[paneRoom] {
		t.Fatal("changed inactive pane was not marked unread")
	}
	model.unread[paneRoom] = false
	model.replacePane(paneRoom, updatedLines)
	if model.unread[paneRoom] {
		t.Fatal("identical replacement marked unread")
	}
}

func TestEnhancedModelAppendPreservesScrolledInactiveFamiliarOffset(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.width = 80
	model.height = 18
	model.resizePanes()
	lines := make([]string, 0, 20)
	for i := range 20 {
		lines = append(lines, fmt.Sprintf("familiar-%02d", i))
	}
	model.replacePane(paneFamiliar, lines)
	model.activePane = paneFamiliar
	model.familiarView.GotoTop()
	scrolledOffset := model.familiarView.YOffset()
	model.activePane = paneMain
	model.unread[paneFamiliar] = false

	model.appendPane(paneFamiliar, "new familiar line")
	if model.familiarView.YOffset() != scrolledOffset {
		t.Fatalf("familiar offset = %d, want %d", model.familiarView.YOffset(), scrolledOffset)
	}
	if !model.unread[paneFamiliar] {
		t.Fatal("changed inactive familiar pane was not marked unread")
	}
}

func TestEnhancedModelRecordsConnectionTransitionsAsDurableHistory(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready})
	model.applySessionUpdate(presentation.Update{Connection: presentation.Reconnecting})
	model.applySessionUpdate(presentation.Update{Connection: presentation.Reconnecting})
	output := strings.Join(model.mainOutput, "\n")
	if strings.Count(output, "connection: READY") != 1 || strings.Count(output, "connection: RECONNECTING") != 1 {
		t.Fatalf("connection history = %q", output)
	}
	if !strings.Contains(output, "[system 01:02:03] connection: READY") {
		t.Fatalf("timestamped connection history missing: %q", output)
	}
}

func TestEnhancedModelUsesTextInputHistoryAndEditorResult(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.input.SetValue("look")
	updated, _ := model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)
	model.input.SetValue("north")
	updated, _ = model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)

	updated, _ = model.Update(key(tea.KeyUp))
	model = updated.(EnhancedModel)
	if got := model.input.Value(); got != "north" {
		t.Fatalf("history up=%q", got)
	}
	model.finishEditor(editorFinishedMsg{path: writeEditorFile(t, "dance\n")})
	if got := model.input.Value(); got != "dance" {
		t.Fatalf("editor result=%q", got)
	}
	model.finishEditor(editorFinishedMsg{path: writeEditorFile(t, "one\ntwo\n"), draft: "dance"})
	if got := model.input.Value(); got != "dance" {
		t.Fatalf("multiline editor did not preserve draft: %q", got)
	}

	removeEditorFile = func(string) error { return errors.New("remove failed") }
	t.Cleanup(func() { removeEditorFile = os.Remove })
	model.finishEditor(editorFinishedMsg{path: writeEditorFile(t, "kick\n"), draft: "dance"})
	if got := model.input.Value(); got != "dance" {
		t.Fatalf("remove failure did not preserve draft: %q", got)
	}
	if output := strings.Join(model.mainOutput, "\n"); !strings.Contains(output, "editor failed: remove failed") {
		t.Fatalf("remove failure was not reported: %q", output)
	}
}

func TestEnhancedModelCleansEditorDraftAfterProcessFailure(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	path := writeEditorFile(t, "changed\n")
	removed := false
	removeEditorFile = func(got string) error {
		if got != path {
			t.Fatalf("removed %q, want %q", got, path)
		}
		removed = true
		return nil
	}
	t.Cleanup(func() { removeEditorFile = os.Remove })

	model.finishEditor(editorFinishedMsg{path: path, draft: "look", err: errors.New("editor exited 1")})
	if !removed {
		t.Fatal("editor temp file was not removed after process failure")
	}
	if got := model.input.Value(); got != "look" {
		t.Fatalf("draft = %q, want look", got)
	}
	if output := strings.Join(model.mainOutput, "\n"); !strings.Contains(output, "editor failed: editor exited 1") {
		t.Fatalf("process failure not reported: %q", output)
	}
}

func TestEnhancedModelCleansEditorDraftAfterReadFailure(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	path := writeEditorFile(t, "changed\n")
	readEditorFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
	removed := false
	removeEditorFile = func(got string) error {
		removed = true
		return os.Remove(got)
	}
	t.Cleanup(func() {
		readEditorFile = os.ReadFile
		removeEditorFile = os.Remove
	})

	model.finishEditor(editorFinishedMsg{path: path, draft: "look"})
	if !removed {
		t.Fatal("editor temp file was not removed after read failure")
	}
	if got := model.input.Value(); got != "look" {
		t.Fatalf("draft = %q, want look", got)
	}
	if output := strings.Join(model.mainOutput, "\n"); !strings.Contains(output, "editor failed: read failed") {
		t.Fatalf("read failure not reported: %q", output)
	}
}

func TestEnhancedModelLoggingCanStartDisabledAndToggleCurrentSession(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.Init()
	if model.logger.IsEnabled() || !strings.Contains(model.buildStatusBar(), "LOG off") {
		t.Fatalf("logging initial state enabled")
	}
	updated, _ := model.Update(key(tea.KeyF4))
	model = updated.(EnhancedModel)
	if !model.logger.IsEnabled() || !strings.Contains(model.buildStatusBar(), "LOG on") {
		t.Fatalf("logging did not start")
	}
	updated, _ = model.Update(key(tea.KeyF4))
	model = updated.(EnhancedModel)
	if model.logger.IsEnabled() || !strings.Contains(model.buildStatusBar(), "LOG off") {
		t.Fatalf("logging did not stop")
	}
}

func newTestModel(t *testing.T, session gameSession, logging bool) EnhancedModel {
	t.Helper()
	return newTestModelWithLogDir(t, session, t.TempDir(), logging)
}

func newTestModelWithLogDir(t *testing.T, session gameSession, logDir string, logging bool) EnhancedModel {
	t.Helper()
	model := InitialEnhancedModel(session, Options{
		Character: "Hero",
		LogDir:    logDir,
		ThemeDir:  t.TempDir(),
		Logging:   logging,
	})
	model.now = func() time.Time {
		return time.Date(2026, time.August, 30, 1, 2, 3, 0, time.Local)
	}
	return model
}

type fakeSession struct {
	updates chan presentation.Update
	sent    []string
	err     error
}

type fakeLogger struct {
	enabled   bool
	writeErr  error
	stopErr   error
	stopCalls int
}

func (l *fakeLogger) Start(string) (telemetry.StartResult, error) {
	l.enabled = true
	return telemetry.StartResult{Path: "/tmp/dr-charm.log"}, nil
}

func (l *fakeLogger) Stop() error {
	l.stopCalls++
	l.enabled = false
	return l.stopErr
}

func (l *fakeLogger) Write(string) error { return l.writeErr }
func (l *fakeLogger) IsEnabled() bool    { return l.enabled }
func (l *fakeLogger) Path() string       { return "/tmp/dr-charm.log" }

func (s *fakeSession) Send(command string) error {
	s.sent = append(s.sent, command)
	return s.err
}

func (s *fakeSession) Next() (presentation.Update, bool) {
	update, ok := <-s.updates
	return update, ok
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func ctrlKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: tea.ModCtrl})
}

func writeEditorFile(t *testing.T, contents string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "editor-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(contents); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}
