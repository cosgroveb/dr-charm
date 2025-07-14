package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TriggerAction defines what happens when a trigger matches
type TriggerAction int

const (
	TriggerHighlight TriggerAction = iota
	TriggerSound
	TriggerCommand
	TriggerScript
)

// Trigger represents a text pattern that triggers an action
type Trigger struct {
	ID            string
	Pattern       string
	Regex         *regexp.Regexp
	Action        TriggerAction
	Color         string // For highlights
	Background    string // For highlight background
	Command       string // For command triggers
	Script        string // For script triggers
	SoundFile     string // For sound triggers
	Enabled       bool
	CaseSensitive bool
}

// TriggerManager handles all triggers and text processing
type TriggerManager struct {
	triggers []Trigger
	aliases  map[string]string
}

// NewTriggerManager creates a new trigger manager
func NewTriggerManager() *TriggerManager {
	tm := &TriggerManager{
		triggers: []Trigger{},
		aliases:  make(map[string]string),
	}

	// Add some default triggers for common DragonRealms events
	tm.AddDefaultTriggers()
	tm.AddDefaultAliases()

	return tm
}

// AddDefaultTriggers adds common DR triggers
func (tm *TriggerManager) AddDefaultTriggers() {
	// Combat triggers
	tm.AddTrigger(Trigger{
		ID:      "combat-hit",
		Pattern: "You \\w+ .+ with ",
		Action:  TriggerHighlight,
		Color:   "46", // Green
		Enabled: true,
	})

	tm.AddTrigger(Trigger{
		ID:      "combat-miss",
		Pattern: "You miss ",
		Action:  TriggerHighlight,
		Color:   "226", // Yellow
		Enabled: true,
	})

	tm.AddTrigger(Trigger{
		ID:      "combat-hit-you",
		Pattern: "\\w+ \\w+ you",
		Action:  TriggerHighlight,
		Color:   "196", // Red
		Enabled: true,
	})

	// Death and danger
	tm.AddTrigger(Trigger{
		ID:         "death",
		Pattern:    "You are dead",
		Action:     TriggerHighlight,
		Color:      "196", // Red
		Background: "52",  // Dark red background
		Enabled:    true,
	})

	tm.AddTrigger(Trigger{
		ID:      "stunned",
		Pattern: "You are stunned",
		Action:  TriggerHighlight,
		Color:   "226", // Yellow
		Enabled: true,
	})

	// Arrivals and departures
	tm.AddTrigger(Trigger{
		ID:      "arrival",
		Pattern: "\\w+ just arrived",
		Action:  TriggerHighlight,
		Color:   "33", // Blue
		Enabled: true,
	})

	tm.AddTrigger(Trigger{
		ID:      "departure",
		Pattern: "\\w+ just went",
		Action:  TriggerHighlight,
		Color:   "33", // Blue
		Enabled: true,
	})

	// Whispers and speech
	tm.AddTrigger(Trigger{
		ID:      "whisper",
		Pattern: "\\w+ whispers to you",
		Action:  TriggerHighlight,
		Color:   "135", // Purple
		Enabled: true,
	})

	// Experience
	tm.AddTrigger(Trigger{
		ID:         "exp-gain",
		Pattern:    "You've gained a new rank",
		Action:     TriggerHighlight,
		Color:      "46", // Green
		Background: "22", // Dark green background
		Enabled:    true,
	})
}

// AddDefaultAliases adds common command aliases
func (tm *TriggerManager) AddDefaultAliases() {
	// Movement shortcuts
	tm.aliases["n"] = "north"
	tm.aliases["s"] = "south"
	tm.aliases["e"] = "east"
	tm.aliases["w"] = "west"
	tm.aliases["ne"] = "northeast"
	tm.aliases["nw"] = "northwest"
	tm.aliases["se"] = "southeast"
	tm.aliases["sw"] = "southwest"
	tm.aliases["u"] = "up"
	tm.aliases["d"] = "down"
	tm.aliases["o"] = "out"

	// Common commands
	tm.aliases["l"] = "look"
	tm.aliases["i"] = "inventory"
	tm.aliases["sta"] = "stand"
	tm.aliases["kne"] = "kneel"
	tm.aliases["sit"] = "sit"
	tm.aliases["ly"] = "lie"
	tm.aliases["hi"] = "hide"
	tm.aliases["sn"] = "sneak"
	tm.aliases["sea"] = "search"

	// Combat
	tm.aliases["att"] = "attack"
	tm.aliases["ki"] = "kill"
	tm.aliases["sk"] = "skin"
	tm.aliases["arr"] = "arrange"
	tm.aliases["loot"] = "loot all"
}

// AddTrigger adds a new trigger
func (tm *TriggerManager) AddTrigger(trigger Trigger) error {
	flags := ""
	if !trigger.CaseSensitive {
		flags = "(?i)"
	}

	regex, err := regexp.Compile(flags + trigger.Pattern)
	if err != nil {
		return fmt.Errorf("invalid trigger pattern: %w", err)
	}

	trigger.Regex = regex
	tm.triggers = append(tm.triggers, trigger)
	return nil
}

// RemoveTrigger removes a trigger by ID
func (tm *TriggerManager) RemoveTrigger(id string) {
	var filtered []Trigger
	for _, t := range tm.triggers {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}
	tm.triggers = filtered
}

// ProcessLine applies triggers to a line of text and returns styled text
func (tm *TriggerManager) ProcessLine(line string) string {
	// Track all matches and their positions
	type match struct {
		start, end int
		trigger    *Trigger
	}

	var matches []match

	// Find all trigger matches
	for i := range tm.triggers {
		if !tm.triggers[i].Enabled {
			continue
		}

		if tm.triggers[i].Regex == nil {
			continue
		}

		locs := tm.triggers[i].Regex.FindAllStringIndex(line, -1)
		for _, loc := range locs {
			matches = append(matches, match{
				start:   loc[0],
				end:     loc[1],
				trigger: &tm.triggers[i],
			})
		}
	}

	// If no matches, return original line
	if len(matches) == 0 {
		return line
	}

	// Sort matches by start position
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].start > matches[j].start {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Build the styled line
	var result strings.Builder
	lastEnd := 0

	for _, m := range matches {
		// Skip overlapping matches
		if m.start < lastEnd {
			continue
		}

		// Add unstyles text before match
		if m.start > lastEnd {
			result.WriteString(line[lastEnd:m.start])
		}

		// Style the matched text
		matchedText := line[m.start:m.end]
		style := lipgloss.NewStyle()

		if m.trigger.Color != "" {
			style = style.Foreground(lipgloss.Color(m.trigger.Color))
		}
		if m.trigger.Background != "" {
			style = style.Background(lipgloss.Color(m.trigger.Background))
		}

		result.WriteString(style.Render(matchedText))
		lastEnd = m.end
	}

	// Add remaining text
	if lastEnd < len(line) {
		result.WriteString(line[lastEnd:])
	}

	return result.String()
}

// ProcessCommand checks for aliases and expands them
func (tm *TriggerManager) ProcessCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}

	// Check if first word is an alias
	if expanded, ok := tm.aliases[parts[0]]; ok {
		parts[0] = expanded
		return strings.Join(parts, " ")
	}

	return cmd
}

// AddAlias adds a command alias
func (tm *TriggerManager) AddAlias(alias, command string) {
	tm.aliases[alias] = command
}

// RemoveAlias removes a command alias
func (tm *TriggerManager) RemoveAlias(alias string) {
	delete(tm.aliases, alias)
}

// GetTriggers returns all triggers
func (tm *TriggerManager) GetTriggers() []Trigger {
	return tm.triggers
}

// GetAliases returns all aliases
func (tm *TriggerManager) GetAliases() map[string]string {
	return tm.aliases
}
