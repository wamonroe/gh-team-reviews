package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const DefaultRequired = 2

// Config is read from ~/.config/gh-team-reviews/config.yml. Every field is
// optional; command-line flags override whatever is set here.
type Config struct {
	// Team to track as "org/slug". When empty, only direct review requests
	// are listed.
	Team string `yaml:"team"`
	// Team approvals needed before a team-requested PR drops off the list.
	Required int `yaml:"required"`
}

// DefaultPath honors $XDG_CONFIG_HOME and otherwise uses ~/.config on every
// platform, matching gh itself.
func DefaultPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "gh-team-reviews", "config.yml")
}

// Load reads the config at path. A missing file is not an error.
func Load(path string) (Config, error) {
	cfg := Config{Required: DefaultRequired}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("reading %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Team != "" {
		org, slug, ok := strings.Cut(c.Team, "/")
		if !ok || org == "" || slug == "" || strings.Contains(slug, "/") {
			return fmt.Errorf("team must look like ORG/TEAM, got %q", c.Team)
		}
	}
	if c.Required < 1 {
		return fmt.Errorf("required must be at least 1, got %d", c.Required)
	}
	return nil
}
