package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type layout struct {
	borderStyle lipgloss.Style
}

type paneViews struct {
	main     paneView
	room     paneView
	hands    paneView
	familiar paneView
}

type paneView struct {
	title   string
	body    string
	active  bool
	unread  bool
	visible bool
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

func paneHeights(height int, hasFamiliar bool) (roomHeight, handsHeight, familiarHeight int) {
	if hasFamiliar {
		roomHeight = int(float64(height) * 0.3)
		handsHeight = int(float64(height) * 0.2)
		familiarHeight = height - roomHeight - handsHeight - 2
		return roomHeight, handsHeight, familiarHeight
	}
	roomHeight = int(float64(height) * 0.5)
	handsHeight = height - roomHeight - 1
	return roomHeight, handsHeight, 0
}

func (l layout) render(width, height int, panes paneViews) string {
	leftWidth := int(float64(width) * 0.7)
	rightWidth := width - leftWidth - 1
	roomHeight, handsHeight, familiarHeight := paneHeights(height, panes.familiar.visible)

	mainView := l.renderPane(panes.main, leftWidth, height)
	rightPanes := []string{
		l.renderPane(panes.room, rightWidth, roomHeight),
		l.renderPane(panes.hands, rightWidth, handsHeight),
	}
	if panes.familiar.visible {
		rightPanes = append(rightPanes, l.renderPane(panes.familiar, rightWidth, familiarHeight))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, mainView, lipgloss.JoinVertical(lipgloss.Left, rightPanes...))
}

func (l layout) renderPane(pane paneView, width, height int) string {
	style := l.borderStyle
	if pane.active {
		style = style.BorderForeground(lipgloss.Color("170"))
	}

	contentWidth := max(width-4, 0)
	title := pane.title
	if pane.active {
		title = "> " + title
	} else if pane.unread {
		title = "* " + title
	}
	titleLine := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Width(contentWidth).
		Align(lipgloss.Center).
		Render(title)

	body := pane.body
	content := titleLine + "\n" + body
	contentLines := strings.Split(content, "\n")
	limit := max(height-2, 0)
	if len(contentLines) > limit {
		content = strings.Join(contentLines[:limit], "\n")
	}
	return style.Width(width).Render(content)
}

func renderPlainContent(content []string, maxLines int) string {
	lines := content
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
