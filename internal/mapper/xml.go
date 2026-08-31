package mapper

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dr-charm/internal/terminaltext"
)

func loadZone(path string) (*Zone, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newZone(), nil
	}
	if err != nil {
		return nil, err
	}
	zone, err := parseZone(data, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if err != nil {
		return nil, err
	}
	zone.rebuild()
	return zone, nil
}

func saveZone(path string, zone *Zone) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".Map00_Learned-*.xml")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(serializeZone(zone)); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func parseZone(data []byte, fallbackName string) (*Zone, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var raw rawZone
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	name := clean(raw.Name)
	if name == "" {
		name = fallbackName
	}
	zone := &Zone{Name: name, GenieID: clean(raw.ID), Nodes: make(map[int]*Node, len(raw.Nodes))}
	for _, rawNode := range raw.Nodes {
		id, err := strconv.Atoi(strings.TrimSpace(rawNode.ID))
		if err != nil {
			continue
		}
		node := &Node{
			ID:          id,
			Title:       clean(rawNode.Name),
			Description: clean(rawNode.Description),
			Notes:       clean(rawNode.Note),
			Color:       clean(rawNode.Color),
			ServerID:    clean(rawNode.ServerID),
			Tags:        splitPipe(clean(rawNode.Tags)),
			X:           rawNode.Position.X / 20,
			Y:           rawNode.Position.Y / 20,
			Z:           rawNode.Position.Z,
		}
		for _, rawExit := range rawNode.Arcs {
			exitName := clean(rawExit.Exit)
			node.Exits = append(node.Exits, Exit{
				Direction:     parseDirection(exitName),
				MoveCommand:   fallbackMove(clean(rawExit.Move), exitName),
				DestinationID: destination(rawExit.Destination),
				Requires:      clean(rawExit.Requires),
				RtCost:        optionalInt(rawExit.Rt),
				WaitMin:       optionalInt(rawExit.WaitMin),
				WaitMax:       optionalInt(rawExit.WaitMax),
				Environment:   clean(rawExit.Environment),
				Notes:         clean(rawExit.Notes),
			})
		}
		zone.Nodes[id] = node
	}
	for _, rawLabel := range raw.Labels {
		text := clean(rawLabel.Text)
		if text == "" {
			continue
		}
		zone.Labels = append(zone.Labels, Label{
			Text: text,
			X:    float64(rawLabel.Position.X) / 20,
			Y:    float64(rawLabel.Position.Y) / 20,
			Z:    rawLabel.Position.Z,
		})
	}
	return zone, nil
}

func clean(value string) string {
	return strings.TrimSpace(terminaltext.Sanitize(value))
}

func serializeZone(zone *Zone) []byte {
	var out bytes.Buffer
	out.WriteString(xml.Header)
	out.WriteString(`<zone name="`)
	out.WriteString(escape(zone.Name))
	out.WriteByte('"')
	if zone.GenieID != "" {
		out.WriteString(` id="`)
		out.WriteString(escape(zone.GenieID))
		out.WriteByte('"')
	}
	out.WriteString(">\n")

	ids := make([]int, 0, len(zone.Nodes))
	for id := range zone.Nodes {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		writeNode(&out, zone.Nodes[id])
	}
	for _, label := range zone.Labels {
		fmt.Fprintf(&out, `  <label text="%s">`+"\n", escape(label.Text))
		fmt.Fprintf(&out, `    <position x="%d" y="%d" z="%d"></position>`+"\n", int(label.X*20), int(label.Y*20), label.Z)
		out.WriteString("  </label>\n")
	}
	out.WriteString("</zone>\n")
	return out.Bytes()
}

func writeNode(out *bytes.Buffer, node *Node) {
	fmt.Fprintf(out, `  <node id="%d" name="%s"`, node.ID, escape(node.Title))
	if node.Notes != "" {
		fmt.Fprintf(out, ` note="%s"`, escape(node.Notes))
	}
	if node.Color != "" {
		fmt.Fprintf(out, ` color="%s"`, escape(node.Color))
	}
	if node.ServerID != "" {
		fmt.Fprintf(out, ` server_id="%s"`, escape(node.ServerID))
	}
	if len(node.Tags) > 0 {
		tags := append([]string(nil), node.Tags...)
		sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i]) < strings.ToLower(tags[j]) })
		fmt.Fprintf(out, ` tags="%s"`, escape(strings.Join(tags, "|")))
	}
	out.WriteString(">\n")
	if node.Description != "" {
		fmt.Fprintf(out, "    <description>%s</description>\n", escape(node.Description))
	}
	fmt.Fprintf(out, `    <position x="%d" y="%d" z="%d"></position>`+"\n", node.X*20, node.Y*20, node.Z)
	for _, exit := range node.Exits {
		writeExit(out, exit)
	}
	out.WriteString("  </node>\n")
}

func writeExit(out *bytes.Buffer, exit Exit) {
	fmt.Fprintf(out, `    <arc exit="%s" move="%s"`, escape(string(exit.Direction)), escape(exit.MoveCommand))
	if exit.DestinationID != 0 {
		fmt.Fprintf(out, ` destination="%d"`, exit.DestinationID)
	}
	if exit.Requires != "" {
		fmt.Fprintf(out, ` requires="%s"`, escape(exit.Requires))
	}
	if exit.RtCost != nil {
		fmt.Fprintf(out, ` rt="%d"`, *exit.RtCost)
	}
	if exit.WaitMin != nil {
		fmt.Fprintf(out, ` wait_min="%d"`, *exit.WaitMin)
	}
	if exit.WaitMax != nil {
		fmt.Fprintf(out, ` wait_max="%d"`, *exit.WaitMax)
	}
	if exit.Environment != "" {
		fmt.Fprintf(out, ` env="%s"`, escape(exit.Environment))
	}
	if exit.Notes != "" {
		fmt.Fprintf(out, ` notes="%s"`, escape(exit.Notes))
	}
	out.WriteString("></arc>\n")
}

func escape(s string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(s))
	return out.String()
}

func fallbackMove(move, exit string) string {
	if strings.TrimSpace(move) != "" {
		return strings.TrimSpace(move)
	}
	return strings.TrimSpace(exit)
}

func destination(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func optionalInt(value string) *int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	return &n
}

func splitPipe(value string) []string {
	parts := strings.Split(value, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

type rawZone struct {
	XMLName xml.Name   `xml:"zone"`
	Name    string     `xml:"name,attr"`
	ID      string     `xml:"id,attr"`
	Nodes   []rawNode  `xml:"node"`
	Labels  []rawLabel `xml:"label"`
}

type rawNode struct {
	ID          string      `xml:"id,attr"`
	Name        string      `xml:"name,attr"`
	Note        string      `xml:"note,attr"`
	Color       string      `xml:"color,attr"`
	ServerID    string      `xml:"server_id,attr"`
	Tags        string      `xml:"tags,attr"`
	Description string      `xml:"description"`
	Position    rawPosition `xml:"position"`
	Arcs        []rawArc    `xml:"arc"`
}

type rawArc struct {
	Exit        string `xml:"exit,attr"`
	Move        string `xml:"move,attr"`
	Destination string `xml:"destination,attr"`
	Requires    string `xml:"requires,attr"`
	Rt          string `xml:"rt,attr"`
	WaitMin     string `xml:"wait_min,attr"`
	WaitMax     string `xml:"wait_max,attr"`
	Environment string `xml:"env,attr"`
	Notes       string `xml:"notes,attr"`
}

type rawLabel struct {
	Text     string      `xml:"text,attr"`
	Position rawPosition `xml:"position"`
}

type rawPosition struct {
	X int `xml:"x,attr"`
	Y int `xml:"y,attr"`
	Z int `xml:"z,attr"`
}
