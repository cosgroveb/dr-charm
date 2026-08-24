package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config contains the credentials for one DragonRealms character.
type Config struct {
	Account   string `yaml:"account"`
	Password  string `yaml:"password"`
	Character string `yaml:"character"`
}

// Load applies config-file discovery, environment overrides, and validation.
func Load(explicitPath string) (*Config, error) {
	cfg := &Config{}
	if explicitPath != "" {
		loaded, err := LoadFromFile(explicitPath)
		if err != nil {
			return nil, err
		}
		cfg = loaded
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("find working directory: %w", err)
		}
		loaded, err := LoadFromFile(filepath.Join(cwd, ".dr-charm.yaml"))
		if err == nil {
			cfg = loaded
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("find home directory: %w", err)
			}
			paths := []string{
				filepath.Join(home, ".dr-charm", "config.yaml"),
				filepath.Join(home, ".config", "dr-charm", "config.yaml"),
			}
			for _, path := range paths {
				loaded, err := LoadFromFile(path)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						continue
					}
					return nil, err
				}
				cfg = loaded
				break
			}
		}
	}

	for key, target := range map[string]*string{
		"DR_ACCOUNT":   &cfg.Account,
		"DR_PASSWORD":  &cfg.Password,
		"DR_CHARACTER": &cfg.Character,
	} {
		if value := os.Getenv(key); value != "" {
			*target = value
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadFromFile reads one YAML configuration file.
func LoadFromFile(path string) (*Config, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", expanded, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", expanded, err)
	}
	return &cfg, nil
}

// Validate reports missing credential fields without including their values.
func (c Config) Validate() error {
	var missing []string
	if c.Account == "" {
		missing = append(missing, "account")
	}
	if c.Password == "" {
		missing = append(missing, "password")
	}
	if c.Character == "" {
		missing = append(missing, "character")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config fields: %s", strings.Join(missing, ", "))
	}
	return nil
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
