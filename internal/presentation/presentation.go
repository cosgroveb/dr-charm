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
	RoomPane
	HandsPane
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
type Update struct {
	Connection    ConnectionState
	Status        []StatusField
	Entries       []Entry
	Notices       []Notice
	Title, Prompt string
	Character     string
	Map           string
}
