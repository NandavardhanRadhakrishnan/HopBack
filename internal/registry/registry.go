// Package registry keeps the machine-local list of Hopback projects in
// ~/.hopback/projects.json.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

var ErrUnknownProject = errors.New("unknown project")

type Entry struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	RegisteredAt time.Time `json:"registered_at"`
}

type file struct {
	SchemaVersion int     `json:"schema_version"`
	Projects      []Entry `json:"projects"`
}

// Home returns HOPBACK_HOME if set, otherwise ~/.hopback.
func Home(env func(string) string) (string, error) {
	if h := env("HOPBACK_HOME"); h != "" {
		return h, nil
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(u, ".hopback"), nil
}

func path(home string) string { return filepath.Join(home, "projects.json") }

// Load returns the registered projects. A missing file is an empty list.
func Load(home string) ([]Entry, error) {
	b, err := os.ReadFile(path(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("invalid %s: %v (Hopback did not modify it)", path(home), err)
	}
	return f.Projects, nil
}

// Register adds e, or replaces the entry with the same ID.
func Register(home string, e Entry) error {
	lock, err := fsutil.AcquireLock(filepath.Join(home, "projects.lock"), 10*time.Second)
	if err != nil {
		return err
	}
	defer lock.Release()
	es, err := Load(home)
	if err != nil {
		return err
	}
	replaced := false
	for i := range es {
		if es[i].ID == e.ID {
			if e.RegisteredAt.IsZero() {
				e.RegisteredAt = es[i].RegisteredAt
			}
			es[i], replaced = e, true
		}
	}
	if !replaced {
		es = append(es, e)
	}
	return fsutil.WriteJSON(path(home), file{SchemaVersion: model.SchemaVersion, Projects: es})
}

// Lookup finds a project by exact ID or case-insensitive name.
func Lookup(home, nameOrID string) (Entry, error) {
	es, err := Load(home)
	if err != nil {
		return Entry{}, err
	}
	for _, e := range es {
		if e.ID == nameOrID || strings.EqualFold(e.Name, nameOrID) {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("%w %q (see `hopback projects`)", ErrUnknownProject, nameOrID)
}
