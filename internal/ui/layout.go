package ui

import (
	"strings"

	"dr-charm/internal/dragonrealms"
	"github.com/charmbracelet/lipgloss"
)

var paneOrder = [...]string{"main", "room", "hands", "familiar"}

type layout struct {
	borderStyle lipgloss.Style
}

func newLayout() layout {
	return layout{
		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")),
	}
}

func familiarAvailable(output []string) bool {
	return len(output) > 0 && output[0] != ""
}

func (l layout) render(width, height int, snapshot dragonrealms.Snapshot, mainOutput, familiarOutput []string, activePane string) string {
	leftWidth := int(float64(width) * 0.7)
	rightWidth := width - leftWidth - 1

	hasFamiliar := familiarAvailable(familiarOutput)
	var roomHeight, handsHeight, familiarHeight int
	if hasFamiliar {
		roomHeight = int(float64(height) * 0.3)
		handsHeight = int(float64(height) * 0.2)
		familiarHeight = height - roomHeight - handsHeight - 2
	} else {
		roomHeight = int(float64(height) * 0.5)
		handsHeight = height - roomHeight - 1
	}

	mainView := l.renderPane("main", "Game", leftWidth, height, mainOutput, activePane)
	rightPanes := []string{
		l.renderPane("room", "Room", rightWidth, roomHeight, roomContent(snapshot), activePane),
		l.renderPane("hands", "Hands", rightWidth, handsHeight, handsContent(snapshot), activePane),
	}
	if hasFamiliar {
		rightPanes = append(rightPanes, l.renderPane("familiar", "Familiar", rightWidth, familiarHeight, familiarOutput, activePane))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, mainView, lipgloss.JoinVertical(lipgloss.Left, rightPanes...))
}

func (l layout) renderPane(id, title string, width, height int, lines []string, activePane string) string {
	style := l.borderStyle
	if id == activePane {
		style = style.BorderForeground(lipgloss.Color("170"))
	}

	contentWidth := max(width-4, 0)
	contentHeight := height - 4
	titleLine := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Width(contentWidth).
		Align(lipgloss.Center).
		Render(title)

	var body string
	switch id {
	case "room":
		body = renderRoomContent(lines, contentHeight-1)
	case "hands":
		body = renderHandsContent(lines, contentHeight-1)
	default:
		body = renderPlainContent(lines, contentHeight-1)
	}
	content := titleLine + "\n" + body
	contentLines := strings.Split(content, "\n")
	limit := max(height-2, 0)
	if len(contentLines) > limit {
		content = strings.Join(contentLines[:limit], "\n")
	}
	return style.Width(width).Render(content)
}

func roomContent(snapshot dragonrealms.Snapshot) []string {
	var lines []string
	room := snapshot.Room
	if room.Title != "" {
		lines = append(lines, room.Title, "")
	}
	if room.Description != "" {
		lines = append(lines, strings.TrimSpace(room.Description), "")
	}
	validExits := make([]string, 0, len(room.Exits))
	for _, exit := range room.Exits {
		if exit != "" {
			validExits = append(validExits, exit)
		}
	}
	if len(validExits) > 0 {
		lines = append(lines, "Exits: "+strings.Join(validExits, ", "))
	}
	if len(room.Objects) > 0 {
		lines = append(lines, "", "You also see:")
		for _, object := range room.Objects {
			lines = append(lines, "  "+object)
		}
	}
	if len(room.Players) > 0 {
		lines = append(lines, "", "Also here:")
		for _, player := range room.Players {
			lines = append(lines, "  "+player)
		}
	}
	if len(room.Creatures) > 0 {
		lines = append(lines, "", "Creatures:")
		for _, creature := range room.Creatures {
			lines = append(lines, "  "+creature)
		}
	}
	return lines
}

func handsContent(snapshot dragonrealms.Snapshot) []string {
	lines := []string{
		"Right: " + snapshot.Hands.Right,
		"Left: " + snapshot.Hands.Left,
	}
	if snapshot.PreparedSpell != "" {
		lines = append(lines, "", "Spell: "+snapshot.PreparedSpell)
	}
	return lines
}

func renderRoomContent(content []string, maxLines int) string {
	lines := make([]string, len(content))
	for index, line := range content {
		switch {
		case strings.HasPrefix(line, "Exits:"):
			lines[index] = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(line)
		case strings.HasPrefix(line, "You also see"):
			lines[index] = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(line)
		default:
			lines[index] = line
		}
	}
	return renderPlainContent(lines, maxLines)
}

func renderHandsContent(content []string, maxLines int) string {
	lines := make([]string, len(content))
	for index, line := range content {
		if strings.Contains(line, "Right:") || strings.Contains(line, "Left:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("170")).Render(parts[0] + ":")
				lines[index] = label + " " + parts[1]
				continue
			}
		}
		lines[index] = line
	}
	return renderPlainContent(lines, maxLines)
}

func renderPlainContent(content []string, maxLines int) string {
	lines := content
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
