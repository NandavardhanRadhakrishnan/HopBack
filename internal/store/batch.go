package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

var batchOps = map[string]func() Op{
	"task_add":            func() Op { return &TaskAdd{} },
	"task_update":         func() Op { return &TaskUpdate{} },
	"task_reorder":        func() Op { return &TaskReorder{} },
	"decision_add":        func() Op { return &DecisionAdd{} },
	"decision_resolve":    func() Op { return &DecisionResolve{} },
	"architecture_change": func() Op { return &ArchitectureChange{} },
	"review_clear":        func() Op { return &ReviewClear{} },
	"state_set":           func() Op { return &StateSet{} },
	"next_set":            func() Op { return &NextSet{} },
}

// DecodeBatch parses a JSON array of {"op": "<name>", ...fields} items.
// Unknown ops and unknown fields are rejected, naming the item.
func DecodeBatch(r io.Reader) ([]Op, error) {
	var raw []json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, &ValidationError{Msg: fmt.Sprintf("batch must be a JSON array of operations: %v", err)}
	}
	if len(raw) == 0 {
		return nil, invalid("batch has no operations")
	}
	ops := make([]Op, 0, len(raw))
	for i, item := range raw {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			return nil, invalid("batch item %d: %v", i, err)
		}
		var name string
		if err := json.Unmarshal(fields["op"], &name); err != nil || name == "" {
			return nil, invalid(`batch item %d: missing "op"`, i)
		}
		mk, ok := batchOps[name]
		if !ok {
			return nil, invalid("batch item %d: unknown op %q", i, name)
		}
		delete(fields, "op")
		rest, _ := json.Marshal(fields)
		op := mk()
		dec := json.NewDecoder(bytes.NewReader(rest))
		dec.DisallowUnknownFields()
		if err := dec.Decode(op); err != nil {
			return nil, invalid("batch item %d (%s): %v", i, name, err)
		}
		ops = append(ops, deref(op))
	}
	return ops, nil
}

func deref(op Op) Op {
	switch v := op.(type) {
	case *TaskAdd:
		return *v
	case *TaskUpdate:
		return *v
	case *TaskReorder:
		return *v
	case *DecisionAdd:
		return *v
	case *DecisionResolve:
		return *v
	case *ArchitectureChange:
		return *v
	case *ReviewClear:
		return *v
	case *StateSet:
		return *v
	case *NextSet:
		return *v
	}
	return op
}
