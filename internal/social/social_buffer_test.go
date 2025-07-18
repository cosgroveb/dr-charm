package social

import (
	"sync"
	"testing"
	"time"
)

func TestSocialEventBuffer_Add(t *testing.T) {
	buffer := NewSocialEventBuffer(3)
	
	// Add events up to capacity
	event1 := &SocialEvent{From: "Player1", Message: "Hello"}
	event2 := &SocialEvent{From: "Player2", Message: "Hi"}
	event3 := &SocialEvent{From: "Player3", Message: "Hey"}
	
	buffer.Add(event1)
	buffer.Add(event2)
	buffer.Add(event3)
	
	recent := buffer.GetRecent(3)
	if len(recent) != 3 {
		t.Errorf("Expected 3 events, got %d", len(recent))
	}
	
	// Verify order
	if recent[0].From != "Player1" {
		t.Errorf("Expected first event from Player1, got %s", recent[0].From)
	}
	if recent[2].From != "Player3" {
		t.Errorf("Expected last event from Player3, got %s", recent[2].From)
	}
}

func TestSocialEventBuffer_Overwrite(t *testing.T) {
	buffer := NewSocialEventBuffer(2)
	
	event1 := &SocialEvent{From: "Player1", Message: "First"}
	event2 := &SocialEvent{From: "Player2", Message: "Second"}
	event3 := &SocialEvent{From: "Player3", Message: "Third"}
	
	buffer.Add(event1)
	buffer.Add(event2)
	buffer.Add(event3) // Should overwrite event1
	
	recent := buffer.GetRecent(2)
	if len(recent) != 2 {
		t.Errorf("Expected 2 events, got %d", len(recent))
	}
	
	// event1 should be gone, we should have event2 and event3
	if recent[0].From != "Player2" {
		t.Errorf("Expected first event from Player2, got %s", recent[0].From)
	}
	if recent[1].From != "Player3" {
		t.Errorf("Expected second event from Player3, got %s", recent[1].From)
	}
}

func TestSocialEventBuffer_GetRecent(t *testing.T) {
	buffer := NewSocialEventBuffer(5)
	
	// Add 3 events
	for i := 1; i <= 3; i++ {
		buffer.Add(&SocialEvent{From: "Player", Message: string(rune('A' + i - 1))})
	}
	
	// Request more than available
	recent := buffer.GetRecent(10)
	if len(recent) != 3 {
		t.Errorf("Expected 3 events when requesting 10, got %d", len(recent))
	}
	
	// Request fewer than available
	recent = buffer.GetRecent(2)
	if len(recent) != 2 {
		t.Errorf("Expected 2 events when requesting 2, got %d", len(recent))
	}
	
	// Should get the most recent 2
	if recent[0].Message != "B" || recent[1].Message != "C" {
		t.Errorf("Expected most recent 2 events (B,C), got (%s,%s)", 
			recent[0].Message, recent[1].Message)
	}
}

func TestSocialEventBuffer_ThreadSafety(t *testing.T) {
	buffer := NewSocialEventBuffer(100)
	var wg sync.WaitGroup
	
	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				buffer.Add(&SocialEvent{
					From:    "Player",
					Message: "Message",
				})
			}
		}(i)
	}
	
	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				buffer.GetRecent(10)
				time.Sleep(time.Millisecond)
			}
		}()
	}
	
	wg.Wait()
	
	// Should have 100 events (buffer is full)
	recent := buffer.GetRecent(200)
	if len(recent) != 100 {
		t.Errorf("Expected 100 events after concurrent operations, got %d", len(recent))
	}
}

func TestSocialEventBuffer_Clear(t *testing.T) {
	buffer := NewSocialEventBuffer(5)
	
	// Add some events
	buffer.Add(&SocialEvent{From: "Player1"})
	buffer.Add(&SocialEvent{From: "Player2"})
	
	// Clear
	buffer.Clear()
	
	recent := buffer.GetRecent(10)
	if len(recent) != 0 {
		t.Errorf("Expected 0 events after clear, got %d", len(recent))
	}
}