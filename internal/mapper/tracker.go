package mapper

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Tracker struct {
	path          string
	zone          *Zone
	currentID     int
	lastCommand   string
	lastDirection Direction
	lastRoomKey   string
	warnings      []string
}

func Open(dir string) *Tracker {
	tracker := &Tracker{path: filepath.Join(dir, learnedFile)}
	zone, err := loadZone(tracker.path)
	if err != nil {
		tracker.warnings = append(tracker.warnings, fmt.Sprintf("mapper load failed: %v", err))
		zone = newZone()
	}
	tracker.zone = zone
	return tracker
}

func (t *Tracker) ObserveCommand(command string) {
	command = strings.TrimSpace(strings.Split(command, ";")[0])
	direction, ok := movementCommand(command)
	if !ok {
		return
	}
	t.lastCommand = command
	t.lastDirection = direction
}

func (t *Tracker) ObserveRoom(room Room) {
	if strings.TrimSpace(room.Title) == "" {
		return
	}
	key := roomKey(room)
	if key == t.lastRoomKey {
		return
	}
	t.lastRoomKey = key

	previousID := t.currentID
	node := t.resolve(room)
	if node == nil {
		node = t.createRoom(room, previousID)
	}
	changed := updateNode(node, room)
	t.currentID = node.ID

	if previousID != 0 && previousID != node.ID && t.lastCommand != "" {
		if previous, ok := t.zone.Nodes[previousID]; ok {
			if previous.setExit(t.lastDirection, t.lastCommand, node.ID) {
				changed = true
			}
		}
	}
	t.lastCommand = ""
	t.lastDirection = DirectionNone
	if changed {
		t.zone.rebuild()
		t.save()
	}
}

func movementCommand(command string) (Direction, bool) {
	direction := parseDirection(command)
	if direction != DirectionNone {
		return direction, true
	}
	verb, _, ok := strings.Cut(strings.ToLower(strings.TrimSpace(command)), " ")
	if !ok {
		return DirectionNone, false
	}
	switch verb {
	case "go", "climb", "swim", "crawl", "jump", "enter", "leave", "exit":
		return DirectionNone, true
	default:
		return DirectionNone, false
	}
}

func (t *Tracker) Render() string {
	if t.zone == nil || len(t.zone.Nodes) == 0 {
		return "No rooms learned yet."
	}
	return render(t.zone, t.currentID)
}

func (t *Tracker) Status() string {
	if t.zone == nil || len(t.zone.Nodes) == 0 {
		return "0 rooms"
	}
	if t.currentID == 0 {
		return fmt.Sprintf("%d rooms", len(t.zone.Nodes))
	}
	return fmt.Sprintf("#%d/%d rooms", t.currentID, len(t.zone.Nodes))
}

func (t *Tracker) Warnings() []string {
	out := append([]string(nil), t.warnings...)
	t.warnings = nil
	return out
}

func (t *Tracker) resolve(room Room) *Node {
	if room.ID != "" {
		if id, ok := t.zone.serverID[strings.ToLower(room.ID)]; ok {
			return t.zone.Nodes[id]
		}
	}
	if t.currentID != 0 && t.lastCommand != "" {
		if current := t.zone.Nodes[t.currentID]; current != nil {
			if exit := current.findExit(t.lastDirection, t.lastCommand); exit != nil && exit.DestinationID != 0 {
				if destination := t.zone.Nodes[exit.DestinationID]; destination != nil && sameTitle(destination.Title, room.Title) {
					return destination
				}
			}
		}
	}
	matches := t.zone.fingers[fingerprint(room.Title, room.Exits)]
	if len(matches) == 1 {
		return t.zone.Nodes[matches[0]]
	}
	return nil
}

func (t *Tracker) createRoom(room Room, previousID int) *Node {
	x, y, zed := t.positionForNewRoom(previousID)
	return t.zone.addNode(room, x, y, zed)
}

func (t *Tracker) positionForNewRoom(previousID int) (int, int, int) {
	if previousID != 0 && t.lastCommand != "" {
		if previous := t.zone.Nodes[previousID]; previous != nil {
			x, y, zed := stepFrom(previous.X, previous.Y, previous.Z, t.lastDirection)
			return t.firstFree(x, y, zed)
		}
	}
	if len(t.zone.Nodes) == 0 {
		return 0, 0, 0
	}
	maxX := 0
	for _, node := range t.zone.Nodes {
		if node.X > maxX {
			maxX = node.X
		}
	}
	return t.firstFree(maxX+50, 0, 0)
}

func (t *Tracker) firstFree(x, y, zed int) (int, int, int) {
	occupied := func(x, y, zed int) bool {
		for _, node := range t.zone.Nodes {
			if node.X == x && node.Y == y && node.Z == zed {
				return true
			}
		}
		return false
	}
	if !occupied(x, y, zed) {
		return x, y, zed
	}
	for radius := 1; radius < 20; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if !occupied(x+dx, y+dy, zed) {
					return x + dx, y + dy, zed
				}
			}
		}
	}
	return x + 1, y, zed
}

func (t *Tracker) save() {
	if err := saveZone(t.path, t.zone); err != nil {
		t.warnings = append(t.warnings, fmt.Sprintf("mapper save failed: %v", err))
	}
}

func updateNode(node *Node, room Room) bool {
	changed := false
	if room.Title != "" && node.Title != room.Title {
		node.Title = room.Title
		changed = true
	}
	if room.Description != "" && node.Description != room.Description {
		node.Description = room.Description
		changed = true
	}
	if room.ID != "" && node.ServerID != room.ID {
		node.ServerID = room.ID
		changed = true
	}
	for _, exit := range room.Exits {
		direction := parseDirection(exit)
		if direction == DirectionNone {
			continue
		}
		if node.setExit(direction, string(direction), 0) {
			changed = true
		}
	}
	return changed
}

func stepFrom(x, y, zed int, direction Direction) (int, int, int) {
	switch direction {
	case DirectionNorth:
		return x, y - 1, zed
	case DirectionNortheast:
		return x + 1, y - 1, zed
	case DirectionEast:
		return x + 1, y, zed
	case DirectionSoutheast:
		return x + 1, y + 1, zed
	case DirectionSouth:
		return x, y + 1, zed
	case DirectionSouthwest:
		return x - 1, y + 1, zed
	case DirectionWest:
		return x - 1, y, zed
	case DirectionNorthwest:
		return x - 1, y - 1, zed
	case DirectionUp:
		return x, y, zed + 1
	case DirectionDown:
		return x, y, zed - 1
	default:
		return x + 1, y, zed
	}
}

func sameTitle(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func roomKey(room Room) string {
	return room.ID + "\x00" + room.Title + "\x00" + room.Description + "\x00" + strings.Join(room.Exits, "\x00")
}
