package config

import (
	"os"
	"path/filepath"
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
	if cfg != (Config{Required: DefaultRequired}) {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoadEmptyFileUsesDefaults(t *testing.T) {
	cfg, err := Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (Config{Required: DefaultRequired}) {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoadReadsFields(t *testing.T) {
	cfg, err := Load(write(t, "team: acme/payments\nrequired: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (Config{Team: "acme/payments", Required: 3}) {
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
