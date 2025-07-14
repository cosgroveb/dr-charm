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
	Health      int
	MaxHealth   int
	Mana        int
	MaxMana     int
	Stamina     int
	MaxStamina  int
	Stance      int
	Roundtime   int
	CastRT      int
	Room        RoomInfo
	RightHand   string
	LeftHand    string
	Spell       string
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
	// Parse health bar
	if idx := strings.Index(text, `<progressBar id="health`); idx >= 0 {
		end := strings.Index(text[idx:], "/>")
		if end > 0 {
			healthBar := text[idx : idx+end+2]
			// Extract value
			if valueIdx := strings.Index(healthBar, `value="`); valueIdx >= 0 {
				valueEnd := strings.Index(healthBar[valueIdx+7:], `"`)
				if valueEnd > 0 {
					value := healthBar[valueIdx+7 : valueIdx+7+valueEnd]
					if val, err := fmt.Sscanf(value, "%d", &p.state.Health); err == nil && val == 1 {
						p.state.MaxHealth = 100 // Percentage based
						if p.debug {
							fmt.Printf("[DEBUG] Health: %d%%\n", p.state.Health)
						}
					}
				}
			}
		}
	}
	
	// Parse mana bar
	if idx := strings.Index(text, `<progressBar id="mana`); idx >= 0 {
		end := strings.Index(text[idx:], "/>")
		if end > 0 {
			manaBar := text[idx : idx+end+2]
			// Extract value
			if valueIdx := strings.Index(manaBar, `value="`); valueIdx >= 0 {
				valueEnd := strings.Index(manaBar[valueIdx+7:], `"`)
				if valueEnd > 0 {
					value := manaBar[valueIdx+7 : valueIdx+7+valueEnd]
					if val, err := fmt.Sscanf(value, "%d", &p.state.Mana); err == nil && val == 1 {
						p.state.MaxMana = 100 // Percentage based
						if p.debug {
							fmt.Printf("[DEBUG] Mana: %d%%\n", p.state.Mana)
						}
					}
				}
			}
		}
	}
	
	// Parse stamina/fatigue bar
	if idx := strings.Index(text, `<progressBar id="stamina`); idx >= 0 {
		end := strings.Index(text[idx:], "/>")
		if end > 0 {
			staminaBar := text[idx : idx+end+2]
			// Extract value
			if valueIdx := strings.Index(staminaBar, `value="`); valueIdx >= 0 {
				valueEnd := strings.Index(staminaBar[valueIdx+7:], `"`)
				if valueEnd > 0 {
					value := staminaBar[valueIdx+7 : valueIdx+7+valueEnd]
					if val, err := fmt.Sscanf(value, "%d", &p.state.Stamina); err == nil && val == 1 {
						p.state.MaxStamina = 100 // Percentage based
						if p.debug {
							fmt.Printf("[DEBUG] Stamina: %d%%\n", p.state.Stamina)
						}
					}
				}
			}
		}
	} else if idx := strings.Index(text, `<progressBar id="fatigue`); idx >= 0 {
		// Some versions use fatigue instead of stamina
		end := strings.Index(text[idx:], "/>")
		if end > 0 {
			fatigueBar := text[idx : idx+end+2]
			// Extract value
			if valueIdx := strings.Index(fatigueBar, `value="`); valueIdx >= 0 {
				valueEnd := strings.Index(fatigueBar[valueIdx+7:], `"`)
				if valueEnd > 0 {
					value := fatigueBar[valueIdx+7 : valueIdx+7+valueEnd]
					if val, err := fmt.Sscanf(value, "%d", &p.state.Stamina); err == nil && val == 1 {
						p.state.MaxStamina = 100 // Percentage based
						if p.debug {
							fmt.Printf("[DEBUG] Fatigue: %d%%\n", p.state.Stamina)
						}
					}
				}
			}
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