package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/x/ansi"
)

const transcriptRows = 12

type dashboardGeometry struct {
	width, rows                    int
	innerWidth, contentWidth       int
	mapWidth, equipmentWidth       int
	mapHeight, padding, inputWidth int
	mapVisible, chrome             bool
}

func calculateDashboardGeometry(width, height int, mapAvailable bool, currentTheme theme, inputLabel string) dashboardGeometry {
	width = max(0, width)
	available := max(1, height-transcriptRows)
	resolved := currentTheme.presentation()
	geometry := dashboardGeometry{width: width, padding: resolved.Padding}

	compactMapHeight := min(8, available-2)
	geometry.mapVisible = mapAvailable && compactMapHeight >= 5 && compactMapHeight+2 <= available
	if geometry.mapVisible {
		geometry.mapHeight = compactMapHeight
		geometry.rows = compactMapHeight + 2
	} else {
		geometry.rows = min(4, available)
	}

	for geometry.padding > 0 && width-2-(2*geometry.padding) < 12 {
		geometry.padding--
	}
	geometry.innerWidth = max(0, width-2)
	geometry.contentWidth = max(0, geometry.innerWidth-(2*geometry.padding))
	geometry.chrome = geometry.contentWidth >= 12
	if geometry.mapVisible {
		geometry.chrome = geometry.chrome && available >= 10
		if geometry.chrome {
			geometry.mapHeight = min(8, available-5)
			geometry.rows = geometry.mapHeight + 5
		}
	} else {
		geometry.chrome = geometry.chrome && available >= 6
		if geometry.chrome {
			geometry.rows = 6
		}
	}

	if geometry.mapVisible {
		usable := geometry.contentWidth
		if geometry.chrome {
			usable = max(0, usable-1)
		}
		geometry.mapWidth = max(0, usable*2/3)
		geometry.equipmentWidth = max(0, usable-geometry.mapWidth)
	}
	geometry.inputWidth = max(1, geometry.contentWidth-ansi.StringWidth(inputLabel)-1)
	if !geometry.chrome {
		geometry.innerWidth = width
		geometry.contentWidth = width
		geometry.padding = 0
		geometry.inputWidth = max(1, width-ansi.StringWidth(inputLabel)-1)
	}
	return geometry
}

func renderDashboard(width, height int, update presentation.Update, status, input string, mapLines []string, panLine, panColumn int, navigating bool, currentTheme theme) string {
	rows, _ := dashboardRows(width, height, update, status, input, mapLines, panLine, panColumn, navigating, currentTheme)
	return strings.Join(rows, "\n")
}

func dashboardRows(width, height int, update presentation.Update, status, input string, mapLines []string, panLine, panColumn int, navigating bool, currentTheme theme) ([]string, bool) {
	resolved := currentTheme.presentation()
	geometry := calculateDashboardGeometry(width, height, len(mapLines) > 0, resolved, "")
	location := locationText(update)
	hands := handsText(update)
	if !geometry.chrome {
		return compactDashboardRows(geometry, update, location, hands, status, input, mapLines, panLine, panColumn, navigating, resolved), geometry.mapVisible
	}
	return framedDashboardRows(geometry, update, location, status, input, mapLines, panLine, panColumn, navigating, resolved), geometry.mapVisible
}

func compactDashboardRows(geometry dashboardGeometry, update presentation.Update, location, hands, status, input string, mapLines []string, panLine, panColumn int, navigating bool, currentTheme theme) []string {
	if geometry.mapVisible {
		mapWidth := mapWidth(geometry.width)
		mapRows := cropMap(mapLines, panLine, panColumn, geometry.mapHeight, mapWidth)
		rows := compactMapRows(geometry.width, mapRows, mapSideRows(update, location, navigating, geometry.width), geometry.mapHeight, currentTheme.Foreground)
		return append(rows,
			paint(status, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg),
			paint(input, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg),
		)
	}
	metadata := []string{location, hands, status}
	rows := make([]string, 0, geometry.rows)
	for index, row := range metadata {
		if len(rows)+1 >= geometry.rows {
			break
		}
		if index == len(metadata)-1 {
			rows = append(rows, paint(row, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg))
		} else {
			rows = append(rows, paint(row, geometry.width, currentTheme.Foreground, ""))
		}
	}
	return append(rows, paint(input, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg))
}

func framedDashboardRows(geometry dashboardGeometry, update presentation.Update, location, status, input string, mapLines []string, panLine, panColumn int, navigating bool, currentTheme theme) []string {
	border := dashboardBorder(currentTheme.BorderType)
	rows := make([]string, 0, geometry.rows)
	if geometry.mapVisible {
		rows = append(rows, splitTopRule(border, geometry, location, navigating, currentTheme))
		cropped := cropMap(mapLines, panLine, panColumn, geometry.mapHeight, geometry.mapWidth)
		equipment := equipmentRows(update)
		for index := 0; index < geometry.mapHeight; index++ {
			mapLine := ""
			if index < len(cropped) {
				mapLine = cropped[index]
			}
			equipmentLine := ""
			if index < len(equipment) {
				equipmentLine = equipment[index]
			}
			leftColor := currentTheme.Border
			dividerColor := currentTheme.Border
			if navigating {
				leftColor = currentTheme.TitleBar
				dividerColor = currentTheme.TitleBar
			}
			rows = append(rows,
				paint(border.Left, 1, leftColor, "")+
					paint(strings.Repeat(" ", geometry.padding)+mapLine, geometry.padding+geometry.mapWidth, currentTheme.Foreground, "")+
					paint(border.Left, 1, dividerColor, "")+
					paint(truncate(equipmentLine, geometry.equipmentWidth)+strings.Repeat(" ", geometry.padding), geometry.equipmentWidth+geometry.padding, currentTheme.Foreground, "")+
					paint(border.Right, 1, currentTheme.Border, ""),
			)
		}
		dividerTitle := ""
		if navigating {
			dividerTitle = "Map navigation"
		}
		rows = append(rows, splitRule(border, geometry, dividerTitle, currentTheme))
	} else {
		rows = append(rows, titledRule(border.TopLeft, border.Top, border.TopRight, geometry.width, location, currentTheme.Border, currentTheme.TitleBar))
		rows = append(rows, framedContentRow(border, geometry, handsText(update), currentTheme))
		rows = append(rows, titledRule(border.MiddleLeft, border.Top, border.MiddleRight, geometry.width, "", currentTheme.Border, currentTheme.TitleBar))
	}
	rows = append(rows, framedStripRow(border, geometry, status, currentTheme))
	rows = append(rows, framedStripRow(border, geometry, input, currentTheme))
	rows = append(rows, titledRule(border.BottomLeft, border.Bottom, border.BottomRight, geometry.width, "", currentTheme.Border, currentTheme.TitleBar))
	return rows
}

func renderModal(width, height int, mapAvailable bool, title string, lines []string, offset int, currentTheme theme) string {
	resolved := currentTheme.presentation()
	geometry := calculateDashboardGeometry(width, height, mapAvailable, resolved, "")
	if !geometry.chrome {
		bodyRows := max(0, geometry.rows-1)
		offset = min(max(offset, 0), max(0, len(lines)-bodyRows))
		rows := make([]string, 0, geometry.rows)
		rows = append(rows, paint(title, width, resolved.TitleBar, ""))
		for _, line := range lines[offset:min(len(lines), offset+bodyRows)] {
			rows = append(rows, paint(line, width, resolved.Foreground, ""))
		}
		for len(rows) < geometry.rows {
			rows = append(rows, paint("", width, resolved.Foreground, ""))
		}
		return strings.Join(rows, "\n")
	}
	border := dashboardBorder(resolved.BorderType)
	bodyRows := geometry.rows - 4
	offset = min(max(offset, 0), max(0, len(lines)-bodyRows))
	visible := lines[offset:min(len(lines), offset+bodyRows)]
	rows := make([]string, 0, geometry.rows)
	rows = append(rows, titledRule(border.TopLeft, border.Top, border.TopRight, width, title, resolved.Border, resolved.TitleBar))
	for index := 0; index < bodyRows; index++ {
		line := ""
		if index < len(visible) {
			line = visible[index]
		}
		rows = append(rows, framedContentRow(border, geometry, line, resolved))
	}
	rows = append(rows, titledRule(border.MiddleLeft, border.Top, border.MiddleRight, width, "", resolved.Border, resolved.TitleBar))
	rows = append(rows, framedStripRow(border, geometry, "Up/Down move  Enter/Esc close", resolved))
	rows = append(rows, titledRule(border.BottomLeft, border.Bottom, border.BottomRight, width, "", resolved.Border, resolved.TitleBar))
	return strings.Join(rows, "\n")
}

func splitTopRule(border lipgloss.Border, geometry dashboardGeometry, title string, navigating bool, currentTheme theme) string {
	leftWidth := geometry.padding + geometry.mapWidth
	rightWidth := geometry.equipmentWidth + geometry.padding
	leftColor := currentTheme.Border
	if navigating {
		leftColor = currentTheme.TitleBar
	}
	return paint(border.TopLeft, 1, leftColor, "") +
		ruleSegment(border.Top, leftWidth, title, leftColor, currentTheme.TitleBar) +
		paint(border.MiddleTop, 1, leftColor, "") +
		ruleSegment(border.Top, rightWidth, "", currentTheme.Border, currentTheme.TitleBar) +
		paint(border.TopRight, 1, currentTheme.Border, "")
}

func framedContentRow(border lipgloss.Border, geometry dashboardGeometry, value string, currentTheme theme) string {
	padding := strings.Repeat(" ", geometry.padding)
	inner := padding + truncate(value, geometry.contentWidth) + padding
	return paint(border.Left, 1, currentTheme.Border, "") +
		paint(inner, geometry.innerWidth, currentTheme.Foreground, "") +
		paint(border.Right, 1, currentTheme.Border, "")
}

func framedStripRow(border lipgloss.Border, geometry dashboardGeometry, value string, currentTheme theme) string {
	padding := strings.Repeat(" ", geometry.padding)
	inner := padding + truncate(value, geometry.contentWidth) + padding
	return paint(border.Left, 1, currentTheme.Border, "") +
		paint(inner, geometry.innerWidth, currentTheme.StatusBar, currentTheme.StatusBarBg) +
		paint(border.Right, 1, currentTheme.Border, "")
}

func splitRule(border lipgloss.Border, geometry dashboardGeometry, title string, currentTheme theme) string {
	leftWidth := geometry.padding + geometry.mapWidth
	rightWidth := geometry.equipmentWidth + geometry.padding
	leftColor := currentTheme.Border
	if title != "" {
		leftColor = currentTheme.TitleBar
	}
	return paint(border.MiddleLeft, 1, leftColor, "") +
		ruleSegment(border.Top, leftWidth, title, leftColor, currentTheme.TitleBar) +
		paint(border.MiddleBottom, 1, leftColor, "") +
		ruleSegment(border.Top, rightWidth, "", currentTheme.Border, currentTheme.TitleBar) +
		paint(border.MiddleRight, 1, currentTheme.Border, "")
}

func titledRule(left, fill, right string, width int, title, ruleColor, titleColor string) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return paint(left, 1, ruleColor, "")
	}
	return paint(left, 1, ruleColor, "") + ruleSegment(fill, width-2, title, ruleColor, titleColor) + paint(right, 1, ruleColor, "")
}

func ruleSegment(fill string, width int, title, ruleColor, titleColor string) string {
	if width <= 0 {
		return ""
	}
	if title == "" || width < 3 {
		return paint(strings.Repeat(fill, width), width, ruleColor, "")
	}
	label := truncate(" "+title+" ", max(0, width-1))
	labelWidth := ansi.StringWidth(label)
	return paint(fill, 1, ruleColor, "") + paint(label, labelWidth, titleColor, "") + paint(strings.Repeat(fill, max(0, width-labelWidth-1)), max(0, width-labelWidth-1), ruleColor, "")
}

func dashboardBorder(kind string) lipgloss.Border {
	switch kind {
	case "normal":
		return lipgloss.NormalBorder()
	case "thick":
		return lipgloss.ThickBorder()
	case "double":
		return lipgloss.DoubleBorder()
	default:
		return lipgloss.RoundedBorder()
	}
}

func paint(value string, width int, foreground, background string) string {
	value = truncate(value, width)
	value += strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
	style := lipgloss.NewStyle()
	if foreground != "" {
		style = style.Foreground(lipgloss.Color(foreground))
	}
	if background != "" {
		style = style.Background(lipgloss.Color(background))
	}
	return style.Render(value)
}

func locationText(update presentation.Update) string {
	location := update.Location.Title
	if len(update.Location.Exits) > 0 {
		location += "  Exits: " + strings.Join(update.Location.Exits, ", ")
	}
	return location
}

func handsText(update presentation.Update) string {
	hands := "L: " + emptyHand(update.Hands.Left) + "  R: " + emptyHand(update.Hands.Right)
	if update.Hands.PreparedSpell != "" {
		hands += "  Sp: " + update.Hands.PreparedSpell
	}
	return hands
}

func equipmentRows(update presentation.Update) []string {
	rows := []string{"L: " + emptyHand(update.Hands.Left), "R: " + emptyHand(update.Hands.Right)}
	if update.Hands.PreparedSpell != "" {
		rows = append(rows, "Sp: "+update.Hands.PreparedSpell)
	}
	return rows
}

func mapSideRows(update presentation.Update, location string, navigating bool, width int) []string {
	if navigating {
		location += "  Map navigation"
	}
	left := "L: " + emptyHand(update.Hands.Left)
	if update.Hands.PreparedSpell != "" {
		left += "  Sp: " + update.Hands.PreparedSpell
	}
	right := "R: " + emptyHand(update.Hands.Right)
	return []string{truncate(location, width), truncate(left, width), truncate(right, width)}
}

func compactMapRows(width int, lines, side []string, height int, foreground string) []string {
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
			rows[index] = paint(line, width, foreground, "")
			continue
		}
		row := line + strings.Repeat(" ", max(0, mapWidth-ansi.StringWidth(line))) + "  " + truncate(side[index], sideWidth)
		rows[index] = paint(row, width, foreground, "")
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
