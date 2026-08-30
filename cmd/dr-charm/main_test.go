package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"dr-charm/internal/config"
	"dr-charm/internal/userdirs"
)

func TestExecuteVersionAndHelpBypassConfiguration(t *testing.T) {
	originalVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = originalVersion })

	deps := dependencies{
		resolvePaths: func() (userdirs.Paths, error) { t.Fatal("resolved paths"); return userdirs.Paths{}, nil },
		loadConfig:   func(string, string) (config.Result, error) { t.Fatal("loaded config"); return config.Result{}, nil },
		runApp:       func(context.Context, config.Config, userdirs.Paths, bool) error { t.Fatal("ran app"); return nil },
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "version long", args: []string{"--version"}, want: "dr-charm 1.2.3\n"},
		{name: "version short", args: []string{"-V"}, want: "dr-charm 1.2.3\n"},
		{name: "help", args: []string{"--help"}, want: "Usage: dr-charm [OPTIONS]\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := execute(tt.args, &stdout, &stderr, deps); code != 0 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.want) || stderr.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestExecuteParseErrorsUseStderrAndExitTwo(t *testing.T) {
	deps := dependencies{
		resolvePaths: func() (userdirs.Paths, error) { t.Fatal("resolved paths"); return userdirs.Paths{}, nil },
		loadConfig:   func(string, string) (config.Result, error) { t.Fatal("loaded config"); return config.Result{}, nil },
		runApp:       func(context.Context, config.Config, userdirs.Paths, bool) error { t.Fatal("ran app"); return nil },
	}

	for _, args := range [][]string{{"-version"}, {"-version=true"}, {"-config=/tmp/config.yaml"}, {"--bogus"}, {"extra"}} {
		var stdout, stderr bytes.Buffer
		if code := execute(args, &stdout, &stderr, deps); code != 2 {
			t.Fatalf("args=%v exit=%d", args, code)
		}
		errText := stderr.String()
		if stdout.Len() != 0 || strings.Count(errText, "dr-charm:") != 1 ||
			!strings.Contains(errText, "Usage: dr-charm [OPTIONS]") ||
			!strings.Contains(errText, "Try 'dr-charm --help' for more information.") {
			t.Fatalf("args=%v stdout=%q stderr=%q", args, stdout.String(), errText)
		}
	}
}

func TestExecuteFirstRunCreatesTemplateAndDoesNotRunApp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := dependencies{
		resolvePaths: func() (userdirs.Paths, error) {
			return userdirs.Paths{ConfigFile: "/tmp/dr-charm/config.yaml", LogDir: "/tmp/dr-charm/logs"}, nil
		},
		loadConfig: func(explicit, defaultPath string) (config.Result, error) {
			if explicit != "" || defaultPath != "/tmp/dr-charm/config.yaml" {
				t.Fatalf("load args explicit=%q default=%q", explicit, defaultPath)
			}
			return config.Result{Path: defaultPath, Created: true}, nil
		},
		runApp: func(context.Context, config.Config, userdirs.Paths, bool) error {
			t.Fatal("ran app")
			return nil
		},
	}

	if code := execute(nil, &stdout, &stderr, deps); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "Created config template at /tmp/dr-charm/config.yaml") ||
		!strings.Contains(stderr.String(), "account:") ||
		!strings.Contains(stderr.String(), "fill in the config file") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecuteRunsApplicationWithConfigAndNoLog(t *testing.T) {
	var gotLogging bool
	deps := dependencies{
		resolvePaths: func() (userdirs.Paths, error) {
			return userdirs.Paths{ConfigFile: "/tmp/config.yaml", ThemeDir: "/tmp/themes", LogDir: "/tmp/logs"}, nil
		},
		loadConfig: func(explicit, defaultPath string) (config.Result, error) {
			if explicit != "/chosen.yaml" || defaultPath != "/tmp/config.yaml" {
				t.Fatalf("load args explicit=%q default=%q", explicit, defaultPath)
			}
			return config.Result{Config: config.Config{Account: "a", Password: "p", Character: "c"}}, nil
		},
		runApp: func(_ context.Context, cfg config.Config, paths userdirs.Paths, logging bool) error {
			if cfg.Character != "c" || paths.LogDir != "/tmp/logs" || paths.ThemeDir != "/tmp/themes" {
				t.Fatalf("app args cfg=%+v paths=%+v", cfg, paths)
			}
			gotLogging = logging
			return nil
		},
	}

	var stdout, stderr bytes.Buffer
	if code := execute([]string{"--config", "/chosen.yaml", "--no-log"}, &stdout, &stderr, deps); code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if gotLogging {
		t.Fatal("--no-log did not disable logging")
	}
}

func TestExecuteRuntimeErrorsDoNotPrintUsage(t *testing.T) {
	deps := dependencies{
		resolvePaths: func() (userdirs.Paths, error) { return userdirs.Paths{}, errors.New("xdg failed") },
		loadConfig:   func(string, string) (config.Result, error) { t.Fatal("loaded config"); return config.Result{}, nil },
		runApp:       func(context.Context, config.Config, userdirs.Paths, bool) error { t.Fatal("ran app"); return nil },
	}
	var stdout, stderr bytes.Buffer
	if code := execute(nil, &stdout, &stderr, deps); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "dr-charm: xdg failed") || strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
