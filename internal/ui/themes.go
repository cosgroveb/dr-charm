package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	Name        string `json:"name"`
	Foreground  string `json:"foreground"`
	Border      string `json:"border"`
	TitleBar    string `json:"title_bar"`
	StatusBar   string `json:"status_bar"`
	StatusBarBg string `json:"status_bar_bg"`
	BorderType  string `json:"border_type"`
	Padding     int    `json:"padding"`
}

type themeCatalog struct {
	themes       []theme
	currentIndex int
}

func newThemeCatalog(themesDir string) *themeCatalog {
	catalog := &themeCatalog{
		themes: []theme{
			{Name: "default", Foreground: "7", Border: "62", TitleBar: "170", StatusBar: "240", StatusBarBg: "235", BorderType: "rounded", Padding: 1},
			{Name: "dark", Foreground: "252", Border: "237", TitleBar: "33", StatusBar: "252", StatusBarBg: "237", BorderType: "rounded", Padding: 1},
			{Name: "high-contrast", Foreground: "15", Border: "15", TitleBar: "226", StatusBar: "0", StatusBarBg: "15", BorderType: "thick", Padding: 1},
		},
		currentIndex: 0,
	}
	catalog.loadCustomThemes(themesDir)
	return catalog
}

func (c *themeCatalog) loadCustomThemes(themesDir string) {
	if themesDir == "" {
		return
	}
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		fmt.Printf("Failed to load themes from %s: %v\n", themesDir, err)
		return
	}
	files, err := filepath.Glob(filepath.Join(themesDir, "*.json"))
	if err != nil {
		fmt.Printf("Failed to load themes from %s: %v\n", themesDir, err)
		return
	}
	for _, file := range files {
		loaded, err := loadThemeFromFile(file)
		if err != nil {
			fmt.Printf("Failed to load theme %s: %v\n", file, err)
			continue
		}
		c.add(loaded)
	}
}

func loadThemeFromFile(path string) (theme, error) {
	file, err := os.Open(path)
	if err != nil {
		return theme{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var loaded theme
	if err := decoder.Decode(&loaded); err != nil {
		return theme{}, err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return theme{}, errors.New("theme file contains multiple JSON values")
		}
		return theme{}, err
	}
	loaded.Name = strings.TrimSpace(loaded.Name)
	if loaded.Name == "" {
		return theme{}, errors.New("theme name is required")
	}
	return loaded, nil
}

func (c *themeCatalog) add(loaded theme) {
	for index := range c.themes {
		if c.themes[index].Name == loaded.Name {
			c.themes[index] = loaded
			return
		}
	}
	c.themes = append(c.themes, loaded)
}

func (c *themeCatalog) current() theme {
	return c.themes[c.currentIndex]
}

func (c *themeCatalog) names() []string {
	names := make([]string, len(c.themes))
	for index := range c.themes {
		names[index] = c.themes[index].Name
	}
	return names
}

func (c *themeCatalog) previous() {
	if c.currentIndex > 0 {
		c.currentIndex--
	}
}

func (c *themeCatalog) next() {
	if c.currentIndex < len(c.themes)-1 {
		c.currentIndex++
	}
}

func (c *themeCatalog) borderStyle() lipgloss.Style {
	current := c.current()
	style := lipgloss.NewStyle()
	switch current.BorderType {
	case "normal":
		style = style.Border(lipgloss.NormalBorder())
	case "hidden":
		style = style.Border(lipgloss.HiddenBorder())
	case "thick":
		style = style.Border(lipgloss.ThickBorder())
	case "double":
		style = style.Border(lipgloss.DoubleBorder())
	default:
		style = style.Border(lipgloss.RoundedBorder())
	}
	return style.BorderForeground(lipgloss.Color(current.Border)).Padding(current.Padding)
}
