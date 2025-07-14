package main

import (
	"regexp"
	"strconv"
	"strings"
)

// VitalsParser extracts vitals from game text
type VitalsParser struct {
	// Regex patterns for different vitals formats
	healthPattern  *regexp.Regexp
	manaPattern    *regexp.Regexp
	staminaPattern *regexp.Regexp
	promptPattern  *regexp.Regexp
}

// NewVitalsParser creates a new parser
func NewVitalsParser() *VitalsParser {
	return &VitalsParser{
		// Common patterns from DR output
		healthPattern:  regexp.MustCompile(`(?i)health\s*:\s*(\d+)\s*/\s*(\d+)`),
		manaPattern:    regexp.MustCompile(`(?i)mana\s*:\s*(\d+)\s*/\s*(\d+)`),
		staminaPattern: regexp.MustCompile(`(?i)stamina\s*:\s*(\d+)\s*/\s*(\d+)`),
		// XML prompt pattern
		promptPattern: regexp.MustCompile(`<prompt[^>]*>`),
	}
}

// ParseFromText extracts vitals from game text output
func (v *VitalsParser) ParseFromText(text string, state *GameState) {
	// Try to find health
	if matches := v.healthPattern.FindStringSubmatch(text); len(matches) == 3 {
		if current, err := strconv.Atoi(matches[1]); err == nil {
			state.Health = current
		}
		if max, err := strconv.Atoi(matches[2]); err == nil {
			state.MaxHealth = max
		}
	}
	
	// Try to find mana
	if matches := v.manaPattern.FindStringSubmatch(text); len(matches) == 3 {
		if current, err := strconv.Atoi(matches[1]); err == nil {
			state.Mana = current
		}
		if max, err := strconv.Atoi(matches[2]); err == nil {
			state.MaxMana = max
		}
	}
	
	// Try to find stamina
	if matches := v.staminaPattern.FindStringSubmatch(text); len(matches) == 3 {
		if current, err := strconv.Atoi(matches[1]); err == nil {
			state.Stamina = current
		}
		if max, err := strconv.Atoi(matches[2]); err == nil {
			state.MaxStamina = max
		}
	}
}

// ParsePromptXML extracts vitals from DragonRealms XML prompt
func (v *VitalsParser) ParsePromptXML(xmlData string, state *GameState) bool {
	// Look for prompt tag
	if !strings.Contains(xmlData, "<prompt") {
		return false
	}
	
	// Extract the prompt tag
	start := strings.Index(xmlData, "<prompt")
	if start < 0 {
		return false
	}
	
	end := strings.Index(xmlData[start:], ">")
	if end < 0 {
		return false
	}
	
	promptTag := xmlData[start : start+end+1]
	
	// Parse attributes - DragonRealms might use different names
	// Try common variations
	attrs := extractAttributes(promptTag)
	
	// Health variations
	if val, ok := attrs["health"]; ok {
		state.Health = parseInt(val)
	} else if val, ok := attrs["hp"]; ok {
		state.Health = parseInt(val)
	} else if val, ok := attrs["hitpoints"]; ok {
		state.Health = parseInt(val)
	}
	
	if val, ok := attrs["maxhealth"]; ok {
		state.MaxHealth = parseInt(val)
	} else if val, ok := attrs["maxhp"]; ok {
		state.MaxHealth = parseInt(val)
	} else if val, ok := attrs["maxhitpoints"]; ok {
		state.MaxHealth = parseInt(val)
	}
	
	// Mana variations
	if val, ok := attrs["mana"]; ok {
		state.Mana = parseInt(val)
	} else if val, ok := attrs["mp"]; ok {
		state.Mana = parseInt(val)
	} else if val, ok := attrs["manapoints"]; ok {
		state.Mana = parseInt(val)
	}
	
	if val, ok := attrs["maxmana"]; ok {
		state.MaxMana = parseInt(val)
	} else if val, ok := attrs["maxmp"]; ok {
		state.MaxMana = parseInt(val)
	} else if val, ok := attrs["maxmanapoints"]; ok {
		state.MaxMana = parseInt(val)
	}
	
	// Stamina variations
	if val, ok := attrs["stamina"]; ok {
		state.Stamina = parseInt(val)
	} else if val, ok := attrs["fatigue"]; ok {
		state.Stamina = parseInt(val)
	} else if val, ok := attrs["endurance"]; ok {
		state.Stamina = parseInt(val)
	}
	
	if val, ok := attrs["maxstamina"]; ok {
		state.MaxStamina = parseInt(val)
	} else if val, ok := attrs["maxfatigue"]; ok {
		state.MaxStamina = parseInt(val)
	} else if val, ok := attrs["maxendurance"]; ok {
		state.MaxStamina = parseInt(val)
	}
	
	// Stance
	if val, ok := attrs["stance"]; ok {
		state.Stance = parseInt(val)
	} else if val, ok := attrs["position"]; ok {
		state.Stance = parseInt(val)
	}
	
	return true
}

// Helper to extract attributes from XML tag
func extractAttributes(tag string) map[string]string {
	attrs := make(map[string]string)
	
	// Remove tag brackets and name
	tag = strings.TrimPrefix(tag, "<")
	tag = strings.TrimSuffix(tag, ">")
	tag = strings.TrimSuffix(tag, "/")
	
	// Skip tag name
	parts := strings.Fields(tag)
	if len(parts) <= 1 {
		return attrs
	}
	
	// Parse attributes
	attrPattern := regexp.MustCompile(`(\w+)=["']([^"']+)["']`)
	matches := attrPattern.FindAllStringSubmatch(tag, -1)
	
	for _, match := range matches {
		if len(match) == 3 {
			attrs[strings.ToLower(match[1])] = match[2]
		}
	}
	
	return attrs
}

// Helper to parse int with default 0
func parseInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}