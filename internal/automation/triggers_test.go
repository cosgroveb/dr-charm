package automation

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func TestBuiltInHighlights(t *testing.T) {
	useANSI256(t)
	manager := NewTriggerManager()

	for _, tt := range []struct {
		name       string
		hit        string
		miss       string
		matched    string
		foreground string
		background string
	}{
		{name: "combat hit", hit: "You swing a sword with force.", miss: "You swing a sword.", matched: "You swing a sword with ", foreground: "46"},
		{name: "combat miss", hit: "You miss a goblin.", miss: "You strike a goblin.", matched: "You miss ", foreground: "226"},
		{name: "hit you", hit: "Goblin strikes you.", miss: "Goblin strikes Cennedig.", matched: "Goblin strikes you", foreground: "196"},
		{name: "death", hit: "You are dead", miss: "You are alive", matched: "You are dead", foreground: "196", background: "52"},
		{name: "stunned", hit: "You are stunned", miss: "You recover quickly", matched: "You are stunned", foreground: "226"},
		{name: "arrival", hit: "Cennedig just arrived", miss: "Cennedig stands nearby", matched: "Cennedig just arrived", foreground: "33"},
		{name: "departure", hit: "Cennedig just went", miss: "Cennedig remains here", matched: "Cennedig just went", foreground: "33"},
		{name: "whisper", hit: "Cennedig whispers to you", miss: "Cennedig speaks aloud", matched: "Cennedig whispers to you", foreground: "135"},
		{name: "experience", hit: "You've gained a new rank", miss: "Your rank remains unchanged", matched: "You've gained a new rank", foreground: "46", background: "22"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := strings.Replace(tt.hit, tt.matched, ansi256(tt.matched, tt.foreground, tt.background), 1)
			if got := manager.ProcessLine(tt.hit); got != want {
				t.Fatalf("hit = %q, want %q", got, want)
			}
			if got := manager.ProcessLine(tt.miss); got != tt.miss {
				t.Fatalf("miss = %q, want unchanged", got)
			}
		})
	}
}

func TestHighlightInteractions(t *testing.T) {
	useANSI256(t)
	manager := NewTriggerManager()

	for _, tt := range []struct {
		name string
		line string
		want string
	}{
		{
			name: "non-overlap",
			line: "You miss a goblin. Cennedig just arrived",
			want: ansi256("You miss ", "226", "") + "a goblin. " + ansi256("Cennedig just arrived", "33", ""),
		},
		{
			name: "equal start keeps earlier rule",
			line: "You are dead with finality.",
			want: ansi256("You are dead with ", "46", "") + "finality.",
		},
		{
			name: "earlier match preserves equal start rule order",
			line: "Cennedig just arrived. You are dead with finality.",
			want: ansi256("Cennedig just arrived", "33", "") + ". " + ansi256("You are dead with ", "46", "") + "finality.",
		},
		{
			name: "overlap keeps earlier start",
			line: "They tell you are dead.",
			want: ansi256("They tell you", "196", "") + " are dead.",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := manager.ProcessLine(tt.line); got != tt.want {
				t.Fatalf("ProcessLine = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHighlightMatchingCase(t *testing.T) {
	useANSI256(t)
	manager := NewTriggerManager()
	for _, tt := range []struct {
		name string
		line string
	}{
		{name: "uppercase", line: "YOU MISS A GOBLIN."},
		{name: "mixed-case", line: "You MISS A GOBLIN."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := ansi256(tt.line[:9], "226", "") + tt.line[9:]
			if got := manager.ProcessLine(tt.line); got != want {
				t.Fatalf("ProcessLine = %q, want %q", got, want)
			}
		})
	}
}

func TestBuiltInAliases(t *testing.T) {
	manager := NewTriggerManager()
	aliases := map[string]string{
		"n": "north", "s": "south", "e": "east", "w": "west",
		"ne": "northeast", "nw": "northwest", "se": "southeast", "sw": "southwest",
		"u": "up", "d": "down", "o": "out", "l": "look", "i": "inventory",
		"sta": "stand", "kne": "kneel", "sit": "sit", "ly": "lie", "hi": "hide",
		"sn": "sneak", "sea": "search", "att": "attack", "ki": "kill", "sk": "skin",
		"arr": "arrange", "loot": "loot all",
	}
	for alias, want := range aliases {
		if got := manager.ProcessCommand(alias); got != want {
			t.Errorf("ProcessCommand(%q) = %q, want %q", alias, got, want)
		}
	}
	for _, tt := range []struct {
		command string
		want    string
	}{
		{command: "l", want: "look"},
		{command: "  l   at   target  ", want: "look at target"},
		{command: "L north", want: "L north"},
		{command: "  unknown   value  ", want: "  unknown   value  "},
		{command: "   ", want: "   "},
	} {
		if got := manager.ProcessCommand(tt.command); got != tt.want {
			t.Errorf("ProcessCommand(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func BenchmarkProcessLineHit(b *testing.B) {
	manager := NewTriggerManager()
	for range b.N {
		manager.ProcessLine("You are dead")
	}
}

func BenchmarkProcessLineMiss(b *testing.B) {
	manager := NewTriggerManager()
	for range b.N {
		manager.ProcessLine("A quiet breeze passes.")
	}
}

func useANSI256(t *testing.T) {
	t.Helper()
	profile := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.ANSI256
	t.Cleanup(func() { lipgloss.Writer.Profile = profile })
}

func ansi256(text, foreground, background string) string {
	var codes []string
	if foreground != "" {
		codes = append(codes, "38;5;"+foreground)
	}
	if background != "" {
		codes = append(codes, "48;5;"+background)
	}
	return "\x1b[" + strings.Join(codes, ";") + "m" + text + "\x1b[m"
}
