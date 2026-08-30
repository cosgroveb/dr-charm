//go:build e2e

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"dr-charm/internal/config"
	"dr-charm/internal/dragonrealms"
	"dr-charm/internal/dragonrealms/presenter"
	"dr-charm/internal/ui"
)

func TestDragonRealmsEndToEnd(t *testing.T) {
	cfg := liveE2EConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	session, model, err := newApplication(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to DragonRealms: %v", err)
	}
	defer session.Close()

	var observed dragonrealms.Snapshot
	for observed.Connection != dragonrealms.ConnectionReady || observed.Room.Title == "" || observed.Prompt == "" {
		select {
		case update, ok := <-session.Updates():
			if !ok {
				t.Fatal("DragonRealms update stream closed before a ready room and prompt")
			}
			if update.Err != nil {
				t.Fatalf("DragonRealms session ended before a ready room and prompt: %v", update.Err)
			}
			observed = update.Snapshot
			updated, _ := model.Update(presenter.Translate(update))
			model = updated.(ui.EnhancedModel)
		case <-ctx.Done():
			t.Fatalf("wait for a ready DragonRealms room and prompt: %v", ctx.Err())
		}
	}

	view := model.View()
	if !strings.Contains(view.Content, observed.Room.Title) {
		t.Fatal("rendered UI does not contain the observed room title")
	}
	if !strings.Contains(view.Content, observed.Prompt) {
		t.Fatal("rendered UI does not contain the observed prompt")
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close DragonRealms session: %v", err)
	}
	for range session.Updates() {
	}
}

func liveE2EConfig(t *testing.T) config.Config {
	t.Helper()
	if path := os.Getenv("DR_E2E_CONFIG"); path != "" {
		cfg, err := config.LoadFromFile(path)
		if err != nil {
			t.Fatalf("load DR_E2E_CONFIG: %v", err)
		}
		validateLiveConfig(t, *cfg)
		return *cfg
	}

	cfg := config.Config{
		Account:   os.Getenv("DR_ACCOUNT"),
		Password:  os.Getenv("DR_PASSWORD"),
		Character: os.Getenv("DR_CHARACTER"),
	}
	validateLiveConfig(t, cfg)
	return cfg
}

func validateLiveConfig(t *testing.T, cfg config.Config) {
	t.Helper()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("load live E2E credentials: %v", err)
	}
	for _, value := range []string{cfg.Account, cfg.Password, cfg.Character} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "your_account_name", "your_password", "your_character_name":
			t.Fatal("live E2E credential source contains placeholder values")
		}
	}
}
