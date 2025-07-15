package main

import (
	"regexp"
)

// TriggerCache caches compiled regex patterns and optimizes trigger matching
type TriggerCache struct {
	// Quick check patterns - if these strings aren't in the line, skip the regex
	quickChecks map[string][]string // triggerID -> required substrings
	
	// Pre-compiled patterns grouped by type
	combatTriggers  []Trigger
	commTriggers    []Trigger
	generalTriggers []Trigger
}

// BuildTriggerCache creates an optimized trigger cache
func BuildTriggerCache(triggers []Trigger) *TriggerCache {
	cache := &TriggerCache{
		quickChecks: make(map[string][]string),
	}
	
	for _, trigger := range triggers {
		if !trigger.Enabled || trigger.Regex == nil {
			continue
		}
		
		// Extract literal strings from patterns for quick checks
		literals := extractLiterals(trigger.Pattern)
		if len(literals) > 0 {
			cache.quickChecks[trigger.ID] = literals
		}
		
		// Group by type for better cache locality
		switch trigger.ID {
		case "attack", "attacked", "death", "stunned":
			cache.combatTriggers = append(cache.combatTriggers, trigger)
		case "whisper", "arrival", "departure":
			cache.commTriggers = append(cache.commTriggers, trigger)
		default:
			cache.generalTriggers = append(cache.generalTriggers, trigger)
		}
	}
	
	return cache
}

// extractLiterals finds literal strings in regex patterns for quick pre-filtering
func extractLiterals(pattern string) []string {
	var literals []string
	
	// Simple extraction of literal words
	re := regexp.MustCompile(`\b\w{3,}\b`)
	matches := re.FindAllString(pattern, -1)
	for _, match := range matches {
		// Skip regex metacharacters
		if match != "w" && match != "d" && match != "s" {
			literals = append(literals, match)
		}
	}
	
	// Also extract specific phrases
	if contains := regexp.MustCompile(`([A-Za-z\s]+)`).FindAllString(pattern, -1); len(contains) > 0 {
		for _, phrase := range contains {
			if len(phrase) > 3 {
				literals = append(literals, phrase)
			}
		}
	}
	
	return literals
}

// QuickMatch checks if a line could possibly match this trigger
func (tc *TriggerCache) QuickMatch(triggerID string, line string) bool {
	literals, ok := tc.quickChecks[triggerID]
	if !ok {
		return true // No quick check available, must do full regex
	}
	
	// Line must contain at least one literal string
	for _, literal := range literals {
		if contains(line, literal) {
			return true
		}
	}
	return false
}

// Simple string contains that might be faster than strings.Contains for short strings
func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}