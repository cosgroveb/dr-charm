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

func TestAgentConfiguration(t *testing.T) {
	valid := "account: acct\npassword: pass\ncharacter: Hero\nagent:\n  endpoint: http://localhost:4000/v1/responses\n  api_key: key\n  model: local\n  character: cautious\n"
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := LoadResolved(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Agent == nil || result.Config.Agent.APIKey != "key" || result.Config.Agent.Endpoint != "http://localhost:4000/v1/responses" {
		t.Fatalf("agent=%+v", result.Config.Agent)
	}
	withoutKey := strings.Replace(valid, "  api_key: key\n", "", 1)
	if err := os.WriteFile(path, []byte(withoutKey), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResolved(path, ""); err != nil {
		t.Fatalf("optional API key rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte(valid+"  unknown: value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResolved(path, ""); err == nil {
		t.Fatal("unknown agent key accepted")
	}
	if !strings.Contains(Template, "# agent:") || !strings.Contains(Template, "#   endpoint:") {
		t.Fatalf("template missing optional agent block: %q", Template)
	}
}

func TestAgentConfigurationValidation(t *testing.T) {
	base := Config{Account: "a", Password: "p", Character: "c"}
	tests := map[string]*Agent{
		"missing endpoint":   {Model: "m", Character: "c"},
		"missing model":      {Endpoint: "http://localhost/v1/responses", Character: "c"},
		"missing character":  {Endpoint: "http://localhost/v1/responses", Model: "m"},
		"unsupported scheme": {Endpoint: "ftp://localhost/v1/responses", Model: "m", Character: "c"},
		"malformed URL":      {Endpoint: "http://[::1/v1/responses", Model: "m", Character: "c"},
		"missing host":       {Endpoint: "http:///v1/responses", Model: "m", Character: "c"},
		"userinfo":           {Endpoint: "http://user:pass@localhost/v1/responses", Model: "m", Character: "c"},
		"fragment":           {Endpoint: "http://localhost/v1/responses#x", Model: "m", Character: "c"},
		"wrong path":         {Endpoint: "http://localhost/v1/chat", Model: "m", Character: "c"},
	}
	for name, agent := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := base
			cfg.Agent = agent
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid agent configuration accepted")
			}
		})
	}
	for _, endpoint := range []string{"http://localhost/v1/responses", "https://example.test/responses"} {
		cfg := base
		cfg.Agent = &Agent{Endpoint: endpoint, Model: "m", Character: "c"}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("endpoint %q rejected: %v", endpoint, err)
		}
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
