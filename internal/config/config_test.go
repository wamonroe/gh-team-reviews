package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team != "" || cfg.Required != DefaultRequired || len(cfg.Repos) != 0 {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoadEmptyFileUsesDefaults(t *testing.T) {
	cfg, err := Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team != "" || cfg.Required != DefaultRequired || len(cfg.Repos) != 0 {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoadReadsFields(t *testing.T) {
	cfg, err := Load(write(t, "team: acme/payments\nrequired: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team != "acme/payments" || cfg.Required != 3 {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoadKeepsDefaultRequiredWhenOmitted(t *testing.T) {
	cfg, err := Load(write(t, "team: acme/payments\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Required != DefaultRequired {
		t.Errorf("Required = %d, want %d", cfg.Required, DefaultRequired)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load(write(t, "teem: acme/payments\n"))
	if err == nil || !strings.Contains(err.Error(), "teem") {
		t.Errorf("Load() error = %v, want unknown field error", err)
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := DefaultPath(); got != "/tmp/xdg/gh-team-reviews/config.yml" {
		t.Errorf("DefaultPath() = %q", got)
	}
}

func TestValidate(t *testing.T) {
	valid := []Config{
		{Required: 1},
		{Team: "acme/payments", Required: 2},
	}
	for _, c := range valid {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", c, err)
		}
	}

	invalid := []Config{
		{Team: "payments", Required: 2},
		{Team: "acme/", Required: 2},
		{Team: "/payments", Required: 2},
		{Team: "acme/pay/ments", Required: 2},
		{Team: "acme/payments", Required: 0},
	}
	for _, c := range invalid {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", c)
		}
	}
}

const withRepos = `team: acme/payments
required: 2
repos:
  Acme/Giving:
    required: 1
  acme/other:
    team: acme/other-team
  acme/legacy:
    team: ""
`

func TestLoadReadsRepoOverrides(t *testing.T) {
	cfg, err := Load(write(t, withRepos))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		repo     string
		team     string
		required int
	}{
		{"acme/giving", "acme/payments", 1},
		{"ACME/GIVING", "acme/payments", 1}, // matched case-insensitively
		{"acme/other", "acme/other-team", 2},
		{"acme/legacy", "", 2}, // explicit empty team differs from unset
		{"acme/unlisted", "acme/payments", 2},
	}
	for _, tt := range tests {
		team, required := cfg.For(tt.repo)
		if team != tt.team || required != tt.required {
			t.Errorf("For(%q) = %q, %d; want %q, %d", tt.repo, team, required, tt.team, tt.required)
		}
	}

	want := []string{"acme/other-team", "acme/payments"}
	if got := cfg.Teams(); !slices.Equal(got, want) {
		t.Errorf("Teams() = %v, want %v", got, want)
	}
}

func TestTeamsWithoutGlobalTeam(t *testing.T) {
	cfg, err := Load(write(t, "repos:\n  acme/api:\n    team: acme/web\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Teams(); !slices.Equal(got, []string{"acme/web"}) {
		t.Errorf("Teams() = %v", got)
	}
}

func TestLoadRejectsUnknownRepoFields(t *testing.T) {
	_, err := Load(write(t, "repos:\n  acme/api:\n    requird: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "requird") {
		t.Errorf("Load() error = %v, want unknown field error", err)
	}
}

func TestLoadRejectsRepoListedTwice(t *testing.T) {
	_, err := Load(write(t, "repos:\n  acme/api:\n    required: 1\n  ACME/api:\n    required: 2\n"))
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("Load() error = %v, want duplicate error", err)
	}
}

func TestValidateRepoOverrides(t *testing.T) {
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }

	valid := Config{Team: "acme/payments", Required: 2, Repos: map[string]RepoConfig{
		"acme/a": {Team: str("")},
		"acme/b": {Team: str("acme/web"), Required: num(1)},
	}}
	if err := valid.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}

	invalid := map[string]RepoConfig{
		"acme":       {},
		"acme/a/b":   {},
		"acme/api":   {Team: str("payments")},
		"acme/api2":  {Required: num(0)},
		"/api":       {},
		"acme/api3/": {},
	}
	for name, repo := range invalid {
		cfg := Config{Required: 2, Repos: map[string]RepoConfig{name: repo}}
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate(%q: %+v) = nil, want error", name, repo)
		}
	}
}
