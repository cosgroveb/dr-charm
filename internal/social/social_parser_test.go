package social

import (
	"testing"
)

func TestExtractSimpleName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Lord Eldara", "Eldara"},
		{"Eldara, the Battle Mage", "Eldara"},
		{"D'marcus", "D'marcus"},
		{"Mary Sue", "Mary Sue"},
		{"Sir Marcus the Bold", "Marcus the Bold"},
		{"Lady Eldara, Keeper of Secrets", "Eldara"},
		{"Captain Jack", "Jack"},
		{"Simple", "Simple"},
	}
	
	for _, test := range tests {
		result := extractSimpleName(test.input)
		if result != test.expected {
			t.Errorf("extractSimpleName(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestParseSocialInteraction(t *testing.T) {
	tests := []struct {
		name         string
		presetType   string
		content      string
		playerName   string
		shouldParse  bool
		expectedType string
		expectedFrom string
		directedAtMe bool
	}{
		{
			name:         "directed whisper",
			presetType:   "whisper",
			content:      `Eldara whispers to you, "Hello there!"`,
			playerName:   "TestPlayer",
			shouldParse:  true,
			expectedType: "whisper",
			expectedFrom: "Eldara",
			directedAtMe: true,
		},
		{
			name:         "general whisper",
			presetType:   "whisper",
			content:      `Lord Marcus whispers, "Anyone around?"`,
			playerName:   "TestPlayer",
			shouldParse:  true,
			expectedType: "whisper",
			expectedFrom: "Lord Marcus",
			directedAtMe: false,
		},
		{
			name:         "directed say",
			presetType:   "speech",
			content:      `Mary Sue says to you, "Nice to meet you!"`,
			playerName:   "TestPlayer",
			shouldParse:  true,
			expectedType: "say",
			expectedFrom: "Mary Sue",
			directedAtMe: true,
		},
		{
			name:         "general ask",
			presetType:   "speech",
			content:      `D'marcus asks, "Where is the bank?"`,
			playerName:   "TestPlayer",
			shouldParse:  true,
			expectedType: "ask",
			expectedFrom: "D'marcus",
			directedAtMe: false,
		},
		{
			name:         "thought",
			presetType:   "thought",
			content:      `You hear the faint thoughts of Eldara echo in your mind`,
			playerName:   "TestPlayer",
			shouldParse:  true,
			expectedType: "thought",
			expectedFrom: "Eldara",
			directedAtMe: true,
		},
		{
			name:         "invalid format",
			presetType:   "whisper",
			content:      `This is not a valid whisper format`,
			playerName:   "TestPlayer",
			shouldParse:  false,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ParseSocialInteraction(test.presetType, test.content, test.playerName)
			
			if test.shouldParse && result == nil {
				t.Fatalf("Expected parse to succeed but got nil")
			}
			
			if !test.shouldParse && result != nil {
				t.Fatalf("Expected parse to fail but got result")
			}
			
			if test.shouldParse {
				if result.Subtype != test.expectedType {
					t.Errorf("Subtype = %q, want %q", result.Subtype, test.expectedType)
				}
				if result.From != test.expectedFrom {
					t.Errorf("From = %q, want %q", result.From, test.expectedFrom)
				}
				if result.DirectedAtMe != test.directedAtMe {
					t.Errorf("DirectedAtMe = %v, want %v", result.DirectedAtMe, test.directedAtMe)
				}
				if result.Type != "social_interaction" {
					t.Errorf("Type = %q, want %q", result.Type, "social_interaction")
				}
			}
		})
	}
}