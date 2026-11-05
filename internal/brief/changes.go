package brief

import (
	"fmt"
	"sort"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

// ChangesSince lists events after since, oldest first, one line each, capped
// at limit lines plus a "... and N more" line.
func ChangesSince(events []model.Event, since time.Time, limit int) []string {
	var evs []model.Event
	for _, e := range events {
		if e.TS.After(since) {
			evs = append(evs, e)
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].TS.Before(evs[j].TS) })
	var out []string
	for i, e := range evs {
		if limit > 0 && i == limit {
			out = append(out, fmt.Sprintf("... and %d more", len(evs)-limit))
			break
		}
		detail := ""
		if id, ok := e.Data["task_id"].(string); ok && id != "" {
			detail = " " + id
		} else if id, ok := e.Data["decision_id"].(string); ok && id != "" {
			detail = " " + id
		}
		if t, ok := e.Data["title"].(string); ok && t != "" {
			detail += ` "` + t + `"`
		} else if s, ok := e.Data["summary"].(string); ok && s != "" {
			detail += ` "` + s + `"`
		}
		out = append(out, fmt.Sprintf("%s  %s%s (%s)", e.TS.Format("2006-01-02"), e.Type, detail, e.Agent))
	}
	return out
}
