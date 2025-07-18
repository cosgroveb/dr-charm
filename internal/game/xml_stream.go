package game

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dr-charm/internal/social"
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
	rawLog     *os.File    // Keep file open instead of opening/closing each time
	gameClient *GameClient // Reference to game client for social events
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

	// Open raw log file once if in debug mode
	if debug {
		home, _ := os.UserHomeDir()
		logDir := filepath.Join(home, ".dr-charm", "logs", "debug")
		os.MkdirAll(logDir, 0755)

		filename := fmt.Sprintf("raw-xml-%s.log", time.Now().Format("2006-01-02"))
		logPath := filepath.Join(logDir, filename)

		if rawLog, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			p.rawLog = rawLog
		}
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
	// Don't register preset as a handler - we need to handle it specially for text output
	// p.handlers["preset"] = p.handlePreset
	p.handlers["openDialog"] = p.handleSkipElement
	p.handlers["closeDialog"] = p.handleSkipElement
	p.handlers["exposeDialog"] = p.handleSkipElement
	p.handlers["logData"] = p.handleSkipElement
	p.handlers["exp"] = p.handleSkipElement
	p.handlers["dir"] = p.handleSkipElement
	p.handlers["style"] = p.handleSkipElement
	p.handlers["pushBold"] = p.handleSkipElement
	p.handlers["popBold"] = p.handleSkipElement
	p.handlers["output"] = p.handleSkipElement
	p.handlers["cmdPrompt"] = p.handleSkipElement
	p.handlers["compDef"] = p.handleSkipElement
	p.handlers["inv"] = p.handleSkipElement
	p.handlers["clearContainer"] = p.handleSkipElement
	p.handlers["mode"] = p.handleSkipElement

	return p
}

// SetGameClient sets the game client reference for social event handling
func (p *XMLStreamParser) SetGameClient(gc *GameClient) {
	p.gameClient = gc
	if gc != nil {
		p.state.Character = gc.GetCharacter()
	}
}

// ParseChunk processes a chunk of XML data
func (p *XMLStreamParser) ParseChunk(data []byte) (string, error) {
	// Update activity on game client when we receive data
	if p.gameClient != nil {
		p.gameClient.UpdateActivity()
	}

	var parseStart time.Time
	if p.debug {
		parseStart = time.Now()
	}
	// Log raw XML data to file when debug is enabled
	if p.debug && p.rawLog != nil {
		p.rawLog.Write([]byte("\n=== NEW CHUNK ===\n"))
		p.rawLog.Write(data)
		p.rawLog.Write([]byte("\n=== END CHUNK ===\n"))
		// Don't close - keep file open
	}

	// Create a temporary buffer for this chunk
	tempBuf := bytes.NewBuffer(nil)
	tempBuf.Write(p.buffer.Bytes()) // Add any leftover from previous chunk
	tempBuf.Write(data)             // Add new data

	// Debug: log raw data for problematic rooms
	if p.debug && bytes.Contains(data, []byte("Via Iltesh")) {
		fmt.Printf("[DEBUG] Raw data containing Via Iltesh:\n%s\n", string(data))
	}

	decoder := xml.NewDecoder(tempBuf)
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var output strings.Builder
	lastGoodPos := int64(0)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			// Clear the buffer - we processed everything
			p.buffer.Reset()
			break
		}
		if err != nil {
			// Get current position in buffer
			currentPos := decoder.InputOffset()

			// Save unparsed data for next chunk
			allBytes := tempBuf.Bytes()
			if currentPos > 0 && int(currentPos) <= len(allBytes) {
				// We made some progress, save from current position
				remaining := allBytes[currentPos:]
				p.buffer.Reset()
				p.buffer.Write(remaining)
			} else if int(lastGoodPos) < len(allBytes) {
				// No progress made, save everything from last good position
				remaining := allBytes[lastGoodPos:]
				p.buffer.Reset()
				p.buffer.Write(remaining)
			} else {
				// Can't determine good position, clear buffer
				p.buffer.Reset()
			}
			break
		}

		// Update last good position after successful token
		lastGoodPos = decoder.InputOffset()

		switch t := token.(type) {
		case xml.StartElement:
			if handler, ok := p.handlers[t.Name.Local]; ok {
				if err := handler(decoder, t, p.state); err != nil && p.debug {
					fmt.Printf("[DEBUG] Error handling %s: %v\n", t.Name.Local, err)
				}
			} else if t.Name.Local == "d" {
				// Special handling for <d> direction tags - include their content in output
				var direction string
				decoder.DecodeElement(&direction, &t)
				if direction != "" {
					output.WriteString(direction)
				}
			} else if t.Name.Local == "preset" {
				// Handle preset tags (like room descriptions) - include in output
				var id string
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
						break
					}
				}

				var content string
				decoder.DecodeElement(&content, &t)

				if p.debug {
					fmt.Printf("[DEBUG] Found preset tag id='%s', content length=%d\n", id, len(content))
				}

				// Include preset content in output AND update game state
				if id == "roomDesc" && content != "" {
					// Update game state
					p.state.Room.Description = content

					// Add newline before room description if we have content
					if output.Len() > 0 {
						output.WriteString("\n")
					}
					output.WriteString(content)
					if p.debug {
						fmt.Printf("[DEBUG] Added room description to output and state\n")
					}
				} else if id == "speech" || id == "whisper" || id == "thought" {
					// Handle social presets
					if p.debug {
						fmt.Printf("[DEBUG] Detected social preset: id=%s content=%s\n", id, content)
					}

					// Parse the social interaction
					if p.gameClient != nil && content != "" {
						playerName := p.state.GetPlayerName()
						if event := social.ParseSocialInteraction(id, content, playerName); event != nil {
							p.gameClient.AddSocialEvent(event)
							if p.debug {
								fmt.Printf("[DEBUG] Parsed social event: %s from %s\n", event.Subtype, event.From)
							}
						}
					}

					// Include in output
					if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
						output.WriteString("\n")
					}
					output.WriteString(content)
				}
			} else if t.Name.Local == "pushBold" || t.Name.Local == "popBold" {
				// Skip bold formatting tags
				decoder.Skip()
			} else {
				// Skip unknown elements
				decoder.Skip()
			}

		case xml.CharData:
			text := string(t)
			if p.inStream {
				p.streamText.WriteString(text)
			} else if text != "" {
				// Skip pure whitespace but preserve spaces within text
				trimmed := strings.TrimSpace(text)
				if trimmed != "" {
					// Filter out text that looks like XML attributes or fragments
					if !strings.Contains(trimmed, "='") && !strings.Contains(trimmed, "=\"") &&
						!strings.HasSuffix(trimmed, ">") && !strings.HasPrefix(trimmed, "<") {
						output.WriteString(text) // Use original text to preserve formatting
					}
				}
			}

		case xml.EndElement:
			if t.Name.Local == "prompt" {
				output.WriteString(">")
			}
		}
	}

	result := output.String()

	if p.debug && !parseStart.IsZero() {
		elapsed := time.Since(parseStart)
		if elapsed > 10*time.Millisecond {
			fmt.Printf("[PERF] XML parsing took %v for %d bytes, output %d chars\n",
				elapsed, len(data), len(result))
		}
	}

	return result, nil
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
		if p.debug {
			fmt.Printf("[DEBUG] Set room title from streamwindow: %s\n", state.Room.Title)
		}
	}

	decoder.Skip()
	return nil
}

func (p *XMLStreamParser) handleDialogData(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	// DialogData is a container element, we need to skip it after processing
	// to prevent its content from being treated as text
	decoder.Skip()
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
	// Only clear exits if we're going to add new ones
	// Some rooms have empty compass elements but exits in the text
	foundExits := []string{}

	if p.debug {
		fmt.Printf("[DEBUG] Parsing compass element\n")
	}

	// Parse compass directions
	for {
		token, err := decoder.Token()
		if err != nil {
			if p.debug {
				fmt.Printf("[DEBUG] Error in compass parsing: %v\n", err)
			}
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "dir" {
				for _, attr := range t.Attr {
					if attr.Name.Local == "value" {
						if p.debug {
							fmt.Printf("[DEBUG] Found dir with value: '%s' (empty: %v)\n", attr.Value, attr.Value == "")
						}
						if attr.Value != "" {
							foundExits = append(foundExits, attr.Value)
						}
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "compass" {
				// Only update exits if we found any in the compass element
				if len(foundExits) > 0 {
					state.Room.Exits = foundExits
					if p.debug {
						fmt.Printf("[DEBUG] Compass parsing complete. Found exits: %v\n", foundExits)
					}
				} else if p.debug {
					fmt.Printf("[DEBUG] Compass parsing complete. No exits found, keeping existing.\n")
				}
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

	// For room exits, we need to parse the nested XML content
	if id == "room exits" {
		var exits []string
		var textContent strings.Builder

		// Parse the content manually to handle nested <d> tags
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}

			switch t := token.(type) {
			case xml.StartElement:
				if t.Name.Local == "d" {
					// Found a direction tag, extract its content
					var direction string
					decoder.DecodeElement(&direction, &t)
					if direction != "" {
						exits = append(exits, direction)
					}
				} else if t.Name.Local == "compass" {
					// Skip the compass element, it's handled separately
					decoder.Skip()
				}
			case xml.CharData:
				// Collect text content
				textContent.WriteString(string(t))
			case xml.EndElement:
				if t.Name.Local == "component" {
					// We've reached the end
					goto done
				}
			}
		}

	done:
		if len(exits) > 0 {
			state.Room.Exits = exits
			state.Room.ExitsString = strings.Join(exits, ", ")
			if p.debug {
				fmt.Printf("[DEBUG] Parsed exits from component: %v\n", exits)
			}
		}

		return nil
	}

	// For other components, use the standard decode
	var content string
	decoder.DecodeElement(&content, &start)

	switch id {
	case "room objs":
		// Clean up bold tags before parsing
		content = strings.ReplaceAll(content, "<pushBold/>", "")
		content = strings.ReplaceAll(content, "<popBold/>", "")
		state.Room.Objects = parseRoomList(content)
	case "room players":
		// Clean up bold tags before parsing
		content = strings.ReplaceAll(content, "<pushBold/>", "")
		content = strings.ReplaceAll(content, "<popBold/>", "")
		state.Room.Players = parseRoomList(content)
	case "room desc":
		// Sometimes room description comes in component too
		if content != "" {
			state.Room.Description = content
		}
	}

	return nil
}

func (p *XMLStreamParser) handlePreset(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	var id string
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			id = attr.Value
			break
		}
	}

	// Get the preset content
	var content string
	decoder.DecodeElement(&content, &start)

	// Handle room description preset
	if id == "roomDesc" && content != "" {
		state.Room.Description = content
		if p.debug {
			fmt.Printf("[DEBUG] Set room description from preset: %s\n", content)
		}
	} else if p.debug && id == "roomDesc" {
		fmt.Printf("[DEBUG] Empty room description preset\n")
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

// handleSkipElement is a generic handler for elements we want to skip
func (p *XMLStreamParser) handleSkipElement(decoder *xml.Decoder, start xml.StartElement, state *GameState) error {
	decoder.Skip()
	return nil
}

// GetState returns the current game state
func (p *XMLStreamParser) GetState() *GameState {
	return p.state
}

// Close closes any open resources
func (p *XMLStreamParser) Close() {
	if p.rawLog != nil {
		p.rawLog.Close()
		p.rawLog = nil
	}
}

// IsDebug returns whether debug mode is enabled
func (p *XMLStreamParser) IsDebug() bool {
	return p.debug
}
