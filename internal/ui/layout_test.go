package ui

import (
	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestDashboardReservesTranscriptBeforeShowingMap(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "界界界"}, Hands: presentation.Hands{}}
	mapLines := []string{"@ map"}
	below := renderDashboard(20, 18, update, "status", ">", mapLines, 0, 0, false, theme{Foreground: "7"})
	if strings.Contains(below, "@ map") {
		t.Fatalf("map shown below threshold: %q", below)
	}
	at := renderDashboard(20, 19, update, "status", ">", mapLines, 0, 0, false, theme{Foreground: "7"})
	if !strings.Contains(at, "@ map") {
		t.Fatalf("map hidden at threshold: %q", at)
	}
}

func TestDashboardMapUsesCombinedPhysicalRowsAndFullWidthStatus(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "location"}, Hands: presentation.Hands{Left: "left", Right: strings.Repeat("right ", 12)}}
	rows, visible := dashboardRows(60, 19, update, "READY | LOG off", ">", []string{"@"}, 0, 0, false)
	if !visible || len(rows) != 7 {
		t.Fatalf("map visible=%v rows=%d, want 7", visible, len(rows))
	}
	if !contains(rows[0], "location") || !contains(rows[1], "L: left") || !contains(rows[2], "R: right") {
		t.Fatalf("map side rows=%q", rows[:5])
	}
	if rows[5] != "READY | LOG off" || rows[6] != ">" {
		t.Fatalf("status/input rows=%q", rows[5:])
	}
	for _, row := range rows {
		if ansi.StringWidth(row) > 60 {
			t.Fatalf("wrapped dashboard row=%q width=%d", row, ansi.StringWidth(row))
		}
	}
}

func TestCropMapClampsUnevenLines(t *testing.T) {
	got := cropMap([]string{"x", "long line", ""}, 99, 99, 5, 4)
	if len(got) == 0 {
		t.Fatal("empty crop")
	}
}

func TestDashboardHidesRowsInPriorityOrder(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "location"}, Hands: presentation.Hands{Left: "left", Right: "right"}}
	mapLines := []string{"map", "map", "map", "map", "map"}
	for _, test := range []struct {
		height int
		want   []string
		omit   []string
	}{
		{21, []string{"location", "L: left", "map", "status", ">"}, nil},
		{16, []string{"location", "L: left  R: right", "status", ">"}, []string{"map"}},
		{15, []string{"location", "L: left  R: right", ">"}, []string{"map", "status"}},
		{14, []string{"location", ">"}, []string{"map", "status", "L: left"}},
		{13, []string{">"}, []string{"map", "status", "L: left", "location"}},
	} {
		view := renderDashboard(60, test.height, update, "status", ">", mapLines, 0, 0, false, theme{Foreground: "7"})
		for _, value := range test.want {
			if !strings.Contains(view, value) {
				t.Fatalf("height %d omitted %q: %q", test.height, value, view)
			}
		}
		for _, value := range test.omit {
			if strings.Contains(view, value) {
				t.Fatalf("height %d retained %q: %q", test.height, value, view)
			}
		}
	}
}

func TestTerminalCellTruncationAndMapCrop(t *testing.T) {
	if got := truncate("界界界", 5); got != "界界…" || ansi.StringWidth(got) != 5 {
		t.Fatalf("wide truncation=%q width=%d", got, ansi.StringWidth(got))
	}
	if got := truncate("🙂🙂🙂", 5); got != "🙂🙂…" || ansi.StringWidth(got) != 5 {
		t.Fatalf("emoji truncation=%q width=%d", got, ansi.StringWidth(got))
	}
	lines := []string{"界界界界界", "界界@界界", "界界界界界"}
	got := cropMap(lines, 1, 6, 3, 4)
	if want := []string{"界界", "@界", "界界"}; !equalStrings(got, want) {
		t.Fatalf("crop=%q want %q", got, want)
	}
	for _, row := range got {
		if ansi.StringWidth(row) > 4 {
			t.Fatalf("crop wrapped: %q width=%d", row, ansi.StringWidth(row))
		}
	}
	view := renderDashboard(5, 13, presentation.Update{Location: presentation.Location{Title: "界界界"}}, "🙂🙂🙂", "🙂🙂🙂", nil, 0, 0, false, theme{Foreground: "7"})
	for _, row := range strings.Split(view, "\n") {
		if lipgloss.Width(row) != 5 {
			t.Fatalf("row width=%d: %q", lipgloss.Width(row), row)
		}
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
