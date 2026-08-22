package dragonrealms

import (
	"context"
	"testing"
	"time"
)

func TestSessionIntegration(t *testing.T) {
	servers := startProtocolServers(t)
	defer servers.Close()

	options := servers.SessionOptions()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "test", Password: "pass", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var observed Snapshot
	deadline := time.After(2 * time.Second)
	for observed.Connection != ConnectionReady || observed.Room.Title == "" || observed.Prompt == "" {
		select {
		case update, ok := <-session.Updates():
			if !ok {
				if serverErr := servers.Wait(); serverErr != nil {
					t.Fatalf("session closed before ready room update: %v", serverErr)
				}
				t.Fatal("session closed before ready room update")
			}
			observed = update.Snapshot
		case <-deadline:
			t.Fatal("ready room update timed out")
		}
	}
	if observed.Room.Title != "[Jalapeño Plaza]" || observed.Room.ID != "77" || observed.Prompt != ">" {
		t.Fatalf("snapshot = %#v", observed)
	}
	if err := session.Send("north"); err != nil {
		t.Fatal(err)
	}
	if err := servers.Wait(); err != nil {
		t.Fatal(err)
	}
	for range session.Updates() {
	}
}
