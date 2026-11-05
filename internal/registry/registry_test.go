package registry

import (
	"errors"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestHome_EnvOverride(t *testing.T) {
	h, _ := Home(func(k string) string {
		if k == "HOPBACK_HOME" {
			return "/x"
		}
		return ""
	})
	if h != "/x" {
		t.Fatal(h)
	}
}

func TestRegister_UpsertAndLookup(t *testing.T) {
	home := t.TempDir()
	must(t, Register(home, Entry{ID: "a", Name: "Docslet", Path: "/x"}))
	must(t, Register(home, Entry{ID: "a", Name: "Docslet", Path: "/y"}))
	es, _ := Load(home)
	if len(es) != 1 || es[0].Path != "/y" {
		t.Fatal(es)
	}
	if e, err := Lookup(home, "docslet"); err != nil || e.ID != "a" {
		t.Fatal(e, err)
	}
	if _, err := Lookup(home, "nope"); !errors.Is(err, ErrUnknownProject) {
		t.Fatal(err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if es, err := Load(t.TempDir()); err != nil || len(es) != 0 {
		t.Fatal(es, err)
	}
}
