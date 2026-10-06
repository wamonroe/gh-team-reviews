// Package dismissed stores the PRs you've hidden from your review list.
package dismissed

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/wamonroe/gh-team-reviews/internal/github"
)

// List maps each dismissed PR, as a lowercased "owner/repo#123", to when it
// was dismissed. GitHub names are case-insensitive, so lookups are too.
type List map[string]time.Time

// Path puts the list next to the config file.
func Path(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "dismissed.yml")
}

// Load reads the list at path. A missing file is an empty list.
func Load(path string) (List, error) {
	list := List{}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return list, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&list); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return list, nil
}

func (l List) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// At returns when ref was dismissed, or the zero time if it wasn't.
func (l List) At(ref github.Ref) time.Time {
	return l[key(ref)]
}

func (l List) Add(ref github.Ref, at time.Time) {
	l[key(ref)] = at
}

// Remove reports whether ref was on the list.
func (l List) Remove(ref github.Ref) bool {
	k := key(ref)
	_, ok := l[k]
	delete(l, k)
	return ok
}

// Refs returns every PR on the list, sorted.
func (l List) Refs() []github.Ref {
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	refs := make([]github.Ref, 0, len(keys))
	for _, k := range keys {
		if ref, err := github.ParseRef(k); err == nil {
			refs = append(refs, ref)
		}
	}
	return refs
}

func key(ref github.Ref) string {
	return strings.ToLower(ref.String())
}
