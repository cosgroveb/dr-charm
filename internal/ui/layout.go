package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/x/ansi"
)

func renderDashboard(width, height int, update presentation.Update, status, input string, mapLines []string, panLine, panColumn int, navigating bool, theme theme) string {
	rows, _ := dashboardRows(width, height, update, status, input, mapLines, panLine, panColumn, navigating)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Foreground)).Width(width).Render(strings.Join(rows, "\n"))
}

func dashboardRows(width, height int, update presentation.Update, status, input string, mapLines []string, panLine, panColumn int, navigating bool) ([]string, bool) {
	location := update.Location.Title
	if len(update.Location.Exits) > 0 {
		location += "  Exits: " + strings.Join(update.Location.Exits, ", ")
	}
	hands := "L: " + emptyHand(update.Hands.Left) + "  R: " + emptyHand(update.Hands.Right)
	if update.Hands.PreparedSpell != "" {
		hands += "  Sp: " + update.Hands.PreparedSpell
	}
	available := max(0, height-12)
	metadata := []string{truncate(location, width), truncate(hands, width), truncate(status, width)}
	mapSide := mapSideRows(update, location, navigating, width)
	mapHeight := min(8, available-2)
	physicalMapRows := max(mapHeight, len(mapSide))
	if len(mapLines) > 0 && mapHeight >= 5 && physicalMapRows+2 <= available {
		rows := mapRows(width, cropMap(mapLines, panLine, panColumn, mapHeight, mapWidth(width)), mapSide, physicalMapRows)
		rows = append(rows, truncate(status, width), truncate(input, width))
		return rows, true
	}
	rows := make([]string, 0, min(available, len(metadata))+1)
	for _, row := range metadata {
		if len(rows)+1 >= available {
			break
		}
		rows = append(rows, row)
	}
	rows = append(rows, truncate(input, width))
	return rows, false
}

func mapSideRows(update presentation.Update, location string, navigating bool, width int) []string {
	if navigating {
		location += "  map navigation"
	}
	left := "L: " + emptyHand(update.Hands.Left)
	if update.Hands.PreparedSpell != "" {
		left += "  Sp: " + update.Hands.PreparedSpell
	}
	right := "R: " + emptyHand(update.Hands.Right)
	return []string{truncate(location, width), truncate(left, width), truncate(right, width)}
}

func mapRows(width int, lines, side []string, height int) []string {
	mapWidth := mapWidth(width)
	sideWidth := max(0, width-mapWidth-2)
	rows := make([]string, height)
	for index := range rows {
		line := ""
		if index < len(lines) {
			line = lines[index]
		}
		line = truncate(line, mapWidth)
		if index >= len(side) || sideWidth == 0 {
			rows[index] = line
			continue
		}
		rows[index] = line + strings.Repeat(" ", max(0, mapWidth-ansi.StringWidth(line))) + "  " + truncate(side[index], sideWidth)
	}
	return rows
}

func mapWidth(width int) int {
	if width <= 2 {
		return width
	}
	return max(1, width*2/3)
}

func emptyHand(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}
func cropMap(lines []string, line, column, height, width int) []string {
	if height <= 0 || width <= 0 {
		return nil
	}
	line = min(max(line, 0), len(lines)-1)
	start := min(max(0, line-height/2), max(0, len(lines)-height))
	mapWidth := 0
	for _, value := range lines {
		mapWidth = max(mapWidth, ansi.StringWidth(value))
	}
	column = min(max(0, column-width/2), max(0, mapWidth-width))
	result := make([]string, 0, min(height, len(lines)-start))
	for _, value := range lines[start:min(len(lines), start+height)] {
		result = append(result, ansi.Cut(value, column, column+width))
	}
	return result
}
