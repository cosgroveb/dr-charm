package ui

import (
	"fmt"
	"time"
)

// GamePaneMetrics tracks detailed performance for game pane operations
type GamePaneMetrics struct {
	ParseStart  time.Time
	FilterTime  time.Duration
	TriggerTime time.Duration
	OutputTime  time.Duration
	LayoutTime  time.Duration
	TotalTime   time.Duration
	LineCount   int
}

func (g GamePaneMetrics) String() string {
	if g.TotalTime < 10*time.Millisecond {
		return ""
	}
	return fmt.Sprintf("[PERF] Game pane slow: total=%dms (filter=%dms trigger=%dms output=%dms layout=%dms) lines=%d",
		g.TotalTime.Milliseconds(),
		g.FilterTime.Milliseconds(),
		g.TriggerTime.Milliseconds(),
		g.OutputTime.Milliseconds(),
		g.LayoutTime.Milliseconds(),
		g.LineCount)
}

// StartGamePaneMetrics begins tracking game pane performance
func StartGamePaneMetrics() *GamePaneMetrics {
	return &GamePaneMetrics{
		ParseStart: time.Now(),
	}
}
