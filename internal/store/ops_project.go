package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

func (tx *Tx) decision(id string) (*model.Decision, error) {
	for i := range tx.Decisions.Items {
		if tx.Decisions.Items[i].ID == id {
			return &tx.Decisions.Items[i], nil
		}
	}
	return nil, invalid("unknown decision %s", id)
}

func checkPicks(tx *Tx, picks ...model.Pick) error {
	for _, p := range picks {
		if _, err := tx.Task(p.TaskID); err != nil {
			return err
		}
	}
	return nil
}

// DecisionAdd records an open decision (no Decision) or a decided one.
type DecisionAdd struct {
	Title      string         `json:"title"`
	Context    string         `json:"context,omitempty"`
	Options    []model.Option `json:"options,omitempty"`
	Decision   string         `json:"decision,omitempty"`
	Rationale  string         `json:"rationale,omitempty"`
	Supersedes string         `json:"supersedes,omitempty"`
}

func (op DecisionAdd) Apply(tx *Tx) error {
	title := strings.TrimSpace(op.Title)
	if title == "" {
		return invalid("title is required")
	}
	for _, o := range op.Options {
		if strings.TrimSpace(o.Name) == "" {
			return invalid("every option needs a name")
		}
	}
	var superseded *model.Decision
	if op.Supersedes != "" {
		d, err := tx.decision(op.Supersedes)
		if err != nil {
			return err
		}
		superseded = d
	}
	ids := make([]string, 0, len(tx.Decisions.Items))
	for _, d := range tx.Decisions.Items {
		ids = append(ids, d.ID)
	}
	d := model.Decision{
		ID: model.NextSeqID("D-", ids), Title: title, Status: "open", Context: op.Context, Options: op.Options,
		Decision: op.Decision, Rationale: op.Rationale, Supersedes: op.Supersedes, CreatedAt: tx.Now,
		SourceSession: tx.Attr.Session, RecordedBy: tx.Attr.Agent,
	}
	if strings.TrimSpace(op.Decision) != "" {
		now := tx.Now
		d.Status, d.DecidedAt = "decided", &now
	}
	if superseded != nil {
		superseded.Status = "superseded"
	}
	tx.Decisions.Items = append(tx.Decisions.Items, d)
	tx.MarkDecisions()
	tx.Emit(model.EvDecisionRecorded, map[string]any{"decision_id": d.ID, "status": string(d.Status), "title": d.Title})
	tx.created = append(tx.created, d.ID)
	return nil
}

// DecisionResolve closes an open decision.
type DecisionResolve struct {
	ID        string `json:"id"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale,omitempty"`
}

func (op DecisionResolve) Apply(tx *Tx) error {
	d, err := tx.decision(op.ID)
	if err != nil {
		return err
	}
	if d.Status != "open" {
		return invalid("decision %s is %s, not open", op.ID, d.Status)
	}
	if strings.TrimSpace(op.Decision) == "" {
		return invalid("decision is required")
	}
	now := tx.Now
	d.Status, d.Decision, d.Rationale, d.DecidedAt = "decided", op.Decision, op.Rationale, &now
	tx.MarkDecisions()
	tx.Emit(model.EvDecisionResolved, map[string]any{"decision_id": op.ID, "title": d.Title})
	return nil
}

// ArchitectureChange records a change and flags the tasks it may affect for review.
// It never changes or cancels those tasks.
type ArchitectureChange struct {
	Summary string   `json:"summary"`
	Affects []string `json:"affects,omitempty"`
}

func (op ArchitectureChange) Apply(tx *Tx) error {
	summary := strings.TrimSpace(op.Summary)
	if summary == "" {
		return invalid("summary is required")
	}
	if err := checkTasksExist(tx, op.Affects); err != nil {
		return err
	}
	evID := tx.Emit(model.EvArchitectureChanged, map[string]any{"summary": summary, "affects": op.Affects})
	for _, id := range op.Affects {
		t, _ := tx.Task(id)
		t.NeedsReview = &model.Review{Reason: summary, EventID: evID}
	}
	if len(op.Affects) > 0 {
		tx.MarkBacklog()
	}
	return nil
}

// ReviewClear removes a task's needs_review flag after review.
type ReviewClear struct {
	TaskID string `json:"task_id"`
}

func (op ReviewClear) Apply(tx *Tx) error {
	t, err := tx.Task(op.TaskID)
	if err != nil {
		return err
	}
	if t.NeedsReview == nil {
		return invalid("task %s is not flagged for review", op.TaskID)
	}
	t.NeedsReview = nil
	tx.MarkBacklog()
	tx.Emit(model.EvReviewCleared, map[string]any{"task_id": op.TaskID})
	return nil
}

// StateInput is the agent-written part of state.json.
type StateInput struct {
	ProjectStatus   model.ProjectStatus   `json:"project_status,omitempty"`
	Mode            model.Mode            `json:"mode"`
	BottomLine      string                `json:"bottom_line"`
	CurrentFeature  *model.CurrentFeature `json:"current_feature,omitempty"`
	Blockers        []string              `json:"blockers,omitempty"`
	RecommendedNext *model.Pick           `json:"recommended_next,omitempty"`
	Alternatives    []model.Pick          `json:"alternatives,omitempty"`

	// Hopback-managed fields of state.json. They are accepted so the output
	// of `hopback state get` can be edited and sent back, and are ignored.
	SchemaVersion int                `json:"schema_version,omitempty"`
	Revision      int                `json:"revision,omitempty"`
	UpdatedAt     *time.Time         `json:"updated_at,omitempty"`
	UpdatedBy     *model.Attribution `json:"updated_by,omitempty"`
}

// StaleError rejects a whole-state write based on an outdated revision.
type StaleError struct {
	Expected, Current int
	UpdatedBy         *model.Attribution
	UpdatedAt         *time.Time
}

func (e *StaleError) Error() string {
	by, at := "unknown", "unknown time"
	if e.UpdatedBy != nil {
		by = e.UpdatedBy.Session + "/" + e.UpdatedBy.Agent
	}
	if e.UpdatedAt != nil {
		at = e.UpdatedAt.Format(time.RFC3339)
	}
	return fmt.Sprintf("state is at revision %d (updated by %s at %s); you sent %d. Re-read with `hopback state get` and retry.",
		e.Current, by, at, e.Expected)
}

// StateSet replaces the agent-written state if ExpectRevision is current.
type StateSet struct {
	ExpectRevision int        `json:"expect_revision"`
	State          StateInput `json:"state"`
}

func (op StateSet) Apply(tx *Tx) error {
	cur := tx.State
	if op.ExpectRevision != cur.Revision {
		return &StaleError{Expected: op.ExpectRevision, Current: cur.Revision, UpdatedBy: cur.UpdatedBy, UpdatedAt: cur.UpdatedAt}
	}
	in := op.State
	if !in.Mode.Valid() {
		return enumErr("mode", string(in.Mode), model.ModeValues())
	}
	if strings.TrimSpace(in.BottomLine) == "" {
		return invalid("bottom_line is required")
	}
	if in.Mode == "mid_feature" && in.CurrentFeature == nil {
		return invalid("current_feature is required when mode is mid_feature")
	}
	if in.CurrentFeature != nil {
		if _, err := tx.Task(in.CurrentFeature.TaskID); err != nil {
			return err
		}
	}
	if in.RecommendedNext != nil {
		if err := checkPicks(tx, *in.RecommendedNext); err != nil {
			return err
		}
	}
	if err := checkPicks(tx, in.Alternatives...); err != nil {
		return err
	}
	status := cur.ProjectStatus
	if in.ProjectStatus != "" {
		if !in.ProjectStatus.Valid() {
			return enumErr("project_status", string(in.ProjectStatus), model.ProjectStatusValues())
		}
		status = in.ProjectStatus
	}
	now, by := tx.Now, tx.Attr
	tx.State = model.State{
		SchemaVersion: model.SchemaVersion, Revision: cur.Revision + 1, UpdatedAt: &now, UpdatedBy: &by,
		ProjectStatus: status, Mode: in.Mode, BottomLine: in.BottomLine, CurrentFeature: in.CurrentFeature,
		Blockers: in.Blockers, RecommendedNext: in.RecommendedNext, Alternatives: in.Alternatives,
	}
	tx.MarkState()
	tx.Emit(model.EvStateUpdated, map[string]any{"revision": tx.State.Revision})
	return nil
}

// NextSet records the recommended next task and alternatives.
type NextSet struct {
	TaskID       string       `json:"task_id"`
	Reason       string       `json:"reason"`
	Alternatives []model.Pick `json:"alternatives,omitempty"`
}

func (op NextSet) Apply(tx *Tx) error {
	if _, err := tx.Task(op.TaskID); err != nil {
		return err
	}
	if strings.TrimSpace(op.Reason) == "" {
		return invalid("reason is required")
	}
	if err := checkPicks(tx, op.Alternatives...); err != nil {
		return err
	}
	now, by := tx.Now, tx.Attr
	tx.State.SchemaVersion = model.SchemaVersion
	tx.State.RecommendedNext = &model.Pick{TaskID: op.TaskID, Reason: op.Reason}
	tx.State.Alternatives = op.Alternatives
	tx.State.Revision++
	tx.State.UpdatedAt, tx.State.UpdatedBy = &now, &by
	tx.MarkState()
	tx.Emit(model.EvNextSet, map[string]any{"task_id": op.TaskID})
	return nil
}
