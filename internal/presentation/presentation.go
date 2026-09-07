package presentation

type ConnectionState uint8

const (
	Connecting ConnectionState = iota
	Ready
	Reconnecting
	Disconnected
)

type PaneID uint8

const (
	Game PaneID = iota
	Familiar
)

type Operation uint8

const (
	Append Operation = iota
	Replace
	Clear
)

type Entry struct {
	Pane      PaneID
	Text      string
	Operation Operation
}

type StatusField struct{ Label, Value string }
type Notice struct{ Text string }
type Location struct {
	Title string
	Exits []string
}
type Hands struct{ Left, Right, PreparedSpell string }
type Map struct {
	Lines                      []string
	CurrentToken               string
	CurrentLine, CurrentColumn int
}
type Update struct {
	Connection ConnectionState
	Prompted   bool
	Status     []StatusField
	Entries    []Entry
	Notices    []Notice
	Location   Location
	Hands      Hands
	Prompt     string
	Character  string
	Map        Map
}
