package ui

import (
	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/colorprofile"
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

func TestDashboardGeometryDegradesDecorationBeforeSemanticRows(t *testing.T) {
	currentTheme := theme{Foreground: "7", Padding: 2}
	for _, test := range []struct {
		name                                 string
		width, height                        int
		wantRows, wantMapHeight, wantPadding int
		wantMap, wantChrome                  bool
	}{
		{name: "full", width: 100, height: 30, wantRows: 13, wantMapHeight: 8, wantPadding: 2, wantMap: true, wantChrome: true},
		{name: "compact", width: 100, height: 20, wantRows: 8, wantMapHeight: 6, wantPadding: 0, wantMap: true},
		{name: "height 19", width: 100, height: 19, wantRows: 7, wantMapHeight: 5, wantPadding: 0, wantMap: true},
		{name: "reduced padding", width: 17, height: 30, wantRows: 13, wantMapHeight: 8, wantPadding: 1, wantMap: true, wantChrome: true},
		{name: "zero padding", width: 14, height: 30, wantRows: 13, wantMapHeight: 8, wantPadding: 0, wantMap: true, wantChrome: true},
		{name: "frame removed", width: 13, height: 30, wantRows: 10, wantMapHeight: 8, wantPadding: 0, wantMap: true},
		{name: "short", width: 60, height: 15, wantRows: 3, wantPadding: 0},
		{name: "five cells", width: 5, height: 24, wantRows: 10, wantMapHeight: 8, wantPadding: 0, wantMap: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := calculateDashboardGeometry(test.width, test.height, true, currentTheme, "Command > ")
			if got.rows != test.wantRows || got.mapHeight != test.wantMapHeight || got.padding != test.wantPadding || got.mapVisible != test.wantMap || got.chrome != test.wantChrome {
				t.Fatalf("geometry=%+v", got)
			}
		})
	}
}

func TestDashboardBorderTypes(t *testing.T) {
	for _, test := range []struct {
		kind, corners string
	}{
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

func TestFramedDashboardUsesCellWidthsDividerAndTextModes(t *testing.T) {
	profile := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.NoTTY
	t.Cleanup(func() { lipgloss.Writer.Profile = profile })
	update := presentation.Update{
		Location: presentation.Location{Title: "界界界🙂", Exits: []string{"北", "南"}},
		Hands:    presentation.Hands{Left: "盾🙂", Right: "剣"},
	}
	view := renderDashboard(60, 30, update, "準備🙂", "Command > draft🙂", []string{"界─@", " │", " 界"}, 0, 0, true, theme{Foreground: "7", Border: "62", TitleBar: "170", StatusBar: "252", StatusBarBg: "235", Padding: 1})
	rows := strings.Split(view, "\n")
	if len(rows) != 13 {
		t.Fatalf("rows=%d want 13\n%s", len(rows), view)
	}
	for _, row := range rows {
		if got := ansi.StringWidth(row); got != 60 {
			t.Fatalf("row width=%d want 60: %q", got, row)
		}
	}
	mapParts := strings.Split(rows[1], "│")
	if len(mapParts) != 4 || ansi.StringWidth(mapParts[1]) != 37 || ansi.StringWidth(mapParts[2]) != 20 {
		t.Fatalf("divider positions changed: %q", rows[1])
	}
	if !strings.Contains(view, "Map navigation") || !strings.Contains(view, "Command") {
		t.Fatalf("text mode marker missing: %q", view)
	}
	if divider := ansi.Strip(rows[len(rows)-4]); !strings.Contains(divider, dashboardBorder("rounded").MiddleBottom) {
		t.Fatalf("map divider does not end above status rows: %q", divider)
	}
	whisper := renderDashboard(60, 30, update, "準備🙂", "Whisper > draft🙂", []string{"@"}, 0, 0, false, theme{Foreground: "7"})
	if !strings.Contains(ansi.Strip(whisper), "Whisper >") {
		t.Fatalf("whisper marker missing: %q", whisper)
	}
}

func TestFramedDashboardPreservesHorizontalPadding(t *testing.T) {
	currentTheme := theme{Foreground: "7", Border: "7", TitleBar: "7", StatusBar: "7", Padding: 2}
	long := strings.Repeat("x", 40)
	update := presentation.Update{Hands: presentation.Hands{Left: long, Right: long}}

	mapRows := strings.Split(ansi.Strip(renderDashboard(30, 30, update, long, long, []string{"@"}, 0, 0, false, currentTheme)), "\n")
	for _, index := range []int{1, len(mapRows) - 3, len(mapRows) - 2} {
		if !strings.HasPrefix(mapRows[index], "│  ") || !strings.HasSuffix(mapRows[index], "  │") || ansi.StringWidth(mapRows[index]) != 30 {
			t.Fatalf("map row did not preserve padding: %q", mapRows[index])
		}
	}

	contentRows := strings.Split(ansi.Strip(renderDashboard(30, 30, update, long, long, nil, 0, 0, false, currentTheme)), "\n")
	if !strings.HasPrefix(contentRows[1], "│  ") || !strings.HasSuffix(contentRows[1], "  │") || ansi.StringWidth(contentRows[1]) != 30 {
		t.Fatalf("content row did not preserve padding: %q", contentRows[1])
	}
}

func TestFramedDashboardAppliesThemeColors(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", Border: "39", TitleBar: "214", StatusBar: "16", StatusBarBg: "220", BorderType: "double", Padding: 2}
	view := renderDashboard(60, 30,
		presentation.Update{Location: presentation.Location{Title: "Hall"}},
		"READY", "Command > ", []string{"@"}, 0, 0, false, currentTheme,
	)
	rows := strings.Split(view, "\n")
	for _, sequence := range []string{"38;5;39", "38;5;214", "38;5;231"} {
		if !strings.Contains(view, sequence) {
			t.Fatalf("missing ANSI sequence %q in %q", sequence, view)
		}
	}
	for _, index := range []int{len(rows) - 3, len(rows) - 2} {
		if !strings.Contains(rows[index], "38;5;16") || !strings.Contains(rows[index], "48;5;220") {
			t.Fatalf("strip row missing foreground/background: %q", rows[index])
		}
	}
	if plain := ansi.Strip(view); !strings.Contains(plain, "╔") || !strings.Contains(plain, "Hall") {
		t.Fatalf("double border/title missing: %q", plain)
	}
}

func TestMapNavigationColorsMapSideTopBorder(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", Border: "39", TitleBar: "214", StatusBar: "16", BorderType: "double", Padding: 2}
	geometry := calculateDashboardGeometry(60, 30, true, currentTheme, "")
	border := dashboardBorder(currentTheme.BorderType)
	title := strings.Repeat("Hall ", 20)
	got := strings.Split(renderDashboard(60, 30, presentation.Update{Location: presentation.Location{Title: title}}, "READY", "Command >", []string{"@"}, 0, 0, true, currentTheme), "\n")[0]
	want := paint(border.TopLeft, 1, currentTheme.TitleBar, "") +
		ruleSegment(border.Top, geometry.padding+geometry.mapWidth, title, currentTheme.TitleBar, currentTheme.TitleBar) +
		paint(border.MiddleTop, 1, currentTheme.TitleBar, "") +
		ruleSegment(border.Top, geometry.equipmentWidth+geometry.padding, "", currentTheme.Border, currentTheme.TitleBar) +
		paint(border.TopRight, 1, currentTheme.Border, "")
	if got != want || ansi.StringWidth(got) != 60 || !strings.Contains(ansi.Strip(got), border.MiddleTop) || !strings.Contains(ansi.Strip(got), "Hall") {
		t.Fatalf("focused top rule=%q want %q", got, want)
	}
}

func TestDashboardMapUsesCombinedPhysicalRowsAndFullWidthStatus(t *testing.T) {
	update := presentation.Update{Location: presentation.Location{Title: "location"}, Hands: presentation.Hands{Left: "left", Right: strings.Repeat("right ", 12)}}
	rows, visible := dashboardRows(60, 19, update, "READY | LOG off", ">", []string{"@"}, 0, 0, false, theme{Foreground: "7"})
	if !visible || len(rows) != 7 {
		t.Fatalf("map visible=%v rows=%d, want 7", visible, len(rows))
	}
	if !contains(rows[0], "location") || !contains(rows[1], "L: left") || !contains(rows[2], "R: right") {
		t.Fatalf("map side rows=%q", rows[:5])
	}
	if strings.TrimSpace(ansi.Strip(rows[5])) != "READY | LOG off" || strings.TrimSpace(ansi.Strip(rows[6])) != ">" {
		t.Fatalf("status/input rows=%q", rows[5:])
	}
	for _, row := range rows {
		if ansi.StringWidth(row) > 60 {
			t.Fatalf("wrapped dashboard row=%q width=%d", row, ansi.StringWidth(row))
		}
	}
}

func TestCompactMapRowsApplyThemeForeground(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", StatusBar: "16", StatusBarBg: "220"}
	update := presentation.Update{Location: presentation.Location{Title: "location"}}
	for _, size := range []struct{ width, height int }{{width: 100, height: 19}, {width: 10, height: 24}} {
		rows, visible := dashboardRows(size.width, size.height, update, "READY", "Command >", []string{"@"}, 0, 0, false, currentTheme)
		if !visible || !strings.Contains(rows[0], "38;5;231") {
			t.Fatalf("compact map %dx%d omitted foreground: %q", size.width, size.height, rows)
		}
	}
}

func TestCompactRowsStyleStatusByRole(t *testing.T) {
	useANSI256(t)
	currentTheme := theme{Foreground: "231", StatusBar: "16", StatusBarBg: "220"}
	rows, _ := dashboardRows(13, 16, presentation.Update{Location: presentation.Location{Title: "same"}}, "same", "Command >", nil, 0, 0, false, currentTheme)
	if !strings.Contains(rows[0], "38;5;231") || strings.Contains(rows[0], "48;5;220") {
		t.Fatalf("location received status style: %q", rows[0])
	}
	if !strings.Contains(rows[2], "38;5;16") || !strings.Contains(rows[2], "48;5;220") {
		t.Fatalf("status omitted status style: %q", rows[2])
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
