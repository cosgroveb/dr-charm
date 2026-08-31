package mapper

import (
	"sort"
	"strings"
)

const learnedFile = "Map00_Learned.xml"

type Direction string

const (
	DirectionNone      Direction = "none"
	DirectionNorth     Direction = "north"
	DirectionNortheast Direction = "northeast"
	DirectionEast      Direction = "east"
	DirectionSoutheast Direction = "southeast"
	DirectionSouth     Direction = "south"
	DirectionSouthwest Direction = "southwest"
	DirectionWest      Direction = "west"
	DirectionNorthwest Direction = "northwest"
	DirectionUp        Direction = "up"
	DirectionDown      Direction = "down"
	DirectionOut       Direction = "out"
)

type Zone struct {
	Name     string
	GenieID  string
	Nodes    map[int]*Node
	Labels   []Label
	nextID   int
	serverID map[string]int
	fingers  map[string][]int
}

type Node struct {
	ID          int
	Title       string
	Description string
	ServerID    string
	X, Y, Z     int
	Notes       string
	Color       string
	Tags        []string
	Exits       []Exit
}

type Exit struct {
	Direction     Direction
	MoveCommand   string
	DestinationID int
	Requires      string
	RtCost        *int
	WaitMin       *int
	WaitMax       *int
	Environment   string
	Notes         string
}

type Label struct {
	Text string
	X, Y float64
	Z    int
}

type Room struct {
	ID          string
	Title       string
	Description string
	Exits       []string
}

func newZone() *Zone {
	z := &Zone{
		Name:    "Learned",
		GenieID: "0",
		Nodes:   make(map[int]*Node),
	}
	z.rebuild()
	return z
}

func (z *Zone) rebuild() {
	z.serverID = make(map[string]int, len(z.Nodes))
	z.fingers = make(map[string][]int, len(z.Nodes))
	z.nextID = 1
	for id, node := range z.Nodes {
		if id >= z.nextID {
			z.nextID = id + 1
		}
		if node.ServerID != "" {
			z.serverID[strings.ToLower(node.ServerID)] = id
		}
		fp := fingerprint(node.Title, exitsFromNode(node))
		z.fingers[fp] = append(z.fingers[fp], id)
	}
	for fp := range z.fingers {
		sort.Ints(z.fingers[fp])
	}
}

func (z *Zone) addNode(room Room, x, y, zed int) *Node {
	id := z.nextID
	z.nextID++
	node := &Node{
		ID:          id,
		Title:       room.Title,
		Description: room.Description,
		ServerID:    room.ID,
		X:           x,
		Y:           y,
		Z:           zed,
	}
	z.Nodes[id] = node
	z.rebuild()
	return node
}

func (n *Node) findExit(direction Direction, move string) *Exit {
	for i := range n.Exits {
		exit := &n.Exits[i]
		if move != "" && strings.EqualFold(exit.MoveCommand, move) {
			return exit
		}
		if direction != DirectionNone && exit.Direction == direction {
			return exit
		}
	}
	return nil
}

func (n *Node) setExit(direction Direction, move string, destination int) bool {
	if move == "" {
		move = string(direction)
	}
	if direction == "" {
		direction = DirectionNone
	}
	exit := n.findExit(direction, move)
	if exit == nil {
		n.Exits = append(n.Exits, Exit{Direction: direction, MoveCommand: move, DestinationID: destination})
		return true
	}
	changed := exit.DestinationID != destination || exit.MoveCommand != move
	exit.DestinationID = destination
	exit.MoveCommand = move
	return changed
}

func parseDirection(command string) Direction {
	first := strings.ToLower(strings.TrimSpace(strings.Split(command, ";")[0]))
	switch first {
	case "n", "north":
		return DirectionNorth
	case "ne", "northeast":
		return DirectionNortheast
	case "e", "east":
		return DirectionEast
	case "se", "southeast":
		return DirectionSoutheast
	case "s", "south":
		return DirectionSouth
	case "sw", "southwest":
		return DirectionSouthwest
	case "w", "west":
		return DirectionWest
	case "nw", "northwest":
		return DirectionNorthwest
	case "u", "up":
		return DirectionUp
	case "d", "down":
		return DirectionDown
	case "o", "out":
		return DirectionOut
	default:
		return DirectionNone
	}
}

func fingerprint(title string, exits []string) string {
	clean := make([]string, 0, len(exits))
	for _, exit := range exits {
		exit = strings.ToLower(strings.TrimSpace(exit))
		if exit != "" {
			clean = append(clean, exit)
		}
	}
	sort.Strings(clean)
	return strings.ToLower(strings.TrimSpace(title)) + "\x00" + strings.Join(clean, "\x00")
}

func exitsFromNode(node *Node) []string {
	exits := make([]string, 0, len(node.Exits))
	for _, exit := range node.Exits {
		if exit.Direction != DirectionNone {
			exits = append(exits, string(exit.Direction))
		}
	}
	return exits
}
