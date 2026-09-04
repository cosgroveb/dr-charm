package dragonrealms

import "time"

// Credentials identifies one DragonRealms character login.
type Credentials struct {
	Account   string
	Password  string
	Character string
}

// ConnectionState describes the Session lifecycle.
type ConnectionState uint8

const (
	ConnectionConnected ConnectionState = iota
	ConnectionReady
	ConnectionReconnecting
	ConnectionClosed
)

// Posture is the character posture reported by DragonRealms indicators.
type Posture uint8

const (
	PostureUnknown Posture = iota
	PostureStanding
	PostureKneeling
	PostureSitting
	PostureProne
)

// Update is one ordered protocol publication.
type Update struct {
	Snapshot    Snapshot
	Display     []DisplayEvent
	Diagnostics []Diagnostic
	Prompted    bool
	Err         error
}

// Snapshot is immutable public game state at one protocol action.
type Snapshot struct {
	Connection    ConnectionState
	Character     string
	Room          Room
	Vitals        Vitals
	Timers        Timers
	Hands         Hands
	PreparedSpell string
	Posture       Posture
	Prompt        string
}

// Room is the latest room committed by a prompt.
type Room struct {
	ID          string
	Title       string
	Description string
	Exits       []string
	Objects     []string
	Players     []string
	Creatures   []string
	Compass     []string
	Image       string
}

// Vitals contains DragonRealms percentage bars.
type Vitals struct {
	Health        int
	Mana          int
	Stamina       int
	Spirit        int
	Concentration int
	Encumbrance   int
}

// Timers contains server-reported action times.
type Timers struct {
	Round time.Time
	Cast  time.Time
	Spell time.Time
}

// Hands contains full display names for held items.
type Hands struct {
	Left  string
	Right string
}

// DisplayKind identifies how a UI should apply a display event.
type DisplayKind uint8

const (
	DisplayText DisplayKind = iota
	DisplayClear
	DisplayWindow
)

// DisplayEvent contains resolved display semantics.
type DisplayEvent struct {
	Kind          DisplayKind
	Stream        string
	Text          string
	ID            string
	Title         string
	Mono          bool
	DuplicateEcho bool
	Links         []LinkSpan
	Bold          []Span
	Presets       []PresetSpan
}

// Span identifies a visible rune range in DisplayEvent.Text.
type Span struct {
	Start  int
	Length int
}

// LinkSpan identifies a command or URL range.
type LinkSpan struct {
	Span
	Target string
	URL    bool
}

// PresetSpan identifies a server style range.
type PresetSpan struct {
	Span
	ID string
}

// Diagnostic contains sanitized protocol detail safe for display and logs.
type Diagnostic struct {
	Text string
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Room.Exits = append([]string(nil), snapshot.Room.Exits...)
	snapshot.Room.Objects = append([]string(nil), snapshot.Room.Objects...)
	snapshot.Room.Players = append([]string(nil), snapshot.Room.Players...)
	snapshot.Room.Creatures = append([]string(nil), snapshot.Room.Creatures...)
	snapshot.Room.Compass = append([]string(nil), snapshot.Room.Compass...)
	return snapshot
}

func cloneDisplay(events []DisplayEvent) []DisplayEvent {
	cloned := make([]DisplayEvent, len(events))
	for i := range events {
		cloned[i] = events[i]
		cloned[i].Links = append([]LinkSpan(nil), events[i].Links...)
		cloned[i].Bold = append([]Span(nil), events[i].Bold...)
		cloned[i].Presets = append([]PresetSpan(nil), events[i].Presets...)
	}
	return cloned
}
