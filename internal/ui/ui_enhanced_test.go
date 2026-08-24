package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dr-charm/internal/dragonrealms"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEnhancedModelConsumesConsecutiveSessionUpdates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	updates := make(chan dragonrealms.Update, 2)
	updates <- dragonrealms.Update{Snapshot: dragonrealms.Snapshot{Character: "Hero", Connection: dragonrealms.ConnectionReady, Room: dragonrealms.Room{Title: "[First]"}}, Display: []dragonrealms.DisplayEvent{{Kind: dragonrealms.DisplayText, Stream: "main", Text: "first"}}}
	updates <- dragonrealms.Update{Snapshot: dragonrealms.Snapshot{Character: "Hero", Connection: dragonrealms.ConnectionReady, Room: dragonrealms.Room{Title: "[Second]"}, Prompt: ">"}, Display: []dragonrealms.DisplayEvent{{Kind: dragonrealms.DisplayText, Stream: "main", Text: "second"}}}
	close(updates)
	session := &fakeSession{updates: updates}
	model := InitialEnhancedModel(session, "Hero")

	cmd := model.Init()
	for i := 0; i < 2; i++ {
		msg := cmd()
		updated, next := model.Update(msg)
		model = updated.(EnhancedModel)
		cmd = next
	}
	if model.snapshot.Room.Title != "[Second]" || model.snapshot.Prompt != ">" || !strings.Contains(model.View(), "second") {
		t.Fatalf("model did not apply consecutive updates: %#v\n%s", model.snapshot, model.View())
	}
	msg := cmd()
	updated, next := model.Update(msg)
	model = updated.(EnhancedModel)
	if next == nil || !model.quitting {
		t.Fatalf("closed update stream rearmed: next=%v quitting=%v", next, model.quitting)
	}
	if _, ok := next().(tea.QuitMsg); !ok {
		t.Fatalf("closed update stream command returned %T, want tea.QuitMsg", next())
	}
}

func TestEnhancedModelUsesSnapshotWithoutParsingDisplay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := &fakeSession{updates: make(chan dragonrealms.Update)}
	model := InitialEnhancedModel(session, "Hero")
	update := dragonrealms.Update{
		Snapshot: dragonrealms.Snapshot{Room: dragonrealms.Room{Title: "[Canonical]", Description: "Canonical description"}},
		Display:  []dragonrealms.DisplayEvent{{Kind: dragonrealms.DisplayText, Stream: "main", Text: "[Misleading Display Room]"}},
	}
	updated, _ := model.Update(update)
	model = updated.(EnhancedModel)
	if model.snapshot.Room.Title != "[Canonical]" || !strings.Contains(model.View(), "Canonical description") {
		t.Fatalf("model reparsed display instead of using snapshot: %#v", model.snapshot)
	}
}

func TestEnhancedModelSendsThroughSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	session := &fakeSession{updates: make(chan dragonrealms.Update)}
	model := InitialEnhancedModel(session, "Hero")
	if err := model.logger.Start(model.character); err != nil {
		t.Fatal(err)
	}
	model.input = "l"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(EnhancedModel)
	model.logger.Stop()
	if len(session.sent) != 1 || session.sent[0] != "look" {
		t.Fatalf("sent = %#v", session.sent)
	}
	history := strings.Join(model.mainOutput, "\n")
	if !strings.Contains(history, "> l") || strings.Contains(history, "> look") {
		t.Fatalf("command history = %q", history)
	}
	logDir := filepath.Join(home, ".dr-charm", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("log entries = %d", len(entries))
	}
	logData, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if logText := string(logData); !strings.Contains(logText, "> l") || strings.Contains(logText, "> look") {
		t.Fatalf("command log = %q", logText)
	}
	session.err = errors.New("send failed")
	model.input = "north"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if model.err == nil {
		t.Fatal("send error was not displayed")
	}
}

func TestEnhancedModelOwnsFamiliarHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := &fakeSession{updates: make(chan dragonrealms.Update)}
	model := InitialEnhancedModel(session, "Hero")
	updated, _ := model.Update(dragonrealms.Update{Display: []dragonrealms.DisplayEvent{{Kind: dragonrealms.DisplayText, Stream: "familiar", Text: "A raven speaks."}}})
	model = updated.(EnhancedModel)
	if got := model.familiarOutput; len(got) != 1 || got[0] != "A raven speaks." {
		t.Fatalf("familiar content = %#v", got)
	}
	updated, _ = model.Update(dragonrealms.Update{Display: []dragonrealms.DisplayEvent{{Kind: dragonrealms.DisplayClear, Stream: "familiar"}}})
	model = updated.(EnhancedModel)
	if got := model.familiarOutput; len(got) != 0 {
		t.Fatalf("familiar clear left %#v", got)
	}
}

func TestEnhancedModelProjectsSnapshotIntoRoomAndHands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	snapshot := dragonrealms.Snapshot{
		Room: dragonrealms.Room{
			Title: "[Square]", Description: "A broad square.", Exits: []string{"north", "", "east"},
			Objects: []string{"a bench"}, Players: []string{"Cennedig"}, Creatures: []string{"a rat"},
		},
		Hands: dragonrealms.Hands{Right: "a sword", Left: "a shield"}, PreparedSpell: "Fire Ball",
	}
	model.applySessionUpdate(dragonrealms.Update{Snapshot: snapshot})

	wantRoom := []string{"[Square]", "", "A broad square.", "", "Exits: north, east", "", "You also see:", "  a bench", "", "Also here:", "  Cennedig", "", "Creatures:", "  a rat"}
	if got := roomContent(model.snapshot); !reflect.DeepEqual(got, wantRoom) {
		t.Fatalf("room content = %#v, want %#v", got, wantRoom)
	}
	wantHands := []string{"Right: a sword", "Left: a shield", "", "Spell: Fire Ball"}
	if got := handsContent(model.snapshot); !reflect.DeepEqual(got, wantHands) {
		t.Fatalf("hands content = %#v, want %#v", got, wantHands)
	}
}

func TestEnhancedModelRoutesHistoriesDiagnosticsCommandsAndClears(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := &fakeSession{updates: make(chan dragonrealms.Update)}
	model := InitialEnhancedModel(session, "Hero")
	model.applySessionUpdate(dragonrealms.Update{
		Display: []dragonrealms.DisplayEvent{
			{Kind: dragonrealms.DisplayText, Stream: "main", Text: "main line"},
			{Kind: dragonrealms.DisplayText, Stream: "familiar", Text: "familiar line"},
			{Kind: dragonrealms.DisplayText, Stream: "main", Text: "duplicate", DuplicateEcho: true},
		},
		Diagnostics: []dragonrealms.Diagnostic{{Text: "parser warning"}},
	})
	if got := model.mainOutput; !reflect.DeepEqual(got, []string{"Connected to DragonRealms", "main line", "[protocol] parser warning"}) {
		t.Fatalf("main history = %#v", got)
	}
	if got := model.familiarOutput; !reflect.DeepEqual(got, []string{"familiar line"}) {
		t.Fatalf("familiar history = %#v", got)
	}
	model.input = "look"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if got := model.mainOutput[len(model.mainOutput)-1]; got != "> look" {
		t.Fatalf("command echo = %q", got)
	}
	model.activePane = "familiar"
	model.applySessionUpdate(dragonrealms.Update{Display: []dragonrealms.DisplayEvent{
		{Kind: dragonrealms.DisplayClear, Stream: "main"},
		{Kind: dragonrealms.DisplayClear, Stream: "familiar"},
	}})
	if len(model.mainOutput) != 0 || len(model.familiarOutput) != 0 || model.activePane != "main" {
		t.Fatalf("clear left histories/focus: main=%#v familiar=%#v active=%q", model.mainOutput, model.familiarOutput, model.activePane)
	}
}

func TestEnhancedModelHistoryCapsVisibilityAndFocusCycling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	for index := range 501 {
		model.addOutput(fmt.Sprintf("main-%03d", index))
	}
	familiar := make([]dragonrealms.DisplayEvent, 101)
	for index := range familiar {
		familiar[index] = dragonrealms.DisplayEvent{Kind: dragonrealms.DisplayText, Stream: "familiar", Text: fmt.Sprintf("familiar-%03d", index)}
	}
	model.applySessionUpdate(dragonrealms.Update{Display: familiar})
	if len(model.mainOutput) != 500 || model.mainOutput[0] != "main-001" {
		t.Fatalf("main cap = len %d first %q", len(model.mainOutput), model.mainOutput[0])
	}
	if got := model.familiarOutput; len(got) != 100 || got[0] != "familiar-001" {
		t.Fatalf("familiar cap = len %d first %q", len(got), got[0])
	}
	if !strings.Contains(model.View(), "Familiar") {
		t.Fatal("available familiar pane was not rendered")
	}
	for _, want := range []string{"room", "hands", "familiar", "main"} {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
		model = updated.(EnhancedModel)
		if got := model.activePane; got != want {
			t.Fatalf("active pane = %q, want %q", got, want)
		}
	}
}

func TestEnhancedModelRepeatedResizeAndRender(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 40}, {Width: 20, Height: 5}, {Width: 80, Height: 24}} {
		updated, _ := model.Update(size)
		model = updated.(EnhancedModel)
		if got := model.View(); got == "" {
			t.Fatalf("empty render at %#v", size)
		}
	}
}

func TestEnhancedModelSnapshotReplacementDropsPriorProjection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	first := completeLayoutSnapshot()
	second := dragonrealms.Snapshot{Connection: dragonrealms.ConnectionReady, Character: "Hero", Room: dragonrealms.Room{Title: "[Second Room]"}}
	model.applySessionUpdate(dragonrealms.Update{Snapshot: first})
	model.applySessionUpdate(dragonrealms.Update{Snapshot: second})
	if !reflect.DeepEqual(model.snapshot, second) {
		t.Fatalf("snapshot merged instead of replaced: %#v", model.snapshot)
	}
	view := model.View()
	for _, stale := range []string{"A broad green.", "a bench", "a sword", "Fire Ball"} {
		if strings.Contains(view, stale) {
			t.Fatalf("second snapshot render retained %q: %q", stale, view)
		}
	}
	if !strings.Contains(view, "[Second Room]") {
		t.Fatalf("second snapshot missing from %q", view)
	}
}

func TestEnhancedModelPaneCyclesWithAndWithoutFamiliar(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	for _, want := range []string{"room", "hands", "main"} {
		model.cyclePane()
		if model.activePane != want {
			t.Fatalf("no-familiar cycle = %q, want %q", model.activePane, want)
		}
	}
	model.familiarOutput = []string{"", "not visible"}
	if familiarAvailable(model.familiarOutput) || strings.Contains(model.View(), "Familiar") {
		t.Fatal("empty first familiar entry was treated as available")
	}
	for _, want := range []string{"room", "hands", "main"} {
		model.cyclePane()
		if model.activePane != want {
			t.Fatalf("empty-first familiar cycle = %q, want %q", model.activePane, want)
		}
	}
	model.familiarOutput = []string{"visible"}
	for _, want := range []string{"room", "hands", "familiar", "main"} {
		model.cyclePane()
		if model.activePane != want {
			t.Fatalf("familiar cycle = %q, want %q", model.activePane, want)
		}
	}
}

func TestEnhancedModelFamiliarCapRolloverRestoresMainFocus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	model.familiarOutput = make([]string, 100)
	model.familiarOutput[0] = "visible oldest"
	model.activePane = "familiar"
	model.addFamiliarOutput("newest")
	if len(model.familiarOutput) != 100 || model.familiarOutput[0] != "" || model.activePane != "main" {
		t.Fatalf("rollover = len %d first %q active %q", len(model.familiarOutput), model.familiarOutput[0], model.activePane)
	}
}

func TestEnhancedModelScrollOffsetOnlyAffectsSinglePane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	model.width = 80
	model.height = 20
	model.mainOutput = nil
	for index := range 20 {
		model.mainOutput = append(model.mainOutput, fmt.Sprintf("line-%02d", index))
	}
	model.scrollOffset = 5
	model.viewMode = ViewModeSingle
	single := model.View()
	if !strings.Contains(single, "line-14") || strings.Contains(single, "line-19") {
		t.Fatalf("single-pane scroll ignored offset: %q", single)
	}
	model.viewMode = ViewModeMulti
	multi := model.View()
	if !strings.Contains(multi, "line-19") {
		t.Fatalf("multi-pane render did not show independent tail: %q", multi)
	}
}

func TestConnectionBannerAppearsInSingleAndMultiPane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	for _, mode := range []ViewMode{ViewModeSingle, ViewModeMulti} {
		model.viewMode = mode
		if view := model.View(); !strings.Contains(view, "Connected to DragonRealms") {
			t.Fatalf("mode %v omitted connection banner", mode)
		}
	}
}

type fakeSession struct {
	updates chan dragonrealms.Update
	sent    []string
	err     error
}

func (s *fakeSession) Send(command string) error {
	s.sent = append(s.sent, command)
	return s.err
}

func (s *fakeSession) Updates() <-chan dragonrealms.Update { return s.updates }
