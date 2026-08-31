package mapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrackerLearnsRoomsEdgesAndPersistsGenieXML(t *testing.T) {
	dir := t.TempDir()
	tracker := Open(dir)
	tracker.ObserveRoom(Room{ID: "100", Title: "[Town Square]", Description: "A square.", Exits: []string{"north"}})
	tracker.ObserveCommand("north")
	tracker.ObserveRoom(Room{ID: "101", Title: "[North Road]", Description: "A road.", Exits: []string{"south"}})

	if tracker.Status() != "#2/2 rooms" {
		t.Fatalf("status = %q", tracker.Status())
	}
	data, err := os.ReadFile(filepath.Join(dir, learnedFile))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`<zone name="Learned" id="0">`,
		`<node id="1" name="[Town Square]" server_id="100">`,
		`<position x="0" y="0" z="0"></position>`,
		`<arc exit="north" move="north" destination="2"></arc>`,
		`<node id="2" name="[North Road]" server_id="101">`,
		`<position x="0" y="-20" z="0"></position>`,
		`<arc exit="south" move="south"></arc>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved map missing %q:\n%s", want, text)
		}
	}

	reloaded := Open(dir)
	reloaded.ObserveRoom(Room{ID: "100", Title: "[Town Square]", Description: "A square.", Exits: []string{"north"}})
	if reloaded.Status() != "#1/2 rooms" {
		t.Fatalf("reloaded status = %q", reloaded.Status())
	}
}

func TestTrackerCreatesDisconnectedClusterWithoutMovementEvidence(t *testing.T) {
	tracker := Open(t.TempDir())
	tracker.ObserveRoom(Room{Title: "[Shard]", Exits: []string{"north"}})
	tracker.ObserveRoom(Room{Title: "[Crossing]", Exits: []string{"east"}})

	first := tracker.zone.Nodes[1]
	second := tracker.zone.Nodes[2]
	if first.X != 0 || first.Y != 0 || second.X < 50 || second.Y != 0 {
		t.Fatalf("positions first=%#v second=%#v", first, second)
	}
	if len(first.Exits) != 0 {
		for _, exit := range first.Exits {
			if exit.DestinationID != 0 {
				t.Fatalf("unexpected edge without movement evidence: %#v", first.Exits)
			}
		}
	}
}

func TestTrackerIgnoresNonMovementCommandsForEdges(t *testing.T) {
	tracker := Open(t.TempDir())
	tracker.ObserveRoom(Room{Title: "[One]", Exits: []string{"north"}})
	tracker.ObserveCommand("look")
	tracker.ObserveRoom(Room{Title: "[Two]", Exits: []string{"south"}})

	first := tracker.zone.Nodes[1]
	for _, exit := range first.Exits {
		if exit.MoveCommand == "look" || exit.DestinationID != 0 {
			t.Fatalf("non-movement command learned edge: %#v", first.Exits)
		}
	}
}

func TestTrackerFingerprintsReloadedVisibleExits(t *testing.T) {
	dir := t.TempDir()
	tracker := Open(dir)
	tracker.ObserveRoom(Room{Title: "[Crossing]", Exits: []string{"north", "east"}})

	reloaded := Open(dir)
	reloaded.ObserveRoom(Room{Title: "[Crossing]", Exits: []string{"east", "north"}})
	if reloaded.Status() != "#1/1 rooms" {
		t.Fatalf("status=%q", reloaded.Status())
	}
}

func TestTrackerFollowsKnownGraphBeforeFingerprint(t *testing.T) {
	tracker := Open(t.TempDir())
	tracker.ObserveRoom(Room{ID: "1", Title: "[Start]", Exits: []string{"north", "east"}})
	tracker.ObserveCommand("north")
	tracker.ObserveRoom(Room{ID: "2", Title: "[Twin]", Exits: []string{"south"}})
	tracker.ObserveCommand("south")
	tracker.ObserveRoom(Room{ID: "1", Title: "[Start]", Exits: []string{"north", "east"}})
	tracker.ObserveCommand("east")
	tracker.ObserveRoom(Room{ID: "3", Title: "[Twin]", Exits: []string{"south"}})
	tracker.ObserveCommand("west")
	tracker.ObserveRoom(Room{ID: "1", Title: "[Start]", Exits: []string{"north", "east"}})

	tracker.ObserveCommand("east")
	tracker.ObserveRoom(Room{Title: "[Twin]", Exits: []string{"south"}})

	if tracker.currentID != 3 {
		t.Fatalf("currentID=%d, want graph destination 3", tracker.currentID)
	}
}

func TestParseAndSerializePreserveGenieExtensions(t *testing.T) {
	rt, waitMin, waitMax := 7, 300, 600
	zone := &Zone{
		Name:    "Crossing",
		GenieID: "1",
		Nodes: map[int]*Node{1: {
			ID:          1,
			Title:       "Room",
			Description: "Desc",
			Notes:       "bank",
			Color:       "#ff00ff",
			ServerID:    "123",
			Tags:        []string{"vault", "bank"},
			X:           13,
			Y:           -2,
			Z:           1,
			Exits: []Exit{{
				Direction:     DirectionOut,
				MoveCommand:   "go gate",
				DestinationID: 2,
				Requires:      "athletics>=50",
				RtCost:        &rt,
				WaitMin:       &waitMin,
				WaitMax:       &waitMax,
				Environment:   "Gate",
				Notes:         "guarded",
			}},
		}},
		Labels: []Label{{Text: "Bank", X: 1.5, Y: 2.5, Z: 1}},
	}
	parsed, err := parseZone(serializeZone(zone), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	node := parsed.Nodes[1]
	if parsed.Name != "Crossing" || parsed.GenieID != "1" || node.ServerID != "123" || node.X != 13 || node.Y != -2 || node.Z != 1 {
		t.Fatalf("parsed zone/node = %#v %#v", parsed, node)
	}
	if len(node.Tags) != 2 || node.Tags[0] != "bank" || node.Tags[1] != "vault" {
		t.Fatalf("tags = %#v", node.Tags)
	}
	exit := node.Exits[0]
	if exit.MoveCommand != "go gate" || exit.DestinationID != 2 || exit.Requires != "athletics>=50" || *exit.RtCost != 7 || *exit.WaitMin != 300 || *exit.WaitMax != 600 || exit.Environment != "Gate" || exit.Notes != "guarded" {
		t.Fatalf("exit = %#v", exit)
	}
	if len(parsed.Labels) != 1 || parsed.Labels[0].Text != "Bank" {
		t.Fatalf("labels = %#v", parsed.Labels)
	}
}

func TestRenderShowsCurrentRoomAndEdges(t *testing.T) {
	tracker := Open(t.TempDir())
	tracker.ObserveRoom(Room{Title: "[Town Square]", Exits: []string{"north"}})
	tracker.ObserveCommand("north")
	tracker.ObserveRoom(Room{Title: "[North Road]", Exits: []string{"south"}})

	got := tracker.Render()
	for _, want := range []string{"@", "o", "│", "@ #2 [North Road]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q:\n%s", want, got)
		}
	}
}
