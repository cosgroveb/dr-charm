package presenter

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"dr-charm/internal/dragonrealms"
	"dr-charm/internal/presentation"
)

func TestClientTranslatesSessionUpdate(t *testing.T) {
	source := &fakeSource{updates: make(chan dragonrealms.Update, 1)}
	source.updates <- dragonrealms.Update{
		Prompted: true,
		Snapshot: dragonrealms.Snapshot{
			Character:  "Hero",
			Connection: dragonrealms.ConnectionReady,
			Room: dragonrealms.Room{
				Title: "[Room]", Description: "A room.", Exits: []string{"north", "", "east"},
				Objects: []string{"a bench"}, Players: []string{"Cennedig"}, Creatures: []string{"a rat"},
			},
			Hands:         dragonrealms.Hands{Right: "sword", Left: "shield"},
			PreparedSpell: "Fire Ball",
			Vitals:        dragonrealms.Vitals{Health: 90, Mana: 80, Stamina: 70, Spirit: 60, Concentration: 50},
			Posture:       dragonrealms.PostureStanding,
			Prompt:        ">",
		},
		Display: []dragonrealms.DisplayEvent{
			{Kind: dragonrealms.DisplayText, Stream: "main", Text: "main"},
			{Kind: dragonrealms.DisplayText, Stream: "familiar", Text: "familiar"},
			{Kind: dragonrealms.DisplayText, Stream: "main", Text: "duplicate", DuplicateEcho: true},
		},
		Diagnostics: []dragonrealms.Diagnostic{{Text: "diagnostic"}},
	}

	got, ok := newClient(source, t.TempDir()).Next()
	if !ok {
		t.Fatal("Next closed unexpectedly")
	}
	if got.Connection != presentation.Ready || got.Title != "[Room]" || got.Prompt != ">" || got.Character != "Hero" || !got.Prompted {
		t.Fatalf("top-level fields = %#v", got)
	}
	if want := []presentation.StatusField{
		{Label: "H", Value: "90%"},
		{Label: "M", Value: "80%"},
		{Label: "F", Value: "30%"},
		{Label: "C", Value: "50%"},
		{Label: "Sp", Value: "60%"},
		{Label: "Posture", Value: "Standing"},
		{Label: "Map", Value: "#1/1 rooms"},
	}; !reflect.DeepEqual(got.Status, want) {
		t.Fatalf("status = %#v, want %#v", got.Status, want)
	}
	kinds := []presentation.Operation{got.Entries[0].Operation, got.Entries[1].Operation, got.Entries[2].Operation, got.Entries[3].Operation}
	if want := []presentation.Operation{presentation.Replace, presentation.Replace, presentation.Append, presentation.Append}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("operations = %#v, want %#v", kinds, want)
	}
	body := got.Entries[0].Text + "\n" + got.Entries[1].Text
	for _, value := range []string{"[Room]", "A room.", "Exits: north, east", "Right: sword", "Spell: Fire Ball"} {
		if !strings.Contains(body, value) {
			t.Fatalf("projection omitted %q from %q", value, body)
		}
	}
	if !strings.Contains(got.Map, "@ #1 [Room]") {
		t.Fatalf("map projection = %q", got.Map)
	}
	for _, entry := range got.Entries {
		if entry.Text == "duplicate" {
			t.Fatalf("duplicate display was not suppressed: %#v", got.Entries)
		}
	}
	if len(got.Notices) != 1 || got.Notices[0].Text != "diagnostic" {
		t.Fatalf("notices = %#v", got.Notices)
	}
	retained := Translate(dragonrealms.Update{Snapshot: dragonrealms.Snapshot{Prompt: ">"}})
	if retained.Prompt != ">" || retained.Prompted {
		t.Fatalf("retained prompt=%q prompted=%v", retained.Prompt, retained.Prompted)
	}
}

func TestClientSanitizesNoticesAndVisibleFields(t *testing.T) {
	source := &fakeSource{updates: make(chan dragonrealms.Update, 1)}
	source.updates <- dragonrealms.Update{
		Snapshot: dragonrealms.Snapshot{
			Character:  "Hero\x1b]52;c;secret\a",
			Connection: dragonrealms.ConnectionClosed,
			Room:       dragonrealms.Room{Title: "[Room]\x1bP@kitty-cmd{}\x1b\\"},
			Prompt:     ">\x9d52;c;secret\x9c",
		},
		Diagnostics: []dragonrealms.Diagnostic{{Text: "bad\nnotice\x1b[31m"}},
		Err:         errors.New("boom\x1b]52;c;secret\a"),
	}

	got, ok := newClient(source, t.TempDir()).Next()
	if !ok {
		t.Fatal("Next closed unexpectedly")
	}
	combined := got.Title + got.Prompt + got.Character
	for _, notice := range got.Notices {
		combined += notice.Text
		if strings.ContainsAny(notice.Text, "\n\t") {
			t.Fatalf("notice not bounded to one line: %q", notice.Text)
		}
	}
	if strings.ContainsAny(combined, "\x1b\x07\x9d\x9c") {
		t.Fatalf("unsafe controls leaked: %#v", got)
	}
	if strings.Contains(combined, "secret") || len(got.Notices) != 2 || got.Notices[1].Text != "connection error" {
		t.Fatalf("unsafe notice contents leaked: %#v", got.Notices)
	}
	if got.Connection != presentation.Disconnected {
		t.Fatalf("connection = %v", got.Connection)
	}
}

func TestClientSendForwardsCommands(t *testing.T) {
	source := &fakeSource{updates: make(chan dragonrealms.Update)}
	client := newClient(source, t.TempDir())
	if err := client.Send("l"); err != nil {
		t.Fatal(err)
	}
	if got := source.sent; len(got) != 1 || got[0] != "l" {
		t.Fatalf("sent = %#v", got)
	}
}

func TestClientNextClosesWithSource(t *testing.T) {
	source := &fakeSource{updates: make(chan dragonrealms.Update)}
	close(source.updates)
	if _, ok := newClient(source, t.TempDir()).Next(); ok {
		t.Fatal("Next returned ok after source close")
	}
}

func TestClientFeedsMapperSuccessfulCommandsOnly(t *testing.T) {
	source := &fakeSource{updates: make(chan dragonrealms.Update, 3)}
	client := newClient(source, t.TempDir())
	source.updates <- dragonrealms.Update{Snapshot: dragonrealms.Snapshot{Room: dragonrealms.Room{Title: "[One]", Exits: []string{"north"}}}}
	if _, ok := client.Next(); !ok {
		t.Fatal("first update closed")
	}
	if err := client.Send("north"); err != nil {
		t.Fatal(err)
	}
	source.updates <- dragonrealms.Update{Snapshot: dragonrealms.Snapshot{Room: dragonrealms.Room{Title: "[Two]", Exits: []string{"south"}}}}
	got, ok := client.Next()
	if !ok {
		t.Fatal("second update closed")
	}
	if !strings.Contains(got.Map, "│") {
		t.Fatalf("successful movement did not add map edge:\n%s", got.Map)
	}

	source.err = errors.New("send failed")
	if err := client.Send("south"); err == nil {
		t.Fatal("send unexpectedly succeeded")
	}
	source.updates <- dragonrealms.Update{Snapshot: dragonrealms.Snapshot{Room: dragonrealms.Room{Title: "[Three]", Exits: []string{"north"}}}}
	got, ok = client.Next()
	if !ok {
		t.Fatal("third update closed")
	}
	if strings.Contains(got.Map, "#3") && strings.Contains(got.Map, "south") {
		t.Fatalf("failed movement affected map:\n%s", got.Map)
	}
}

type fakeSource struct {
	updates chan dragonrealms.Update
	sent    []string
	err     error
}

func (s *fakeSource) Updates() <-chan dragonrealms.Update { return s.updates }

func (s *fakeSource) Send(command string) error {
	s.sent = append(s.sent, command)
	return s.err
}
