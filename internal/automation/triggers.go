package automation

import (
	"regexp"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

type highlightRule struct {
	regex *regexp.Regexp
	style lipgloss.Style
}

// TriggerManager applies built-in highlights and command aliases.
type TriggerManager struct {
	rules   []highlightRule
	aliases map[string]string
}

// NewTriggerManager creates a manager with DragonRealms highlights and aliases.
func NewTriggerManager() *TriggerManager {
	return &TriggerManager{
		rules: []highlightRule{
			newHighlightRule("You \\w+ .+ with ", "46", ""),
			newHighlightRule("You miss ", "226", ""),
			newHighlightRule("\\w+ \\w+ you", "196", ""),
			newHighlightRule("You are dead", "196", "52"),
			newHighlightRule("You are stunned", "226", ""),
			newHighlightRule("\\w+ just arrived", "33", ""),
			newHighlightRule("\\w+ just went", "33", ""),
			newHighlightRule("\\w+ whispers to you", "135", ""),
			newHighlightRule("You've gained a new rank", "46", "22"),
		},
		aliases: map[string]string{
			"n": "north", "s": "south", "e": "east", "w": "west",
			"ne": "northeast", "nw": "northwest", "se": "southeast", "sw": "southwest",
			"u": "up", "d": "down", "o": "out",
			"l": "look", "i": "inventory", "sta": "stand", "kne": "kneel",
			"sit": "sit", "ly": "lie", "hi": "hide", "sn": "sneak", "sea": "search",
			"att": "attack", "ki": "kill", "sk": "skin", "arr": "arrange", "loot": "loot all",
		},
	}
}

func newHighlightRule(pattern, foreground, background string) highlightRule {
	style := lipgloss.NewStyle()
	if foreground != "" {
		style = style.Foreground(lipgloss.Color(foreground))
	}
	if background != "" {
		style = style.Background(lipgloss.Color(background))
	}
	return highlightRule{
		regex: regexp.MustCompile("(?i)" + pattern),
		style: style,
	}
}

// ProcessLine applies built-in highlights to one line.
func (tm *TriggerManager) ProcessLine(line string) string {
	if line == "" {
		return line
	}

	type match struct {
		start, end int
		rule       int
	}
	var matches []match
	for i, rule := range tm.rules {
		for _, loc := range rule.regex.FindAllStringIndex(line, -1) {
			matches = append(matches, match{start: loc[0], end: loc[1], rule: i})
		}
	}
	if len(matches) == 0 {
		return line
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].start < matches[j].start
	})

	var result strings.Builder
	lastEnd := 0
	for _, match := range matches {
		if match.start < lastEnd {
			continue
		}
		if match.start > lastEnd {
			result.WriteString(line[lastEnd:match.start])
		}
		result.WriteString(tm.rules[match.rule].style.Render(line[match.start:match.end]))
		lastEnd = match.end
	}
	if lastEnd < len(line) {
		result.WriteString(line[lastEnd:])
	}
	return result.String()
}

// ProcessCommand expands a built-in alias while preserving unknown input.
func (tm *TriggerManager) ProcessCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return command
	}
	expanded, ok := tm.aliases[parts[0]]
	if !ok {
		return command
	}
	parts[0] = expanded
	return strings.Join(parts, " ")
}
