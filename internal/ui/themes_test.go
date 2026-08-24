package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dr-charm/internal/dragonrealms"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestThemeCatalogBuiltins(t *testing.T) {
	useANSI256(t)

	catalog := newThemeCatalog("")
	wantNames := []string{"default", "dark", "high-contrast"}
	if got := catalog.names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("names = %#v, want %#v", got, wantNames)
	}
	for index, tt := range []struct {
		theme      theme
		wantBorder lipgloss.Border
	}{
		{theme: theme{Name: "default", Foreground: "7", Border: "62", TitleBar: "170", StatusBar: "240", StatusBarBg: "235", BorderType: "rounded", Padding: 1}, wantBorder: lipgloss.RoundedBorder()},
		{theme: theme{Name: "dark", Foreground: "252", Border: "237", TitleBar: "33", StatusBar: "252", StatusBarBg: "237", BorderType: "rounded", Padding: 1}, wantBorder: lipgloss.RoundedBorder()},
		{theme: theme{Name: "high-contrast", Foreground: "15", Border: "15", TitleBar: "226", StatusBar: "0", StatusBarBg: "15", BorderType: "thick", Padding: 1}, wantBorder: lipgloss.ThickBorder()},
	} {
		if index > 0 {
			catalog.next()
		}
		if got := catalog.current(); got != tt.theme {
			t.Fatalf("theme %d = %#v, want %#v", index, got, tt.theme)
		}
		style := catalog.borderStyle()
		if got := style.GetBorderStyle(); !reflect.DeepEqual(got, tt.wantBorder) {
			t.Fatalf("%s border = %#v, want %#v", tt.theme.Name, got, tt.wantBorder)
		}
		if got := style.GetBorderTopForeground(); got != lipgloss.Color(tt.theme.Border) {
			t.Fatalf("%s border color = %#v, want %q", tt.theme.Name, got, tt.theme.Border)
		}
		if got := style.GetPaddingTop(); got != tt.theme.Padding {
			t.Fatalf("%s padding = %d, want %d", tt.theme.Name, got, tt.theme.Padding)
		}
	}
}

func TestThemeCatalogRejectsInvalidCustomThemes(t *testing.T) {
	directory := t.TempDir()
	writeThemeFile(t, directory, "legacy.json", `{"name":"legacy","colors":{"foreground":"99"},"styles":{"padding":2}}`)
	writeThemeFile(t, directory, "trailing.json", `{"name":"trailing","foreground":"1"} {"name":"extra"}`)
	writeThemeFile(t, directory, "whitespace.json", `{"name":"   "}`)

	want := []string{"default", "dark", "high-contrast"}
	if got := newThemeCatalog(directory).names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %#v, want %#v", got, want)
	}
}

func TestThemeCatalogLoadsFlatCustomTheme(t *testing.T) {
	useANSI256(t)
	directory := t.TempDir()
	writeThemeFile(t, directory, "flat.json", completeCustomThemeJSON("flat"))

	catalog := newThemeCatalog(directory)
	for range 3 {
		catalog.next()
	}
	want := theme{Name: "flat", Foreground: "101", Border: "102", TitleBar: "103", StatusBar: "104", StatusBarBg: "105", BorderType: "double", Padding: 2}
	if got := catalog.current(); got != want {
		t.Fatalf("custom theme = %#v, want %#v", got, want)
	}
}

func TestThemeCatalogOrderingReplacementAndNavigation(t *testing.T) {
	directory := t.TempDir()
	writeThemeFile(t, directory, "10-zulu.json", `{"name":"zulu","foreground":"10"}`)
	writeThemeFile(t, directory, "20-alpha.json", `{"name":"alpha","foreground":"20"}`)
	writeThemeFile(t, directory, "30-repeat.json", `{"name":"repeat","foreground":"30"}`)
	writeThemeFile(t, directory, "40-default.json", `{"name":"default","foreground":"40"}`)
	writeThemeFile(t, directory, "50-repeat.json", `{"name":"repeat","foreground":"50"}`)

	catalog := newThemeCatalog(directory)
	wantNames := []string{"default", "dark", "high-contrast", "zulu", "alpha", "repeat"}
	if got := catalog.names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("names = %#v, want %#v", got, wantNames)
	}
	if got := catalog.current(); got.Name != "default" || got.Foreground != "40" {
		t.Fatalf("built-in override = %#v", got)
	}
	catalog.previous()
	if got := catalog.current().Name; got != "default" {
		t.Fatalf("previous crossed lower boundary to %q", got)
	}
	for _, want := range wantNames[1:] {
		catalog.next()
		if got := catalog.current().Name; got != want {
			t.Fatalf("next = %q, want %q", got, want)
		}
	}
	if got := catalog.current(); got.Foreground != "50" {
		t.Fatalf("later duplicate did not win in original slot: %#v", got)
	}
	catalog.next()
	if got := catalog.current().Name; got != "repeat" {
		t.Fatalf("next crossed upper boundary to %q", got)
	}
	catalog.previous()
	if got := catalog.current().Name; got != "alpha" {
		t.Fatalf("previous = %q, want alpha", got)
	}
}

func TestThemeSelectorUsesCatalogOrderAndNavigation(t *testing.T) {
	useANSI256(t)
	directory := t.TempDir()
	writeThemeFile(t, directory, "10-zulu.json", `{"name":"zulu"}`)
	writeThemeFile(t, directory, "20-alpha.json", `{"name":"alpha"}`)
	model := EnhancedModel{themes: newThemeCatalog(directory), width: 80, height: 24}

	view := model.renderThemeSelector()
	positions := make([]int, 0, 5)
	for _, name := range []string{"default", "dark", "high-contrast", "zulu", "alpha"} {
		positions = append(positions, strings.Index(view, name))
	}
	for index := 1; index < len(positions); index++ {
		if positions[index] <= positions[index-1] {
			t.Fatalf("selector order positions = %#v", positions)
		}
	}
	model = model.handleThemeKeys(keyDown())
	if got := model.themes.current().Name; got != "dark" {
		t.Fatalf("selected theme = %q, want dark", got)
	}
	if view := model.renderThemeSelector(); !strings.Contains(view, "> dark") {
		t.Fatalf("selected marker missing from %q", view)
	}
}

func TestSelectedCustomThemeRendersEveryProperty(t *testing.T) {
	useANSI256(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	themesDirectory := filepath.Join(home, ".dr-charm", "themes")
	if err := os.MkdirAll(themesDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeThemeFile(t, themesDirectory, "custom.json", completeCustomThemeJSON("custom"))
	model := InitialEnhancedModel(&fakeSession{updates: make(chan dragonrealms.Update)}, "Hero")
	for range 3 {
		model = model.handleThemeKeys(keyDown())
	}
	model.width = 60
	model.height = 20

	help := model.renderHelp()
	selector := model.renderThemeSelector()
	single := model.renderSinglePane()
	multi := model.renderMultiPane()
	border := model.themes.borderStyle().Render("x")
	for _, rendered := range []string{help, selector} {
		if !strings.Contains(rendered, "38;5;101") {
			t.Fatalf("foreground missing from %q", rendered)
		}
	}
	for _, rendered := range []string{single, multi} {
		if !strings.Contains(rendered, "38;5;102") || !strings.Contains(rendered, "╔") {
			t.Fatalf("double border/color missing from %q", rendered)
		}
		if !strings.Contains(rendered, "38;5;103") {
			t.Fatalf("title color missing from %q", rendered)
		}
		if !strings.Contains(rendered, "38;5;104") {
			t.Fatalf("status foreground missing from %q", rendered)
		}
		if !strings.Contains(rendered, "48;5;105") {
			t.Fatalf("status background missing from %q", rendered)
		}
	}
	if got := strings.Count(border, "\n"); got != 6 {
		t.Fatalf("padding render has %d newlines, want 6: %q", got, border)
	}
}

func completeCustomThemeJSON(name string) string {
	return `{"name":"` + name + `","foreground":"101","border":"102","title_bar":"103","status_bar":"104","status_bar_bg":"105","border_type":"double","padding":2}`
}

func writeThemeFile(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func useANSI256(t *testing.T) {
	t.Helper()
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
}

func keyDown() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyDown}
}
