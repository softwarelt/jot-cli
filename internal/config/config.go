// Package config implements jot's configuration resolution: CLI flag >
// environment variable > config file > built-in default (DESIGN.md §5).
// The CLI-flag layer is applied by internal/cli itself, since that's where
// flag values are already parsed; this package handles the rest.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is jot's fully resolved configuration, before any per-invocation
// CLI flag overrides are layered on top.
type Config struct {
	Storage struct {
		Path string `toml:"path"`
	} `toml:"storage"`
	Display struct {
		Timezone         string `toml:"timezone"`
		DefaultListLimit int    `toml:"default_list_limit"`
	} `toml:"display"`

	// Not part of the TOML file — recorded for `jot config` to report.
	JotHome        string `toml:"-"`
	ConfigFilePath string `toml:"-"` // "" if no config file was found
}

func defaults(jotHome string) Config {
	var c Config
	c.JotHome = jotHome
	c.Storage.Path = filepath.Join(jotHome, "data.sqlite")
	c.Display.Timezone = "local"
	c.Display.DefaultListLimit = 20
	return c
}

// Load resolves JOT_HOME/JOT_CONFIG, the config file (if any), and the
// built-in defaults. It does not touch the filesystem beyond reading the
// config file — see EnsureHome for creating ~/.jot itself.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("determining home directory: %w", err)
	}

	jotHome := filepath.Join(home, ".jot")
	if v := os.Getenv("JOT_HOME"); v != "" {
		jotHome = v
	}

	cfg := defaults(jotHome)

	configPath := filepath.Join(jotHome, "config.toml")
	if v := os.Getenv("JOT_CONFIG"); v != "" {
		configPath = v
	}

	if _, statErr := os.Stat(configPath); statErr == nil {
		if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
			return Config{}, fmt.Errorf("parsing config file %s: %w", configPath, err)
		}
		cfg.ConfigFilePath = configPath
		cfg.JotHome = jotHome // DecodeFile may have zeroed it since it's tagged "-"; restore explicitly
	} else if !os.IsNotExist(statErr) {
		return Config{}, fmt.Errorf("reading config file %s: %w", configPath, statErr)
	}

	return cfg, nil
}

// EnsureHome creates the jot home directory (0700) if it doesn't already
// exist (FR-14, NFR-5).
func EnsureHome(jotHome string) error {
	return os.MkdirAll(jotHome, 0o700)
}

// Location resolves Display.Timezone to a *time.Location for rendering
// timestamps: "local" (or empty) means the system's local zone; anything
// else is looked up as an IANA name, falling back to local on a bad name
// rather than erroring on every command that prints a timestamp.
func (c Config) Location() *time.Location {
	if c.Display.Timezone == "" || c.Display.Timezone == "local" {
		return time.Local
	}
	loc, err := time.LoadLocation(c.Display.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}
