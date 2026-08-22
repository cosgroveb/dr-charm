package ui

import (
	"errors"
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
	t.Setenv("HOME", t.TempDir())
	session := &fakeSession{updates: make(chan dragonrealms.Update)}
	model := InitialEnhancedModel(session, "Hero")
	model.input = "l"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if len(session.sent) != 1 || session.sent[0] != "look" {
		t.Fatalf("sent = %#v", session.sent)
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
	if got := model.layout.panes["familiar"].Content; len(got) != 1 || got[0] != "A raven speaks." {
		t.Fatalf("familiar content = %#v", got)
	}
	updated, _ = model.Update(dragonrealms.Update{Display: []dragonrealms.DisplayEvent{{Kind: dragonrealms.DisplayClear, Stream: "familiar"}}})
	model = updated.(EnhancedModel)
	if got := model.layout.panes["familiar"].Content; len(got) != 0 {
		t.Fatalf("familiar clear left %#v", got)
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
