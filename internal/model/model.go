// Package model holds Hopback's data types. Every type here is serialised to
// a documented JSON file in .hopback/ with snake_case field names.
package model

import "time"

const SchemaVersion = 1

// Attribution names the session and agent behind a write.
type Attribution struct {
	Session string `json:"session"`
	Agent   string `json:"agent"`
}

type Project struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Stack         []string  `json:"stack,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Review marks a task flagged for the developer to review, and why.
type Review struct {
	Reason  string `json:"reason"`
	EventID string `json:"event_id"`
}

type Task struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Type          TaskType   `json:"type"`
	Priority      Priority   `json:"priority"`
	Status        TaskStatus `json:"status"`
	Rank          int        `json:"rank"`
	DependsOn     []string   `json:"depends_on,omitempty"`
	NeedsReview   *Review    `json:"needs_review,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	SourceSession string     `json:"source_session,omitempty"`
	RecordedBy    string     `json:"recorded_by,omitempty"`
	Origin        Origin     `json:"origin"`
}

// IsOpen reports whether the task still counts as work left.
func (t Task) IsOpen() bool { return t.Status != "completed" && t.Status != "cancelled" }

type Backlog struct {
	SchemaVersion int    `json:"schema_version"`
	Items         []Task `json:"items"`
}

type Option struct {
	Name      string `json:"name"`
	Tradeoffs string `json:"tradeoffs,omitempty"`
}

type Decision struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Status        DecisionStatus `json:"status"`
	Context       string         `json:"context,omitempty"`
	Options       []Option       `json:"options,omitempty"`
	Decision      string         `json:"decision,omitempty"`
	Rationale     string         `json:"rationale,omitempty"`
	Supersedes    string         `json:"supersedes,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	DecidedAt     *time.Time     `json:"decided_at,omitempty"`
	SourceSession string         `json:"source_session,omitempty"`
	RecordedBy    string         `json:"recorded_by,omitempty"`
}

type Decisions struct {
	SchemaVersion int        `json:"schema_version"`
	Items         []Decision `json:"items"`
}

type CurrentFeature struct {
	TaskID    string `json:"task_id"`
	Why       string `json:"why"`
	StoppedAt string `json:"stopped_at"`
	NextStep  string `json:"next_step"`
}

// Pick is a recommended task with the reason for it.
type Pick struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// State is the latest agent-written project status, the brief's source.
type State struct {
	SchemaVersion   int             `json:"schema_version"`
	Revision        int             `json:"revision"`
	UpdatedAt       *time.Time      `json:"updated_at,omitempty"`
	UpdatedBy       *Attribution    `json:"updated_by,omitempty"`
	ProjectStatus   ProjectStatus   `json:"project_status"`
	Mode            Mode            `json:"mode,omitempty"`
	BottomLine      string          `json:"bottom_line,omitempty"`
	CurrentFeature  *CurrentFeature `json:"current_feature,omitempty"`
	Blockers        []string        `json:"blockers,omitempty"`
	RecommendedNext *Pick           `json:"recommended_next,omitempty"`
	Alternatives    []Pick          `json:"alternatives,omitempty"`
}

type Session struct {
	SchemaVersion   int           `json:"schema_version"`
	ID              string        `json:"id"`
	ProjectID       string        `json:"project_id"`
	Role            Role          `json:"role"`
	Agent           string        `json:"agent"`
	NativeSessionID string        `json:"native_session_id,omitempty"`
	Name            string        `json:"name,omitempty"`
	ObjectiveTaskID string        `json:"objective_task_id,omitempty"`
	Status          SessionStatus `json:"status"`
	Launched        bool          `json:"launched"`
	ResumedFrom     string        `json:"resumed_from,omitempty"`
	StartedAt       time.Time     `json:"started_at"`
	EndedAt         *time.Time    `json:"ended_at,omitempty"`
	ParkedAt        *time.Time    `json:"parked_at,omitempty"`
	Handoff         string        `json:"handoff,omitempty"`
}

// LastActivity is the latest of the session's start, park and end times.
func (s Session) LastActivity() time.Time {
	t := s.StartedAt
	for _, p := range []*time.Time{s.ParkedAt, s.EndedAt} {
		if p != nil && p.After(t) {
			t = *p
		}
	}
	return t
}

// Event is one line of the append-only events.jsonl.
type Event struct {
	ID      string         `json:"id"`
	TS      time.Time      `json:"ts"`
	Type    string         `json:"type"`
	Session string         `json:"session,omitempty"`
	Agent   string         `json:"agent"`
	Data    map[string]any `json:"data,omitempty"`
}
