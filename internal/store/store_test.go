package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
)

var (
	fixed = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	attr  = model.Attribution{Session: "s1", Agent: "fake-agent"}
)

func newStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	if _, _, err := project.Init(root, project.InitOptions{Name: "Docslet"}, fixed); err != nil {
		t.Fatal(err)
	}
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	st.Now = func() time.Time { return fixed }
	return st
}

// ok fails the test if the Apply call it wraps returned an error: ok(t)(st.Apply(...)).
func ok(t *testing.T) func(Result, error) {
	return func(_ Result, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func idsByRank(t *testing.T, st *Store) []string {
	t.Helper()
	s, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	items := append([]model.Task(nil), s.Backlog.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

func TestTaskAdd_AssignsIDRankAndDefaults(t *testing.T) {
	st := newStore(t)
	res, err := st.Apply(attr, TaskAdd{Title: "Auth", Origin: "user_requested"})
	snap, _ := st.Load()
	it := snap.Backlog.Items[0]
	if err != nil || it.ID != "T-1" || it.Rank != 1 || it.Type != "feature" || it.Priority != "medium" || it.Status != "planned" ||
		it.SourceSession != "s1" || it.RecordedBy != "fake-agent" || res.Created[0] != "T-1" {
		t.Fatal(it, res, err)
	}
	evs, _ := st.Events()
	last := evs[len(evs)-1]
	if last.Type != "task_created" || last.Data["task_id"] != "T-1" || last.Session != "s1" {
		t.Fatal(last)
	}
}

func TestTaskAdd_Validation(t *testing.T) {
	st := newStore(t)
	var ve *ValidationError
	for _, op := range []TaskAdd{{Title: "", Origin: "user_requested"}, {Title: "x", Type: "chore", Origin: "user_requested"},
		{Title: "x"}, {Title: "x", Origin: "user_requested", DependsOn: []string{"T-9"}}} {
		if _, err := st.Apply(attr, op); !errors.As(err, &ve) {
			t.Fatalf("%+v: %v", op, err)
		}
	}
	_, err := st.Apply(attr, TaskAdd{Title: "x", Type: "chore", Origin: "user_requested"})
	if !strings.Contains(err.Error(), "feature") {
		t.Fatal("message must list allowed values:", err)
	}
}

func TestTaskUpdate_CompletedAt(t *testing.T) {
	st := newStore(t)
	ok(t)(st.Apply(attr, TaskAdd{Title: "A", Origin: "user_requested"}))
	done := model.TaskStatus("completed")
	ok(t)(st.Apply(attr, TaskUpdate{ID: "T-1", Status: &done}))
	if s, _ := st.Load(); s.Backlog.Items[0].CompletedAt == nil || !s.Backlog.Items[0].CompletedAt.Equal(fixed) {
		t.Fatal("completed_at not set")
	}
	back := model.TaskStatus("in_progress")
	ok(t)(st.Apply(attr, TaskUpdate{ID: "T-1", Status: &back}))
	if s, _ := st.Load(); s.Backlog.Items[0].CompletedAt != nil {
		t.Fatal("completed_at not cleared")
	}
	if _, err := st.Apply(attr, TaskUpdate{ID: "T-9", Status: &back}); err == nil || !strings.Contains(err.Error(), "unknown task T-9") {
		t.Fatal(err)
	}
}

func TestTaskReorder(t *testing.T) {
	st := newStore(t)
	for _, n := range []string{"A", "B", "C", "D"} {
		ok(t)(st.Apply(attr, TaskAdd{Title: n, Origin: "user_requested"}))
	}
	ok(t)(st.Apply(attr, TaskReorder{IDs: []string{"T-3", "T-1"}}))
	if got := idsByRank(t, st); !reflect.DeepEqual(got, []string{"T-3", "T-1", "T-2", "T-4"}) {
		t.Fatal(got)
	}
}

func TestApply_AllOrNothing(t *testing.T) {
	st := newStore(t)
	before, _ := st.Events()
	_, err := st.Apply(attr, TaskAdd{Title: "A", Origin: "user_requested"}, TaskUpdate{ID: "T-99", Title: ptr("x")})
	snap, _ := st.Load()
	after, _ := st.Events()
	if err == nil || len(snap.Backlog.Items) != 0 || len(after) != len(before) {
		t.Fatal("partial apply")
	}
}

func TestApply_ConcurrentWriters(t *testing.T) {
	st := newStore(t)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s2, err := Open(st.Root)
			if err == nil {
				_, err = s2.Apply(attr, TaskAdd{Title: fmt.Sprint(i), Origin: "user_requested"})
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	snap, _ := st.Load()
	seen := map[string]bool{}
	for _, it := range snap.Backlog.Items {
		seen[it.ID] = true
	}
	if len(snap.Backlog.Items) != 10 || len(seen) != 10 {
		t.Fatalf("items=%d unique=%d", len(snap.Backlog.Items), len(seen))
	}
}

func TestLoad_InvalidJSONReportsFile(t *testing.T) {
	st := newStore(t)
	bad := []byte("<<<<<<< HEAD\n{}\n=======\n")
	os.WriteFile(st.Path("backlog.json"), bad, 0o644)
	var fe *FileError
	if _, err := st.Load(); !errors.As(err, &fe) || fe.File != "backlog.json" || !strings.Contains(err.Error(), "Hopback did not modify it") {
		t.Fatal(err)
	}
	st.Apply(attr, TaskAdd{Title: "A", Origin: "user_requested"})
	if got, _ := os.ReadFile(st.Path("backlog.json")); !bytes.Equal(got, bad) {
		t.Fatal("corrupt file was overwritten")
	}
}
