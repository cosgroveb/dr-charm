package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/config"
	"dr-charm/internal/dragonrealms"
	"dr-charm/internal/dragonrealms/presenter"
	"dr-charm/internal/ui"
	"dr-charm/internal/userdirs"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr, defaultDeps()))
}

type dependencies struct {
	resolvePaths func() (userdirs.Paths, error)
	loadConfig   func(explicitPath, defaultPath string) (config.Result, error)
	runApp       func(context.Context, config.Config, userdirs.Paths, bool) error
}

func defaultDeps() dependencies {
	return dependencies{
		resolvePaths: userdirs.Resolve,
		loadConfig:   config.LoadResolved,
		runApp: func(ctx context.Context, cfg config.Config, paths userdirs.Paths, logging bool) error {
			session, model, err := newApplicationWithOptions(ctx, cfg, paths, logging)
			if err != nil {
				return err
			}
			defer session.Close()
			program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithoutSignalHandler())
			finalModel, err := program.Run()
			if closer, ok := finalModel.(interface{ Close() error }); ok {
				if closeErr := closer.Close(); err == nil {
					err = closeErr
				}
			}
			return err
		},
	}
}

func execute(args []string, stdout, stderr io.Writer, deps dependencies) int {
	err := run(args, stdout, stderr, deps)
	if err == nil {
		return 0
	}
	var parseErr *parseError
	if errors.As(err, &parseErr) {
		fmt.Fprintf(stderr, "dr-charm: %s\nUsage: dr-charm [OPTIONS]\nTry 'dr-charm --help' for more information.\n", parseErr.Message)
		return 2
	}
	fmt.Fprintf(stderr, "dr-charm: %v\n", err)
	return 1
}

func run(args []string, stdout, stderr io.Writer, deps dependencies) error {
	if err := rejectGoStyleFlags(args); err != nil {
		return err
	}

	var configPath string
	var noLog bool
	var showVersion bool
	cmd := &cobra.Command{
		Use:                        "dr-charm [OPTIONS]",
		Short:                      "DragonRealms terminal client",
		SilenceErrors:              true,
		SilenceUsage:               true,
		SuggestionsMinimumDistance: -1,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return &parseError{Message: "unexpected argument: " + args[0]}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "dr-charm %s\n", version)
				return err
			}

			paths, err := deps.resolvePaths()
			if err != nil {
				return err
			}
			result, err := deps.loadConfig(configPath, paths.ConfigFile)
			if err != nil {
				return err
			}
			if result.Created {
				fmt.Fprintf(stderr, "Created config template at %s\n\n%s", result.Path, config.Template)
				return errors.New("fill in the config file and run dr-charm again")
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := deps.runApp(ctx, result.Config, paths, !noLog); err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("run terminal UI: %w", err)
			}
			return nil
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &parseError{Message: normalizeFlagError(err.Error())}
	})
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "config file path")
	cmd.Flags().BoolVarP(&noLog, "no-log", "n", false, "disable session logging")
	cmd.Flags().BoolVarP(&showVersion, "version", "V", false, "print version and exit")
	cmd.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), helpText())
	})
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

type parseError struct {
	Message string
}

func (e *parseError) Error() string { return e.Message }

func rejectGoStyleFlags(args []string) error {
	for _, arg := range args {
		if arg == "--" {
			return nil
		}
		if arg == "-config" || strings.HasPrefix(arg, "-config=") ||
			arg == "-no-log" || strings.HasPrefix(arg, "-no-log=") ||
			arg == "-version" || strings.HasPrefix(arg, "-version=") {
			return &parseError{Message: "unknown option: " + arg}
		}
	}
	return nil
}

func normalizeFlagError(message string) string {
	message = strings.TrimSpace(message)
	if strings.HasPrefix(message, "unknown flag: ") {
		return "unknown option: " + strings.TrimPrefix(message, "unknown flag: ")
	}
	if strings.HasPrefix(message, "unknown shorthand flag: ") {
		return "unknown option: " + strings.TrimPrefix(message, "unknown shorthand flag: ")
	}
	return message
}

func helpText() string {
	return `DragonRealms terminal client

Usage: dr-charm [OPTIONS]

Options:
  -c, --config PATH  Use a specific config file.
  -n, --no-log       Start with transcript logging off.
  -V, --version      Print version and exit.
  -h, --help         Show help.

Configuration:
  Default: $XDG_CONFIG_HOME/dr-charm/config.yaml
  Fallback: ~/.config/dr-charm/config.yaml

  A first run with no config creates a commented template and exits.

Maps:
  Learned map: $XDG_DATA_HOME/dr-charm/maps/Map00_Learned.xml
  Fallback: ~/.local/share/dr-charm/maps/Map00_Learned.xml

Examples:
  dr-charm
  dr-charm --config ~/.config/dr-charm/config.yaml
`
}

func newApplication(ctx context.Context, cfg config.Config) (*dragonrealms.Session, ui.EnhancedModel, error) {
	paths, err := userdirs.Resolve()
	if err != nil {
		return nil, ui.EnhancedModel{}, err
	}
	return newApplicationWithOptions(ctx, cfg, paths, true)
}

func newApplicationWithOptions(ctx context.Context, cfg config.Config, paths userdirs.Paths, logging bool) (*dragonrealms.Session, ui.EnhancedModel, error) {
	session, err := dragonrealms.Dial(ctx, dragonrealms.Credentials{
		Account:   cfg.Account,
		Password:  cfg.Password,
		Character: cfg.Character,
	})
	if err != nil {
		return nil, ui.EnhancedModel{}, err
	}
	var agentClient *agent.Client
	if cfg.Agent != nil {
		agentClient = agent.New(agent.Config{Endpoint: cfg.Agent.Endpoint, APIKey: cfg.Agent.APIKey, Model: cfg.Agent.Model, Character: cfg.Agent.Character})
	}
	return session, ui.InitialEnhancedModel(presenter.New(session, paths.MapDir), ui.Options{
		Character: cfg.Character,
		LogDir:    paths.LogDir,
		ThemeDir:  paths.ThemeDir,
		Logging:   logging,
		Agent:     agentClient,
		Context:   ctx,
	}), nil
}
