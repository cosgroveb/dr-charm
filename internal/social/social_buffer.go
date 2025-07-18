package social

import (
	"sync"
)

// SocialEventBuffer is a thread-safe circular buffer for social events
type SocialEventBuffer struct {
	events  []*SocialEvent
	maxSize int
	head    int
	count   int
	mutex   sync.RWMutex
}

// NewSocialEventBuffer creates a new buffer with the specified size
func NewSocialEventBuffer(size int) *SocialEventBuffer {
	return &SocialEventBuffer{
		events:  make([]*SocialEvent, size),
		maxSize: size,
		head:    0,
		count:   0,
	}
}

// Add adds a new event to the buffer, overwriting the oldest if full
func (b *SocialEventBuffer) Add(event *SocialEvent) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	b.events[b.head] = event
	b.head = (b.head + 1) % b.maxSize
	
	if b.count < b.maxSize {
		b.count++
	}
}

// GetRecent returns the most recent N events
func (b *SocialEventBuffer) GetRecent(n int) []*SocialEvent {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	
	if n > b.count {
		n = b.count
	}
	
	result := make([]*SocialEvent, n)
	
	// Calculate starting position
	start := b.head - n
	if start < 0 {
		start += b.maxSize
	}
	
	// Copy events in order
	for i := 0; i < n; i++ {
		idx := (start + i) % b.maxSize
		result[i] = b.events[idx]
	}
	
	return result
}

// GetFiltered returns recent events filtered by type
func (b *SocialEventBuffer) GetFiltered(n int, filter string) []*SocialEvent {
	// For now, "all" and "social" both return all events
	// since we only store social events
	return b.GetRecent(n)
}

// Clear removes all events from the buffer
func (b *SocialEventBuffer) Clear() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	b.events = make([]*SocialEvent, b.maxSize)
	b.head = 0
	b.count = 0
}