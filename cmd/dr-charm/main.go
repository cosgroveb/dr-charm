package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"dr-charm/internal/config"
	"dr-charm/internal/dragonrealms"
	"dr-charm/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "dr-charm: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("dr-charm", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to DragonRealms configuration file")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	session, model, err := newApplication(ctx, *cfg)
	if err != nil {
		return err
	}
	defer session.Close()

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run terminal UI: %w", err)
	}
	return nil
}

func newApplication(ctx context.Context, cfg config.Config) (*dragonrealms.Session, ui.EnhancedModel, error) {
	session, err := dragonrealms.Dial(ctx, dragonrealms.Credentials{
		Account:   cfg.Account,
		Password:  cfg.Password,
		Character: cfg.Character,
	})
	if err != nil {
		return nil, ui.EnhancedModel{}, err
	}
	return session, ui.InitialEnhancedModel(session, cfg.Character), nil
}
