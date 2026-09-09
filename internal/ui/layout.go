package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/x/ansi"
)

const transcriptRows = 12

type dashboardGeometry struct {
	width, rows                   int
	innerWidth, contentWidth      int
	inputInterior, equipmentWidth int
	mapWidth, bodyHeight, padding int
	inputWidth                    int
	mapVisible, chrome            bool
}

func calculateDashboardGeometry(width, height int, mapAvailable bool, currentTheme theme, inputLabel string) dashboardGeometry {
	width = max(0, width)
	available := max(1, height-transcriptRows)
	resolved := currentTheme.presentation()
	geometry := dashboardGeometry{width: width, padding: max(0, resolved.Padding)}

	if mapAvailable && available >= 8 {
		for geometry.padding > 0 && !setMapColumns(&geometry, width-4-(2*geometry.padding)) {
			geometry.padding--
		}
		if setMapColumns(&geometry, width-4-(2*geometry.padding)) {
			geometry.chrome = true
			geometry.mapVisible = true
			geometry.bodyHeight = min(8, available-3)
			geometry.rows = geometry.bodyHeight + 3
			geometry.innerWidth = max(0, width-2)
			geometry.contentWidth = max(0, geometry.innerWidth-(2*geometry.padding))
			geometry.inputWidth = inputFieldWidth(geometry.inputInterior, inputLabel)
			return geometry
		}
	}
	if mapAvailable && available >= 7 && setMapColumns(&geometry, width-2) {
		geometry.padding = 0
		geometry.mapVisible = true
		geometry.bodyHeight = min(8, available-2)
		geometry.rows = geometry.bodyHeight + 2
		geometry.innerWidth = width
		geometry.contentWidth = geometry.inputInterior
		geometry.inputWidth = inputFieldWidth(geometry.inputInterior, inputLabel)
		return geometry
	}

	geometry.padding = max(0, resolved.Padding)
	for geometry.padding > 0 && width-2-(2*geometry.padding) < 12 {
		geometry.padding--
	}
	geometry.innerWidth = max(0, width-2)
	geometry.contentWidth = max(0, geometry.innerWidth-(2*geometry.padding))
	geometry.chrome = available >= 6 && geometry.contentWidth >= 12
	if geometry.chrome {
		geometry.rows = 6
		geometry.inputInterior = geometry.contentWidth
	} else {
		geometry.rows = min(4, available)
		geometry.innerWidth = width
		geometry.contentWidth = width
		geometry.inputInterior = width
		geometry.padding = 0
	}
	geometry.inputWidth = inputFieldWidth(geometry.inputInterior, inputLabel)
	return geometry
}

func setMapColumns(geometry *dashboardGeometry, usable int) bool {
	if usable < 48 {
		return false
	}
	geometry.equipmentWidth = usable / 4
	geometry.mapWidth = usable / 4
	geometry.inputInterior = usable - geometry.equipmentWidth - geometry.mapWidth
	return geometry.inputInterior >= 24 && geometry.equipmentWidth >= 12 && geometry.mapWidth >= 12
}

func inputFieldWidth(interior int, label string) int {
	label = truncate(label, max(0, interior-2))
	return max(1, interior-ansi.StringWidth(label)-1)
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
		if navigating {
			status = "Map navigation | " + status
		}
		rows := []string{
			paint(location, geometry.width, currentTheme.Foreground, ""),
			paint(status, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg),
		}
		cropped := cropMap(mapLines, panLine, panColumn, geometry.bodyHeight, geometry.mapWidth)
		return append(rows, compactMapBodyRows(geometry, input, equipmentRows(update), cropped, currentTheme)...)
	}
	rows := make([]string, 0, geometry.rows)
	if geometry.rows >= 3 {
		rows = append(rows, paint(location, geometry.width, currentTheme.Foreground, ""))
	}
	if geometry.rows >= 4 {
		rows = append(rows, paint(hands, geometry.width, currentTheme.Foreground, ""))
	}
	if geometry.rows >= 2 {
		rows = append(rows, paint(status, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg))
	}
	return append(rows, paint(input, geometry.width, currentTheme.StatusBar, currentTheme.StatusBarBg))
}

func framedDashboardRows(geometry dashboardGeometry, update presentation.Update, location, status, input string, mapLines []string, panLine, panColumn int, navigating bool, currentTheme theme) []string {
	border := dashboardBorder(currentTheme.BorderType)
	rows := make([]string, 0, geometry.rows)
	if geometry.mapVisible {
		rows = append(rows, mapRule(border.TopLeft, border.Top, border.MiddleTop, border.TopRight, geometry, location, navigating, currentTheme))
		if navigating {
			status = "Map navigation | " + status
		}
		rows = append(rows, framedStripRow(border, geometry, status, currentTheme))
		cropped := cropMap(mapLines, panLine, panColumn, geometry.bodyHeight, geometry.mapWidth)
		equipment := equipmentRows(update)
		for index := 0; index < geometry.bodyHeight; index++ {
			mapLine := ""
			if index < len(cropped) {
				mapLine = cropped[index]
			}
			equipmentLine := ""
			if index < len(equipment) {
				equipmentLine = equipment[index]
			}
			inputLine := ""
			if index == 0 {
				inputLine = input
			}
			mapBorderColor := currentTheme.Border
			if navigating {
				mapBorderColor = currentTheme.TitleBar
			}
			inputForeground, inputBackground := currentTheme.Foreground, ""
			if index == 0 {
				inputForeground, inputBackground = currentTheme.StatusBar, currentTheme.StatusBarBg
			}
			rows = append(rows,
				paint(border.Left, 1, currentTheme.Border, "")+
					paint(strings.Repeat(" ", geometry.padding)+inputLine, geometry.padding+geometry.inputInterior, inputForeground, inputBackground)+
					paint(border.Left, 1, currentTheme.Border, "")+
					paint(equipmentLine, geometry.equipmentWidth, currentTheme.Foreground, "")+
					paint(border.Left, 1, mapBorderColor, "")+
					paint(mapLine+strings.Repeat(" ", geometry.padding), geometry.mapWidth+geometry.padding, currentTheme.Foreground, "")+
					paint(border.Right, 1, mapBorderColor, ""),
			)
		}
		rows = append(rows, mapRule(border.BottomLeft, border.Bottom, border.MiddleBottom, border.BottomRight, geometry, "", navigating, currentTheme))
		return rows
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
	bodyRows := max(0, geometry.rows-4)
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

func mapRule(left, fill, divider, right string, geometry dashboardGeometry, title string, navigating bool, currentTheme theme) string {
	mapColor := currentTheme.Border
	if navigating {
		mapColor = currentTheme.TitleBar
	}
	return paint(left, 1, currentTheme.Border, "") +
		ruleSegment(fill, geometry.padding+geometry.inputInterior, title, currentTheme.Border, currentTheme.TitleBar) +
		paint(divider, 1, currentTheme.Border, "") +
		ruleSegment(fill, geometry.equipmentWidth, "", currentTheme.Border, currentTheme.TitleBar) +
		paint(divider, 1, mapColor, "") +
		ruleSegment(fill, geometry.mapWidth+geometry.padding, "", mapColor, currentTheme.TitleBar) +
		paint(right, 1, mapColor, "")
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

func compactMapBodyRows(geometry dashboardGeometry, input string, equipment, lines []string, currentTheme theme) []string {
	rows := make([]string, geometry.bodyHeight)
	for index := range rows {
		inputLine := ""
		if index == 0 {
			inputLine = input
		}
		equipmentLine := ""
		if index < len(equipment) {
			equipmentLine = equipment[index]
		}
		mapLine := ""
		if index < len(lines) {
			mapLine = lines[index]
		}
		inputForeground, inputBackground := currentTheme.Foreground, ""
		if index == 0 {
			inputForeground, inputBackground = currentTheme.StatusBar, currentTheme.StatusBarBg
		}
		rows[index] = paint(inputLine, geometry.inputInterior, inputForeground, inputBackground) +
			paint(" ", 1, currentTheme.Foreground, "") +
			paint(equipmentLine, geometry.equipmentWidth, currentTheme.Foreground, "") +
			paint(" ", 1, currentTheme.Foreground, "") +
			paint(mapLine, geometry.mapWidth, currentTheme.Foreground, "")
	}
	return rows
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
