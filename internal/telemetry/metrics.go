package telemetry

import (
	"fmt"
	"sync"
	"time"
)

// EventMetrics tracks timing through the data processing pipeline
type EventMetrics struct {
	ReceiveTime  time.Time // When data arrived from socket
	ParseEndTime time.Time // When XML parsing completed
	StateUpdTime time.Time // When game state was updated
	UIUpdateTime time.Time // When UI model was updated
	RenderTime   time.Time // When View() was called
}

// Calculate latencies between stages
func (e EventMetrics) Latencies() map[string]time.Duration {
	latencies := make(map[string]time.Duration)

	if !e.ReceiveTime.IsZero() && !e.ParseEndTime.IsZero() {
		latencies["parse"] = e.ParseEndTime.Sub(e.ReceiveTime)
	}
	if !e.ParseEndTime.IsZero() && !e.StateUpdTime.IsZero() {
		latencies["state"] = e.StateUpdTime.Sub(e.ParseEndTime)
	}
	if !e.StateUpdTime.IsZero() && !e.UIUpdateTime.IsZero() {
		latencies["ui_update"] = e.UIUpdateTime.Sub(e.StateUpdTime)
	}
	if !e.UIUpdateTime.IsZero() && !e.RenderTime.IsZero() {
		latencies["render"] = e.RenderTime.Sub(e.UIUpdateTime)
	}
	if !e.ReceiveTime.IsZero() && !e.RenderTime.IsZero() {
		latencies["total"] = e.RenderTime.Sub(e.ReceiveTime)
	}

	return latencies
}

// PerformanceTracker tracks event loop performance
type PerformanceTracker struct {
	mu            sync.Mutex
	events        []EventMetrics
	maxEvents     int
	slowThreshold time.Duration
	debug         bool
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker(debug bool) *PerformanceTracker {
	return &PerformanceTracker{
		maxEvents:     1000,
		slowThreshold: 50 * time.Millisecond,
		debug:         debug,
	}
}

// StartEvent begins tracking a new event
func (p *PerformanceTracker) StartEvent() *EventMetrics {
	return &EventMetrics{
		ReceiveTime: time.Now(),
	}
}

// RecordEvent stores a completed event
func (p *PerformanceTracker) RecordEvent(event *EventMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, *event)

	// Keep bounded history
	if len(p.events) > p.maxEvents {
		p.events = p.events[len(p.events)-p.maxEvents:]
	}

	// Log slow events
	latencies := event.Latencies()
	if total, ok := latencies["total"]; ok && total > p.slowThreshold {
		if p.debug {
			fmt.Printf("[PERF] Slow event (%.1fms total): ", total.Seconds()*1000)
			fmt.Printf("parse=%.1fms state=%.1fms ui=%.1fms render=%.1fms\n",
				latencies["parse"].Seconds()*1000,
				latencies["state"].Seconds()*1000,
				latencies["ui_update"].Seconds()*1000,
				latencies["render"].Seconds()*1000)
		}
	}
}

// GetStats returns performance statistics
func (p *PerformanceTracker) GetStats() map[string]interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.events) == 0 {
		return nil
	}

	// Calculate averages and percentiles
	var totalTimes []time.Duration
	stageTimes := make(map[string][]time.Duration)

	for _, event := range p.events {
		latencies := event.Latencies()
		for stage, duration := range latencies {
			stageTimes[stage] = append(stageTimes[stage], duration)
			if stage == "total" {
				totalTimes = append(totalTimes, duration)
			}
		}
	}

	stats := make(map[string]interface{})
	stats["event_count"] = len(p.events)

	// Calculate stats for each stage
	for stage, times := range stageTimes {
		if len(times) > 0 {
			avg := calculateAverage(times)
			p95 := calculatePercentile(times, 95)
			max := calculateMax(times)

			stats[stage] = map[string]float64{
				"avg_ms": avg.Seconds() * 1000,
				"p95_ms": p95.Seconds() * 1000,
				"max_ms": max.Seconds() * 1000,
			}
		}
	}

	return stats
}

// Helper functions
func calculateAverage(times []time.Duration) time.Duration {
	if len(times) == 0 {
		return 0
	}
	var sum time.Duration
	for _, t := range times {
		sum += t
	}
	return sum / time.Duration(len(times))
}

func calculatePercentile(times []time.Duration, percentile int) time.Duration {
	if len(times) == 0 {
		return 0
	}
	// Simple implementation - not perfectly accurate but good enough
	index := len(times) * percentile / 100
	if index >= len(times) {
		index = len(times) - 1
	}
	return times[index]
}

func calculateMax(times []time.Duration) time.Duration {
	var max time.Duration
	for _, t := range times {
		if t > max {
			max = t
		}
	}
	return max
}

// IsDebug returns whether debug mode is enabled
func (p *PerformanceTracker) IsDebug() bool {
	return p.debug
}
