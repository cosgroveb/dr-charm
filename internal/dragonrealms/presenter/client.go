package presenter

import (
	"fmt"
	"strings"

	"dr-charm/internal/dragonrealms"
	"dr-charm/internal/mapper"
	"dr-charm/internal/presentation"
	"dr-charm/internal/terminaltext"
)

type source interface {
	Updates() <-chan dragonrealms.Update
	Send(string) error
}
type Client struct {
	source source
	mapper *mapper.Tracker
}

func New(session *dragonrealms.Session, mapDir string) *Client { return newClient(session, mapDir) }
func newClient(s source, mapDir string) *Client {
	return &Client{source: s, mapper: mapper.Open(mapDir)}
}
func (c *Client) Send(original string) error {
	if err := c.source.Send(original); err != nil {
		return err
	}
	c.mapper.ObserveCommand(original)
	return nil
}
func (c *Client) Next() (presentation.Update, bool) {
	update, ok := <-c.source.Updates()
	if !ok {
		return presentation.Update{}, false
	}
	return c.translate(update), true
}

func (c *Client) translate(u dragonrealms.Update) presentation.Update {
	p := Translate(u)
	c.mapper.ObserveRoom(mapRoom(u.Snapshot.Room))
	p.Map = c.mapper.Render()
	if status := c.mapper.Status(); status != "" {
		p.Status = append(p.Status, presentation.StatusField{Label: "Map", Value: status})
	}
	for _, warning := range c.mapper.Warnings() {
		p.Notices = append(p.Notices, presentation.Notice{Text: safeNotice(warning)})
	}
	return p
}

func Translate(u dragonrealms.Update) presentation.Update {
	p := presentation.Update{Title: terminaltext.Sanitize(u.Snapshot.Room.Title), Prompt: terminaltext.Sanitize(u.Snapshot.Prompt), Character: terminaltext.Sanitize(u.Snapshot.Character), Prompted: u.Prompted, Status: statusFields(u.Snapshot)}
	p.Entries = append(p.Entries,
		presentation.Entry{Pane: presentation.RoomPane, Text: strings.Join(roomLines(u.Snapshot.Room), "\n"), Operation: presentation.Replace},
		presentation.Entry{Pane: presentation.HandsPane, Text: strings.Join(handsLines(u.Snapshot), "\n"), Operation: presentation.Replace},
	)
	switch u.Snapshot.Connection {
	case dragonrealms.ConnectionReady:
		p.Connection = presentation.Ready
	case dragonrealms.ConnectionReconnecting:
		p.Connection = presentation.Reconnecting
	case dragonrealms.ConnectionClosed:
		p.Connection = presentation.Disconnected
	default:
		p.Connection = presentation.Connecting
	}
	for _, d := range u.Display {
		if d.DuplicateEcho {
			continue
		}
		pane := presentation.Game
		if d.Stream == "familiar" {
			pane = presentation.Familiar
		}
		op := presentation.Append
		if d.Kind == dragonrealms.DisplayClear {
			op = presentation.Clear
		}
		p.Entries = append(p.Entries, presentation.Entry{Pane: pane, Text: terminaltext.Sanitize(d.Text), Operation: op})
	}
	for _, d := range u.Diagnostics {
		p.Notices = append(p.Notices, presentation.Notice{Text: safeNotice(d.Text)})
	}
	if u.Err != nil {
		p.Notices = append(p.Notices, presentation.Notice{Text: "connection error"})
	}
	return p
}

func mapRoom(room dragonrealms.Room) mapper.Room {
	return mapper.Room{
		ID:          terminaltext.Sanitize(room.ID),
		Title:       terminaltext.Sanitize(room.Title),
		Description: terminaltext.Sanitize(room.Description),
		Exits:       sanitizedList(room.Exits),
	}
}

func sanitizedList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = terminaltext.Sanitize(value)
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func statusFields(s dragonrealms.Snapshot) []presentation.StatusField {
	posture := map[dragonrealms.Posture]string{
		dragonrealms.PostureStanding: "Standing",
		dragonrealms.PostureKneeling: "Kneeling",
		dragonrealms.PostureSitting:  "Sitting",
		dragonrealms.PostureProne:    "Prone",
	}[s.Posture]
	if posture == "" {
		posture = "Unknown"
	}
	return []presentation.StatusField{
		{Label: "H", Value: fmt.Sprintf("%d%%", s.Vitals.Health)},
		{Label: "M", Value: fmt.Sprintf("%d%%", s.Vitals.Mana)},
		{Label: "F", Value: fmt.Sprintf("%d%%", 100-s.Vitals.Stamina)},
		{Label: "C", Value: fmt.Sprintf("%d%%", s.Vitals.Concentration)},
		{Label: "Sp", Value: fmt.Sprintf("%d%%", s.Vitals.Spirit)},
		{Label: "Posture", Value: posture},
	}
}

func roomLines(r dragonrealms.Room) []string {
	var lines []string
	if title := terminaltext.Sanitize(r.Title); title != "" {
		lines = append(lines, title, "")
	}
	if description := strings.TrimSpace(terminaltext.Sanitize(r.Description)); description != "" {
		lines = append(lines, description, "")
	}
	var exits []string
	for _, exit := range r.Exits {
		exit = terminaltext.Sanitize(exit)
		if exit != "" {
			exits = append(exits, exit)
		}
	}
	if len(exits) > 0 {
		lines = append(lines, "Exits: "+strings.Join(exits, ", "))
	}
	if len(r.Objects) > 0 {
		lines = append(lines, "", "You also see:")
		for _, v := range r.Objects {
			lines = append(lines, "  "+terminaltext.Sanitize(v))
		}
	}
	if len(r.Players) > 0 {
		lines = append(lines, "", "Also here:")
		for _, v := range r.Players {
			lines = append(lines, "  "+terminaltext.Sanitize(v))
		}
	}
	if len(r.Creatures) > 0 {
		lines = append(lines, "", "Creatures:")
		for _, v := range r.Creatures {
			lines = append(lines, "  "+terminaltext.Sanitize(v))
		}
	}
	return lines
}

func handsLines(s dragonrealms.Snapshot) []string {
	lines := []string{"Right: " + terminaltext.Sanitize(s.Hands.Right), "Left: " + terminaltext.Sanitize(s.Hands.Left)}
	if spell := terminaltext.Sanitize(s.PreparedSpell); spell != "" {
		lines = append(lines, "", "Spell: "+spell)
	}
	return lines
}

func safeNotice(s string) string {
	s = terminaltext.Sanitize(s)
	s = strings.NewReplacer("\n", " ", "\t", " ").Replace(s)
	r := []rune(s)
	if len(r) > 256 {
		r = r[:256]
	}
	if len(r) == 0 {
		return "protocol error"
	}
	return string(r)
}
