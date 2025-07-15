package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Pane represents a window pane in the layout
type Pane struct {
	ID      string
	Title   string
	Content []string
	Width   int
	Height  int
	Style   lipgloss.Style
	Focused bool
}

// Layout manages the multi-pane layout
type Layout struct {
	panes       map[string]*Pane
	width       int
	height      int
	activePane  string
	borderStyle lipgloss.Style
	debug       bool
}

// NewLayout creates a new layout manager
func NewLayout(width, height int, debug bool) *Layout {
	l := &Layout{
		panes:  make(map[string]*Pane),
		width:  width,
		height: height,
		debug:  debug,
		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")),
	}

	// Create default panes
	l.createDefaultPanes()

	return l
}

// createDefaultPanes sets up the standard DR layout
func (l *Layout) createDefaultPanes() {
	// Main game output (left side, takes most space)
	mainPane := &Pane{
		ID:    "main",
		Title: "Game",
		Style: l.borderStyle.Copy(),
	}
	l.panes["main"] = mainPane

	// Room window (top right)
	roomPane := &Pane{
		ID:    "room",
		Title: "Room",
		Style: l.borderStyle.Copy(),
	}
	l.panes["room"] = roomPane

	// Hands window (middle right)
	handsPane := &Pane{
		ID:    "hands",
		Title: "Hands",
		Style: l.borderStyle.Copy(),
	}
	l.panes["hands"] = handsPane

	// Familiar window (bottom right)
	familiarPane := &Pane{
		ID:    "familiar",
		Title: "Familiar",
		Style: l.borderStyle.Copy(),
	}
	l.panes["familiar"] = familiarPane

	// Set main as active
	l.activePane = "main"
	mainPane.Focused = true
}

// Resize updates layout dimensions
func (l *Layout) Resize(width, height int) {
	l.width = width
	l.height = height
}

// UpdatePane updates content for a specific pane
func (l *Layout) UpdatePane(paneID string, content []string) {
	if pane, ok := l.panes[paneID]; ok {
		pane.Content = content
	}
}

// AddLineToPane adds a single line to a pane
func (l *Layout) AddLineToPane(paneID string, line string) {
	if pane, ok := l.panes[paneID]; ok {
		pane.Content = append(pane.Content, line)
		// Keep reasonable history
		if len(pane.Content) > 100 {
			pane.Content = pane.Content[len(pane.Content)-100:]
		}
	}
}

// SetActivePane changes the focused pane
func (l *Layout) SetActivePane(paneID string) {
	// Unfocus previous
	if prev, ok := l.panes[l.activePane]; ok {
		prev.Focused = false
	}

	// Focus new
	if pane, ok := l.panes[paneID]; ok {
		l.activePane = paneID
		pane.Focused = true
	}
}

// NextPane cycles to the next pane
func (l *Layout) NextPane() {
	paneOrder := []string{"main", "room", "hands", "familiar"}

	for i, id := range paneOrder {
		if id == l.activePane {
			nextIdx := (i + 1) % len(paneOrder)
			l.SetActivePane(paneOrder[nextIdx])
			break
		}
	}
}

// Render creates the full layout view
func (l *Layout) Render() string {
	return l.RenderWithHeight(l.height)
}

// RenderWithHeight creates the layout view with a specific height
func (l *Layout) RenderWithHeight(height int) string {
	// Calculate pane dimensions
	// Left column (main) takes 70% width
	leftWidth := int(float64(l.width) * 0.7)
	rightWidth := l.width - leftWidth - 1 // -1 for gap

	// Heights for right column panes
	roomHeight := int(float64(height) * 0.3)
	handsHeight := int(float64(height) * 0.2)
	familiarHeight := height - roomHeight - handsHeight - 2 // -2 for gaps

	// Update pane dimensions
	if mainPane, ok := l.panes["main"]; ok {
		mainPane.Width = leftWidth
		mainPane.Height = height
	}

	if roomPane, ok := l.panes["room"]; ok {
		roomPane.Width = rightWidth
		roomPane.Height = roomHeight
	}

	if handsPane, ok := l.panes["hands"]; ok {
		handsPane.Width = rightWidth
		handsPane.Height = handsHeight
	}

	if familiarPane, ok := l.panes["familiar"]; ok {
		familiarPane.Width = rightWidth
		familiarPane.Height = familiarHeight
	}

	// Render panes
	mainView := l.renderPane("main")
	roomView := l.renderPane("room")
	handsView := l.renderPane("hands")
	familiarView := l.renderPane("familiar")

	// Combine right column
	rightColumn := lipgloss.JoinVertical(lipgloss.Left,
		roomView,
		handsView,
		familiarView,
	)

	// Combine columns
	return lipgloss.JoinHorizontal(lipgloss.Top,
		mainView,
		rightColumn,
	)
}

// renderPane renders a single pane
func (l *Layout) renderPane(paneID string) string {
	pane, ok := l.panes[paneID]
	if !ok {
		return ""
	}

	// Update border style for focused pane
	style := pane.Style.Copy()
	if pane.Focused {
		style = style.BorderForeground(lipgloss.Color("170")) // Bright purple for focused
	}

	// Calculate content area
	contentWidth := pane.Width - 4   // -4 for borders and padding
	contentHeight := pane.Height - 4 // -4 for borders, padding, and title

	// Render content
	var content strings.Builder

	// Add title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Width(contentWidth).
		Align(lipgloss.Center)

	content.WriteString(titleStyle.Render(pane.Title))
	content.WriteString("\n")

	// Add pane-specific content
	switch paneID {
	case "room":
		content.WriteString(l.renderRoomContent(pane, contentHeight-1))
	case "hands":
		content.WriteString(l.renderHandsContent(pane, contentHeight-1))
	case "familiar":
		content.WriteString(l.renderFamiliarContent(pane, contentHeight-1))
	default:
		content.WriteString(l.renderGenericContent(pane, contentHeight-1))
	}

	// Apply style with dimensions
	return style.
		Width(pane.Width).
		Height(pane.Height).
		Render(content.String())
}

// renderRoomContent renders the room pane content
func (l *Layout) renderRoomContent(pane *Pane, maxLines int) string {
	var lines []string

	// Room content is typically structured
	for _, line := range pane.Content {
		if strings.HasPrefix(line, "Exits:") {
			// Highlight exits
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color("46")).
				Render(line))
		} else if strings.HasPrefix(line, "You also see") {
			// Highlight objects
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Render(line))
		} else {
			lines = append(lines, line)
		}
	}

	// Trim to fit
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return strings.Join(lines, "\n")
}

// renderHandsContent renders the hands pane content
func (l *Layout) renderHandsContent(pane *Pane, maxLines int) string {
	var lines []string

	for _, line := range pane.Content {
		if strings.Contains(line, "Right:") || strings.Contains(line, "Left:") {
			// Style hand labels
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				label := lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("170")).
					Render(parts[0] + ":")
				lines = append(lines, label+" "+parts[1])
			} else {
				lines = append(lines, line)
			}
		} else {
			lines = append(lines, line)
		}
	}

	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return strings.Join(lines, "\n")
}

// renderFamiliarContent renders the familiar pane content
func (l *Layout) renderFamiliarContent(pane *Pane, maxLines int) string {
	lines := pane.Content
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

// renderGenericContent renders generic pane content
func (l *Layout) renderGenericContent(pane *Pane, maxLines int) string {
	lines := pane.Content
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

// UpdateFromGameState updates panes based on game state
func (l *Layout) UpdateFromGameState(state *GameState) {
	// Debug output
	if l.debug {
		fmt.Printf("[DEBUG] UpdateFromGameState - Title: '%s', Desc: '%s', Exits: %v, Objects: %v\n", 
		    state.Room.Title, state.Room.Description, state.Room.Exits, state.Room.Objects)
	}
	
	// Update room pane
	var roomContent []string
	if state.Room.Title != "" {
		roomContent = append(roomContent, state.Room.Title)
		roomContent = append(roomContent, "")
	}
	if state.Room.Description != "" {
		// Split long descriptions into paragraphs for better readability
		desc := strings.TrimSpace(state.Room.Description)
		roomContent = append(roomContent, desc)
		roomContent = append(roomContent, "")
	}
	if state.Room.ExitsString != "" {
		// Clean up the exits string - remove empty entries
		exits := strings.TrimSpace(state.Room.ExitsString)
		if exits != "" && exits != "." && exits != ", , ." {
			roomContent = append(roomContent, "Exits: "+exits)
		}
	} else if len(state.Room.Exits) > 0 {
		// Filter out empty exits
		var validExits []string
		for _, exit := range state.Room.Exits {
			if exit != "" {
				validExits = append(validExits, exit)
			}
		}
		if len(validExits) > 0 {
			roomContent = append(roomContent, "Exits: "+strings.Join(validExits, ", "))
		}
	}
	if len(state.Room.Objects) > 0 {
		roomContent = append(roomContent, "")
		roomContent = append(roomContent, "You also see:")
		for _, obj := range state.Room.Objects {
			roomContent = append(roomContent, "  "+obj)
		}
	}
	if len(state.Room.Players) > 0 {
		roomContent = append(roomContent, "")
		roomContent = append(roomContent, "Also here:")
		for _, player := range state.Room.Players {
			roomContent = append(roomContent, "  "+player)
		}
	}
	// fmt.Printf("[DEBUG] Updating room pane with %d lines\n", len(roomContent))
	// if len(roomContent) > 0 {
	//     fmt.Printf("[DEBUG] First line: %s\n", roomContent[0])
	// }
	l.UpdatePane("room", roomContent)

	// Update hands pane
	var handsContent []string
	handsContent = append(handsContent, fmt.Sprintf("Right: %s", state.RightHand))
	handsContent = append(handsContent, fmt.Sprintf("Left: %s", state.LeftHand))
	if state.Spell != "" {
		handsContent = append(handsContent, "")
		handsContent = append(handsContent, fmt.Sprintf("Spell: %s", state.Spell))
	}
	l.UpdatePane("hands", handsContent)

	// Update familiar pane if there's content
	if state.FamiliarWindow != "" {
		l.UpdatePane("familiar", strings.Split(state.FamiliarWindow, "\n"))
	}
}
