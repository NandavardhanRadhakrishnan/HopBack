package session

import (
	"strings"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

// RequiredSections are the handoff headings a park must contain.
var RequiredSections = []string{"Objective", "Done", "Stopped at", "Unfinished", "Gotchas", "Next step"}

// ParkError lists everything that stopped a park.
type ParkError struct{ Missing []string }

func (e *ParkError) Error() string {
	return "parking incomplete:\n  - " + strings.Join(e.Missing, "\n  - ")
}

func headings(handoff []byte) map[string]bool {
	found := map[string]bool{}
	for _, line := range strings.Split(string(handoff), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if !strings.HasPrefix(line, "#") {
			continue
		}
		found[strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#")))] = true
	}
	return found
}

// Park validates the handoff and the session's state update, then marks the
// session parked. On any failure nothing is written and the session keeps its status.
func (m *Manager) Park(sessionID string, handoff []byte) error {
	return m.Store.WithLock(func() error {
		s, err := m.Get(sessionID)
		if err != nil {
			return err
		}
		if s.Role == "concierge" {
			return &store.ValidationError{Msg: "concierge sessions don't park"}
		}
		if s.Status != "active" && s.Status != "interrupted" && s.Status != "parked" {
			return &store.ValidationError{Msg: "session " + sessionID + " is " + string(s.Status) + " and can't be parked"}
		}
		var missing []string
		have := headings(handoff)
		for _, sec := range RequiredSections {
			if !have[strings.ToLower(sec)] {
				missing = append(missing, "handoff section: "+sec)
			}
		}
		snap, err := m.Store.Load()
		if err != nil {
			return err
		}
		st := snap.State
		if st.UpdatedBy == nil || st.UpdatedBy.Session != sessionID || st.UpdatedAt == nil || st.UpdatedAt.Before(s.StartedAt) {
			missing = append(missing, "project state not updated in this session (run `hopback state set`)")
		}
		if len(missing) > 0 {
			return &ParkError{Missing: missing}
		}
		rel := "sessions/" + sessionID + ".md"
		if err := fsutil.WriteFileAtomic(m.Store.Path(rel), handoff, 0o644); err != nil {
			return err
		}
		now := m.Store.Now().UTC()
		s.Status, s.ParkedAt, s.Handoff = "parked", &now, rel
		if err := fsutil.WriteJSON(m.path(s.ID), s); err != nil {
			return err
		}
		ev := m.Store.NewEvent(model.EvSessionParked, model.Attribution{Session: s.ID, Agent: s.Agent}, map[string]any{"handoff": rel})
		return m.Store.AppendEventsLocked(ev)
	})
}
