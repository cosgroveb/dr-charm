package dragonrealms

import (
	"fmt"
	"time"
)

type protocolEventKind uint8

const (
	eventDisplay protocolEventKind = iota
	eventDiagnostic
	eventComponent
	eventProgress
	eventResource
	eventRoomImage
	eventRoundTime
	eventCastTime
	eventSpellTime
	eventIndicator
	eventCreatureStatus
	eventHand
	eventSpell
	eventCompass
	eventPrompt
	eventEndSetup
	eventSettingsInfo
	eventNav
	eventApp
	eventContainer
	eventInjury
)

type protocolEvent struct {
	kind      protocolEventKind
	name      string
	value     string
	aux       string
	number    int
	timestamp time.Time
	flag      bool
	flag2     bool
	flag3     bool
	items     []string
	display   DisplayEvent
}

func (e protocolEvent) String() string {
	if e.kind == eventDisplay {
		return fmt.Sprintf("display(%d,%q,%q)", e.display.Kind, e.display.Stream, e.display.Text)
	}
	return fmt.Sprintf("event(%d,%q,%q,%q,%d,%v,%v,%v)", e.kind, e.name, e.value, e.aux, e.number, e.flag, e.flag2, e.flag3)
}

type protocolAction struct {
	events []protocolEvent
}
