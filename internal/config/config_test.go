package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvedCreatesTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dr-charm", "config.yaml")
	r, err := LoadResolved("", path)
	if err != nil || !r.Created {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	if !strings.Contains(string(mustRead(t, path)), "# DragonRealms") {
		t.Fatal("template missing comment")
	}
}

func TestLoadResolvedExplicitDoesNotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := LoadResolved(path, filepath.Join(t.TempDir(), "default.yaml")); err == nil {
		t.Fatal("missing explicit config succeeded")
	}
}

func TestLoadResolvedExplicitHomeExpansionError(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := LoadResolved("~/missing.yaml", filepath.Join(t.TempDir(), "default.yaml")); err == nil || !strings.Contains(err.Error(), "find home directory") {
		t.Fatalf("home expansion error=%v", err)
	}
}

func TestStrictConfigAndPasswordPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("account: acct\npassword: ' pass '\ncharacter: ' Hero '\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := LoadResolved(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.Password != " pass " {
		t.Fatalf("password=%q", r.Config.Password)
	}
	if _, err := LoadResolved(path, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("account: acct\npassword: p\ncharacter: h\nunknown: x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResolved(path, ""); err == nil || !strings.Contains(err.Error(), "parse config file") {
		t.Fatalf("strict error=%v", err)
	}
}

func TestValidateWhitespace(t *testing.T) {
	if err := (Config{Account: " ", Password: "", Character: "\t"}).Validate(); err == nil {
		t.Fatal("whitespace config accepted")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
