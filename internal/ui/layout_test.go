package ui

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestLayoutRendersPreparedPaneViewsAndFocusMarkers(t *testing.T) {
	useANSI256(t)
	layout := newLayout()
	panes := paneViews{
		main:     paneView{title: "Game", body: "first main\nlatest main", active: true},
		room:     paneView{title: "Room", body: "[Town Green]\nA broad green."},
		hands:    paneView{title: "Hands", body: "Right: sword\nLeft: shield"},
		familiar: paneView{title: "Familiar", body: "familiar line", visible: true, unread: true},
	}
	view := layout.render(100, 30, panes)
	for _, value := range []string{"> Game", "first main", "latest main", "[Town Green]", "Right: sword", "* Familiar", "familiar line"} {
		if !strings.Contains(view, value) {
			t.Fatalf("render omitted %q: %q", value, view)
		}
	}
	if !strings.Contains(view, "\x1b[38;5;170m╭") || !strings.Contains(view, "\x1b[38;5;62m╭") {
		t.Fatalf("focus border colors missing: %q", view)
	}
}

func TestLayoutOmitsInvisibleFamiliarAndClipsHeight(t *testing.T) {
	view := newLayout().render(32, 10, paneViews{
		main:  paneView{title: "Game", body: "one\ntwo\nthree\nfour\nfive", active: true},
		room:  paneView{title: "Room", body: strings.Repeat("room\n", 20)},
		hands: paneView{title: "Hands", body: "Right: sword"},
	})
	if height := lipgloss.Height(view); height > 10 {
		t.Fatalf("render height = %d, want at most 10", height)
	}
	if strings.Contains(view, "Familiar") {
		t.Fatalf("invisible familiar rendered: %q", view)
	}
}

func TestLayoutRenderDoesNotMutateInputs(t *testing.T) {
	layout := newLayout()
	panes := paneViews{
		main:     paneView{title: "Game", body: "main", active: true},
		room:     paneView{title: "Room", body: "room"},
		hands:    paneView{title: "Hands", body: "hands"},
		familiar: paneView{title: "Familiar", body: "familiar", visible: true},
	}
	wantLayout := layout
	wantPanes := panes
	layout.render(100, 30, panes)
	layout.render(44, 12, panes)
	if !reflect.DeepEqual(layout, wantLayout) {
		t.Fatalf("layout mutated: got %#v want %#v", layout, wantLayout)
	}
	if !reflect.DeepEqual(panes, wantPanes) {
		t.Fatalf("panes mutated: got %#v want %#v", panes, wantPanes)
	}
}
