package main

import (
	"html"
	"net"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the application state
type Model struct {
	conn     net.Conn
	output   []string
	input    string
	err      error
	quitting bool
	width    int
	height   int
}

// Message types
type gameOutputMsg string
type errMsg error

// InitialModel creates the initial model
func InitialModel(conn net.Conn) Model {
	return Model{
		conn:   conn,
		output: []string{"Connected to DragonRealms"},
		input:  "",
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	// Start reading from the game connection
	return readGameOutput(m.conn)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.input != "" {
				// Send command to game
				_, err := m.conn.Write([]byte(m.input + "\n"))
				if err != nil {
					m.err = err
					return m, nil
				}
				// Add to output
				m.output = append(m.output, "> "+m.input)
				// Keep only last 100 lines
				if len(m.output) > 100 {
					m.output = m.output[len(m.output)-100:]
				}
				m.input = ""
			}
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case gameOutputMsg:
		// Add game output
		text := string(msg)
		// Decode HTML entities
		text = html.UnescapeString(text)
		// Split by newlines and add each line
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			if line != "" {
				m.output = append(m.output, line)
			}
		}
		// Keep only last 100 lines
		if len(m.output) > 100 {
			m.output = m.output[len(m.output)-100:]
		}
		// Continue reading
		return m, readGameOutput(m.conn)

	case errMsg:
		m.err = msg
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		MarginBottom(1)

	outputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1).
		Width(m.width - 2).
		Height(m.height - 6)

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Width(m.width - 2)

	// Build output
	var output strings.Builder

	// Show last N lines that fit in the box
	boxHeight := m.height - 8
	startIdx := 0
	if len(m.output) > boxHeight {
		startIdx = len(m.output) - boxHeight
	}

	for i := startIdx; i < len(m.output); i++ {
		output.WriteString(m.output[i] + "\n")
	}

	// Build view
	s := titleStyle.Render("DragonRealms") + "\n"
	s += outputStyle.Render(output.String()) + "\n"
	s += inputStyle.Render("> " + m.input)

	if m.err != nil {
		s += "\n\nError: " + m.err.Error()
	}

	return s
}

// readGameOutput reads from the game connection
func readGameOutput(conn net.Conn) tea.Cmd {
	return func() tea.Msg {
		var output strings.Builder

		// Read some data
		for i := 0; i < 100; i++ { // Read up to 100 bytes at a time
			b := make([]byte, 1)
			_, err := conn.Read(b)
			if err != nil {
				return errMsg(err)
			}

			// Basic XML stripping
			char := string(b[0])
			if char == "<" {
				// Skip until we find >
				for {
					_, err := conn.Read(b)
					if err != nil {
						return errMsg(err)
					}
					if string(b[0]) == ">" {
						break
					}
				}
				continue
			}

			output.WriteString(char)

			// If we hit a newline, return what we have
			if char == "\n" {
				break
			}
		}

		return gameOutputMsg(output.String())
	}
}
