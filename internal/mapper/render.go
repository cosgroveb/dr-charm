package mapper

import (
	"fmt"
	"sort"
	"strings"
)

func render(zone *Zone, currentID int) string {
	current := zone.Nodes[currentID]
	z := 0
	if current != nil {
		z = current.Z
	}
	nodes := renderNodes(zone, currentID, z)
	if len(nodes) == 0 {
		return "No rooms on this level."
	}
	minX, maxX, minY, maxY := bounds(nodes)
	width := (maxX-minX)*4 + 1
	height := (maxY-minY)*2 + 1
	grid := makeGrid(width, height)
	for _, node := range nodes {
		drawEdges(grid, zone, node, minX, minY, z)
	}
	for _, node := range nodes {
		x, y := gridPos(node.X, node.Y, minX, minY)
		if node.ID == currentID {
			grid[y][x] = '@'
		} else {
			grid[y][x] = 'o'
		}
	}

	lines := trimGrid(grid)
	if current != nil {
		lines = append(lines, "", fmt.Sprintf("@ #%d %s", current.ID, current.Title))
		if len(current.Exits) > 0 {
			lines = append(lines, "Exits: "+exitSummary(current))
		}
	}
	return strings.Join(lines, "\n")
}

func nodesAtZ(zone *Zone, z int) []*Node {
	nodes := make([]*Node, 0, len(zone.Nodes))
	for _, node := range zone.Nodes {
		if node.Z == z {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Y != nodes[j].Y {
			return nodes[i].Y < nodes[j].Y
		}
		if nodes[i].X != nodes[j].X {
			return nodes[i].X < nodes[j].X
		}
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

func renderNodes(zone *Zone, currentID int, z int) []*Node {
	if currentID == 0 {
		return nodesAtZ(zone, z)
	}
	seen := map[int]bool{currentID: true}
	queue := []int{currentID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		node := zone.Nodes[id]
		if node == nil {
			continue
		}
		for _, exit := range node.Exits {
			if exit.DestinationID != 0 && !seen[exit.DestinationID] {
				seen[exit.DestinationID] = true
				queue = append(queue, exit.DestinationID)
			}
		}
		for _, other := range zone.Nodes {
			for _, exit := range other.Exits {
				if exit.DestinationID == id && !seen[other.ID] {
					seen[other.ID] = true
					queue = append(queue, other.ID)
				}
			}
		}
	}
	nodes := make([]*Node, 0, len(seen))
	for id := range seen {
		if node := zone.Nodes[id]; node != nil && node.Z == z {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Y != nodes[j].Y {
			return nodes[i].Y < nodes[j].Y
		}
		if nodes[i].X != nodes[j].X {
			return nodes[i].X < nodes[j].X
		}
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

func bounds(nodes []*Node) (minX, maxX, minY, maxY int) {
	minX, maxX, minY, maxY = nodes[0].X, nodes[0].X, nodes[0].Y, nodes[0].Y
	for _, node := range nodes[1:] {
		if node.X < minX {
			minX = node.X
		}
		if node.X > maxX {
			maxX = node.X
		}
		if node.Y < minY {
			minY = node.Y
		}
		if node.Y > maxY {
			maxY = node.Y
		}
	}
	return minX, maxX, minY, maxY
}

func makeGrid(width, height int) [][]rune {
	grid := make([][]rune, height)
	for y := range grid {
		grid[y] = make([]rune, width)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}
	return grid
}

func drawEdges(grid [][]rune, zone *Zone, node *Node, minX, minY, z int) {
	x, y := gridPos(node.X, node.Y, minX, minY)
	for _, exit := range node.Exits {
		destination := zone.Nodes[exit.DestinationID]
		if destination == nil || destination.Z != z {
			continue
		}
		dx, dy := gridPos(destination.X, destination.Y, minX, minY)
		switch {
		case dx == x && dy < y:
			grid[y-1][x] = '│'
		case dx == x && dy > y:
			grid[y+1][x] = '│'
		case dy == y && dx > x:
			for i := x + 1; i < dx; i++ {
				grid[y][i] = '─'
			}
		case dy == y && dx < x:
			for i := dx + 1; i < x; i++ {
				grid[y][i] = '─'
			}
		case dx > x && dy < y:
			grid[y-1][x+2] = '╱'
		case dx > x && dy > y:
			grid[y+1][x+2] = '╲'
		case dx < x && dy < y:
			grid[y-1][x-2] = '╲'
		case dx < x && dy > y:
			grid[y+1][x-2] = '╱'
		}
	}
}

func gridPos(x, y, minX, minY int) (int, int) {
	return (x - minX) * 4, (y - minY) * 2
}

func trimGrid(grid [][]rune) []string {
	lines := make([]string, 0, len(grid))
	for _, row := range grid {
		line := strings.TrimRight(string(row), " ")
		if line == "" {
			line = " "
		}
		lines = append(lines, line)
	}
	for len(lines) > 1 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 1 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func exitSummary(node *Node) string {
	parts := make([]string, 0, len(node.Exits))
	for _, exit := range node.Exits {
		value := exit.MoveCommand
		if value == "" {
			value = string(exit.Direction)
		}
		if exit.DestinationID == 0 {
			value += " ?"
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, ", ")
}
