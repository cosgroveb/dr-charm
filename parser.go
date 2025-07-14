package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// GameState holds parsed game state
type GameState struct {
	Health           int
	MaxHealth        int
	Mana             int
	MaxMana          int
	Stamina          int
	MaxStamina       int
	Concentration    int
	MaxConcentration int
	Spirit           int
	MaxSpirit        int
	Stance           int
	Roundtime        int
	CastRT           int
	Room             RoomInfo
	RightHand        string
	LeftHand         string
	Spell            string
}

// RoomInfo holds current room information
type RoomInfo struct {
	ID          string
	Title       string
	Description string
	Exits       []string
	Objects     []string
	Players     []string
}

// XMLParser handles parsing game XML stream
type XMLParser struct {
	decoder    *xml.Decoder
	state      *GameState
	textBuffer strings.Builder
	inPrompt   bool
	debug      bool
}

// NewXMLParser creates a new parser
func NewXMLParser(debug bool) *XMLParser {
	return &XMLParser{
		state: &GameState{},
		debug: debug,
	}
}

// ParseStream parses XML from reader and returns display text
func (p *XMLParser) ParseStream(reader io.Reader) (string, error) {
	// For now, let's do a simpler approach that extracts text and prompt data
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	text := string(data)
	p.textBuffer.Reset()

	// Parse health bars from dialogData
	if strings.Contains(text, "progressBar") {
		p.parseProgressBars(text)
	}

	// Basic XML stripping for display
	result := ""
	inTag := false
	for _, ch := range text {
		if ch == '<' {
			inTag = true
		} else if ch == '>' {
			inTag = false
		} else if !inTag {
			result += string(ch)
		}
	}

	return result, nil
}

func (p *XMLParser) parseProgressBars(text string) {
	// Helper to extract progressBar value
	extractProgressBar := func(text, id string) int {
		searchStr := fmt.Sprintf(`<progressBar id="%s"`, id)
		if idx := strings.Index(text, searchStr); idx >= 0 {
			end := strings.Index(text[idx:], "/>")
			if end > 0 {
				bar := text[idx : idx+end+2]
				// Extract value
				if valueIdx := strings.Index(bar, `value="`); valueIdx >= 0 {
					valueEnd := strings.Index(bar[valueIdx+7:], `"`)
					if valueEnd > 0 {
						value := bar[valueIdx+7 : valueIdx+7+valueEnd]
						var result int
						if val, err := fmt.Sscanf(value, "%d", &result); err == nil && val == 1 {
							return result
						}
					}
				}
			}
		}
		return -1 // Not found
	}

	// Parse all vitals - try both with and without "2" suffix
	if val := extractProgressBar(text, "health"); val >= 0 {
		p.state.Health = val
		p.state.MaxHealth = 100
		if p.debug {
			fmt.Printf("[DEBUG] Health: %d%%\n", val)
		}
	} else if val := extractProgressBar(text, "health2"); val >= 0 {
		p.state.Health = val
		p.state.MaxHealth = 100
		if p.debug {
			fmt.Printf("[DEBUG] Health2: %d%%\n", val)
		}
	}

	if val := extractProgressBar(text, "mana"); val >= 0 {
		p.state.Mana = val
		p.state.MaxMana = 100
		if p.debug {
			fmt.Printf("[DEBUG] Mana: %d%%\n", val)
		}
	} else if val := extractProgressBar(text, "mana2"); val >= 0 {
		p.state.Mana = val
		p.state.MaxMana = 100
		if p.debug {
			fmt.Printf("[DEBUG] Mana2: %d%%\n", val)
		}
	}

	if val := extractProgressBar(text, "stamina"); val >= 0 {
		p.state.Stamina = val
		p.state.MaxStamina = 100
		if p.debug {
			fmt.Printf("[DEBUG] Stamina: %d%%\n", val)
		}
	} else if val := extractProgressBar(text, "fatigue"); val >= 0 {
		p.state.Stamina = val
		p.state.MaxStamina = 100
		if p.debug {
			fmt.Printf("[DEBUG] Fatigue: %d%%\n", val)
		}
	}

	// Parse additional vitals that Outlander tracks
	if val := extractProgressBar(text, "concentration"); val >= 0 {
		p.state.Concentration = val
		p.state.MaxConcentration = 100
		if p.debug {
			fmt.Printf("[DEBUG] Concentration: %d%%\n", val)
		}
	}

	if val := extractProgressBar(text, "spirit"); val >= 0 {
		p.state.Spirit = val
		p.state.MaxSpirit = 100
		if p.debug {
			fmt.Printf("[DEBUG] Spirit: %d%%\n", val)
		}
	}
}

func (p *XMLParser) parsePromptTag(tag string) {
	// Extract attributes from prompt tag
	attrs := make(map[string]string)

	// Simple attribute parsing
	parts := strings.Split(tag, " ")
	for _, part := range parts[1:] {
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := kv[0]
				val := strings.Trim(kv[1], "\"'>")
				attrs[key] = val
			}
		}
	}

	// Update game state from attributes
	if v, ok := attrs["health"]; ok {
		fmt.Sscanf(v, "%d", &p.state.Health)
	}
	if v, ok := attrs["maxhealth"]; ok {
		fmt.Sscanf(v, "%d", &p.state.MaxHealth)
	}
	if v, ok := attrs["mana"]; ok {
		fmt.Sscanf(v, "%d", &p.state.Mana)
	}
	if v, ok := attrs["maxmana"]; ok {
		fmt.Sscanf(v, "%d", &p.state.MaxMana)
	}
	if v, ok := attrs["stamina"]; ok {
		fmt.Sscanf(v, "%d", &p.state.Stamina)
	}
	if v, ok := attrs["maxstamina"]; ok {
		fmt.Sscanf(v, "%d", &p.state.MaxStamina)
	}
	if v, ok := attrs["stance"]; ok {
		fmt.Sscanf(v, "%d", &p.state.Stance)
	}
}

func (p *XMLParser) handleStartElement(start xml.StartElement) {
	switch start.Name.Local {
	case "prompt":
		p.inPrompt = true
		// Parse prompt attributes for vitals
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "health":
				fmt.Sscanf(attr.Value, "%d", &p.state.Health)
			case "maxhealth":
				fmt.Sscanf(attr.Value, "%d", &p.state.MaxHealth)
			case "mana":
				fmt.Sscanf(attr.Value, "%d", &p.state.Mana)
			case "maxmana":
				fmt.Sscanf(attr.Value, "%d", &p.state.MaxMana)
			case "stamina":
				fmt.Sscanf(attr.Value, "%d", &p.state.Stamina)
			case "maxstamina":
				fmt.Sscanf(attr.Value, "%d", &p.state.MaxStamina)
			case "stance":
				fmt.Sscanf(attr.Value, "%d", &p.state.Stance)
			}
		}
		if p.debug {
			p.logVitals()
		}

	case "roundTime":
		// Get roundtime value
		for _, attr := range start.Attr {
			if attr.Name.Local == "value" {
				fmt.Sscanf(attr.Value, "%d", &p.state.Roundtime)
			}
		}

	case "castTime":
		// Get cast roundtime
		for _, attr := range start.Attr {
			if attr.Name.Local == "value" {
				fmt.Sscanf(attr.Value, "%d", &p.state.CastRT)
			}
		}

	case "right":
		// Right hand item
		var text bytes.Buffer
		p.decoder.DecodeElement(&text, &start)
		p.state.RightHand = strings.TrimSpace(text.String())

	case "left":
		// Left hand item
		var text bytes.Buffer
		p.decoder.DecodeElement(&text, &start)
		p.state.LeftHand = strings.TrimSpace(text.String())

	case "spell":
		// Active spell
		var text bytes.Buffer
		p.decoder.DecodeElement(&text, &start)
		p.state.Spell = strings.TrimSpace(text.String())

	case "room":
		// Room info
		p.parseRoom(start)

	case "compass":
		// Room exits
		p.parseCompass(start)

	case "style":
		// Text styling - we'll handle this later
		for _, attr := range start.Attr {
			if attr.Name.Local == "id" && attr.Value == "roomName" {
				// Next text is room name
			}
		}
	}
}

func (p *XMLParser) handleEndElement(end xml.EndElement) {
	switch end.Name.Local {
	case "prompt":
		p.inPrompt = false
		p.textBuffer.WriteString(">")
	}
}

func (p *XMLParser) parseRoom(start xml.StartElement) {
	// Get room ID from attributes
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			p.state.Room.ID = attr.Value
		}
	}
}

func (p *XMLParser) parseCompass(start xml.StartElement) {
	p.state.Room.Exits = []string{}
	// Parse compass directions
	var compass struct {
		Dirs []struct {
			Value string `xml:"value,attr"`
		} `xml:"dir"`
	}
	p.decoder.DecodeElement(&compass, &start)
	for _, dir := range compass.Dirs {
		if dir.Value != "" {
			p.state.Room.Exits = append(p.state.Room.Exits, dir.Value)
		}
	}
}

func (p *XMLParser) logVitals() {
	fmt.Printf("[DEBUG] Vitals - H:%d/%d M:%d/%d S:%d/%d Stance:%d RT:%d\n",
		p.state.Health, p.state.MaxHealth,
		p.state.Mana, p.state.MaxMana,
		p.state.Stamina, p.state.MaxStamina,
		p.state.Stance, p.state.Roundtime)
}

// GetState returns current game state
func (p *XMLParser) GetState() *GameState {
	return p.state
}
