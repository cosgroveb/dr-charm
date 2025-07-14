package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

// Theme defines color and style settings
type Theme struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Colors      ThemeColors `json:"colors"`
	Styles      ThemeStyles `json:"styles"`
}

// ThemeColors defines all color settings
type ThemeColors struct {
	// UI Elements
	Background      string `json:"background"`
	Foreground      string `json:"foreground"`
	Border          string `json:"border"`
	BorderFocused   string `json:"border_focused"`
	TitleBar        string `json:"title_bar"`
	StatusBar       string `json:"status_bar"`
	StatusBarBg     string `json:"status_bar_bg"`
	ScrollIndicator string `json:"scroll_indicator"`

	// Game Text
	RoomName    string `json:"room_name"`
	RoomDesc    string `json:"room_desc"`
	RoomExits   string `json:"room_exits"`
	RoomObjects string `json:"room_objects"`
	RoomPlayers string `json:"room_players"`

	// Combat
	CombatHit     string `json:"combat_hit"`
	CombatMiss    string `json:"combat_miss"`
	CombatDamage  string `json:"combat_damage"`
	CombatDefense string `json:"combat_defense"`

	// Communication
	Whisper string `json:"whisper"`
	Say     string `json:"say"`
	Yell    string `json:"yell"`
	Think   string `json:"think"`
	OOC     string `json:"ooc"`

	// Status
	HealthGood  string `json:"health_good"`
	HealthWarn  string `json:"health_warn"`
	HealthCrit  string `json:"health_crit"`
	ManaGood    string `json:"mana_good"`
	ManaWarn    string `json:"mana_warn"`
	ManaLow     string `json:"mana_low"`
	StaminaGood string `json:"stamina_good"`
	StaminaWarn string `json:"stamina_warn"`
	StaminaLow  string `json:"stamina_low"`

	// Special
	Death      string `json:"death"`
	DeathBg    string `json:"death_bg"`
	Experience string `json:"experience"`
	ExpBg      string `json:"exp_bg"`
	Error      string `json:"error"`
	ErrorBg    string `json:"error_bg"`
}

// ThemeStyles defines style configurations
type ThemeStyles struct {
	BorderType  string `json:"border_type"` // "rounded", "normal", "hidden", "thick", "double"
	Padding     int    `json:"padding"`
	TitleAlign  string `json:"title_align"`  // "left", "center", "right"
	StatusAlign string `json:"status_align"` // "left", "center", "right"
}

// ThemeManager handles theme loading and application
type ThemeManager struct {
	themes       map[string]*Theme
	currentTheme string
	themesDir    string
}

// NewThemeManager creates a new theme manager
func NewThemeManager(themesDir string) *ThemeManager {
	tm := &ThemeManager{
		themes:       make(map[string]*Theme),
		themesDir:    themesDir,
		currentTheme: "default",
	}

	// Load default themes
	tm.loadDefaultThemes()

	// Load custom themes from directory
	tm.loadCustomThemes()

	return tm
}

// loadDefaultThemes creates built-in themes
func (tm *ThemeManager) loadDefaultThemes() {
	// Default theme
	defaultTheme := &Theme{
		Name:        "default",
		Description: "Default DragonRealms theme",
		Colors: ThemeColors{
			Background:      "0",
			Foreground:      "7",
			Border:          "62",
			BorderFocused:   "170",
			TitleBar:        "170",
			StatusBar:       "240",
			StatusBarBg:     "235",
			ScrollIndicator: "240",

			RoomName:    "226",
			RoomDesc:    "7",
			RoomExits:   "46",
			RoomObjects: "226",
			RoomPlayers: "33",

			CombatHit:     "46",
			CombatMiss:    "226",
			CombatDamage:  "196",
			CombatDefense: "33",

			Whisper: "135",
			Say:     "7",
			Yell:    "226",
			Think:   "244",
			OOC:     "33",

			HealthGood:  "46",
			HealthWarn:  "226",
			HealthCrit:  "196",
			ManaGood:    "33",
			ManaWarn:    "226",
			ManaLow:     "196",
			StaminaGood: "214",
			StaminaWarn: "226",
			StaminaLow:  "196",

			Death:      "196",
			DeathBg:    "52",
			Experience: "46",
			ExpBg:      "22",
			Error:      "196",
			ErrorBg:    "0",
		},
		Styles: ThemeStyles{
			BorderType:  "rounded",
			Padding:     1,
			TitleAlign:  "center",
			StatusAlign: "left",
		},
	}
	tm.themes["default"] = defaultTheme

	// Dark theme
	darkTheme := &Theme{
		Name:        "dark",
		Description: "Dark mode theme",
		Colors: ThemeColors{
			Background:      "232",
			Foreground:      "252",
			Border:          "237",
			BorderFocused:   "33",
			TitleBar:        "33",
			StatusBar:       "252",
			StatusBarBg:     "237",
			ScrollIndicator: "245",

			RoomName:    "220",
			RoomDesc:    "252",
			RoomExits:   "40",
			RoomObjects: "214",
			RoomPlayers: "39",

			CombatHit:     "40",
			CombatMiss:    "214",
			CombatDamage:  "197",
			CombatDefense: "39",

			Whisper: "141",
			Say:     "252",
			Yell:    "214",
			Think:   "245",
			OOC:     "39",

			HealthGood:  "40",
			HealthWarn:  "214",
			HealthCrit:  "197",
			ManaGood:    "39",
			ManaWarn:    "214",
			ManaLow:     "197",
			StaminaGood: "208",
			StaminaWarn: "214",
			StaminaLow:  "197",

			Death:      "197",
			DeathBg:    "88",
			Experience: "40",
			ExpBg:      "22",
			Error:      "197",
			ErrorBg:    "232",
		},
		Styles: ThemeStyles{
			BorderType:  "rounded",
			Padding:     1,
			TitleAlign:  "center",
			StatusAlign: "left",
		},
	}
	tm.themes["dark"] = darkTheme

	// High contrast theme
	contrastTheme := &Theme{
		Name:        "high-contrast",
		Description: "High contrast theme for better visibility",
		Colors: ThemeColors{
			Background:      "0",
			Foreground:      "15",
			Border:          "15",
			BorderFocused:   "226",
			TitleBar:        "226",
			StatusBar:       "0",
			StatusBarBg:     "15",
			ScrollIndicator: "226",

			RoomName:    "226",
			RoomDesc:    "15",
			RoomExits:   "46",
			RoomObjects: "226",
			RoomPlayers: "51",

			CombatHit:     "46",
			CombatMiss:    "226",
			CombatDamage:  "196",
			CombatDefense: "51",

			Whisper: "201",
			Say:     "15",
			Yell:    "226",
			Think:   "250",
			OOC:     "51",

			HealthGood:  "46",
			HealthWarn:  "226",
			HealthCrit:  "196",
			ManaGood:    "51",
			ManaWarn:    "226",
			ManaLow:     "196",
			StaminaGood: "214",
			StaminaWarn: "226",
			StaminaLow:  "196",

			Death:      "15",
			DeathBg:    "196",
			Experience: "0",
			ExpBg:      "46",
			Error:      "15",
			ErrorBg:    "196",
		},
		Styles: ThemeStyles{
			BorderType:  "thick",
			Padding:     1,
			TitleAlign:  "center",
			StatusAlign: "left",
		},
	}
	tm.themes["high-contrast"] = contrastTheme
}

// loadCustomThemes loads themes from the themes directory
func (tm *ThemeManager) loadCustomThemes() {
	if tm.themesDir == "" {
		return
	}

	// Create themes directory if it doesn't exist
	os.MkdirAll(tm.themesDir, 0755)

	// Read all .json files in themes directory
	files, err := filepath.Glob(filepath.Join(tm.themesDir, "*.json"))
	if err != nil {
		return
	}

	for _, file := range files {
		theme, err := tm.loadThemeFromFile(file)
		if err != nil {
			fmt.Printf("Failed to load theme %s: %v\n", file, err)
			continue
		}
		tm.themes[theme.Name] = theme
	}
}

// loadThemeFromFile loads a theme from a JSON file
func (tm *ThemeManager) loadThemeFromFile(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var theme Theme
	if err := json.Unmarshal(data, &theme); err != nil {
		return nil, err
	}

	return &theme, nil
}

// SaveTheme saves a theme to a file
func (tm *ThemeManager) SaveTheme(theme *Theme) error {
	if tm.themesDir == "" {
		return fmt.Errorf("themes directory not set")
	}

	data, err := json.MarshalIndent(theme, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(tm.themesDir, theme.Name+".json")
	return os.WriteFile(path, data, 0644)
}

// SetTheme changes the current theme
func (tm *ThemeManager) SetTheme(name string) error {
	if _, ok := tm.themes[name]; !ok {
		return fmt.Errorf("theme '%s' not found", name)
	}
	tm.currentTheme = name
	return nil
}

// GetTheme returns the current theme
func (tm *ThemeManager) GetTheme() *Theme {
	if theme, ok := tm.themes[tm.currentTheme]; ok {
		return theme
	}
	return tm.themes["default"]
}

// GetThemeNames returns all available theme names
func (tm *ThemeManager) GetThemeNames() []string {
	var names []string
	for name := range tm.themes {
		names = append(names, name)
	}
	return names
}

// CreateBorderStyle creates a lipgloss border style from theme
func (tm *ThemeManager) CreateBorderStyle() lipgloss.Style {
	theme := tm.GetTheme()
	style := lipgloss.NewStyle()

	// Set border type
	switch theme.Styles.BorderType {
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

	// Set colors
	style = style.
		BorderForeground(lipgloss.Color(theme.Colors.Border)).
		Padding(theme.Styles.Padding)

	return style
}

// GetColor returns a color value from the current theme
func (tm *ThemeManager) GetColor(colorName string) string {
	theme := tm.GetTheme()

	// Use reflection would be cleaner, but for now manual mapping
	switch colorName {
	case "background":
		return theme.Colors.Background
	case "foreground":
		return theme.Colors.Foreground
	case "border":
		return theme.Colors.Border
	case "border_focused":
		return theme.Colors.BorderFocused
	case "title_bar":
		return theme.Colors.TitleBar
	case "room_name":
		return theme.Colors.RoomName
	case "room_exits":
		return theme.Colors.RoomExits
	case "combat_hit":
		return theme.Colors.CombatHit
	case "combat_miss":
		return theme.Colors.CombatMiss
	case "whisper":
		return theme.Colors.Whisper
	case "health_good":
		return theme.Colors.HealthGood
	case "health_warn":
		return theme.Colors.HealthWarn
	case "health_crit":
		return theme.Colors.HealthCrit
	default:
		return theme.Colors.Foreground
	}
}
