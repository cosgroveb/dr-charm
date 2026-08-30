package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const Template = "# DragonRealms account configuration.\n# Fill in all three values, then run dr-charm again.\n\naccount:\npassword:\ncharacter:\n"

type Config struct {
	Account   string `yaml:"account"`
	Password  string `yaml:"password"`
	Character string `yaml:"character"`
}
type Result struct {
	Config  Config
	Path    string
	Created bool
}

func LoadResolved(explicitPath, defaultPath string) (Result, error) {
	path := explicitPath
	if path != "" {
		var err error
		path, err = expandHome(path)
		if err != nil {
			return Result{}, err
		}
	} else {
		path = defaultPath
		if err := ensureTemplate(path); err == nil {
			return Result{Path: path, Created: true}, nil
		} else if !errors.Is(err, fs.ErrExist) {
			return Result{}, err
		}
	}
	cfg, err := read(path)
	if err != nil {
		return Result{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate config file %s: %w", path, err)
	}
	return Result{Config: cfg, Path: path}, nil
}

func ensureTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(Template); err != nil {
		return fmt.Errorf("write config template: %w", err)
	}
	return nil
}

func read(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Config{}, fmt.Errorf("parse config file %s: extra YAML document", path)
	} else if err != io.EOF {
		return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	missing := []string{}
	if strings.TrimSpace(c.Account) == "" {
		missing = append(missing, "account")
	}
	if c.Password == "" {
		missing = append(missing, "password")
	}
	if strings.TrimSpace(c.Character) == "" {
		missing = append(missing, "character")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func LoadFromFile(path string) (*Config, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return nil, err
	}
	cfg, err := read(expanded)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}
