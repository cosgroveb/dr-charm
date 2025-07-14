package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// XMLStreamParser handles streaming XML parsing for DragonRealms protocol
type XMLStreamParser struct {
	state      *GameState
	buffer     bytes.Buffer
	inStream   bool
	streamID   string
	streamText strings.Builder
	handlers   map[string]XMLHandler
	debug      bool
}

// XMLHandler processes specific XML elements
type XMLHandler func(decoder *xml.Decoder, start xml.StartElement, state *GameState) error

// NewXMLStreamParser creates a new streaming XML parser
func NewXMLStreamParser(debug bool) *XMLStreamParser {
	p := &XMLStreamParser{
		state:    &GameState{},
		debug:    debug,
		handlers: make(map[string]XMLHandler),
	}

	// Register handlers for different XML elements
	p.handlers["prompt"] = p.handlePrompt
	p.handlers["pushStream"] = p.handlePushStream
	p.handlers["popStream"] = p.handlePopStream
	p.handlers["clearStream"] = p.handleClearStream
	p.handlers["streamwindow"] = p.handleStreamWindow
	p.handlers["dialogData"] = p.handleDialogData
	p.handlers["progressBar"] = p.handleProgressBar
	p.handlers["roundTime"] = p.handleRoundTime
	p.handlers["castTime"] = p.handleCastTime
	p.handlers["left"] = p.handleHand
	p.handlers["right"] = p.handleHand
	p.handlers["spell"] = p.handleSpell
	p.handlers["room"] = p.handleRoom
	p.handlers["compass"] = p.handleCompass
	p.handlers["component"] = p.handleComponent
	p.handlers["indicator"] = p.handleIndicator

	return p
}

// ParseChunk processes a chunk of XML data
func (p *XMLStreamParser) ParseChunk(data []byte) (string, error) {
	p.buffer.Write(data)

	decoder := xml.NewDecoder(&p.buffer)
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var output strings.Builder

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Handle partial XML by keeping unparsed data in buffer
			remaining := decoder.InputOffset()
			if remaining > 0 {
				parsed := p.buffer.Next(int(remaining))
				p.buffer.Reset()
				p.buffer.Write(parsed[remaining:])
			}
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if handler, ok := p.handlers[t.Name.Local]; ok {
				if err := handler(decoder, t, p.state); err != nil && p.debug {
					fmt.Printf("[DEBUG] Error handling %s: %v\n", t.Name.Local, err)
				}
			} else {
				// Skip unknown elements
				decoder.Skip()
			}

		case xml.CharData:
			text := string(t)
			if p.inStream {
				p.streamText.WriteString(text)
			} else if strings.TrimSpace(text) != "" {
				output.WriteString(text)
			}

		case xml.EndElement:
			if t.Name.Local == "prompt" {
				output.WriteString(">")
			}
		}
	}

	return output.String(), nil
}

// Handler implementations
func (p *XMLStreamParser) handlePrompt(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	// Update game time
	for _, attr := range start.Attr {
		if attr.Name.Local == "time" {
			state.GameTime = attr.Value
		}
	}

	// The prompt content is usually empty, actual prompt comes from game
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handlePushStream(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			p.streamID = attr.Value
			p.inStream = true
			p.streamText.Reset()
		}
	}
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handlePopStream(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	p.inStream = false
	// Process accumulated stream text
	if p.streamText.Len() > 0 {
		switch p.streamID {
		case "room":
			state.Room.Description = p.streamText.String()
		case "familiar":
			state.FamiliarWindow = p.streamText.String()
		}
	}
	p.streamID = ""
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleClearStream(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			switch attr.Value {
			case "familiar":
				state.FamiliarWindow = ""
			}
		}
	}
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleStreamWindow(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var windowID, subtitle string

	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "id":
			windowID = attr.Value
		case "subtitle":
			subtitle = attr.Value
		}
	}

	if windowID == "main" && strings.HasPrefix(subtitle, " - ") {
		state.Room.Title = subtitle[3:]
	}

	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleDialogData(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	// DialogData contains child elements, don't skip
	return nil
}

func (p *XMLStreamParser) handleProgressBar(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var id, value string

	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "id":
			id = attr.Value
		case "value":
			value = attr.Value
		}
	}

	if val, err := strconv.Atoi(value); err == nil {
		switch id {
		case "health", "health2":
			state.Health = val
			state.MaxHealth = 100
		case "mana", "mana2":
			state.Mana = val
			state.MaxMana = 100
		case "stamina", "fatigue":
			state.Stamina = val
			state.MaxStamina = 100
		case "concentration":
			state.Concentration = val
			state.MaxConcentration = 100
		case "spirit":
			state.Spirit = val
			state.MaxSpirit = 100
		}
	}

	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleRoundTime(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "value" {
			if val, err := strconv.Atoi(attr.Value); err == nil {
				state.Roundtime = val
			}
		}
	}
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleCastTime(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "value" {
			if val, err := strconv.Atoi(attr.Value); err == nil {
				state.CastRT = val
			}
		}
	}
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleHand(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var content string
	decoder.DecodeElement(&content, &start)

	switch start.Name.Local {
	case "left":
		state.LeftHand = content
		if content == "" {
			state.LeftHand = "Empty"
		}
	case "right":
		state.RightHand = content
		if content == "" {
			state.RightHand = "Empty"
		}
	}

	return nil
}

func (p *XMLStreamParser) handleSpell(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var spell string
	decoder.DecodeElement(&spell, &start)
	state.Spell = spell
	return nil
}

func (p *XMLStreamParser) handleRoom(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			state.Room.ID = attr.Value
		}
	}
	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleCompass(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	state.Room.Exits = []string{}

	// Parse compass directions
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "dir" {
				for _, attr := range t.Attr {
					if attr.Name.Local == "value" && attr.Value != "" {
						state.Room.Exits = append(state.Room.Exits, attr.Value)
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "compass" {
				return nil
			}
		}
	}

	return nil
}

func (p *XMLStreamParser) handleComponent(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var id string
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			id = attr.Value
			break
		}
	}

	var content string
	decoder.DecodeElement(&content, &start)

	switch id {
	case "room objs":
		state.Room.Objects = parseRoomList(content)
	case "room players":
		state.Room.Players = parseRoomList(content)
	case "room exits":
		exits := strings.TrimSpace(content)
		if exits != "" {
			state.Room.ExitsString = exits
		}
	}

	return nil
}

func (p *XMLStreamParser) handleIndicator(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var id, visible string

	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "id":
			id = attr.Value
		case "visible":
			visible = attr.Value
		}
	}

	// Update indicators (standing, kneeling, prone, etc.)
	if strings.HasPrefix(id, "Icon") && len(id) > 4 {
		indicator := strings.ToLower(id[4:])
		isVisible := visible == "y"

		// Update stance based on visible indicators
		switch indicator {
		case "standing":
			if isVisible {
				state.Stance = 3
			}
		case "kneeling":
			if isVisible {
				state.Stance = 2
			}
		case "sitting":
			if isVisible {
				state.Stance = 1
			}
		case "prone":
			if isVisible {
				state.Stance = 0
			}
		}
	}

	decoder.Skip()
	return nil
}

// Helper function to parse room lists
func parseRoomList(content string) []string {
	lines := strings.Split(content, "\n")
	var items []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "You also see") {
			items = append(items, line)
		}
	}

	return items
}

// GetState returns the current game state
func (p *XMLStreamParser) GetState() *GameState {
	return p.state
}
