package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	root, home, cwd := prepareLoadTest(t)
	writeConfig(t, filepath.Join(home, ".dr-charm", "config.yaml"), "home-old")
	writeConfig(t, filepath.Join(home, ".config", "dr-charm", "config.yaml"), "home-new")
	writeConfig(t, filepath.Join(cwd, ".dr-charm.yaml"), "working")
	explicit := filepath.Join(root, "explicit.yaml")
	writeConfig(t, explicit, "explicit")

	cfg, err := Load(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "explicit" {
		t.Fatalf("explicit account = %q", cfg.Account)
	}

	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "working" {
		t.Fatalf("working-directory account = %q", cfg.Account)
	}

	if err := os.Remove(filepath.Join(cwd, ".dr-charm.yaml")); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "home-old" {
		t.Fatalf("first home account = %q", cfg.Account)
	}

	if err := os.Remove(filepath.Join(home, ".dr-charm", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "home-new" {
		t.Fatalf("second home account = %q", cfg.Account)
	}

	t.Setenv("DR_ACCOUNT", "environment")
	t.Setenv("DR_PASSWORD", "env-password")
	t.Setenv("DR_CHARACTER", "EnvHero")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if *cfg != (Config{Account: "environment", Password: "env-password", Character: "EnvHero"}) {
		t.Fatalf("environment config = %#v", cfg)
	}
}

func TestLoadExpandsExplicitHome(t *testing.T) {
	_, home, _ := prepareLoadTest(t)
	writeConfig(t, filepath.Join(home, "explicit.yaml"), "explicit-home")

	cfg, err := Load("~/explicit.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "explicit-home" {
		t.Fatalf("explicit home account = %q", cfg.Account)
	}
}

func TestLoadMissingExplicitFileDoesNotFallThrough(t *testing.T) {
	root, _, cwd := prepareLoadTest(t)
	writeConfig(t, filepath.Join(cwd, ".dr-charm.yaml"), "working")

	_, err := Load(filepath.Join(root, "missing.yaml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing explicit config error = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadDefaultCandidateErrorsAreTerminal(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		_, home, cwd := prepareLoadTest(t)
		if err := os.Mkdir(filepath.Join(cwd, ".dr-charm.yaml"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeConfig(t, filepath.Join(home, ".dr-charm", "config.yaml"), "later")

		if _, err := Load(""); err == nil || !strings.Contains(err.Error(), "read config file") {
			t.Fatalf("default read error = %v", err)
		}
	})

	t.Run("parse error", func(t *testing.T) {
		_, home, cwd := prepareLoadTest(t)
		if err := os.WriteFile(filepath.Join(cwd, ".dr-charm.yaml"), []byte("account: ["), 0o600); err != nil {
			t.Fatal(err)
		}
		writeConfig(t, filepath.Join(home, ".dr-charm", "config.yaml"), "later")

		if _, err := Load(""); err == nil || !strings.Contains(err.Error(), "parse config file") {
			t.Fatalf("default parse error = %v", err)
		}
	})
}

func TestLoadValidatesRequiredFields(t *testing.T) {
	prepareLoadTest(t)
	t.Setenv("DR_ACCOUNT", "account")

	_, err := Load("")
	if err == nil || !strings.Contains(err.Error(), "password") || !strings.Contains(err.Error(), "character") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestLoadExplicitPathDoesNotRequireHome(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "explicit.yaml")
	writeConfig(t, explicit, "explicit")
	clearConfigEnvironment(t)
	t.Setenv(userHomeEnvironmentKey(t), "")

	cfg, err := Load(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "explicit" {
		t.Fatalf("explicit account = %q", cfg.Account)
	}
}

func TestLoadCWDConfigDoesNotRequireHome(t *testing.T) {
	cwd := t.TempDir()
	changeWorkingDirectory(t, cwd)
	writeConfig(t, filepath.Join(cwd, ".dr-charm.yaml"), "working")
	clearConfigEnvironment(t)
	t.Setenv(userHomeEnvironmentKey(t), "")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "working" {
		t.Fatalf("working-directory account = %q", cfg.Account)
	}
}

func TestLoadExplicitPathDoesNotRequireWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "explicit.yaml")
	writeConfig(t, explicit, "explicit")
	clearConfigEnvironment(t)
	setHomeDirectory(t, filepath.Join(root, "home"))

	original, err := os.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := original.Chdir(); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
		if err := original.Close(); err != nil {
			t.Errorf("close original working directory: %v", err)
		}
	})
	doomed := filepath.Join(root, "doomed")
	if err := os.Mkdir(doomed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(doomed); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(doomed); err != nil {
		t.Skipf("platform cannot remove current working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("platform still resolves a deleted current working directory")
	}

	cfg, err := Load(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != "explicit" {
		t.Fatalf("explicit account = %q", cfg.Account)
	}
}

func prepareLoadTest(t *testing.T) (root, home, cwd string) {
	t.Helper()
	root = t.TempDir()
	home = filepath.Join(root, "home")
	cwd = filepath.Join(root, "work")
	for _, dir := range []string{home, cwd} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	changeWorkingDirectory(t, cwd)
	setHomeDirectory(t, home)
	clearConfigEnvironment(t)
	return root, home, cwd
}

func changeWorkingDirectory(t *testing.T, path string) {
	t.Helper()
	original, err := os.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := original.Chdir(); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
		if err := original.Close(); err != nil {
			t.Errorf("close original working directory: %v", err)
		}
	})
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
}

func setHomeDirectory(t *testing.T, path string) {
	t.Helper()
	t.Setenv(userHomeEnvironmentKey(t), path)
}

func userHomeEnvironmentKey(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS {
	case "android", "ios":
		t.Skip("os.UserHomeDir uses a platform fallback")
	case "windows":
		return "USERPROFILE"
	case "plan9":
		return "home"
	default:
		return "HOME"
	}
	return ""
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"DR_ACCOUNT", "DR_PASSWORD", "DR_CHARACTER"} {
		t.Setenv(key, "")
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
