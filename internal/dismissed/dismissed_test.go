package dismissed

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wamonroe/gh-team-reviews/internal/github"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	list, err := Load(filepath.Join(t.TempDir(), "dismissed.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("Load() = %v, want empty", list)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dismissed.yml")
	at := time.Date(2026, 9, 29, 16, 30, 0, 0, time.UTC)

	list := List{}
	list.Add(github.Ref{Repo: "acme/api", Number: 42}, at)
	list.Add(github.Ref{Repo: "acme/web", Number: 7}, at)
	if err := list.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.At(github.Ref{Repo: "acme/api", Number: 42}); !got.Equal(at) {
		t.Errorf("At() = %v, want %v", got, at)
	}
	want := []github.Ref{{Repo: "acme/api", Number: 42}, {Repo: "acme/web", Number: 7}}
	if got := loaded.Refs(); !slices.Equal(got, want) {
		t.Errorf("Refs() = %v, want %v", got, want)
	}
}

func TestLookupsIgnoreCase(t *testing.T) {
	at := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	list := List{}
	list.Add(github.Ref{Repo: "Acme/API", Number: 42}, at)

	if got := list.At(github.Ref{Repo: "acme/api", Number: 42}); !got.Equal(at) {
		t.Errorf("At() = %v, want %v", got, at)
	}
	if !list.Remove(github.Ref{Repo: "ACME/Api", Number: 42}) {
		t.Error("Remove() = false, want true")
	}
	if list.Remove(github.Ref{Repo: "acme/api", Number: 42}) {
		t.Error("second Remove() = true, want false")
	}
}

func TestLoadRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dismissed.yml")
	if err := os.WriteFile(path, []byte("- not a map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load() succeeded on a malformed file")
	}
}
