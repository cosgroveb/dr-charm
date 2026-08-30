package userdirs

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigFile string
	ThemeDir   string
	LogDir     string
}

func Resolve() (Paths, error) {
	configBase, err := base("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return Paths{}, err
	}
	stateBase, err := base("XDG_STATE_HOME", filepath.Join(".local", "state"))
	if err != nil {
		return Paths{}, err
	}
	root := filepath.Join(configBase, "dr-charm")
	return Paths{
		ConfigFile: filepath.Join(root, "config.yaml"),
		ThemeDir:   filepath.Join(root, "themes"),
		LogDir:     filepath.Join(stateBase, "dr-charm", "logs"),
	}, nil
}

func base(variable, fallback string) (string, error) {
	if value := os.Getenv(variable); value != "" {
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("%s must be an absolute path", variable)
		}
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for %s: %w", variable, err)
	}
	return filepath.Join(home, fallback), nil
}
