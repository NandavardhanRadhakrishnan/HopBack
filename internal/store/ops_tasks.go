package store

import (
	"slices"
	"sort"
	"strings"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

func enumErr(field, val string, allowed []string) error {
	return invalid("invalid %s %q (allowed: %s)", field, val, strings.Join(allowed, ", "))
}

func checkTasksExist(tx *Tx, ids []string) error {
	for _, id := range ids {
		if _, err := tx.Task(id); err != nil {
			return err
		}
	}
	return nil
}

type TaskAdd struct {
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	Type        model.TaskType   `json:"type,omitempty"`
	Priority    model.Priority   `json:"priority,omitempty"`
	Status      model.TaskStatus `json:"status,omitempty"`
	DependsOn   []string         `json:"depends_on,omitempty"`
	Origin      model.Origin     `json:"origin"`
}

func (op TaskAdd) Apply(tx *Tx) error {
	title := strings.TrimSpace(op.Title)
	if title == "" {
		return invalid("title is required")
	}
	if op.Type == "" {
		op.Type = "feature"
	}
	if op.Priority == "" {
		op.Priority = "medium"
	}
	if op.Status == "" {
		op.Status = "planned"
	}
	if !op.Type.Valid() {
		return enumErr("type", string(op.Type), model.TaskTypeValues())
	}
	if !op.Priority.Valid() {
		return enumErr("priority", string(op.Priority), model.PriorityValues())
	}
	if !op.Status.Valid() {
		return enumErr("status", string(op.Status), model.TaskStatusValues())
	}
	if op.Origin == "" {
		return invalid("origin is required (user or agent)")
	}
	origin, err := model.ParseOrigin(string(op.Origin))
	if err != nil {
		return invalid("%v", err)
	}
	if err := checkTasksExist(tx, op.DependsOn); err != nil {
		return err
	}
	ids := make([]string, 0, len(tx.Backlog.Items))
	rank := 0
	for _, it := range tx.Backlog.Items {
		ids = append(ids, it.ID)
		rank = max(rank, it.Rank)
	}
	t := model.Task{
		ID: model.NextSeqID("T-", ids), Title: title, Description: op.Description,
		Type: op.Type, Priority: op.Priority, Status: op.Status, Rank: rank + 1,
		DependsOn: op.DependsOn, CreatedAt: tx.Now, UpdatedAt: tx.Now,
		SourceSession: tx.Attr.Session, RecordedBy: tx.Attr.Agent, Origin: origin,
	}
	if t.Status == "completed" {
		now := tx.Now
		t.CompletedAt = &now
	}
	tx.Backlog.Items = append(tx.Backlog.Items, t)
	tx.MarkBacklog()
	tx.Emit(model.EvTaskCreated, map[string]any{"task_id": t.ID, "title": t.Title, "origin": string(origin)})
	tx.created = append(tx.created, t.ID)
	return nil
}

// TaskUpdate changes only the fields that are set.
type TaskUpdate struct {
	ID          string            `json:"id"`
	Title       *string           `json:"title,omitempty"`
	Description *string           `json:"description,omitempty"`
	Type        *model.TaskType   `json:"type,omitempty"`
	Priority    *model.Priority   `json:"priority,omitempty"`
	Status      *model.TaskStatus `json:"status,omitempty"`
	DependsOn   *[]string         `json:"depends_on,omitempty"`
}

func (op TaskUpdate) Apply(tx *Tx) error {
	t, err := tx.Task(op.ID)
	if err != nil {
		return err
	}
	var changes []string
	if op.Title != nil {
		title := strings.TrimSpace(*op.Title)
		if title == "" {
			return invalid("title cannot be empty")
		}
		t.Title = title
		changes = append(changes, "title")
	}
	if op.Description != nil {
		t.Description = *op.Description
		changes = append(changes, "description")
	}
	if op.Type != nil {
		if !op.Type.Valid() {
			return enumErr("type", string(*op.Type), model.TaskTypeValues())
		}
		t.Type = *op.Type
		changes = append(changes, "type")
	}
	if op.Priority != nil {
		if !op.Priority.Valid() {
			return enumErr("priority", string(*op.Priority), model.PriorityValues())
		}
		t.Priority = *op.Priority
		changes = append(changes, "priority")
	}
	if op.Status != nil {
		if !op.Status.Valid() {
			return enumErr("status", string(*op.Status), model.TaskStatusValues())
		}
		if *op.Status == "completed" && t.Status != "completed" {
			now := tx.Now
			t.CompletedAt = &now
		} else if *op.Status != "completed" {
			t.CompletedAt = nil
		}
		t.Status = *op.Status
		changes = append(changes, "status")
	}
	if op.DependsOn != nil {
		deps := *op.DependsOn
		if slices.Contains(deps, op.ID) {
			return invalid("task %s cannot depend on itself", op.ID)
		}
		if err := checkTasksExist(tx, deps); err != nil {
			return err
		}
		t.DependsOn = deps
		changes = append(changes, "depends_on")
	}
	if len(changes) == 0 {
		return invalid("nothing to update for %s", op.ID)
	}
	t.UpdatedAt = tx.Now
	tx.MarkBacklog()
	tx.Emit(model.EvTaskUpdated, map[string]any{"task_id": op.ID, "changes": changes})
	return nil
}

// TaskReorder moves the listed tasks to the top of the backlog, in order.
type TaskReorder struct {
	IDs []string `json:"ids"`
}

func (op TaskReorder) Apply(tx *Tx) error {
	if len(op.IDs) == 0 {
		return invalid("reorder needs at least one task ID")
	}
	seen := map[string]bool{}
	for _, id := range op.IDs {
		if seen[id] {
			return invalid("task %s listed twice", id)
		}
		seen[id] = true
		if _, err := tx.Task(id); err != nil {
			return err
		}
	}
	items := tx.Backlog.Items
	sort.SliceStable(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
	byID := map[string]model.Task{}
	for _, it := range items {
		byID[it.ID] = it
	}
	out := make([]model.Task, 0, len(items))
	for _, id := range op.IDs {
		out = append(out, byID[id])
	}
	for _, it := range items {
		if !seen[it.ID] {
			out = append(out, it)
		}
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	tx.Backlog.Items = out
	tx.MarkBacklog()
	tx.Emit(model.EvTasksReordered, map[string]any{"ids": op.IDs})
	return nil
}
