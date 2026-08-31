package userdirs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveUsesAbsoluteXDGRoots(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config")
	stateHome := filepath.Join(t.TempDir(), "state")
	dataHome := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	got, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigFile != filepath.Join(configHome, "dr-charm", "config.yaml") ||
		got.ThemeDir != filepath.Join(configHome, "dr-charm", "themes") ||
		got.LogDir != filepath.Join(stateHome, "dr-charm", "logs") ||
		got.MapDir != filepath.Join(dataHome, "dr-charm", "maps") {
		t.Fatalf("paths=%+v", got)
	}
}

func TestResolveUsesHomeFallbackWhenXDGIsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	got, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigFile != filepath.Join(home, ".config", "dr-charm", "config.yaml") ||
		got.LogDir != filepath.Join(home, ".local", "state", "dr-charm", "logs") ||
		got.MapDir != filepath.Join(home, ".local", "share", "dr-charm", "maps") {
		t.Fatalf("paths=%+v", got)
	}
}

func TestResolveErrorsWhenHomeFallbackIsUnavailable(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	if _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("error=%v", err)
	}
}

func TestResolveRejectsRelativeXDG(t *testing.T) {
	for _, variable := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
			t.Setenv(variable, "relative")
			if _, err := Resolve(); err == nil || !strings.Contains(err.Error(), variable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
