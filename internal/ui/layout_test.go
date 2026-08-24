package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"dr-charm/internal/dragonrealms"
	"github.com/charmbracelet/lipgloss"
)

func TestLayoutRendersTypedSnapshotHistoriesAndStyles(t *testing.T) {
	useANSI256(t)
	layout := newLayout()
	snapshot := completeLayoutSnapshot()
	view := layout.render(100, 80, snapshot, []string{"first main", "latest main"}, []string{"a raven speaks"}, "main")

	ordered := []string{
		"[Town Green]", "A broad green.", "Exits: north, east", "You also see:", "  a bench",
		"Also here:", "  Cennedig", "Creatures:", "  a rat", "Right:", "a sword", "Left:", "a shield",
		"Spell: Fire Ball", "first main", "latest main", "a raven speaks",
	}
	for _, value := range ordered {
		if !strings.Contains(view, value) {
			t.Fatalf("render omitted %q: %q", value, view)
		}
	}
	for _, styled := range []string{
		"\x1b[1;38;5;170mGame\x1b[0m",
		"\x1b[38;5;170m╭",
		"\x1b[38;5;62m╭",
		"\x1b[38;5;46mExits: north, east\x1b[0m",
		"\x1b[38;5;226mYou also see:\x1b[0m",
		"\x1b[1;38;5;170mRight:\x1b[0m  a sword",
	} {
		if !strings.Contains(view, styled) {
			t.Fatalf("render omitted style sequence %q: %q", styled, view)
		}
	}
}

func TestLayoutClipsCompleteContentAtSmallDimensions(t *testing.T) {
	useANSI256(t)
	view := newLayout().render(32, 10, completeLayoutSnapshot(), []string{"one", "two", "three", "four", "five"}, []string{"familiar one", "familiar two"}, "room")
	if height := lipgloss.Height(view); height > 10 {
		t.Fatalf("render height = %d, want at most 10", height)
	}
	if strings.Contains(view, "a bench") || strings.Contains(view, "Cennedig") || strings.Contains(view, "a rat") {
		t.Fatalf("small render did not clip complete room content: %q", view)
	}
}

func TestLayoutRenderDoesNotMutateInputs(t *testing.T) {
	useANSI256(t)
	layout := newLayout()
	snapshot := completeLayoutSnapshot()
	mainOutput := []string{"first", "second", "third"}
	familiarOutput := []string{"familiar first", "familiar second"}
	wantLayout := layout
	wantSnapshot := cloneLayoutSnapshot(snapshot)
	wantMain := append([]string(nil), mainOutput...)
	wantFamiliar := append([]string(nil), familiarOutput...)

	layout.render(100, 30, snapshot, mainOutput, familiarOutput, "main")
	layout.render(44, 12, snapshot, mainOutput, familiarOutput, "familiar")
	if !reflect.DeepEqual(layout, wantLayout) {
		t.Fatalf("layout mutated: got %#v want %#v", layout, wantLayout)
	}
	if !reflect.DeepEqual(snapshot, wantSnapshot) {
		t.Fatalf("snapshot mutated: got %#v want %#v", snapshot, wantSnapshot)
	}
	if !reflect.DeepEqual(mainOutput, wantMain) || !reflect.DeepEqual(familiarOutput, wantFamiliar) {
		t.Fatalf("histories mutated: main %#v familiar %#v", mainOutput, familiarOutput)
	}
}

func completeLayoutSnapshot() dragonrealms.Snapshot {
	stamp := time.Unix(100, 0)
	return dragonrealms.Snapshot{
		Connection: dragonrealms.ConnectionReady,
		Character:  "Hero",
		Room: dragonrealms.Room{
			ID: "42", Title: "[Town Green]", Description: " A broad green. ",
			Exits: []string{"north", "", "east"}, Objects: []string{"a bench"},
			Players: []string{"Cennedig"}, Creatures: []string{"a rat"},
			Compass: []string{"north", "east"}, Image: "7",
		},
		Vitals:        dragonrealms.Vitals{Health: 90, Mana: 80, Stamina: 70, Spirit: 60, Concentration: 50, Encumbrance: 40},
		Timers:        dragonrealms.Timers{Round: stamp, Cast: stamp.Add(time.Second), Spell: stamp.Add(2 * time.Second)},
		Hands:         dragonrealms.Hands{Right: "a sword", Left: "a shield"},
		PreparedSpell: "Fire Ball",
		Posture:       dragonrealms.PostureStanding,
		Prompt:        ">",
	}
}

func cloneLayoutSnapshot(snapshot dragonrealms.Snapshot) dragonrealms.Snapshot {
	snapshot.Room.Exits = append([]string(nil), snapshot.Room.Exits...)
	snapshot.Room.Objects = append([]string(nil), snapshot.Room.Objects...)
	snapshot.Room.Players = append([]string(nil), snapshot.Room.Players...)
	snapshot.Room.Creatures = append([]string(nil), snapshot.Room.Creatures...)
	snapshot.Room.Compass = append([]string(nil), snapshot.Room.Compass...)
	return snapshot
}
