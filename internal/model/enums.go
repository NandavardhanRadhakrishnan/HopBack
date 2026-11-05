package model

import (
	"fmt"
	"slices"
	"strings"
)

type (
	TaskType       string
	Priority       string
	TaskStatus     string
	Origin         string
	DecisionStatus string
	Mode           string
	ProjectStatus  string
	Role           string
	SessionStatus  string
)

func TaskTypeValues() []string {
	return []string{"idea", "feature", "bug", "improvement", "tech-debt", "research", "experiment"}
}
func PriorityValues() []string   { return []string{"high", "medium", "low"} }
func TaskStatusValues() []string { return []string{"idea", "planned", "in_progress", "blocked", "completed", "cancelled"} }
func OriginValues() []string     { return []string{"user_requested", "agent_suggested"} }
func DecisionStatusValues() []string {
	return []string{"open", "decided", "superseded"}
}
func ModeValues() []string { return []string{"mid_feature", "between_features"} }
func ProjectStatusValues() []string {
	return []string{"active", "paused", "blocked", "completed", "archived"}
}
func RoleValues() []string          { return []string{"build", "concierge"} }
func SessionStatusValues() []string { return []string{"active", "parked", "interrupted", "completed"} }

func (v TaskType) Valid() bool       { return slices.Contains(TaskTypeValues(), string(v)) }
func (v Priority) Valid() bool       { return slices.Contains(PriorityValues(), string(v)) }
func (v TaskStatus) Valid() bool     { return slices.Contains(TaskStatusValues(), string(v)) }
func (v Origin) Valid() bool         { return slices.Contains(OriginValues(), string(v)) }
func (v DecisionStatus) Valid() bool { return slices.Contains(DecisionStatusValues(), string(v)) }
func (v Mode) Valid() bool           { return slices.Contains(ModeValues(), string(v)) }
func (v ProjectStatus) Valid() bool  { return slices.Contains(ProjectStatusValues(), string(v)) }
func (v Role) Valid() bool           { return slices.Contains(RoleValues(), string(v)) }
func (v SessionStatus) Valid() bool  { return slices.Contains(SessionStatusValues(), string(v)) }

// ParseOrigin accepts the short forms "user" and "agent" as well as the stored values.
func ParseOrigin(s string) (Origin, error) {
	switch s {
	case "user", "user_requested":
		return "user_requested", nil
	case "agent", "agent_suggested":
		return "agent_suggested", nil
	}
	return "", fmt.Errorf("invalid origin %q (allowed: user, agent, %s)", s, strings.Join(OriginValues(), ", "))
}

// Event types.
const (
	EvProjectInitialized         = "project_initialized"
	EvTaskCreated                = "task_created"
	EvTaskUpdated                = "task_updated"
	EvTasksReordered             = "tasks_reordered"
	EvDecisionRecorded           = "decision_recorded"
	EvDecisionResolved           = "decision_resolved"
	EvArchitectureChanged        = "architecture_changed"
	EvReviewCleared              = "review_cleared"
	EvStateUpdated               = "state_updated"
	EvNextSet                    = "next_set"
	EvSessionStarted             = "session_started"
	EvSessionParked              = "session_parked"
	EvSessionEndedWithoutHandoff = "session_ended_without_handoff"
	EvSessionCompleted           = "session_completed"
	EvSessionRevived             = "session_revived"
)
