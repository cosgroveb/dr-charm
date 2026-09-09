package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func TestDashboardGeometryUsesSharedColumnsAndFallbacks(t *testing.T) {
	currentTheme := theme{Foreground: "7", Padding: 1}
	for _, test := range []struct {
		name                       string
		width, height              int
		rows, body, padding        int
		input, equipment, mapWidth int
		mapVisible, chrome         bool
	}{
		{name: "168x24", width: 168, height: 24, rows: 11, body: 8, padding: 1, input: 82, equipment: 40, mapWidth: 40, mapVisible: true, chrome: true},
		{name: "100x30", width: 100, height: 30, rows: 11, body: 8, padding: 1, input: 48, equipment: 23, mapWidth: 23, mapVisible: true, chrome: true},
		{name: "100x19", width: 100, height: 19, rows: 7, body: 5, input: 50, equipment: 24, mapWidth: 24, mapVisible: true},
		{name: "60x15", width: 60, height: 15, rows: 3, input: 60},
		{name: "10x24", width: 10, height: 24, rows: 4, input: 10},
		{name: "height 20", width: 100, height: 20, rows: 8, body: 5, padding: 1, input: 48, equipment: 23, mapWidth: 23, mapVisible: true, chrome: true},
		{name: "height 19", width: 100, height: 19, rows: 7, body: 5, input: 50, equipment: 24, mapWidth: 24, mapVisible: true},
		{name: "height 18", width: 100, height: 18, rows: 6, padding: 1, input: 96, chrome: true},
		{name: "width 52", width: 52, height: 24, rows: 11, body: 8, input: 24, equipment: 12, mapWidth: 12, mapVisible: true, chrome: true},
		{name: "width 51", width: 51, height: 24, rows: 10, body: 8, input: 25, equipment: 12, mapWidth: 12, mapVisible: true},
		{name: "width 50", width: 50, height: 24, rows: 10, body: 8, input: 24, equipment: 12, mapWidth: 12, mapVisible: true},
		{name: "width 49", width: 49, height: 24, rows: 6, padding: 1, input: 45, chrome: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := calculateDashboardGeometry(test.width, test.height, true, currentTheme, "Command > ")
			if got.rows != test.rows || got.bodyHeight != test.body || got.padding != test.padding || got.inputInterior != test.input || got.equipmentWidth != test.equipment || got.mapWidth != test.mapWidth || got.mapVisible != test.mapVisible || got.chrome != test.chrome {
				t.Fatalf("geometry=%+v", got)
			}
		})
	}
}

func TestDashboardGeometryReducesThemePaddingBeforeChrome(t *testing.T) {
	currentTheme := theme{Foreground: "7", Padding: 2}
	for _, test := range []struct{ width, padding int }{{56, 2}, {54, 1}, {52, 0}} {
		got := calculateDashboardGeometry(test.width, 24, true, currentTheme, "")
		if !got.chrome || !got.mapVisible || got.padding != test.padding {
			t.Fatalf("width %d geometry=%+v", test.width, got)
		}
	}
}

func TestDashboardReservesTranscriptBeforeShowingMap(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "界界界"}}
	mapLines := []string{"@ map"}
	if below := renderDashboard(100, 18, update, "status", ">", mapLines, 0, 0, false, theme{Foreground: "7"}); strings.Contains(below, "@ map") {
		t.Fatalf("map shown below threshold: %q", below)
	}
	if at := renderDashboard(100, 19, update, "status", ">", mapLines, 0, 0, false, theme{Foreground: "7"}); !strings.Contains(at, "@ map") {
		t.Fatalf("map hidden at threshold: %q", at)
	}
}

func TestFramedDashboardUsesSharedBodyAndExactRows(t *testing.T) {
	profile := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.NoTTY
	t.Cleanup(func() { lipgloss.Writer.Profile = profile })
	update := presentation.Update{
		Location: presentation.Location{Title: "Town Square", Exits: []string{"north", "south"}},
		Hands:    presentation.Hands{Left: "shield", Right: "sword", PreparedSpell: "Fire"},
	}
	view := renderDashboard(168, 24, update, "READY | LOG on", "Command > draft", []string{"o─@", "  │", "  o"}, 0, 0, false, theme{Foreground: "7", Border: "62", TitleBar: "170", StatusBar: "252", StatusBarBg: "235", Padding: 1})
	rows := strings.Split(ansi.Strip(view), "\n")
	if len(rows) != 11 {
		t.Fatalf("rows=%d want 11\n%s", len(rows), ansi.Strip(view))
	}
	for _, row := range rows {
		if got := ansi.StringWidth(row); got != 168 {
			t.Fatalf("row width=%d want 168: %q", got, row)
		}
	}
	if !strings.Contains(rows[0], "Town Square  Exits: north, south") || !strings.Contains(rows[1], "READY | LOG on") {
		t.Fatalf("top rows=%q", rows[:2])
	}
	parts := strings.Split(rows[2], "│")
	if len(parts) != 5 || ansi.StringWidth(parts[1]) != 83 || ansi.StringWidth(parts[2]) != 40 || ansi.StringWidth(parts[3]) != 41 {
		t.Fatalf("body divider positions=%q", rows[2])
	}
	for _, value := range []string{"Command > draft", "L: shield", "R: sword", "Sp: Fire", "o─@"} {
		if !strings.Contains(view, value) {
			t.Fatalf("shared body omitted %q: %q", value, ansi.Strip(view))
		}
	}
	if strings.Contains(strings.Join(rows[3:10], "\n"), "Command >") {
		t.Fatalf("input repeated below first body row: %q", rows[3:10])
	}
}

func TestCompactDashboardUsesSharedColumnsAtWidthBoundary(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "location"}, Hands: presentation.Hands{Left: "left", Right: "right"}}
	rows, visible := dashboardRows(50, 24, update, "READY", "Command >", []string{"@"}, 0, 0, false, theme{Foreground: "7"})
	if !visible || len(rows) != 10 {
		t.Fatalf("map visible=%v rows=%d", visible, len(rows))
	}
	plain := make([]string, len(rows))
	for index, row := range rows {
		plain[index] = ansi.Strip(row)
		if got := ansi.StringWidth(row); got != 50 {
			t.Fatalf("row width=%d: %q", got, row)
		}
	}
	if strings.TrimSpace(plain[0]) != "location" || strings.TrimSpace(plain[1]) != "READY" {
		t.Fatalf("location/status=%q", plain[:2])
	}
	if got := plain[2]; !strings.HasPrefix(got, "Command >") || !strings.Contains(got[25:38], "L: left") || !strings.Contains(got[38:], "@") {
		t.Fatalf("compact shared body=%q", got)
	}
}

func TestDashboardHidesMapWhenCompactColumnsDoNotFit(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "location"}, Hands: presentation.Hands{Left: "left", Right: "right"}}
	rows, visible := dashboardRows(49, 24, update, "status", ">", []string{"map"}, 0, 0, false, theme{Foreground: "7"})
	if visible || len(rows) != 6 {
		t.Fatalf("map visible=%v rows=%d", visible, len(rows))
	}
	view := ansi.Strip(strings.Join(rows, "\n"))
	for _, value := range []string{"location", "L: left  R: right", "status", ">"} {
		if !strings.Contains(view, value) {
			t.Fatalf("fallback omitted %q: %q", value, view)
		}
	}
	if strings.Contains(view, "map") {
		t.Fatalf("fallback retained map: %q", view)
	}
}

func TestDashboardHidesRowsInPriorityOrder(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "location"}, Hands: presentation.Hands{Left: "left", Right: "right"}}
	for _, test := range []struct {
		height int
		want   []string
		omit   []string
	}{
		{16, []string{"location", "L: left  R: right", "status", ">"}, nil},
		{15, []string{"location", "status", ">"}, []string{"L: left"}},
		{14, []string{"status", ">"}, []string{"location", "L: left"}},
		{13, []string{">"}, []string{"location", "status", "L: left"}},
	} {
		view := renderDashboard(49, test.height, update, "status", ">", []string{"map"}, 0, 0, false, theme{Foreground: "7"})
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

func TestDashboardWithoutMapRetainsSixRowFrame(t *testing.T) {
	view := renderDashboard(60, 24, presentation.Update{Location: presentation.Location{Title: "location"}}, "status", ">", nil, 0, 0, false, theme{Foreground: "7", Padding: 1})
	rows := strings.Split(ansi.Strip(view), "\n")
	if len(rows) != 6 || !strings.HasPrefix(rows[0], "╭") || !strings.Contains(rows[0], "location") || !strings.Contains(rows[3], "status") || !strings.Contains(rows[4], ">") {
		t.Fatalf("no-map frame=%q", rows)
	}
}

func TestDashboardBorderTypes(t *testing.T) {
	for _, test := range []struct{ kind, corners string }{
		{kind: "rounded", corners: "╭╮╰╯"},
		{kind: "normal", corners: "┌┐└┘"},
		{kind: "thick", corners: "┏┓┗┛"},
		{kind: "double", corners: "╔╗╚╝"},
		{kind: "unknown", corners: "╭╮╰╯"},
	} {
		border := dashboardBorder(test.kind)
		if got := border.TopLeft + border.TopRight + border.BottomLeft + border.BottomRight; got != test.corners {
			t.Fatalf("border %q corners=%q want %q", test.kind, got, test.corners)
		}
	}
}

func TestFramedDashboardPreservesHorizontalPadding(t *testing.T) {
	currentTheme := theme{Foreground: "7", Border: "7", TitleBar: "7", StatusBar: "7", Padding: 2}
	rows := strings.Split(ansi.Strip(renderDashboard(100, 30, presentation.Update{}, "status", "input", []string{"@"}, 0, 0, false, currentTheme)), "\n")
	parts := strings.Split(rows[2], "│")
	if len(parts) != 5 || !strings.HasPrefix(parts[1], "  ") || !strings.HasSuffix(parts[3], "  ") {
		t.Fatalf("outer padding not preserved: %q", rows[2])
	}
}

func TestFramedDashboardAppliesThemeColors(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", Border: "39", TitleBar: "214", StatusBar: "16", StatusBarBg: "220", BorderType: "double", Padding: 2}
	view := renderDashboard(100, 30, presentation.Update{Location: presentation.Location{Title: "Hall"}}, "READY", "Command > ", []string{"@"}, 0, 0, false, currentTheme)
	for _, sequence := range []string{"38;5;39", "38;5;214", "38;5;231", "38;5;16", "48;5;220"} {
		if !strings.Contains(view, sequence) {
			t.Fatalf("missing ANSI sequence %q", sequence)
		}
	}
	if plain := ansi.Strip(view); !strings.Contains(plain, "╔") || !strings.Contains(plain, "Hall") {
		t.Fatalf("double border/title missing: %q", plain)
	}
}

func TestMapNavigationPrefixesStatusAndColorsMapBoundary(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", Border: "39", TitleBar: "214", StatusBar: "16", StatusBarBg: "220", BorderType: "double", Padding: 1}
	status := "READY | LOG on | Health:100 | Mana:100 | Fatigue:0 | Concentration:100 | Spirit:100 | Posture:standing | MAP ready"
	rows := strings.Split(renderDashboard(100, 30, presentation.Update{Location: presentation.Location{Title: "Hall"}}, status, "Command >", []string{"@"}, 0, 0, true, currentTheme), "\n")
	if !strings.Contains(ansi.Strip(rows[1]), "Map navigation | READY") {
		t.Fatalf("navigation cue was truncated: %q", ansi.Strip(rows[1]))
	}
	geometry := calculateDashboardGeometry(100, 30, true, currentTheme, "")
	border := dashboardBorder(currentTheme.BorderType)
	wantTop := mapRule(border.TopLeft, border.Top, border.MiddleTop, border.TopRight, geometry, "Hall", true, currentTheme)
	if rows[0] != wantTop || !strings.Contains(rows[2], "38;5;214") {
		t.Fatalf("map boundary was not highlighted: %q", rows[2])
	}
}

func TestCompactRowsApplyThemeByRole(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", StatusBar: "16", StatusBarBg: "220"}
	rows, visible := dashboardRows(50, 24, presentation.Update{Location: presentation.Location{Title: "location"}}, "READY", "Command >", []string{"@"}, 0, 0, false, currentTheme)
	if !visible || !strings.Contains(rows[0], "38;5;231") || strings.Contains(rows[0], "48;5;220") {
		t.Fatalf("location style=%q", rows[0])
	}
	if !strings.Contains(rows[1], "38;5;16") || !strings.Contains(rows[1], "48;5;220") || !strings.Contains(rows[2], "48;5;220") {
		t.Fatalf("status/input style=%q", rows[1:3])
	}
}

func TestCropMapClampsUnevenLines(t *testing.T) {
	got := cropMap([]string{"x", "long line", ""}, 99, 99, 5, 4)
	if len(got) == 0 {
		t.Fatal("empty crop")
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
