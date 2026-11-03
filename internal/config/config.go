package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	// Repos overrides team and required for individual repos, keyed by
	// "owner/name". Load lowercases the keys, so look repos up with For.
	Repos map[string]RepoConfig `yaml:"repos"`
}

// RepoConfig overrides the global settings for one repo. Fields are pointers
// so that "not set" (inherit the global value) differs from `team: ""` (track
// no team for this repo).
type RepoConfig struct {
	Team     *string `yaml:"team"`
	Required *int    `yaml:"required"`
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
	// GitHub treats repo names case-insensitively, so match them that way.
	repos := make(map[string]RepoConfig, len(cfg.Repos))
	for name, repo := range cfg.Repos {
		key := strings.ToLower(name)
		if _, dup := repos[key]; dup {
			return cfg, fmt.Errorf("reading %s: repo %q is listed more than once", path, name)
		}
		repos[key] = repo
	}
	cfg.Repos = repos
	return cfg, nil
}

// For returns the team and required approvals that apply to a repo: its own
// override where set, otherwise the global value. repo is "owner/name" in any
// case.
func (c Config) For(repo string) (team string, required int) {
	team, required = c.Team, c.Required
	if r, ok := c.Repos[strings.ToLower(repo)]; ok {
		if r.Team != nil {
			team = *r.Team
		}
		if r.Required != nil {
			required = *r.Required
		}
	}
	return team, required
}

// Teams returns every distinct team that is tracked, globally or by a repo
// override, sorted so searches run in a stable order.
func (c Config) Teams() []string {
	set := map[string]bool{}
	if c.Team != "" {
		set[c.Team] = true
	}
	for _, r := range c.Repos {
		if r.Team != nil && *r.Team != "" {
			set[*r.Team] = true
		}
	}
	teams := make([]string, 0, len(set))
	for team := range set {
		teams = append(teams, team)
	}
	slices.Sort(teams)
	return teams
}

func (c Config) Validate() error {
	if err := validate(c.Team, c.Required); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(c.Repos)) {
		org, repo, ok := strings.Cut(name, "/")
		if !ok || org == "" || repo == "" || strings.Contains(repo, "/") {
			return fmt.Errorf("repos: key must look like OWNER/NAME, got %q", name)
		}
		r := c.Repos[name]
		team, required := c.Team, c.Required
		if r.Team != nil {
			team = *r.Team
		}
		if r.Required != nil {
			required = *r.Required
		}
		if err := validate(team, required); err != nil {
			return fmt.Errorf("repos.%s: %w", name, err)
		}
	}
	return nil
}

func validate(team string, required int) error {
	if team != "" {
		org, slug, ok := strings.Cut(team, "/")
		if !ok || org == "" || slug == "" || strings.Contains(slug, "/") {
			return fmt.Errorf("team must look like ORG/TEAM, got %q", team)
		}
	}
	if required < 1 {
		return fmt.Errorf("required must be at least 1, got %d", required)
	}
	return nil
}
