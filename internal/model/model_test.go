package model

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
	"time"
)

var fixed = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func TestNewSessionID_FormatAndOrder(t *testing.T) {
	a, b := NewSessionID(fixed), NewSessionID(fixed.Add(time.Second))
	if !regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{6}$`).MatchString(a) {
		t.Fatal(a)
	}
	if !(a < b) {
		t.Fatalf("%s !< %s", a, b)
	}
}

func TestNewUUID_V4(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	u1, u2 := NewUUID(), NewUUID()
	if !re.MatchString(u1) || u1 == u2 {
		t.Fatal(u1, u2)
	}
}

func TestNextSeqID(t *testing.T) {
	if got := NextSeqID("T-", []string{"T-1", "T-10", "T-3", "X-50"}); got != "T-11" {
		t.Fatal(got)
	}
	if got := NextSeqID("D-", nil); got != "D-1" {
		t.Fatal(got)
	}
}

func TestEnums(t *testing.T) {
	if !TaskType("tech-debt").Valid() || TaskType("chore").Valid() {
		t.Fatal("TaskType")
	}
	if !TaskStatus("in_progress").Valid() || TaskStatus("done").Valid() {
		t.Fatal("TaskStatus")
	}
	if o, _ := ParseOrigin("agent"); o != "agent_suggested" {
		t.Fatal(o)
	}
	if _, err := ParseOrigin("me"); err == nil {
		t.Fatal("ParseOrigin accepted me")
	}
}

func TestTaskIsOpen(t *testing.T) {
	for s, want := range map[TaskStatus]bool{"completed": false, "cancelled": false, "blocked": true, "idea": true} {
		if (Task{Status: s}).IsOpen() != want {
			t.Fatal(s)
		}
	}
}

func TestJSONFieldNames(t *testing.T) {
	b, _ := json.Marshal(Task{NeedsReview: &Review{Reason: "r"}, CompletedAt: &fixed})
	s, _ := json.Marshal(Session{NativeSessionID: "n"})
	for _, k := range []string{`"needs_review"`, `"completed_at"`, `"event_id"`} {
		if !bytes.Contains(b, []byte(k)) {
			t.Fatal(k)
		}
	}
	if !bytes.Contains(s, []byte(`"native_session_id"`)) {
		t.Fatal("native_session_id")
	}
}

func TestSessionLastActivity(t *testing.T) {
	later := fixed.Add(time.Hour)
	if (Session{StartedAt: fixed, ParkedAt: &later}).LastActivity() != later {
		t.Fatal()
	}
}
