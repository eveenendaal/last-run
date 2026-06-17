// Package config manages the per-user config file at
// $XDG_CONFIG_HOME/lastrun/config.json. It is intentionally minimal — only
// settings that must be known before the database is opened live here.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// Config holds user preferences that cannot be stored inside the SQLite
// database (e.g. the database path itself).
type Config struct {
	DBPath string `json:"db_path,omitempty"`
}

func configPath() (string, error) {
	// Prefer os.Getenv so that XDG_CONFIG_HOME changes (e.g. in tests) are
	// picked up immediately rather than relying on xdg.ConfigHome which is
	// cached at package init.
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		cfgHome = xdg.ConfigHome
	}
	if cfgHome == "" {
		return "", errors.New("XDG config home not available")
	}
	return filepath.Join(cfgHome, "lastrun", "config.json"), nil
}

// Load reads the config file. A missing file is treated as an empty config
// (not an error) so callers fall back to their own defaults gracefully.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return &Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return &Config{}, nil
	}
	return &c, nil
}

// Save writes the config to disk. An empty DBPath field is omitted (omitempty),
// so saving an empty Config produces `{}`, which Load treats as "no override".
func Save(c *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
