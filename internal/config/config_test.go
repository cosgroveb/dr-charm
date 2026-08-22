package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	cwd := filepath.Join(root, "work")
	for _, dir := range []string{home, cwd, filepath.Join(home, ".dr-charm"), filepath.Join(home, ".config", "dr-charm")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(t, filepath.Join(home, ".dr-charm", "config.yaml"), "home-old")
	writeConfig(t, filepath.Join(home, ".config", "dr-charm", "config.yaml"), "home-new")
	writeConfig(t, filepath.Join(cwd, ".dr-charm.yaml"), "working")
	explicit := filepath.Join(root, "explicit.yaml")
	writeConfig(t, explicit, "explicit")

	getenv := mapEnvironment(nil)
	cfg, err := load(explicit, cwd, home, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "explicit" {
		t.Fatalf("explicit account = %q", cfg.Account)
	}

	cfg, err = load("", cwd, home, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "working" {
		t.Fatalf("default account = %q", cfg.Account)
	}

	cfg, err = load("", cwd, home, mapEnvironment(map[string]string{
		"DR_ACCOUNT": "environment", "DR_PASSWORD": "env-password", "DR_CHARACTER": "EnvHero",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if *cfg != (Config{Account: "environment", Password: "env-password", Character: "EnvHero"}) {
		t.Fatalf("environment config = %#v", cfg)
	}
}

func TestLoadExplicitAndExistingDefaultErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	cwd := filepath.Join(root, "work")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := load(filepath.Join(root, "missing.yaml"), cwd, home, mapEnvironment(nil)); err == nil {
		t.Fatal("missing explicit config succeeded")
	}
	if err := os.WriteFile(filepath.Join(cwd, ".dr-charm.yaml"), []byte("account: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(home, ".dr-charm", "config.yaml"), "later")
	if _, err := load("", cwd, home, mapEnvironment(nil)); err == nil {
		t.Fatal("invalid first existing default fell through")
	}
}

func TestLoadValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	_, err := load("", t.TempDir(), t.TempDir(), mapEnvironment(map[string]string{"DR_ACCOUNT": "account"}))
	if err == nil || !strings.Contains(err.Error(), "password") || !strings.Contains(err.Error(), "character") {
		t.Fatalf("validation error = %v", err)
	}
}

func writeConfig(t *testing.T, path, account string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte("account: " + account + "\npassword: password\ncharacter: Hero\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mapEnvironment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
