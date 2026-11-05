# Hopback MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `hopback` CLI that stores per-project continuity data in `.hopback/`, shows an instant status brief, and drives Claude Code through build and concierge sessions with deliberate parking.

**Architecture:**
- **Core packages** (model, store, session, brief, project, registry, protocol) know nothing about any specific agent.
- **Claude Code specifics** live in one adapter behind `agent.Adapter`.
- **The launcher** runs agent sessions as attached child processes and records their outcome.
- **The CLI** (cobra) wires everything together.
- **Writes** happen under a per-project file lock, using atomic file replacement and an append-only event log.

**Tech Stack:**
- Go 1.27 (installed: go1.27.3 windows/amd64).
- `github.com/spf13/cobra` for the CLI.
- `github.com/gofrs/flock` for cross-platform file locks.
- The standard library for everything else.

**Spec:** `docs/superpowers/specs/2026-11-05-hopback-mvp-design.md` (read it; this plan argues from it). Product background: `docs/PRD.md`.

## Global Constraints

- **Module and binary:** module path `github.com/NandavardhanRadhakrishnan/HopBack`; the binary is `hopback`, built from `./cmd/hopback`.
- **Dependencies:** only cobra and gofrs/flock. No UUID library, no TUI library, no SQLite.
- **No AI:** no LLM or AI call anywhere in Hopback. Every brief, handoff and recommendation is agent-written; Hopback stores and displays them.
- **Agent isolation:** packages `model`, `store`, `session`, `brief`, `project`, `registry` and `protocol` must not import `internal/agent/claude` and must not mention Claude Code. Agent specifics live only in `internal/agent/claude`.
- **Windows:** must work on Windows (the owner's machine). Use `filepath` for paths. No shell-specific behaviour. Must also build on Linux and macOS.
- **Schema version:** every JSON file Hopback writes carries `"schema_version": 1`. JSON field names are snake_case (e.g. `needs_review`, `native_session_id`, `expect_revision`).
- **Timestamps:** UTC, RFC 3339 (Go's default `time.Time` JSON encoding of UTC times).
- **Writes:** every write to `.hopback/` goes through the project lock plus an atomic temp-and-rename. `events.jsonl` is only ever appended to.
- **Agent-facing commands:** on validation failure, print `error: <message>` to stderr and exit 1. With `--json`, success output is JSON on stdout.
- **Inactivity:** never shown as a warning. "Last worked N days ago" is neutral text.
- **Machine-local files:** `.hopback/.lock`, `.hopback/run/` and temp files `*.tmp-*` are gitignored via `.hopback/.gitignore`.
- **Commits:** end every commit message with the Co-Authored-By trailer the executing session requires.

## Review Focus

Five input classes or failure modes that no task's tests would otherwise exercise. Each is pinned by a test in the task that owns the code.

1. **A `.hopback` JSON file corrupted by a hand edit or git merge conflict markers.** Expect a clear error naming the file, and the file is left untouched. *Task 5:* `TestLoad_InvalidJSONReportsFile`.
2. **Two agents writing to the same project at the same moment.** Both writes land and IDs never collide. *Task 5:* `TestApply_ConcurrentWriters`.
3. **A `HOPBACK_SESSION` value inherited into an unrelated shell or another project.** Ignore it, fall back to native detection, and never error. *Task 7:* `TestCurrent_StaleHopbackSessionFallsBack`.
4. **A handoff written on Windows** (CRLF line endings, odd heading case, trailing spaces). Parking still succeeds. *Task 7:* `TestPark_AcceptsCRLFAndCase`.
5. **Running a `hopback` command outside any project, or from a subdirectory.** From a subdirectory it finds the root. Outside a project it says to run `hopback init`. *Task 4:* `TestFind_WalksUp` and `TestFind_NotProject`. *Task 11:* `TestOutsideProject`.

---

## File Structure

```
cmd/hopback/main.go                 entry point → cli.Execute()
internal/fsutil/atomic.go           WriteFileAtomic (temp + rename, retries)
internal/fsutil/lock.go             TryLock / AcquireLock (gofrs/flock)
internal/model/model.go             all data types + JSON tags
internal/model/enums.go             enum types, Valid(), Parse helpers
internal/model/ids.go               NewSessionID, NewUUID, NextSeqID
internal/protocol/protocol.go       template accessors, marker, prompts
internal/protocol/templates/HOPBACK.md
internal/protocol/templates/park-skill.md
internal/project/project.go         Find, Init, EnsureTemplates, UpsertPointerBlock
internal/registry/registry.go       ~/.hopback/projects.json
internal/store/store.go             Store, Snapshot, Tx, Apply, events, errors
internal/store/ops_tasks.go         TaskAdd, TaskUpdate, TaskReorder
internal/store/ops_project.go       DecisionAdd/Resolve, ArchitectureChange, ReviewClear, StateSet, NextSet
internal/store/batch.go             DecodeBatch
internal/session/session.go         Manager: records, Current, Reconcile, queries
internal/session/park.go            Park + handoff validation
internal/brief/brief.go             Build, Render, Ago, Summary
internal/brief/changes.go           ChangesSince
internal/agent/agent.go             Adapter interface, LaunchSpec, ErrNotOwned
internal/agent/claude/claude.go     Claude Code adapter
internal/agent/agenttest/fake.go    Fake adapter + helper-process scenarios (tests only)
internal/launcher/launcher.go       Build / Reopen / Concierge lifecycle
internal/cli/app.go                 App, Execute, project context, output helpers
internal/cli/agentcmds.go           agent-facing commands
internal/cli/humancmds.go           init, projects, status, sessions, resume, concierge
internal/cli/interactive.go         root `hopback` flow, menu, picker, post-session loop
```

Shared test clock used throughout: `fixed := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)`.

---

### Task 1: Module scaffold and filesystem primitives

**Files:**
- Create: `go.mod`, `.gitignore`, `internal/fsutil/atomic.go`, `internal/fsutil/lock.go`
- Modify: `CLAUDE.md` (replace "Current state" section)
- Test: `internal/fsutil/fsutil_test.go`

**Interfaces:**
- Produces:
  - `func WriteFileAtomic(path string, data []byte, perm os.FileMode) error`. Writes `path + ".tmp-" + 8 random hex chars` in the same dir, fsyncs, then renames over `path`. It retries the rename up to 10 times, 20 ms apart, because Windows sharing violations can make a rename fail briefly. On final failure it removes the temp file.
  - `var renameFunc = os.Rename`: a test hook.
  - `type Lock struct` with `func (l *Lock) Release() error`.
  - `func TryLock(path string) (*Lock, bool, error)`: non-blocking. Creates parent dirs.
  - `func AcquireLock(path string, timeout time.Duration) (*Lock, error)`: retries every 10 ms. Returns `ErrLockTimeout` on timeout.
  - `var ErrLockTimeout = errors.New("timed out waiting for the project lock")`.

- [ ] **Step 1: Initialise the module and repo ignores**

Run: `go mod init github.com/NandavardhanRadhakrishnan/HopBack && go get github.com/gofrs/flock@latest github.com/spf13/cobra@latest`

Create `.gitignore` with these lines: `/hopback`, `/hopback.exe`, `/dist/`.

- [ ] **Step 2: Write the failing tests**

```go
func TestWriteFileAtomic_ReplacesContent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	must(t, WriteFileAtomic(p, []byte("a"), 0o644))
	must(t, WriteFileAtomic(p, []byte("b"), 0o644))
	got, _ := os.ReadFile(p)
	if string(got) != "b" { t.Fatalf("got %q", got) }
	if m, _ := filepath.Glob(p + ".tmp-*"); len(m) != 0 { t.Fatalf("leftover temp files %v", m) }
}
func TestWriteFileAtomic_RetriesRename(t *testing.T) {
	calls := 0
	renameFunc = func(a, b string) error { calls++; if calls < 3 { return errors.New("sharing violation") }; return os.Rename(a, b) }
	defer func() { renameFunc = os.Rename }()
	must(t, WriteFileAtomic(filepath.Join(t.TempDir(), "f"), []byte("x"), 0o644))
	if calls != 3 { t.Fatalf("calls=%d", calls) }
}
func TestWriteFileAtomic_GivesUpAndCleansTemp(t *testing.T) {
	renameFunc = func(a, b string) error { return errors.New("nope") }
	defer func() { renameFunc = os.Rename }()
	dir := t.TempDir()
	if err := WriteFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0o644); err == nil { t.Fatal("want error") }
	if m, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*")); len(m) != 0 { t.Fatalf("temp not removed: %v", m) }
}
func TestTryLock_Exclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	l1, ok1, _ := TryLock(p); _, ok2, _ := TryLock(p)
	if !ok1 || ok2 { t.Fatalf("ok1=%v ok2=%v", ok1, ok2) }
	must(t, l1.Release())
	if _, ok3, _ := TryLock(p); !ok3 { t.Fatal("lock not reacquirable after release") }
}
func TestAcquireLock_Timeout(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	_, _, _ = TryLock(p)
	if _, err := AcquireLock(p, 50*time.Millisecond); !errors.Is(err, ErrLockTimeout) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run: `go test ./internal/fsutil/`

Expected: build failure (undefined: WriteFileAtomic).

- [ ] **Step 4: Implement `atomic.go` and `lock.go`**

Implement them per the Interfaces block. `Lock` wraps `*flock.Flock`. `TryLock` uses `flock.New(path).TryLock()`.

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./internal/fsutil/`

Expected: `ok`.

- [ ] **Step 6: Update `CLAUDE.md`**

Replace the "Current state" section with:

```markdown
## Commands

- Build: `go build ./...` (binary: `go build -o hopback.exe ./cmd/hopback`; install: `go install ./cmd/hopback`)
- Test all: `go test ./...`
- Single test: `go test ./internal/store -run TestTaskAdd_AssignsIDRankAndDefaults -v`
- Vet: `go vet ./...`

The MVP design (`docs/superpowers/specs/2026-11-05-hopback-mvp-design.md`) refines the PRD and wins where they differ (e.g. no SQLite). The implementation plan is `docs/superpowers/plans/2026-11-05-hopback-mvp.md`.
```

- [ ] **Step 7: Commit**

`git add -A && git commit -m "Scaffold Go module with atomic writes and file locks"`

---

### Task 2: Data model

**Files:**
- Create: `internal/model/model.go`, `internal/model/enums.go`, `internal/model/ids.go`
- Test: `internal/model/model_test.go`

**Interfaces:**
- Produces (all JSON tags snake_case; optional fields `omitempty`; pointers for optional times and structs):

```go
const SchemaVersion = 1
type Attribution struct { Session, Agent string }
type Project struct { SchemaVersion int; ID, Name, Description string; Stack []string; CreatedAt time.Time }
type Review struct { Reason, EventID string }
type Task struct {
	ID, Title, Description string; Type TaskType; Priority Priority; Status TaskStatus; Rank int
	DependsOn []string; NeedsReview *Review; CreatedAt, UpdatedAt time.Time; CompletedAt *time.Time
	SourceSession, RecordedBy string; Origin Origin }
type Backlog struct { SchemaVersion int; Items []Task }
type Option struct { Name, Tradeoffs string }
type Decision struct {
	ID, Title string; Status DecisionStatus; Context string; Options []Option
	Decision, Rationale, Supersedes string; CreatedAt time.Time; DecidedAt *time.Time
	SourceSession, RecordedBy string }
type Decisions struct { SchemaVersion int; Items []Decision }
type CurrentFeature struct { TaskID, Why, StoppedAt, NextStep string }
type Pick struct { TaskID, Reason string }
type State struct {
	SchemaVersion, Revision int; UpdatedAt *time.Time; UpdatedBy *Attribution
	ProjectStatus ProjectStatus; Mode Mode; BottomLine string; CurrentFeature *CurrentFeature
	Blockers []string; RecommendedNext *Pick; Alternatives []Pick }
type Session struct {
	SchemaVersion int; ID, ProjectID string; Role Role; Agent, NativeSessionID, Name, ObjectiveTaskID string
	Status SessionStatus; Launched bool; ResumedFrom string; StartedAt time.Time; EndedAt, ParkedAt *time.Time
	Handoff string }
type Event struct { ID string; TS time.Time; Type, Session, Agent string; Data map[string]any }
func (t Task) IsOpen() bool                 // false for completed, cancelled
func (s Session) LastActivity() time.Time   // max(StartedAt, *ParkedAt, *EndedAt)
```

- Enums are string types with `Valid() bool` and `func XValues() []string`, used in error messages:
  - `TaskType`: `idea feature bug improvement tech-debt research experiment`
  - `Priority`: `high medium low`
  - `TaskStatus`: `idea planned in_progress blocked completed cancelled`
  - `Origin`: `user_requested agent_suggested`
  - `DecisionStatus`: `open decided superseded`
  - `Mode`: `mid_feature between_features`
  - `ProjectStatus`: `active paused blocked completed archived`
  - `Role`: `build concierge`
  - `SessionStatus`: `active parked interrupted completed`
- `func ParseOrigin(s string) (Origin, error)` accepts `user`, `agent`, `user_requested` and `agent_suggested`.
- Event type constants: `EvProjectInitialized="project_initialized"`, `EvTaskCreated="task_created"`, `EvTaskUpdated="task_updated"`, `EvTasksReordered="tasks_reordered"`, `EvDecisionRecorded="decision_recorded"`, `EvDecisionResolved="decision_resolved"`, `EvArchitectureChanged="architecture_changed"`, `EvReviewCleared="review_cleared"`, `EvStateUpdated="state_updated"`, `EvNextSet="next_set"`, `EvSessionStarted="session_started"`, `EvSessionParked="session_parked"`, `EvSessionEndedWithoutHandoff="session_ended_without_handoff"`, `EvSessionCompleted="session_completed"`, `EvSessionRevived="session_revived"`.
- `func NewSessionID(now time.Time) string`: `now.UTC().Format("20060102T150405Z") + "-" + 6 random hex chars`. Also used for event IDs.
- `func NewUUID() string`: RFC 4122 v4 from `crypto/rand`.
- `func NextSeqID(prefix string, existing []string) string`: max numeric suffix + 1, ignoring IDs without the prefix.

- [ ] **Step 1: Write the failing tests**

```go
func TestNewSessionID_FormatAndOrder(t *testing.T) {
	a, b := NewSessionID(fixed), NewSessionID(fixed.Add(time.Second))
	if !regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{6}$`).MatchString(a) { t.Fatal(a) }
	if !(a < b) { t.Fatalf("%s !< %s", a, b) }
}
func TestNewUUID_V4(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	u1, u2 := NewUUID(), NewUUID()
	if !re.MatchString(u1) || u1 == u2 { t.Fatal(u1, u2) }
}
func TestNextSeqID(t *testing.T) {
	if got := NextSeqID("T-", []string{"T-1", "T-10", "T-3", "X-50"}); got != "T-11" { t.Fatal(got) }
	if got := NextSeqID("D-", nil); got != "D-1" { t.Fatal(got) }
}
func TestEnums(t *testing.T) {
	if !TaskType("tech-debt").Valid() || TaskType("chore").Valid() { t.Fatal("TaskType") }
	if !TaskStatus("in_progress").Valid() || TaskStatus("done").Valid() { t.Fatal("TaskStatus") }
	if o, _ := ParseOrigin("agent"); o != "agent_suggested" { t.Fatal(o) }
	if _, err := ParseOrigin("me"); err == nil { t.Fatal("ParseOrigin accepted me") }
}
func TestTaskIsOpen(t *testing.T) {
	for s, want := range map[TaskStatus]bool{"completed": false, "cancelled": false, "blocked": true, "idea": true} {
		if (Task{Status: s}).IsOpen() != want { t.Fatal(s) }
	}
}
func TestJSONFieldNames(t *testing.T) {
	b, _ := json.Marshal(Task{NeedsReview: &Review{Reason: "r"}, CompletedAt: &fixed})
	s, _ := json.Marshal(Session{NativeSessionID: "n"})
	for _, k := range []string{`"needs_review"`, `"completed_at"`, `"event_id"`} { if !bytes.Contains(b, []byte(k)) { t.Fatal(k) } }
	if !bytes.Contains(s, []byte(`"native_session_id"`)) { t.Fatal("native_session_id") }
}
func TestSessionLastActivity(t *testing.T) {
	later := fixed.Add(time.Hour)
	if (Session{StartedAt: fixed, ParkedAt: &later}).LastActivity() != later { t.Fatal() }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/model/`

Expected: build failure.

- [ ] **Step 3: Implement `model.go`, `enums.go` and `ids.go`**

Implement them per the Interfaces block.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/model/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git commit -am "Add data model, enums and ID generation"` (`git add internal/model` first).

---

### Task 3: Protocol templates and prompts

**Files:**
- Create: `internal/protocol/protocol.go`, `internal/protocol/templates/HOPBACK.md`, `internal/protocol/templates/park-skill.md`
- Test: `internal/protocol/protocol_test.go`

**Interfaces:**
- Consumes: `model.Task` (Task 2).
- Produces:
  - `const TemplateVersion = 1`.
  - `func Marker() string`: returns `"<!-- hopback-template v1 -->"`.
  - `func ParseMarkerVersion(b []byte) (int, bool)`: finds `<!-- hopback-template v(\d+) -->` anywhere in `b`.
  - `func HopbackMD() []byte` and `func ParkSkill() []byte`: embedded via `//go:embed`.
  - `func PointerBlock() string`.
  - `func BuildInstructions() string` and `func ConciergeInstructions() string`: role text for the system prompt, each under 2000 bytes to stay within Windows command-line limits.
  - `func BuildPrompt(objective *model.Task) string`.
  - `func ReopenPrompt() string`.
  - `func ConciergePrompt(changes []string, firstTime bool) string`.

- [ ] **Step 1: Write the failing tests**

```go
func TestHopbackMD_Content(t *testing.T) {
	s := string(HopbackMD())
	for _, want := range []string{Marker(), "## Objective", "## Done", "## Stopped at", "## Unfinished", "## Gotchas", "## Next step",
		"hopback session park", "hopback batch", "--origin user|agent", "hopback architecture change"} {
		if !strings.Contains(s, want) { t.Fatalf("HOPBACK.md missing %q", want) }
	}
}
func TestParseMarkerVersion(t *testing.T) {
	if v, ok := ParseMarkerVersion([]byte("x\n<!-- hopback-template v3 -->\n")); v != 3 || !ok { t.Fatal(v, ok) }
	if _, ok := ParseMarkerVersion([]byte("plain")); ok { t.Fatal("found marker in plain text") }
}
func TestParkSkill_FrontmatterFirst(t *testing.T) {
	s := string(ParkSkill())
	if !strings.HasPrefix(s, "---\nname: park\n") || !strings.Contains(s, Marker()) { t.Fatal(s) }
}
func TestPointerBlock(t *testing.T) {
	b := PointerBlock()
	if !strings.HasPrefix(b, "<!-- hopback:begin -->") || !strings.HasSuffix(b, "<!-- hopback:end -->\n") { t.Fatal(b) }
}
func TestPrompts(t *testing.T) {
	if p := BuildPrompt(&model.Task{ID: "T-12", Title: "Diff view"}); !strings.Contains(p, "T-12: Diff view") { t.Fatal(p) }
	if p := BuildPrompt(nil); !strings.Contains(p, "ask me what to work on") { t.Fatal(p) }
	if p := ConciergePrompt([]string{"a", "b"}, false); !strings.Contains(p, "- a\n- b") { t.Fatal(p) }
	if p := ConciergePrompt(nil, true); !strings.Contains(p, "first concierge session") { t.Fatal(p) }
	if len(BuildInstructions()) >= 2000 || len(ConciergeInstructions()) >= 2000 { t.Fatal("instructions too long") }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/protocol/`

Expected: build failure.

- [ ] **Step 3: Write the templates and accessors.** Use this exact copy (the spec fixes this content).

`templates/HOPBACK.md`:

````markdown
<!-- hopback-template v1 -->
# Hopback

This project uses **Hopback** to keep track of where the work stands across agent sessions. The files in `.hopback/` are the project's memory: the backlog, decisions, the latest status, and a handoff from every parked session. The developer reads a brief built from these files when they return to the project, so keep them accurate.

You, the agent, do the thinking: you write the status, the handoffs and the recommendations. Hopback only stores and shows them.

## Files

| File | Contents |
|---|---|
| `project.json` | Project identity |
| `state.json` | Latest status: mode, bottom line, current feature, blockers, recommended next |
| `backlog.json` | Backlog items (`T-n`), ordered by `rank` |
| `decisions.json` | Decisions (`D-n`): open questions for the developer, and decisions made |
| `events.jsonl` | Append-only history of every change |
| `sessions/<id>.json` | One record per agent session |
| `sessions/<id>.md` | Handoff written when a build session parks |

Read these files directly whenever you like. **Change them only through the `hopback` command.** It locks, validates and records history. Never edit the JSON by hand.

## Starting a session

1. Run `hopback context`. It prints one JSON document with the project, state, open tasks, items needing review, open decisions, the latest handoff and your session.
2. Tell the developer briefly where things stand: the bottom line, what needs their decision, and what you'll do next.
3. Get to work.

## Recording changes as you work

| Situation | What to do |
|---|---|
| The developer explicitly asks ("add this to the backlog", "let's go with Postgres") | Record it directly, with `--origin user`. |
| You come up with an idea, feature, refactor or decision yourself | Ask the developer first. Record it only if they agree, with `--origin agent`. |
| You start, finish or get blocked on a task | Update its status directly. |
| A change may affect other backlog items | `hopback architecture change --summary "..." --affects T-3,T-7`. Never change or cancel those tasks yourself; they are flagged for the developer to review. |
| You need the developer to make a call you can't | Record an open decision: `hopback decision add --title "..." --option "name: tradeoffs"` |

Record project-level changes, not every edit.

## Commands

```
hopback context
hopback task add --title T [--description D] [--type feature|idea|bug|improvement|tech-debt|research|experiment]
                 [--priority high|medium|low] [--status idea|planned|in_progress|blocked] [--depends-on T-1,T-2]
                 --origin user|agent
hopback task update T-n [--status S] [--priority P] [--title T] [--description D] [--type Y] [--depends-on T-1,T-2]
hopback task list [--all] [--json]
hopback task reorder T-5 T-2              # listed tasks move to the top, in that order
hopback decision add --title T [--context C] [--option "name: tradeoffs"]... [--decision D --rationale R] [--supersedes D-n]
hopback decision resolve D-n --decision D [--rationale R]
hopback architecture change --summary S --affects T-1,T-2
hopback review clear T-n
hopback state get
hopback state set --expect-revision N --file state.json      # --file - reads stdin
hopback next set T-n --reason R [--alt "T-m: reason"]...
hopback batch --file ops.json                                # many changes in one call, all or nothing
hopback session park --handoff-file handoff.md               # --handoff-file - reads stdin
```

`batch` takes a JSON array of operations. Each one looks like `{"op": "<name>", ...fields}`. The op names are `task_add`, `task_update`, `task_reorder`, `decision_add`, `decision_resolve`, `architecture_change`, `review_clear`, `state_set` and `next_set`. Each takes the same fields as its command, in snake_case: `depends_on`, `task_id`, `ids`, `expect_revision`, and `state` for `state_set`.

`state set` takes:

```json
{
  "mode": "mid_feature | between_features",
  "bottom_line": "One or two sentences a busy reader needs first.",
  "current_feature": {"task_id": "T-n", "why": "...", "stopped_at": "...", "next_step": "..."},
  "blockers": ["..."],
  "recommended_next": {"task_id": "T-n", "reason": "..."},
  "alternatives": [{"task_id": "T-m", "reason": "..."}],
  "project_status": "active | paused | blocked | completed | archived"
}
```

`current_feature` is required when `mode` is `mid_feature`. Pass the `revision` from `hopback state get` as `--expect-revision`. If someone else has changed the state since you read it, the write is refused: read it again and retry.

## Parking (when the developer types /park or asks to park)

1. Update the status of the tasks you worked on.
2. Record any decisions not yet recorded.
3. Run `hopback state set` with the new status. Write it like a status email to a busy executive: bottom line first.
4. Write the handoff for the next agent. It must contain these sections (any heading level; a section may just say "none"):
   - `## Objective`
   - `## Done`
   - `## Stopped at`
   - `## Unfinished`
   - `## Gotchas`
   - `## Next step`
5. Run `hopback session park --handoff-file <file>`.

If `session park` refuses, fix what it lists and run it again. If you can't complete it, tell the developer plainly that parking failed. Never say the session is parked unless that command succeeded.

## The concierge

A concierge session is for planning and doesn't change project code. It reviews the backlog with the developer, reorders and updates it, records decisions, and records what to build next with `hopback next set`. Use `hopback batch` for bulk changes. Every concierge starts fresh, so write down anything worth keeping as it happens.
````

`templates/park-skill.md`:

```markdown
---
name: park
description: Park this Hopback session - update the project state and write the handoff so the developer can pick up later.
---
<!-- hopback-template v1 -->
Park this session following the "Parking" section of `.hopback/HOPBACK.md`:

1. Update the tasks you worked on (`hopback task update`).
2. Record decisions not yet recorded.
3. Run `hopback state get`, then `hopback state set --expect-revision <revision>` with the new status.
4. Write the handoff with the sections Objective, Done, Stopped at, Unfinished, Gotchas and Next step.
5. Run `hopback session park --handoff-file <file>`.

Report the result honestly. If `hopback session park` refuses, fix what it lists and retry. If you can't, say that parking failed.
```

`PointerBlock()`:

```
<!-- hopback:begin -->
## Hopback

This project uses Hopback for project continuity. Read `.hopback/HOPBACK.md` at the start of a session and follow it: record project-level changes with the `hopback` command, and when the developer asks to park, follow its parking procedure.
<!-- hopback:end -->
```

`BuildInstructions()`:

> You are a build session in a project managed by Hopback. Follow .hopback/HOPBACK.md: record meaningful project-level changes with the `hopback` command as you work (ask the developer before recording your own ideas), and when the developer types /park or asks to park, follow its parking procedure. Never claim the session is parked unless `hopback session park` succeeded.

`ConciergeInstructions()`:

> You are the Hopback concierge for this project: a planning session. Do not change project code. Help the developer decide what to build next using the backlog, decisions and latest handoff. Change project state only through the `hopback` command (use `hopback batch` for bulk changes) and never edit .hopback JSON by hand. Write down anything worth keeping (priorities, decisions, reasons for postponing) as it happens, because the next concierge starts fresh and only sees the files. Record the chosen next task with `hopback next set`.

`BuildPrompt(t)`:

> Read .hopback/HOPBACK.md and run `hopback context`. Give me a short brief of where things stand, bottom line first. Then work on {ID}: {Title}.

When `t == nil`, the last sentence is "Then ask me what to work on."

`ReopenPrompt()`:

> Welcome back. Things may have changed since this conversation: run `hopback context`, then give me a short brief of where we were and what has changed.

`ConciergePrompt(changes, first)`:

- It opens with "Read .hopback/HOPBACK.md and run `hopback context`.", then a blank line.
- Next comes one of three lines:
  - first session: "This is the first concierge session for this project."
  - changes exist: "Changes since the last concierge session:" followed by `- <line>` for each change
  - no changes: "No changes since the last concierge session."
- After a blank line, it ends: "Brief me like a consultant briefing an executive: bottom line, what needs my decision, and what you recommend building next with a reason and a couple of alternatives. Then help me decide, and record the outcome with `hopback next set`."

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/protocol/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/protocol && git commit -m "Add Hopback protocol templates and session prompts"`

---

### Task 4: Project init/discovery and the machine-local registry

**Files:**
- Create: `internal/project/project.go`, `internal/registry/registry.go`
- Test: `internal/project/project_test.go`, `internal/registry/registry_test.go`

**Interfaces:**
- Consumes: `fsutil.WriteFileAtomic`, `model.*`, `protocol.*`.
- Produces (`project`):
  - `const Dir = ".hopback"`.
  - `var ErrNotProject = errors.New("not inside a Hopback project (run `hopback init`)")`.
  - `func Find(start string) (string, error)`: walks up to the first dir containing `.hopback/project.json`.
  - `type InitOptions struct { Name string; Installers []func(root string) error }`.
  - `func Init(dir string, opts InitOptions, now time.Time) (model.Project, bool, error)`. The bool is `created`.
    - If `.hopback/project.json` already exists, it only refreshes templates and pointer blocks, runs the installers, and returns the existing project with `false`.
    - Otherwise it writes:
      - `project.json`: new UUID, name defaulting to `filepath.Base(dir)`.
      - `state.json`: revision 0, `project_status: active`.
      - `backlog.json` and `decisions.json`: empty `items`.
      - `events.jsonl`: one `project_initialized` event.
      - the `sessions/` and `run/` directories.
      - `.gitignore` with the lines `.lock`, `run/`, `*.tmp-*`.
      - `HOPBACK.md`.
    - It also adds the pointer block to `CLAUDE.md` (creating it if missing) and to `AGENTS.md` (only if that file exists), then runs the installers.
  - `func EnsureTemplates(root string) error`: rewrites `.hopback/HOPBACK.md` when its marker version is below `protocol.TemplateVersion` or it's missing. Never downgrades.
  - `func UpsertPointerBlock(path string, create bool) error`: replaces the text between `<!-- hopback:begin -->` and `<!-- hopback:end -->` if present. Otherwise appends the block after a blank line. If the file is missing and `create` is false, it does nothing.
- Produces (`registry`):
  - `type Entry struct { ID, Name, Path string; RegisteredAt time.Time }` (JSON snake_case). The file is `{"schema_version":1,"projects":[...]}`.
  - `func Home(env func(string) string) (string, error)`: returns `HOPBACK_HOME` if set, otherwise `~/.hopback`.
  - `func Load(home string) ([]Entry, error)`: a missing file gives an empty list.
  - `func Register(home string, e Entry) error`: upsert by ID, under the lock `home/projects.lock`.
  - `func Lookup(home, nameOrID string) (Entry, error)`: case-insensitive name or exact ID.
  - `var ErrUnknownProject`.

- [ ] **Step 1: Write the failing tests**

```go
// project_test.go
func TestInit_CreatesLayout(t *testing.T) {
	root := t.TempDir()
	p, created, err := Init(root, InitOptions{Name: "Docslet"}, fixed)
	if err != nil || !created || p.Name != "Docslet" || p.SchemaVersion != 1 { t.Fatal(p, created, err) }
	for _, f := range []string{"project.json", "state.json", "backlog.json", "decisions.json", "events.jsonl", "HOPBACK.md", ".gitignore", "sessions", "run"} {
		if _, err := os.Stat(filepath.Join(root, Dir, f)); err != nil { t.Fatal(f, err) }
	}
	var st model.State; readJSON(t, filepath.Join(root, Dir, "state.json"), &st)
	if st.Revision != 0 || st.ProjectStatus != "active" { t.Fatal(st) }
	lines := readLines(t, filepath.Join(root, Dir, "events.jsonl"))
	if len(lines) != 1 || !strings.Contains(lines[0], `"project_initialized"`) { t.Fatal(lines) }
}
func TestInit_IdempotentAndPointerBlocks(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("hello\n"), 0o644)
	p1, _, _ := Init(root, InitOptions{}, fixed)
	p2, created, _ := Init(root, InitOptions{}, fixed)
	if created || p1.ID != p2.ID { t.Fatal("second init must reuse project") }
	claude := readFile(t, filepath.Join(root, "CLAUDE.md"))
	agents := readFile(t, filepath.Join(root, "AGENTS.md"))
	if strings.Count(claude, "<!-- hopback:begin -->") != 1 || strings.Count(agents, "<!-- hopback:begin -->") != 1 || !strings.HasPrefix(agents, "hello") { t.Fatal(claude, agents) }
}
func TestInit_NoAgentsMdCreated(t *testing.T) {
	root := t.TempDir(); Init(root, InitOptions{}, fixed)
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) { t.Fatal("AGENTS.md must not be created") }
}
func TestUpsertPointerBlock_ReplacesExisting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CLAUDE.md")
	os.WriteFile(p, []byte("top\n<!-- hopback:begin -->\nOLD\n<!-- hopback:end -->\nbottom\n"), 0o644)
	must(t, UpsertPointerBlock(p, false))
	s := readFile(t, p)
	if strings.Contains(s, "OLD") || !strings.HasPrefix(s, "top\n") || !strings.HasSuffix(s, "bottom\n") { t.Fatal(s) }
}
func TestInit_RunsInstallers(t *testing.T) {
	var got []string; root := t.TempDir()
	Init(root, InitOptions{Installers: []func(string) error{func(r string) error { got = append(got, r); return nil }}}, fixed)
	if len(got) != 1 || got[0] != root { t.Fatal(got) }
}
func TestFind_WalksUp(t *testing.T) {
	root := t.TempDir(); Init(root, InitOptions{}, fixed)
	sub := filepath.Join(root, "a", "b"); os.MkdirAll(sub, 0o755)
	if got, err := Find(sub); err != nil || got != root { t.Fatal(got, err) }
}
func TestFind_NotProject(t *testing.T) {
	_, err := Find(t.TempDir())
	if !errors.Is(err, ErrNotProject) || !strings.Contains(err.Error(), "hopback init") { t.Fatal(err) }
}
func TestEnsureTemplates(t *testing.T) {
	root := t.TempDir(); Init(root, InitOptions{}, fixed)
	md := filepath.Join(root, Dir, "HOPBACK.md")
	os.WriteFile(md, []byte("<!-- hopback-template v0 -->"), 0o644); must(t, EnsureTemplates(root))
	if !strings.Contains(readFile(t, md), protocol.Marker()) { t.Fatal("not refreshed") }
	os.WriteFile(md, []byte("<!-- hopback-template v999 -->"), 0o644); must(t, EnsureTemplates(root))
	if readFile(t, md) != "<!-- hopback-template v999 -->" { t.Fatal("downgraded") }
}

// registry_test.go
func TestHome_EnvOverride(t *testing.T) {
	h, _ := Home(func(k string) string { if k == "HOPBACK_HOME" { return "/x" }; return "" })
	if h != "/x" { t.Fatal(h) }
}
func TestRegister_UpsertAndLookup(t *testing.T) {
	home := t.TempDir()
	must(t, Register(home, Entry{ID: "a", Name: "Docslet", Path: "/x"}))
	must(t, Register(home, Entry{ID: "a", Name: "Docslet", Path: "/y"}))
	es, _ := Load(home)
	if len(es) != 1 || es[0].Path != "/y" { t.Fatal(es) }
	if e, err := Lookup(home, "docslet"); err != nil || e.ID != "a" { t.Fatal(e, err) }
	if _, err := Lookup(home, "nope"); !errors.Is(err, ErrUnknownProject) { t.Fatal(err) }
}
func TestLoad_MissingFile(t *testing.T) { if es, err := Load(t.TempDir()); err != nil || len(es) != 0 { t.Fatal(es, err) } }
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/project/ ./internal/registry/`

Expected: build failure.

- [ ] **Step 3: Implement `project.go` and `registry.go`**

Implement them per the Interfaces block. Write every JSON file with `json.MarshalIndent(v, "", "  ")` plus a trailing newline, through `fsutil.WriteFileAtomic`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/project/ ./internal/registry/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/project internal/registry && git commit -m "Add project init/discovery and project registry"`

---

### Task 5: Store core and task operations

**Files:**
- Create: `internal/store/store.go`, `internal/store/ops_tasks.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `fsutil.*`, `model.*`, `project.Dir`.
- Produces:

```go
type Store struct { Root string; Project model.Project; Now func() time.Time }
func Open(root string) (*Store, error)                       // reads project.json; Now = time.Now
type Snapshot struct { State model.State; Backlog model.Backlog; Decisions model.Decisions }
func (s *Store) Load() (Snapshot, error)                     // no lock; bad JSON → *FileError
type Tx struct { Snapshot; Attr model.Attribution; Now time.Time /* unexported: events, dirty flags */ }
func (tx *Tx) Emit(typ string, data map[string]any) string  // returns event ID (model.NewSessionID(tx.Now))
func (tx *Tx) Task(id string) (*model.Task, error)           // ValidationError "unknown task T-9"
func (tx *Tx) MarkBacklog(); func (tx *Tx) MarkDecisions(); func (tx *Tx) MarkState()
type Op interface { Apply(tx *Tx) error }
type Result struct { Events []model.Event; Created []string }
func (s *Store) Apply(attr model.Attribution, ops ...Op) (Result, error)
func (s *Store) WithLock(fn func() error) error              // AcquireLock(.hopback/.lock, 10s)
func (s *Store) AppendEventsLocked(evs ...model.Event) error // caller holds lock
func (s *Store) NewEvent(typ string, attr model.Attribution, data map[string]any) model.Event
func (s *Store) Events() ([]model.Event, error)
func (s *Store) Path(name string) string                     // filepath.Join(Root, ".hopback", name)
type ValidationError struct{ Msg string }                    // Error() = Msg
type FileError struct{ File string; Err error }              // Error() = "invalid .hopback/<File>: <Err> (Hopback did not modify it)"
type TaskAdd struct { Title, Description string; Type model.TaskType; Priority model.Priority; Status model.TaskStatus; DependsOn []string; Origin model.Origin }
type TaskUpdate struct { ID string; Title, Description *string; Type *model.TaskType; Priority *model.Priority; Status *model.TaskStatus; DependsOn *[]string }
type TaskReorder struct { IDs []string }
```

- `Apply` acquires the lock, loads a snapshot, and runs each op in order against the in-memory `Tx`. If any op errors, it returns before writing anything. Otherwise it writes the dirty files (backlog, then decisions, then state) atomically and appends the events. Op structs carry snake_case JSON tags, which Task 6's batch decoding uses.
- `TaskAdd` behaviour:
  - Defaults: `Type=feature`, `Priority=medium`, `Status=planned`.
  - `Origin` is required.
  - `Rank` = max rank + 1; ID = `NextSeqID("T-")`.
  - Sets `SourceSession=attr.Session` and `RecordedBy=attr.Agent`.
  - Emits `task_created {task_id, title, origin}` and appends the ID to `Result.Created`.
- `TaskUpdate` behaviour:
  - Requires at least one field.
  - Status `completed` sets `CompletedAt=now`; leaving `completed` clears it.
  - Emits `task_updated {task_id, changes: [field names]}`.
- `TaskReorder` moves the listed IDs to the front in the given order. The rest keep their relative rank order. Ranks are renumbered 1..n. Rejects unknown or duplicate IDs. Emits `tasks_reordered {ids}`.
- Enum validation messages list the allowed values, e.g. `invalid type "chore" (allowed: idea, feature, ...)`.

- [ ] **Step 1: Write the failing tests**

Helper `newStore(t)`:
- `root := t.TempDir()`
- `project.Init(root, project.InitOptions{Name: "Docslet"}, fixed)`
- `st, _ := Open(root)`
- `st.Now = func() time.Time { return fixed }`
- `attr := model.Attribution{Session: "s1", Agent: "fake-agent"}`

```go
func TestTaskAdd_AssignsIDRankAndDefaults(t *testing.T) {
	st := newStore(t)
	res, err := st.Apply(attr, TaskAdd{Title: "Auth", Origin: "user_requested"})
	snap, _ := st.Load(); it := snap.Backlog.Items[0]
	if err != nil || it.ID != "T-1" || it.Rank != 1 || it.Type != "feature" || it.Priority != "medium" || it.Status != "planned" ||
		it.SourceSession != "s1" || it.RecordedBy != "fake-agent" || res.Created[0] != "T-1" { t.Fatal(it, res, err) }
	evs, _ := st.Events(); last := evs[len(evs)-1]
	if last.Type != "task_created" || last.Data["task_id"] != "T-1" || last.Session != "s1" { t.Fatal(last) }
}
func TestTaskAdd_Validation(t *testing.T) {
	st := newStore(t); var ve *ValidationError
	for _, op := range []TaskAdd{{Title: "", Origin: "user_requested"}, {Title: "x", Type: "chore", Origin: "user_requested"},
		{Title: "x"}, {Title: "x", Origin: "user_requested", DependsOn: []string{"T-9"}}} {
		if _, err := st.Apply(attr, op); !errors.As(err, &ve) { t.Fatalf("%+v: %v", op, err) }
	}
	_, err := st.Apply(attr, TaskAdd{Title: "x", Type: "chore", Origin: "user_requested"})
	if !strings.Contains(err.Error(), "feature") { t.Fatal("message must list allowed values:", err) }
}
func TestTaskUpdate_CompletedAt(t *testing.T) {
	st := newStore(t); st.Apply(attr, TaskAdd{Title: "A", Origin: "user_requested"})
	done := model.TaskStatus("completed"); st.Apply(attr, TaskUpdate{ID: "T-1", Status: &done})
	if s, _ := st.Load(); s.Backlog.Items[0].CompletedAt == nil || !s.Backlog.Items[0].CompletedAt.Equal(fixed) { t.Fatal("completed_at not set") }
	back := model.TaskStatus("in_progress"); st.Apply(attr, TaskUpdate{ID: "T-1", Status: &back})
	if s, _ := st.Load(); s.Backlog.Items[0].CompletedAt != nil { t.Fatal("completed_at not cleared") }
	if _, err := st.Apply(attr, TaskUpdate{ID: "T-9", Status: &back}); err == nil || !strings.Contains(err.Error(), "unknown task T-9") { t.Fatal(err) }
}
func TestTaskReorder(t *testing.T) {
	st := newStore(t)
	for _, n := range []string{"A", "B", "C", "D"} { st.Apply(attr, TaskAdd{Title: n, Origin: "user_requested"}) }
	must(t, st.Apply(attr, TaskReorder{IDs: []string{"T-3", "T-1"}}))
	if got := idsByRank(st); !reflect.DeepEqual(got, []string{"T-3", "T-1", "T-2", "T-4"}) { t.Fatal(got) }
}
func TestApply_AllOrNothing(t *testing.T) {
	st := newStore(t); before, _ := st.Events()
	_, err := st.Apply(attr, TaskAdd{Title: "A", Origin: "user_requested"}, TaskUpdate{ID: "T-99", Title: ptr("x")})
	snap, _ := st.Load(); after, _ := st.Events()
	if err == nil || len(snap.Backlog.Items) != 0 || len(after) != len(before) { t.Fatal("partial apply") }
}
func TestApply_ConcurrentWriters(t *testing.T) {
	st := newStore(t); var wg sync.WaitGroup
	for i := 0; i < 10; i++ { wg.Add(1); go func(i int) { defer wg.Done(); s2, _ := Open(st.Root); s2.Apply(attr, TaskAdd{Title: fmt.Sprint(i), Origin: "user_requested"}) }(i) }
	wg.Wait(); snap, _ := st.Load(); seen := map[string]bool{}
	for _, it := range snap.Backlog.Items { seen[it.ID] = true }
	if len(snap.Backlog.Items) != 10 || len(seen) != 10 { t.Fatalf("items=%d unique=%d", len(snap.Backlog.Items), len(seen)) }
}
func TestLoad_InvalidJSONReportsFile(t *testing.T) {
	st := newStore(t); bad := []byte("<<<<<<< HEAD\n{}\n=======\n")
	os.WriteFile(st.Path("backlog.json"), bad, 0o644)
	var fe *FileError
	if _, err := st.Load(); !errors.As(err, &fe) || fe.File != "backlog.json" || !strings.Contains(err.Error(), "Hopback did not modify it") { t.Fatal(err) }
	st.Apply(attr, TaskAdd{Title: "A", Origin: "user_requested"})
	if got, _ := os.ReadFile(st.Path("backlog.json")); !bytes.Equal(got, bad) { t.Fatal("corrupt file was overwritten") }
}
```

(`must` with a `(Result, error)` pair: write `if _, err := ...; err != nil { t.Fatal(err) }`. The compact form above is shorthand.)

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/store/`

Expected: build failure.

- [ ] **Step 3: Implement `store.go` and `ops_tasks.go`**

Implement them per the Interfaces block. `Events()` reads `events.jsonl` line by line, skipping blank lines; a malformed line gives a `*FileError` for `events.jsonl`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test -race ./internal/store/`

Expected: `ok`. Drop `-race` on Windows if cgo is unavailable.

- [ ] **Step 5: Commit**

`git add internal/store && git commit -m "Add locked transactional store with task operations"`

---

### Task 6: Decision, impact, state and next operations, and batch

**Files:**
- Create: `internal/store/ops_project.go`, `internal/store/batch.go`
- Test: `internal/store/ops_project_test.go`

**Interfaces:**
- Consumes: Task 5's `Tx`, `Op`, `ValidationError`.
- Produces:

```go
type DecisionAdd struct { Title, Context string; Options []model.Option; Decision, Rationale, Supersedes string }
type DecisionResolve struct { ID, Decision, Rationale string }
type ArchitectureChange struct { Summary string; Affects []string }
type ReviewClear struct { TaskID string }
type StateInput struct { ProjectStatus model.ProjectStatus; Mode model.Mode; BottomLine string; CurrentFeature *model.CurrentFeature; Blockers []string; RecommendedNext *model.Pick; Alternatives []model.Pick }
type StateSet struct { ExpectRevision int; State StateInput }
type NextSet struct { TaskID, Reason string; Alternatives []model.Pick }
type StaleError struct { Expected, Current int; UpdatedBy *model.Attribution; UpdatedAt *time.Time }
func DecodeBatch(r io.Reader) ([]Op, error)
```

- **`DecisionAdd`:**
  - With an empty `Decision`, status is `open`. Otherwise it's `decided`, with `DecidedAt=now`.
  - ID is `NextSeqID("D-")`.
  - `Supersedes` must exist; that decision becomes `superseded`.
  - Emits `decision_recorded {decision_id, status}`.
- **`DecisionResolve`:** the decision must exist and be `open`, and `Decision` is required. Emits `decision_resolved`.
- **`ArchitectureChange`:**
  - `Summary` is required, and every ID in `Affects` must exist.
  - Emits `architecture_changed {summary, affects}` first.
  - Sets `NeedsReview{Reason: Summary, EventID: <that event's ID>}` on each affected task, without changing any other task field.
- **`ReviewClear`:** the task must currently be flagged. Emits `review_cleared`.
- **`StateSet`:**
  - `ExpectRevision` must equal the current revision; otherwise return `*StaleError`. Its `Error()` is: `state is at revision {Current} (updated by {session}/{agent} at {time}); you sent {Expected}. Re-read with `hopback state get` and retry.`
  - Validation:
    - `Mode` must be valid.
    - `BottomLine` must be non-empty.
    - `CurrentFeature` is required (with an existing task) when the mode is `mid_feature`.
    - Every pick's task must exist.
    - An empty `ProjectStatus` keeps the current value.
  - Sets `Revision++`, `UpdatedAt` and `UpdatedBy=attr`. Emits `state_updated {revision}`.
- **`NextSet`:** the task must exist and `Reason` must be non-empty; alternative tasks must exist. Sets `RecommendedNext` and `Alternatives`, then `Revision++`, `UpdatedAt` and `UpdatedBy`. Emits `next_set {task_id}`.
- **Batch:**
  - JSON op names: `task_add`, `task_update`, `task_reorder`, `decision_add`, `decision_resolve`, `architecture_change`, `review_clear`, `state_set` (fields `expect_revision`, `state`), `next_set` (`task_id`, `reason`, `alternatives`).
  - Field names are the snake_case of the struct fields. `TaskReorder.IDs` is `ids`, `ReviewClear.TaskID` is `task_id`, and `TaskUpdate.ID` is `id`.
  - An unknown op gives the error `batch item {i}: unknown op "{name}"` (0-based index).

- [ ] **Step 1: Write the failing tests**

```go
func TestDecisionAdd_OpenDecidedSupersedes(t *testing.T) {
	st := newStore(t)
	st.Apply(attr, DecisionAdd{Title: "Store diffs?", Options: []model.Option{{"full", "simple"}, {"diffs", "compact"}}})
	st.Apply(attr, DecisionAdd{Title: "Use diffs", Decision: "diffs", Rationale: "disk", Supersedes: "D-1"})
	s, _ := st.Load(); d1, d2 := s.Decisions.Items[0], s.Decisions.Items[1]
	if d1.Status != "superseded" || len(d1.Options) != 2 || d2.ID != "D-2" || d2.Status != "decided" || d2.DecidedAt == nil { t.Fatal(d1, d2) }
}
func TestDecisionResolve(t *testing.T) {
	st := newStore(t); st.Apply(attr, DecisionAdd{Title: "Q"})
	must(t, st.Apply(attr, DecisionResolve{ID: "D-1", Decision: "yes"}))
	if _, err := st.Apply(attr, DecisionResolve{ID: "D-1", Decision: "again"}); err == nil { t.Fatal("resolved twice") }
}
func TestArchitectureChange_FlagsOnlyAffected(t *testing.T) {
	st := newStore(t); addTasks(st, "A", "B")
	res, _ := st.Apply(attr, ArchitectureChange{Summary: "Auth moved to sessions", Affects: []string{"T-2"}})
	s, _ := st.Load(); a, b := s.Backlog.Items[0], s.Backlog.Items[1]
	if a.NeedsReview != nil || b.NeedsReview == nil || b.NeedsReview.Reason != "Auth moved to sessions" ||
		b.NeedsReview.EventID != res.Events[0].ID || b.Status != "planned" { t.Fatal(a, b) }
}
func TestReviewClear(t *testing.T) {
	st := newStore(t); addTasks(st, "A"); st.Apply(attr, ArchitectureChange{Summary: "x", Affects: []string{"T-1"}})
	must(t, st.Apply(attr, ReviewClear{TaskID: "T-1"}))
	if _, err := st.Apply(attr, ReviewClear{TaskID: "T-1"}); err == nil { t.Fatal("cleared unflagged task") }
}
func TestStateSet_StaleRevision(t *testing.T) {
	st := newStore(t); in := StateInput{Mode: "between_features", BottomLine: "Auth done."}
	must(t, st.Apply(attr, StateSet{ExpectRevision: 0, State: in}))
	s, _ := st.Load()
	if s.State.Revision != 1 || s.State.UpdatedBy.Session != "s1" { t.Fatal(s.State) }
	_, err := st.Apply(attr, StateSet{ExpectRevision: 0, State: in}); var se *StaleError
	if !errors.As(err, &se) || se.Current != 1 || !strings.Contains(err.Error(), "revision 1") || !strings.Contains(err.Error(), "hopback state get") { t.Fatal(err) }
}
func TestStateSet_Validation(t *testing.T) {
	st := newStore(t)
	for _, in := range []StateInput{{BottomLine: "x"}, {Mode: "mid_feature", BottomLine: "x"}, {Mode: "between_features"},
		{Mode: "between_features", BottomLine: "x", RecommendedNext: &model.Pick{TaskID: "T-9", Reason: "r"}}} {
		if _, err := st.Apply(attr, StateSet{State: in}); err == nil { t.Fatalf("accepted %+v", in) }
	}
}
func TestNextSet(t *testing.T) {
	st := newStore(t); addTasks(st, "A", "B")
	must(t, st.Apply(attr, NextSet{TaskID: "T-1", Reason: "unblocks B", Alternatives: []model.Pick{{"T-2", "smaller"}}}))
	s, _ := st.Load(); evs, _ := st.Events()
	if s.State.RecommendedNext.TaskID != "T-1" || len(s.State.Alternatives) != 1 || s.State.Revision != 1 || evs[len(evs)-1].Type != "next_set" { t.Fatal(s.State) }
}
func TestDecodeBatch(t *testing.T) {
	ops, err := DecodeBatch(strings.NewReader(`[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"task_reorder","ids":["T-1"]}]`))
	if err != nil || len(ops) != 2 { t.Fatal(ops, err) }
	if _, ok := ops[0].(TaskAdd); !ok { t.Fatalf("%T", ops[0]) }
	if _, err := DecodeBatch(strings.NewReader(`[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"task_delete"}]`)); err == nil ||
		!strings.Contains(err.Error(), `batch item 1: unknown op "task_delete"`) { t.Fatal(err) }
}
func TestBatch_AllOrNothing(t *testing.T) {
	st := newStore(t)
	ops, _ := DecodeBatch(strings.NewReader(`[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"review_clear","task_id":"T-1"}]`))
	if _, err := st.Apply(attr, ops...); err == nil { t.Fatal("want error") }
	if s, _ := st.Load(); len(s.Backlog.Items) != 0 { t.Fatal("partial batch applied") }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/store/`

Expected: build failure on the new types.

- [ ] **Step 3: Implement `ops_project.go` and `batch.go`**

Implement them per the Interfaces block. `DecodeBatch` first unmarshals into `[]json.RawMessage`, reads each item's `op`, then unmarshals the item into the matching struct.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/store/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/store && git commit -m "Add decision, impact, state, next and batch operations"`

---

### Task 7: Session records, attribution, parking and reconciliation

**Files:**
- Create: `internal/session/session.go`, `internal/session/park.go`
- Test: `internal/session/session_test.go`

**Interfaces:**
- Consumes: `store.Store` (`WithLock`, `AppendEventsLocked`, `NewEvent`, `Load`, `Path`, `Now`), `fsutil.TryLock`, `model.*`.
- Produces:

```go
type Detector struct { Agent string; Detect func(env func(string) string) (nativeID string, ok bool) }
type Manager struct { Store *store.Store; Env func(string) string; Detectors []Detector }
func NewManager(st *store.Store, env func(string) string, detectors ...Detector) *Manager
func RunLockPath(root, sessionID string) string // <root>/.hopback/run/<id>.lock
func (m *Manager) List() ([]model.Session, error) // sorted by StartedAt ascending
func (m *Manager) Get(id string) (model.Session, error)
func (m *Manager) Create(s model.Session) error   // sets SchemaVersion/ProjectID; writes sessions/<id>.json; emits session_started
func (m *Manager) Save(s model.Session) error
func (m *Manager) Current() (model.Attribution, *model.Session, error)
func (m *Manager) Park(sessionID string, handoff []byte) error
type ParkError struct { Missing []string } // Error(): "parking incomplete:\n  - item\n  - item"
func (m *Manager) Reconcile() ([]model.Session, error) // returns sessions it marked interrupted
func (m *Manager) LatestBuild() (*model.Session, error)
func (m *Manager) LatestParkedBuild() (*model.Session, error)
func (m *Manager) LastConcierge() (*model.Session, error)
```

- **`Current()` resolution order:**
  1. `Env("HOPBACK_SESSION")`, if it names a session in this project.
  2. Each detector's native ID: the most recently started session with that `NativeSessionID`. If none exists, create one with `Role=build`, `Agent=detector.Agent`, `Launched=false`, `Status=active` and `StartedAt=now`.
  3. Otherwise return `Attribution{Agent: "unknown"}` and a nil session, without error and without writing anything.

  A matched session whose status is `interrupted` is set back to `active` and emits `session_revived`.
- **`Park` validation.** The session must exist, have role `build`, and status `active`, `interrupted` or `parked` (re-parking is allowed). Missing items are reported in this order with exactly this text:
  - `handoff section: <Name>` for each required section that's absent. The required sections are Objective, Done, Stopped at, Unfinished, Gotchas, Next step. Match headings by trimming `\r` and spaces and comparing lowercase heading text, at any `#` level.
  - `project state not updated in this session (run ` + "`hopback state set`" + `)`, unless `state.UpdatedBy.Session == sessionID` and `state.UpdatedAt >= session.StartedAt`.
  - A concierge session gives `*store.ValidationError{"concierge sessions don't park"}`.

  On success, under the lock, it:
  - writes `sessions/<id>.md` atomically;
  - sets `Status=parked`, `ParkedAt=now` and `Handoff="sessions/<id>.md"`;
  - emits `session_parked`.
- **`Reconcile`** marks `active` sessions as `interrupted`, sets `EndedAt=now` and emits `session_ended_without_handoff` for each one that is either:
  - `Launched` with no live holder of its run lock (`TryLock` succeeds; release it and remove the file); or
  - not `Launched`. The documented MVP rule: Hopback can't tell whether those are still running, and a later write revives them.

- [ ] **Step 1: Write the failing tests**

Helper `newManager(t, env map[string]string)`:
- builds a store via the Task 5 `newStore` pattern;
- `NewManager(st, mapEnv(env), Detector{Agent: "fake-agent", Detect: func(e func(string) string) (string, bool) { v := e("FAKE_AGENT_SESSION_ID"); return v, v != "" }})`.

`goodHandoff := "## Objective\nx\n## Done\nx\n## Stopped at\nx\n## Unfinished\nnone\n## Gotchas\nnone\n## Next step\nx\n"`.

```go
func TestCreateAndList(t *testing.T) {
	m := newManager(t, nil)
	m.Create(model.Session{ID: "b", Role: "build", StartedAt: fixed.Add(time.Hour)}); m.Create(model.Session{ID: "a", Role: "build", StartedAt: fixed})
	l, _ := m.List()
	if len(l) != 2 || l[0].ID != "a" { t.Fatal(l) }
}
func TestCurrent_HopbackSessionEnv(t *testing.T) {
	m := newManager(t, map[string]string{"HOPBACK_SESSION": "s1"}); m.Create(model.Session{ID: "s1", Role: "build", Agent: "fake-agent", Status: "active", StartedAt: fixed})
	if a, s, _ := m.Current(); a.Session != "s1" || a.Agent != "fake-agent" || s.ID != "s1" { t.Fatal(a) }
}
func TestCurrent_StaleHopbackSessionFallsBack(t *testing.T) {
	m := newManager(t, map[string]string{"HOPBACK_SESSION": "bogus", "FAKE_AGENT_SESSION_ID": "n1"})
	a, s, err := m.Current()
	if err != nil || s == nil || s.NativeSessionID != "n1" || s.Launched || s.Role != "build" || s.Status != "active" || a.Agent != "fake-agent" { t.Fatal(a, s, err) }
}
func TestCurrent_NativeMatchesLatest(t *testing.T) {
	m := newManager(t, map[string]string{"FAKE_AGENT_SESSION_ID": "n1"})
	m.Create(model.Session{ID: "old", NativeSessionID: "n1", Role: "build", Status: "parked", StartedAt: fixed})
	m.Create(model.Session{ID: "new", NativeSessionID: "n1", Role: "build", Status: "active", StartedAt: fixed.Add(time.Hour)})
	if a, _, _ := m.Current(); a.Session != "new" { t.Fatal(a) }
}
func TestCurrent_NoSignals(t *testing.T) {
	m := newManager(t, nil); a, s, err := m.Current(); l, _ := m.List()
	if err != nil || s != nil || a.Agent != "unknown" || len(l) != 0 { t.Fatal(a, s, err, l) }
}
func TestCurrent_RevivesInterrupted(t *testing.T) {
	m := newManager(t, map[string]string{"HOPBACK_SESSION": "s1"}); m.Create(model.Session{ID: "s1", Role: "build", Status: "interrupted", StartedAt: fixed})
	_, s, _ := m.Current()
	if s.Status != "active" || lastEventType(m) != "session_revived" { t.Fatal(s.Status) }
}
func TestPark_Success(t *testing.T) {
	m := newManager(t, nil); m.Create(model.Session{ID: "s1", Role: "build", Status: "active", StartedAt: fixed})
	m.Store.Apply(model.Attribution{Session: "s1", Agent: "fake-agent"}, store.StateSet{State: store.StateInput{Mode: "between_features", BottomLine: "Done."}})
	must(t, m.Park("s1", []byte(goodHandoff)))
	s, _ := m.Get("s1"); md, _ := os.ReadFile(m.Store.Path("sessions/s1.md"))
	if s.Status != "parked" || s.ParkedAt == nil || s.Handoff != "sessions/s1.md" || string(md) != goodHandoff || lastEventType(m) != "session_parked" { t.Fatal(s) }
}
func TestPark_ReportsEverythingMissing(t *testing.T) {
	m := newManager(t, nil); m.Create(model.Session{ID: "s1", Role: "build", Status: "active", StartedAt: fixed})
	err := m.Park("s1", []byte(strings.Replace(goodHandoff, "## Gotchas\nnone\n", "", 1))); var pe *ParkError
	if !errors.As(err, &pe) || !reflect.DeepEqual(pe.Missing, []string{"handoff section: Gotchas", "project state not updated in this session (run `hopback state set`)"}) { t.Fatal(err) }
	if s, _ := m.Get("s1"); s.Status != "active" { t.Fatal("status changed on failed park") }
	if _, err := os.Stat(m.Store.Path("sessions/s1.md")); !os.IsNotExist(err) { t.Fatal("handoff written on failed park") }
}
func TestPark_AcceptsCRLFAndCase(t *testing.T) {
	m := newManager(t, nil); m.Create(model.Session{ID: "s1", Role: "build", Status: "active", StartedAt: fixed})
	m.Store.Apply(model.Attribution{Session: "s1"}, store.StateSet{State: store.StateInput{Mode: "between_features", BottomLine: "x"}})
	h := strings.ReplaceAll(strings.Replace(goodHandoff, "## Stopped at", "### stopped AT  ", 1), "\n", "\r\n")
	must(t, m.Park("s1", []byte(h)))
}
func TestPark_RejectsConcierge(t *testing.T) {
	m := newManager(t, nil); m.Create(model.Session{ID: "c1", Role: "concierge", Status: "active", StartedAt: fixed})
	if err := m.Park("c1", []byte(goodHandoff)); err == nil || !strings.Contains(err.Error(), "concierge sessions don't park") { t.Fatal(err) }
}
func TestReconcile(t *testing.T) {
	m := newManager(t, nil); root := m.Store.Root
	m.Create(model.Session{ID: "dead", Role: "build", Status: "active", Launched: true, StartedAt: fixed})
	m.Create(model.Session{ID: "live", Role: "build", Status: "active", Launched: true, StartedAt: fixed})
	m.Create(model.Session{ID: "unl", Role: "build", Status: "active", Launched: false, StartedAt: fixed})
	l, _, _ := fsutil.TryLock(RunLockPath(root, "live")); defer l.Release()
	got, _ := m.Reconcile()
	dead, _ := m.Get("dead"); live, _ := m.Get("live"); unl, _ := m.Get("unl")
	if dead.Status != "interrupted" || dead.EndedAt == nil || live.Status != "active" || unl.Status != "interrupted" || len(got) != 2 { t.Fatal(dead, live, unl) }
}
func TestLatestBuild_IgnoresConcierge(t *testing.T) {
	m := newManager(t, nil)
	m.Create(model.Session{ID: "b", Role: "build", StartedAt: fixed}); m.Create(model.Session{ID: "c", Role: "concierge", StartedAt: fixed.Add(time.Hour)})
	if s, _ := m.LatestBuild(); s.ID != "b" { t.Fatal(s.ID) }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/session/`

Expected: build failure.

- [ ] **Step 3: Implement `session.go` and `park.go`**

Implement them per the Interfaces block. Session files are written with `fsutil.WriteFileAtomic` under `Store.WithLock`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/session/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/session && git commit -m "Add session records, attribution, parking and reconciliation"`

---

### Task 8: The brief

**Files:**
- Create: `internal/brief/brief.go`, `internal/brief/changes.go`
- Test: `internal/brief/brief_test.go`

**Interfaces:**
- Consumes: `model.*`, `store.Snapshot`.
- Produces:

```go
type Input struct { Project model.Project; Snapshot store.Snapshot; Sessions []model.Session; Now time.Time; AgentNames map[string]string }
type Brief struct {
	ProjectName, LastWorked, LastAgent, Warning string; Empty bool; Mode model.Mode; BottomLine string
	NeedsDecision []string; RecommendedNext string; Why, StoppedAt, NextStep string; RecentlyDone []string; StillLeft string }
type ProjectSummary struct { Status model.ProjectStatus; LastWorked, Headline string; Open, NeedsReview int }
func Build(in Input) Brief
func Render(w io.Writer, b Brief)
func Summary(in Input) ProjectSummary
func Ago(now, t time.Time) string
func ChangesSince(events []model.Event, since time.Time, limit int) []string
```

- `LastWorked` and `LastAgent` come from the build session with the greatest `LastActivity()`. Concierge sessions are ignored. `LastAgent` is looked up in `AgentNames`, falling back to the raw agent name.
- `Warning`: if that latest build session is `interrupted`, it reads `! Last session ended without a handoff ({LastAgent}, {Ago})`.
- `Empty` is true when `State.Revision == 0`.
- `NeedsDecision`, in this order:
  - open decisions, each as `fmt.Sprintf("%-6s%s", id, title)`;
  - tasks with `NeedsReview`, each as `fmt.Sprintf("%-6s%s (review: %s)", id, title, reason)`;
  - each blocker, as `"Blocked: " + text`.
- `RecommendedNext` is `fmt.Sprintf("%-6s%s: %s", id, title, reason)`.
- The mid-feature fields come from `CurrentFeature`.
- `RecentlyDone` is the titles of the 3 completed tasks with the newest `CompletedAt`, newest first.
- `StillLeft` is `"{open} open ({high} high): {top3 titles by rank, joined with ", "}"`, with `" ..."` appended when open > 3. It's empty when open == 0.
- `Ago` rules, where d = whole days:
  - d < 1: `today`
  - d == 1: `yesterday`
  - d < 14: `{d} days ago`
  - d < 60: `{d/7} weeks ago`
  - d < 365: `{d/30} months ago`
  - otherwise `{d/365} year(s) ago`, singular when 1
- `ChangesSince` covers events with `TS > since`, oldest first:
  - Each line is `{TS:2006-01-02}  {type}{detail} ({agent})`.
  - `detail` is `" " + data["task_id"]`, or else `" " + data["decision_id"]`, if present. Then, if present, `" \"" + data["title"]` (or `data["summary"]`) + `"\""`.
  - Beyond `limit` lines, it appends `... and {n} more`.
- `Summary`:
  - `Headline` is `BottomLine` truncated to 60 runes with `...`. If that's empty, it's `"next: " + RecommendedNext`. If that's empty too, it's `""`.
  - `Open` is the count of open tasks; `NeedsReview` is the count of flagged tasks.

- [ ] **Step 1: Write the failing tests**

The fixture `midFeatureInput()`:
- Project name "Docslet".
- A build session `parked`, with `StartedAt = ParkedAt = fixed - 12 days` and `EndedAt` nil, agent `claude-code`, and `AgentNames{"claude-code": "Claude Code"}`.
- A concierge session newer than that.
- Tasks:
  - `T-9` "Sharing API", flagged "Auth moved to sessions";
  - `T-12` "Build the diff view", in_progress.
- Open decision `D-4` "Store full revisions or only diffs?".
- State: revision 3; `mid_feature`; bottom line "Version history is half built. Storage works; diff view not started."; `CurrentFeature{T-12, "Users need to restore old drafts after bad edits.", "Revision table + save hook done.", "Diff endpoint."}`; `RecommendedNext{T-12, "compare before restoring"}`.

```go
func TestRender_MidFeatureGolden(t *testing.T) {
	var buf bytes.Buffer; Render(&buf, Build(midFeatureInput()))
	want := "Docslet · status brief · last worked 12 days ago · Claude Code\n\n" +
		"BOTTOM LINE\n  Version history is half built. Storage works; diff view not started.\n\n" +
		"NEEDS YOUR DECISION\n  D-4   Store full revisions or only diffs?\n  T-9   Sharing API (review: Auth moved to sessions)\n\n" +
		"RECOMMENDED NEXT\n  T-12  Build the diff view: compare before restoring\n\n" +
		"BACKGROUND\n  Why:         Users need to restore old drafts after bad edits.\n  Stopped at:  Revision table + save hook done.\n  Next step:   Diff endpoint.\n"
	if buf.String() != want { t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want) }
}
func TestBuild_BetweenFeatures(t *testing.T) {
	in := betweenFeaturesInput() // 4 completed tasks at fixed-4h..fixed-1h titled C1..C4 (C4 newest); 7 open tasks O1..O7 by rank, 3 high
	b := Build(in)
	if !reflect.DeepEqual(b.RecentlyDone, []string{"C4", "C3", "C2"}) || b.StillLeft != "7 open (3 high): O1, O2, O3 ..." { t.Fatal(b) }
	var buf bytes.Buffer; Render(&buf, b)
	if !strings.Contains(buf.String(), "  Recently done:  C4 · C3 · C2\n  Still left:     7 open (3 high): O1, O2, O3 ...\n") { t.Fatal(buf.String()) }
}
func TestBuild_InterruptedWarning(t *testing.T) {
	in := midFeatureInput(); in.Sessions[0].Status = "interrupted"; in.Sessions[0].StartedAt = fixed.AddDate(0, 0, -3); in.Sessions[0].ParkedAt = nil
	var buf bytes.Buffer; Render(&buf, Build(in))
	if !strings.HasPrefix(buf.String(), "! Last session ended without a handoff (Claude Code, 3 days ago)\n") { t.Fatal(buf.String()) }
}
func TestBuild_Empty(t *testing.T) {
	in := Input{Project: model.Project{Name: "New"}, Now: fixed}
	var buf bytes.Buffer; Render(&buf, Build(in))
	if !strings.Contains(buf.String(), "No brief yet") || !strings.Contains(buf.String(), "concierge") { t.Fatal(buf.String()) }
}
func TestAgo(t *testing.T) {
	for days, want := range map[int]string{0: "today", 1: "yesterday", 5: "5 days ago", 13: "13 days ago", 14: "2 weeks ago", 60: "2 months ago", 400: "1 year ago", 800: "2 years ago"} {
		if got := Ago(fixed, fixed.AddDate(0, 0, -days)); got != want { t.Fatalf("%d: %s", days, got) }
	}
}
func TestChangesSince(t *testing.T) {
	evs := []model.Event{{TS: fixed.Add(-time.Hour), Type: "task_created", Agent: "a", Data: map[string]any{"task_id": "T-1", "title": "Old"}},
		{TS: fixed.Add(time.Hour), Type: "task_created", Agent: "claude-code", Data: map[string]any{"task_id": "T-7", "title": "Auth"}},
		{TS: fixed.Add(2 * time.Hour), Type: "decision_recorded", Agent: "a", Data: map[string]any{"decision_id": "D-1"}},
		{TS: fixed.Add(3 * time.Hour), Type: "next_set", Agent: "a", Data: map[string]any{"task_id": "T-7"}}}
	got := ChangesSince(evs, fixed, 2)
	want := []string{`2026-11-05  task_created T-7 "Auth" (claude-code)`, `2026-11-05  decision_recorded D-1 (a)`, "... and 1 more"}
	if !reflect.DeepEqual(got, want) { t.Fatal(got) }
}
func TestSummary(t *testing.T) {
	s := Summary(midFeatureInput())
	if s.LastWorked != "12 days ago" || s.Open != 2 || s.NeedsReview != 1 || !strings.HasPrefix(s.Headline, "Version history is half built.") || !strings.HasSuffix(s.Headline, "...") { t.Fatal(s) }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/brief/`

Expected: build failure.

- [ ] **Step 3: Implement `brief.go` and `changes.go`**

`Render` layout:
- The header line is `{ProjectName} · status brief`, followed by ` · last worked {LastWorked} · {LastAgent}` only when a build session exists. `Warning`, if set, goes on the line above the header. A blank line follows the header.
- Each section is an uppercase title followed by two-space-indented lines. Sections are separated by a blank line, and empty sections are omitted.
- Between features, BACKGROUND holds `Recently done:  ` (titles joined with `" · "`) and `Still left:     ` lines.
- The empty case renders `No brief yet. Talk to the concierge to plan, or start building.` after the header.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/brief/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/brief && git commit -m "Add status brief rendering and change summaries"`

---

### Task 9: Agent adapter interface, Claude Code adapter, and test fake

**Files:**
- Create: `internal/agent/agent.go`, `internal/agent/claude/claude.go`, `internal/agent/agenttest/fake.go`
- Test: `internal/agent/claude/claude_test.go`

**Interfaces:**
- Consumes: `protocol.ParkSkill`, `protocol.ParseMarkerVersion`, `protocol.TemplateVersion`, `fsutil.WriteFileAtomic`, `store`, `session`, `model` (agenttest only).
- Produces (`agent`):

```go
var ErrNotOwned = errors.New("file exists and is not managed by Hopback; left untouched")
type LaunchSpec struct { Role model.Role; ProjectDir, SessionName, NativeID, ResumeNativeID, RoleInstructions, OpeningPrompt string; Env []string }
type Adapter interface {
	Name() string; DisplayName() string; Available() error
	Command(spec LaunchSpec) *exec.Cmd
	DetectSession(env func(string) string) (string, bool)
	Install(projectRoot string) error }
```

- Produces (`claude`): `type Claude struct { LookPath func(string) (string, error) }` and `func New() *Claude` (`LookPath = exec.LookPath`).
  - `Name()` returns `"claude-code"`; `DisplayName()` returns `"Claude Code"`.
  - `Available()` wraps a lookup failure as: ``Claude Code (`claude`) was not found on your PATH. Install it or add it to PATH, then try again.``
  - `Command`:
    - Args are either `--resume <ResumeNativeID>` or `--session-id <NativeID>`.
    - Then `--name <SessionName>` and `--append-system-prompt <RoleInstructions>`, each only when non-empty.
    - Then `OpeningPrompt` as the last positional argument, when non-empty.
    - `Dir=ProjectDir`, `Env=append(os.Environ(), spec.Env...)`.
    - No other flags. The user's model, permission mode and effort are left alone, and no tools are blocked.
  - `DetectSession` reads `CLAUDE_CODE_SESSION_ID`.
  - `Install` writes `<root>/.claude/skills/park/SKILL.md` from `protocol.ParkSkill()`:
    - when the file is missing, or has a Hopback marker with a lower version: write it;
    - when it exists without a marker: return `ErrNotOwned` (wrapped with the path);
    - when it's at the same or a newer version: do nothing.
- Produces (`agenttest`, test support):
  - `type Fake struct { Scenarios []string; Specs []agent.LaunchSpec; Unavailable error; Installs []string }`.
  - `Name()` returns `"fake-agent"`; `DisplayName()` returns `"Fake Agent"`.
  - `DetectSession` reads `FAKE_AGENT_SESSION_ID`.
  - `Command` records the spec, pops the next scenario (default `"exit"`), and returns `exec.Command(os.Args[0], "-test.run=^$")` with:
    - `Dir=spec.ProjectDir`;
    - `Env=append(os.Environ(), spec.Env..., "HOPBACK_AGENTTEST_HELPER=1", "HOPBACK_AGENTTEST_SCENARIO="+s, "FAKE_AGENT_SESSION_ID="+nativeOrResume)`.
  - `func IsHelper() bool` and `func RunHelper()`. Each test package that launches sessions calls them from `TestMain`: `if agenttest.IsHelper() { agenttest.RunHelper(); return }`.
  - `RunHelper` opens the store at the working directory, builds a `session.Manager` with `os.Getenv` and the fake detector, takes attribution from `Current()`, runs the scenario, then calls `os.Exit(0)`:
    - `exit`: does nothing;
    - `park`: `StateSet{ExpectRevision: current, State: {Mode: between_features, BottomLine: "Parked by fake agent."}}`, then `Park` with a valid six-section handoff;
    - `next`: `NextSet{TaskID: "T-1", Reason: "fake pick"}`;
    - `checklock`: writes `held` or `free` to `<root>/lockcheck`, depending on whether `fsutil.TryLock(session.RunLockPath(root, os.Getenv("HOPBACK_SESSION")))` fails or succeeds.

- [ ] **Step 1: Write the failing tests**

```go
func newClaude() *Claude { return &Claude{LookPath: func(string) (string, error) { return "claude", nil }} }
func TestCommand_Fresh(t *testing.T) {
	cmd := newClaude().Command(agent.LaunchSpec{NativeID: "u1", SessionName: "hopback build: T-12 Diff view", RoleInstructions: "R", OpeningPrompt: "P", ProjectDir: "/p", Env: []string{"HOPBACK_SESSION=s1"}})
	if !reflect.DeepEqual(cmd.Args[1:], []string{"--session-id", "u1", "--name", "hopback build: T-12 Diff view", "--append-system-prompt", "R", "P"}) { t.Fatal(cmd.Args) }
	if cmd.Dir != "/p" || !slices.Contains(cmd.Env, "HOPBACK_SESSION=s1") { t.Fatal(cmd.Dir) }
}
func TestCommand_ResumeNoPrompt(t *testing.T) {
	cmd := newClaude().Command(agent.LaunchSpec{ResumeNativeID: "u0", SessionName: "n"})
	if !reflect.DeepEqual(cmd.Args[1:], []string{"--resume", "u0", "--name", "n"}) { t.Fatal(cmd.Args) }
}
func TestDetectSession(t *testing.T) {
	c := newClaude()
	if id, ok := c.DetectSession(func(k string) string { if k == "CLAUDE_CODE_SESSION_ID" { return "abc" }; return "" }); id != "abc" || !ok { t.Fatal(id) }
	if _, ok := c.DetectSession(func(string) string { return "" }); ok { t.Fatal("detected from empty env") }
}
func TestInstall(t *testing.T) {
	root := t.TempDir(); p := filepath.Join(root, ".claude", "skills", "park", "SKILL.md"); c := newClaude()
	must(t, c.Install(root))
	if got, _ := os.ReadFile(p); !bytes.Equal(got, protocol.ParkSkill()) { t.Fatal("not written") }
	os.WriteFile(p, []byte("---\nname: park\n---\n<!-- hopback-template v0 -->"), 0o644); must(t, c.Install(root))
	if got, _ := os.ReadFile(p); !bytes.Equal(got, protocol.ParkSkill()) { t.Fatal("older owned file not refreshed") }
	os.WriteFile(p, []byte("my own park skill"), 0o644)
	if err := c.Install(root); !errors.Is(err, agent.ErrNotOwned) { t.Fatal(err) }
	if got, _ := os.ReadFile(p); string(got) != "my own park skill" { t.Fatal("foreign file overwritten") }
}
func TestAvailable_Missing(t *testing.T) {
	c := &Claude{LookPath: func(string) (string, error) { return "", exec.ErrNotFound }}
	if err := c.Available(); err == nil || !strings.Contains(err.Error(), "Claude Code (`claude`) was not found on your PATH") { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/agent/...`

Expected: build failure.

- [ ] **Step 3: Implement `agent.go`, `claude/claude.go` and `agenttest/fake.go`**

Implement them per the Interfaces block. `Command` uses the path from `LookPath("claude")`, falling back to `"claude"` when lookup fails (`Available` reports the problem separately).

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/agent/... && go vet ./...`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/agent && git commit -m "Add agent adapter interface, Claude Code adapter and test fake"`

---

### Task 10: Launcher (session lifecycle)

**Files:**
- Create: `internal/launcher/launcher.go`
- Test: `internal/launcher/launcher_test.go`

**Interfaces:**
- Consumes: `agent.Adapter`, `agent.LaunchSpec`, `session.Manager` (`Create`, `Get`, `Save`, `LastConcierge`, `RunLockPath`), `store.Store` (`Events`, `Load`, `WithLock`, `AppendEventsLocked`, `NewEvent`), `protocol.*`, `brief.ChangesSince`, `fsutil.TryLock`, `model.*`.
- Produces:

```go
type Launcher struct { Store *store.Store; Sessions *session.Manager; Adapter agent.Adapter; Stdin io.Reader; Stdout, Stderr io.Writer }
type Outcome struct { Session model.Session; Parked bool; NextSet *model.Pick }
func (l *Launcher) Build(objective *model.Task) (Outcome, error)
func (l *Launcher) Reopen(prev model.Session) (Outcome, error)
func (l *Launcher) Concierge() (Outcome, error)
```

- **The lifecycle** shared by all three methods:
  1. `Adapter.Available()`. On error, return it without creating anything. For Concierge, compute the opening prompt here, before the new session record exists, so its own `session_started` event isn't listed as a change.
  2. Create a session record:
     - `Status=active`, `Launched=true`, `Agent=Adapter.Name()`, `StartedAt=now`, ID from `NewSessionID`.
     - Build: `NativeSessionID = NewUUID()`, `ObjectiveTaskID`, and `Name = "hopback build: {ID} {Title}"` (title truncated to 40 runes), or `"hopback build"` with no objective.
     - Reopen: `NativeSessionID = prev.NativeSessionID`, `ResumedFrom = prev.ID`, `Name = "hopback build (reopened)"`.
     - Concierge: `NativeSessionID = NewUUID()`, `Name = "hopback concierge: {Project.Name}"`.
  3. `TryLock(RunLockPath)`, which must succeed; hold it for the child's lifetime.
  4. Build the `LaunchSpec`:
     - `Env = ["HOPBACK_SESSION=" + id]`.
     - Build: `RoleInstructions = BuildInstructions()`, `OpeningPrompt = BuildPrompt(objective)`, `NativeID = native`.
     - Reopen: `ResumeNativeID = native`, `OpeningPrompt = ReopenPrompt()`, `RoleInstructions = BuildInstructions()`.
     - Concierge: `RoleInstructions = ConciergeInstructions()`. `OpeningPrompt = ConciergePrompt(ChangesSince(events, since, 30), first)`, where `since` is the last concierge's `EndedAt` (or `StartedAt`) and `first` means there was no prior concierge.
  5. Run the command with stdio attached (`l.Stdin/Stdout/Stderr`). The child's exit code is not an error.
  6. Release the lock and remove the lock file.
  7. Reload the session and finish it:
     - Build or Reopen, status `parked`: set `EndedAt`; `Outcome.Parked = true`.
     - Build or Reopen, any other status: set `interrupted` and `EndedAt`, and emit `session_ended_without_handoff`.
     - Concierge: set `completed` and `EndedAt`, and emit `session_completed`. If any `next_set` event has `Session == id`, set `Outcome.NextSet = state.RecommendedNext`.

- [ ] **Step 1: Write the failing tests**

Setup:
- `TestMain` calls `agenttest.IsHelper/RunHelper` first.
- Helper `newLauncher(t, scenarios...)` inits the project "Docslet", adds task `T-1` "Auth" via `store.Apply`, builds a `session.Manager` with the fake detector, and returns `(*Launcher, *agenttest.Fake)` with `Stdout`/`Stderr` set to `io.Discard`.

```go
func TestBuild_Parked(t *testing.T) {
	l, f := newLauncher(t, "park"); task := mustTask(t, l, "T-1")
	out, err := l.Build(&task)
	if err != nil || !out.Parked || out.Session.Status != "parked" || out.Session.EndedAt == nil { t.Fatal(out, err) }
	sp := f.Specs[0]
	if sp.Role != "build" || sp.NativeID == "" || sp.SessionName != "hopback build: T-1 Auth" || !strings.Contains(sp.OpeningPrompt, "T-1: Auth") ||
		!slices.Contains(sp.Env, "HOPBACK_SESSION="+out.Session.ID) { t.Fatal(sp) }
}
func TestBuild_ExitWithoutPark(t *testing.T) {
	l, _ := newLauncher(t, "exit"); out, _ := l.Build(nil)
	if out.Parked || out.Session.Status != "interrupted" || lastEventType(l.Store) != "session_ended_without_handoff" { t.Fatal(out) }
}
func TestConcierge_NextSetAndFirstTime(t *testing.T) {
	l, f := newLauncher(t, "next"); out, _ := l.Concierge()
	if out.Session.Role != "concierge" || out.Session.Status != "completed" || out.NextSet == nil || out.NextSet.TaskID != "T-1" { t.Fatal(out) }
	if f.Specs[0].SessionName != "hopback concierge: Docslet" || !strings.Contains(f.Specs[0].OpeningPrompt, "first concierge session") { t.Fatal(f.Specs[0]) }
}
func TestConcierge_ListsChangesSinceLast(t *testing.T) {
	l, f := newLauncher(t, "exit", "exit"); l.Concierge()
	l.Store.Now = func() time.Time { return time.Now().Add(time.Minute) }
	l.Store.Apply(model.Attribution{Agent: "x"}, store.TaskAdd{Title: "Later", Origin: "user_requested"})
	l.Concierge()
	if p := f.Specs[1].OpeningPrompt; !strings.Contains(p, `task_created T-2 "Later"`) || strings.Contains(p, "first concierge session") { t.Fatal(p) }
}
func TestReopen_UsesNativeID(t *testing.T) {
	l, f := newLauncher(t, "exit"); prev := model.Session{ID: "p", Role: "build", NativeSessionID: "n1", Status: "parked", StartedAt: time.Now()}
	l.Sessions.Create(prev); out, _ := l.Reopen(prev)
	if f.Specs[0].ResumeNativeID != "n1" || out.Session.NativeSessionID != "n1" || out.Session.ResumedFrom != "p" { t.Fatal(f.Specs[0], out) }
}
func TestLaunch_AgentUnavailable(t *testing.T) {
	l, f := newLauncher(t); f.Unavailable = errors.New("no agent")
	if _, err := l.Build(nil); err == nil { t.Fatal("want error") }
	if s, _ := l.Sessions.List(); len(s) != 0 { t.Fatal("session created despite unavailable agent") }
}
func TestRunLockHeldDuringSession(t *testing.T) {
	l, _ := newLauncher(t, "checklock"); l.Build(nil)
	if b, _ := os.ReadFile(filepath.Join(l.Store.Root, "lockcheck")); string(b) != "held" { t.Fatal(string(b)) }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/launcher/`

Expected: build failure.

- [ ] **Step 3: Implement `launcher.go`**

Implement it per the Interfaces block.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/launcher/`

Expected: `ok`.

- [ ] **Step 5: Commit**

`git add internal/launcher && git commit -m "Add launcher for build, reopen and concierge sessions"`

---

### Task 11: CLI wiring and agent-facing commands

**Files:**
- Create: `cmd/hopback/main.go`, `internal/cli/app.go`, `internal/cli/agentcmds.go`
- Test: `internal/cli/agentcmds_test.go`, `internal/cli/helpers_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces:

```go
type App struct { WorkDir, Home string; Env func(string) string; In io.Reader; Out, Err io.Writer; Adapter agent.Adapter; Now func() time.Time }
func NewRootCmd(app *App) *cobra.Command
func Execute() int // real App: os.Getwd, registry.Home(os.Getenv), os.Getenv, os.Stdin/Stdout/Stderr, claude.New(), time.Now
type projectCtx struct { Root string; Store *store.Store; Sessions *session.Manager }
func (a *App) open(projectArg string) (*projectCtx, error)
// projectArg "" → project.Find(WorkDir); else registry.Lookup(Home, arg).Path.
// Then project.EnsureTemplates and Adapter.Install. ErrNotOwned is ignored here; Task 12's init reports it.
// Store.Now = App.Now. Manager detectors = [{Adapter.Name(), Adapter.DetectSession}].
```

- `main.go` is `func main() { os.Exit(cli.Execute()) }`.
- `Execute` prints `error: <msg>` to stderr and returns 1 on any command error. Silence cobra's own usage and error printing.
- Agent-facing commands use the exact syntax shown in the HOPBACK.md template (Task 3). Every write command gets attribution from `Sessions.Current()`.
- `--depends-on` and `--affects` take comma-separated values. `--option` and `--alt` are repeatable and split on the first `:` (name/ID before it, tradeoffs/reason after, both trimmed).
- Text-mode success output:
  - `task add`: the new ID plus a newline.
  - Other writes: `ok`.
  - `session park`: `Parked session {id}.`
  - `task list`: one line per task: `{ID:<6}{status:<12}{priority:<7}{title}`.
- `context` always prints JSON with the keys `project`, `state`, `open_tasks` (rank order), `needs_review`, `open_decisions`, `latest_handoff` (text of the latest parked build's handoff, or `""`) and `current_session`.
- `state get` prints the state JSON. `session current` prints the current session JSON, or `{"session":"","agent":"unknown"}`.
- `--file -` and `--handoff-file -` read `App.In`.

- [ ] **Step 1: Write the failing tests**

`helpers_test.go`:
- `TestMain` calls `agenttest.IsHelper/RunHelper` first.
- `newApp(t, env map[string]string) (*App, *agenttest.Fake, string)` inits the project "Docslet" in a temp dir, sets `Home` to another temp dir, registers the project, and wires `Fake`. It uses `Now: time.Now` (the fake agent runs in a separate process on the real clock, so a fixed clock would break the park-time check).
- It also defines its own copy of `goodHandoff` (the same six-section text as in Task 7).
- `run(app, stdin string, args...) (stdout, stderr string, code int)` executes `NewRootCmd` in-process and maps an error to code 1 with `error: ` written to `Err`.

```go
func TestTaskAddAndList(t *testing.T) {
	app, _, _ := newApp(t, nil)
	if out, _, code := run(app, "", "task", "add", "--title", "Auth", "--origin", "user"); code != 0 || out != "T-1\n" { t.Fatal(out) }
	out, _, _ := run(app, "", "task", "list", "--json"); var ts []model.Task; json.Unmarshal([]byte(out), &ts)
	if len(ts) != 1 || ts[0].Title != "Auth" || ts[0].Origin != "user_requested" { t.Fatal(out) }
}
func TestTaskAdd_InvalidType(t *testing.T) {
	app, _, _ := newApp(t, nil)
	out, errOut, code := run(app, "", "task", "add", "--title", "X", "--origin", "user", "--type", "chore")
	if code != 1 || out != "" || !strings.Contains(errOut, "chore") || !strings.Contains(errOut, "feature") { t.Fatal(code, errOut) }
}
func TestTaskUpdate_OnlyChangedFlags(t *testing.T) {
	app, _, root := newApp(t, nil); run(app, "", "task", "add", "--title", "Auth", "--origin", "user")
	run(app, "", "task", "update", "T-1", "--status", "completed")
	it := loadTask(t, root, "T-1")
	if it.Status != "completed" || it.Title != "Auth" || it.Priority != "medium" { t.Fatal(it) }
}
func TestDecisionAddOptions(t *testing.T) {
	app, _, root := newApp(t, nil)
	run(app, "", "decision", "add", "--title", "Store diffs?", "--option", "full: simple, more disk", "--option", "diffs: compact")
	d := loadDecision(t, root, "D-1")
	if d.Status != "open" || d.Options[0] != (model.Option{Name: "full", Tradeoffs: "simple, more disk"}) || len(d.Options) != 2 { t.Fatal(d) }
}
func TestArchitectureChangeCLI(t *testing.T) {
	app, _, root := newApp(t, nil); run(app, "", "task", "add", "--title", "API", "--origin", "user")
	run(app, "", "architecture", "change", "--summary", "Auth moved", "--affects", "T-1")
	if loadTask(t, root, "T-1").NeedsReview == nil { t.Fatal("not flagged") }
}
func TestStateSetStdinAndStale(t *testing.T) {
	app, _, _ := newApp(t, nil); in := `{"mode":"between_features","bottom_line":"Auth done."}`
	if _, e, code := run(app, in, "state", "set", "--expect-revision", "0", "--file", "-"); code != 0 { t.Fatal(e) }
	out, _, _ := run(app, "", "state", "get"); if !strings.Contains(out, `"revision": 1`) { t.Fatal(out) }
	if _, e, code := run(app, in, "state", "set", "--expect-revision", "0", "--file", "-"); code != 1 || !strings.Contains(e, "revision 1") { t.Fatal(e) }
}
func TestNextSetCLI(t *testing.T) {
	app, _, root := newApp(t, nil); run(app, "", "task", "add", "--title", "A", "--origin", "user"); run(app, "", "task", "add", "--title", "B", "--origin", "user")
	run(app, "", "next", "set", "T-1", "--reason", "R", "--alt", "T-2: maybe")
	st := loadState(t, root)
	if st.RecommendedNext.TaskID != "T-1" || st.Alternatives[0] != (model.Pick{TaskID: "T-2", Reason: "maybe"}) { t.Fatal(st) }
}
func TestBatchCLI(t *testing.T) {
	app, _, root := newApp(t, nil)
	run(app, `[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"task_add","title":"B","origin":"agent_suggested"}]`, "batch", "--file", "-")
	if loadTask(t, root, "T-2").Origin != "agent_suggested" { t.Fatal("batch not applied") }
}
func TestSessionParkCLI(t *testing.T) {
	app, _, _ := newApp(t, map[string]string{"FAKE_AGENT_SESSION_ID": "n1"})
	cur, _, _ := run(app, "", "session", "current"); if !strings.Contains(cur, `"native_session_id": "n1"`) { t.Fatal(cur) }
	if _, e, code := run(app, "## Objective\nx\n", "session", "park", "--handoff-file", "-"); code != 1 || !strings.Contains(e, "handoff section: Done") { t.Fatal(e) }
	run(app, `{"mode":"between_features","bottom_line":"x"}`, "state", "set", "--expect-revision", "0", "--file", "-")
	if out, e, code := run(app, goodHandoff, "session", "park", "--handoff-file", "-"); code != 0 || !strings.HasPrefix(out, "Parked session ") { t.Fatal(e) }
}
func TestContextJSON(t *testing.T) {
	app, _, _ := newApp(t, nil); out, _, _ := run(app, "", "context"); var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil { t.Fatal(err) }
	for _, k := range []string{"project", "state", "open_tasks", "needs_review", "open_decisions", "latest_handoff", "current_session"} { if _, ok := m[k]; !ok { t.Fatal(k) } }
}
func TestWritesAttributedToDetectedSession(t *testing.T) {
	app, _, root := newApp(t, map[string]string{"FAKE_AGENT_SESSION_ID": "n1"}); run(app, "", "task", "add", "--title", "A", "--origin", "user")
	it := loadTask(t, root, "T-1")
	if it.SourceSession == "" || it.RecordedBy != "fake-agent" { t.Fatal(it) }
}
func TestOutsideProject(t *testing.T) {
	app, _, _ := newApp(t, nil); app.WorkDir = t.TempDir()
	if _, e, code := run(app, "", "task", "list"); code != 1 || !strings.Contains(e, "hopback init") { t.Fatal(e) }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/cli/`

Expected: build failure.

- [ ] **Step 3: Implement `app.go`, `agentcmds.go` and `main.go`**

Implement them per the Interfaces block. Use `cmd.Flags().Changed(name)` to build the pointer fields for `TaskUpdate`.

- [ ] **Step 4: Run the tests and build the binary**

Run: `go test ./internal/cli/ && go build -o hopback.exe ./cmd/hopback && ./hopback.exe task --help`

Expected: tests `ok`; help lists `add`, `update`, `list` and `reorder`.

- [ ] **Step 5: Commit**

`git add cmd internal/cli && git commit -m "Add CLI wiring and agent-facing commands"`

---

### Task 12: Human-facing commands and the interactive loop

**Files:**
- Create: `internal/cli/humancmds.go`, `internal/cli/interactive.go`
- Test: `internal/cli/humancmds_test.go`

**Interfaces:**
- Consumes: Task 11's `App`, `open`, `run` test helper; `launcher.Launcher`; `brief.*`; `registry.*`; `project.Init`.
- Produces:
  - Commands: `init [--name N]`, `projects`, `status [project]`, `sessions [--concierge] [--json]`, `resume [project] [--fresh]`, `concierge [project]`.
  - The root command with no args runs `func (a *App) interactive() error`.
- Human-facing commands call `Sessions.Reconcile()` before reading.
- **`init`:**
  - Runs `project.Init(WorkDir, {Name, Installers: [Adapter.Install]}, now)`, then `registry.Register`.
  - Prints `Initialized Hopback in {root}` or `Hopback already initialized in {root}`, then `Registered as "{name}".`
  - An `ErrNotOwned` from the installer prints `warning: ` plus the error and still exits 0.
- **`projects`:** one row per registry entry: `{name:<20}{status:<10}{lastWorked:<14}{open} open  {needsReview} to review  {headline}`. If the path is gone, the row is `{name:<20}(missing: {path})`.
- **`status`:** `brief.Render` output.
- **`sessions`:** build sessions only (or concierge only with `--concierge`), newest first, as `{StartedAt:2006-01-02 15:04}  {status:<12}{agent:<12}{objective or name}`.
- **`resume`:**
  - Without `--fresh`, reopens `LatestBuild()` if it has a `NativeSessionID`. Otherwise it prints `The last build session can't be reopened; starting a fresh build session.` and starts fresh.
  - The fresh objective is the mid-feature `CurrentFeature.TaskID`, else `RecommendedNext.TaskID`, else nil.
  - It then runs the post-session loop.
- **`concierge`:** a concierge session, then the post-session loop.
- **`interactive`:**
  - Outside a project, it lists registry entries as `  {n}) {name}  {headline}`, prompts `Project number (q to quit): ` and opens the pick. An empty registry prints `No projects yet. Run \`hopback init\` in a repo.`
  - In a project, it renders the brief, then the menu, numbering only the available options in this order:
    - `Continue building: {ID} {Title}`, or `Start building` with no objective;
    - `Reopen last build conversation ({DisplayName}, {Ago})`, only if the latest build has a native ID;
    - `Talk to the concierge`;
    - `q) Quit`.
  - The default is "Continue building" when the mode is `mid_feature`, and "Talk to the concierge" otherwise (including when empty). The prompt is `Choose [{default}]: `, and empty input takes the default.
- **The post-session loop:**
  - A parked build asks `Parked. Talk to the concierge about what's next? [Y/n] ` and on yes runs the concierge.
  - A build that wasn't parked prints `Session ended without a handoff. Next time, type /park before closing so the next session knows where you stopped.` and stops.
  - A concierge with `NextSet` asks `Start building {ID} {Title}? [Y/n] ` and on yes runs a build. Otherwise it stops.
  - `[Y/n]` treats empty and `y`/`yes` (case-insensitive) as yes.

- [ ] **Step 1: Write the failing tests**

```go
func TestInitCLI(t *testing.T) {
	app, f, _ := newApp(t, nil); dir := t.TempDir(); app.WorkDir = dir
	out, _, code := run(app, "", "init", "--name", "Smoke")
	if code != 0 || !strings.Contains(out, "Initialized Hopback in") || len(f.Installs) != 1 { t.Fatal(out) }
	if _, err := registry.Lookup(app.Home, "smoke"); err != nil { t.Fatal(err) }
	if out, _, _ := run(app, "", "init"); !strings.Contains(out, "already initialized") { t.Fatal(out) }
}
func TestStatusCLI(t *testing.T) {
	app, _, _ := newApp(t, nil); run(app, `{"mode":"between_features","bottom_line":"Auth done."}`, "state", "set", "--expect-revision", "0", "--file", "-")
	if out, _, _ := run(app, "", "status"); !strings.Contains(out, "BOTTOM LINE\n  Auth done.") { t.Fatal(out) }
}
func TestProjectsCLI(t *testing.T) {
	app, _, _ := newApp(t, nil); registry.Register(app.Home, registry.Entry{ID: "gone", Name: "Gone", Path: filepath.Join(t.TempDir(), "nope")})
	out, _, _ := run(app, "", "projects")
	if !strings.Contains(out, "Docslet") || !strings.Contains(out, "(missing: ") { t.Fatal(out) }
}
func TestSessionsCLI(t *testing.T) {
	app, _, root := newApp(t, nil); m := managerFor(t, root)
	m.Create(model.Session{ID: "b", Role: "build", Status: "parked", Name: "hopback build: T-1 Auth", StartedAt: time.Now()})
	m.Create(model.Session{ID: "c", Role: "concierge", Status: "completed", Name: "hopback concierge: Docslet", StartedAt: time.Now()})
	b, _, _ := run(app, "", "sessions"); c, _, _ := run(app, "", "sessions", "--concierge")
	if !strings.Contains(b, "T-1 Auth") || strings.Contains(b, "concierge") || !strings.Contains(c, "concierge") { t.Fatal(b, c) }
}
func TestInteractive_EmptyDefaultsToConcierge(t *testing.T) {
	app, f, _ := newApp(t, nil); f.Scenarios = []string{"exit"}
	run(app, "\n")
	if len(f.Specs) != 1 || f.Specs[0].Role != "concierge" { t.Fatal(f.Specs) }
}
func TestInteractive_MidFeatureBuildThenDeclineConcierge(t *testing.T) {
	app, f, _ := newApp(t, nil); f.Scenarios = []string{"park"}; seedMidFeature(t, app) // task T-1 + state mid_feature on T-1
	out, _, _ := run(app, "\nn\n")
	if len(f.Specs) != 1 || f.Specs[0].Role != "build" || !strings.Contains(f.Specs[0].OpeningPrompt, "T-1") ||
		!strings.Contains(out, "Parked. Talk to the concierge about what's next? [Y/n]") { t.Fatal(out) }
}
func TestInteractive_ParkedThenConciergeThenBuild(t *testing.T) {
	app, f, _ := newApp(t, nil); f.Scenarios = []string{"park", "next", "exit"}; seedMidFeature(t, app)
	out, _, _ := run(app, "\n\n\n")
	if len(f.Specs) != 3 || f.Specs[1].Role != "concierge" || f.Specs[2].Role != "build" || !strings.Contains(out, "Session ended without a handoff") { t.Fatal(out) }
}
func TestInteractive_Quit(t *testing.T) {
	app, f, _ := newApp(t, nil); run(app, "q\n")
	if len(f.Specs) != 0 { t.Fatal("launched on quit") }
}
func TestInteractive_OutsideProjectPicker(t *testing.T) {
	app, f, _ := newApp(t, nil); app.WorkDir = t.TempDir()
	out, _, _ := run(app, "1\nq\n")
	if !strings.Contains(out, "1) Docslet") || !strings.Contains(out, "Docslet · status brief") || len(f.Specs) != 0 { t.Fatal(out) }
}
func TestResume_ReopenFreshAndFallback(t *testing.T) {
	app, f, root := newApp(t, nil); m := managerFor(t, root)
	m.Create(model.Session{ID: "b", Role: "build", Status: "parked", NativeSessionID: "n1", StartedAt: time.Now()})
	run(app, "n\n", "resume"); run(app, "n\n", "resume", "--fresh")
	if f.Specs[0].ResumeNativeID != "n1" || f.Specs[1].ResumeNativeID != "" || f.Specs[1].NativeID == "" { t.Fatal(f.Specs) }
	m.Create(model.Session{ID: "u", Role: "build", Status: "parked", StartedAt: time.Now().Add(time.Hour)})
	if out, _, _ := run(app, "n\n", "resume"); !strings.Contains(out, "can't be reopened; starting a fresh build session") { t.Fatal(out) }
}
func TestStatus_ReconcilesDeadSession(t *testing.T) {
	app, _, root := newApp(t, nil); managerFor(t, root).Create(model.Session{ID: "d", Role: "build", Status: "active", Launched: true, Agent: "fake-agent", StartedAt: time.Now()})
	run(app, `{"mode":"between_features","bottom_line":"x"}`, "state", "set", "--expect-revision", "0", "--file", "-")
	if out, _, _ := run(app, "", "status"); !strings.Contains(out, "ended without a handoff") { t.Fatal(out) }
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/cli/`

Expected: build failure or failures on the new commands.

- [ ] **Step 3: Implement `humancmds.go` and `interactive.go`**

Implement them per the Interfaces block. Read prompts with a single `bufio.Reader` over `App.In`, shared across the whole loop.

- [ ] **Step 4: Run the full suite**

Run: `go test ./... && go vet ./...`

Expected: all `ok`.

- [ ] **Step 5: Commit**

`git add internal/cli && git commit -m "Add human-facing commands and interactive session loop"`

---

### Task 13: Real Claude Code smoke test and docs

**Files:**
- Modify: `CLAUDE.md` (add a package map under "Architecture")

**Interfaces:**
- Consumes: the built `hopback` binary and a real `claude` (2.1.286 verified).

- [ ] **Step 1: Install and confirm it's on PATH**

Run: `go install ./cmd/hopback && hopback --help`

Expected: the help lists `init`, `projects`, `status`, `sessions`, `resume`, `concierge`, `task`, `session`. If `hopback` isn't found, add `$(go env GOPATH)/bin` to PATH and say so in the final report.

- [ ] **Step 2: Create a scratch project** (in the session scratchpad, never in this repo)

Run: `mkdir smoke && cd smoke && git init -q && hopback init --name smoke && ls .claude/skills/park .hopback`

Expected: `SKILL.md`, plus the `.hopback` files from Task 4.

- [ ] **Step 3: Verify session detection inside real Claude Code**

Run, with `U` set to a fresh UUID:

`claude -p --session-id $U --name "hopback smoke" "Run the shell command: hopback session current. Print its output verbatim and nothing else."`

Expected: JSON with `"native_session_id": "<U>"` and `"launched": false`.

- [ ] **Step 4: Verify that `/park` resolves to the Hopback skill**

Run, with `U2` set to a fresh UUID:

`claude -p --session-id $U2 "/park"`

then `hopback sessions --json`.

Expected: a session with `native_session_id` equal to `U2` exists, which proves the skill made the agent call `hopback`. Its status is `parked` if the agent completed the procedure, or `active` with the agent reporting why. Record which. If `/park` wasn't recognised (no session with `U2`), check where Claude Code expects project skills and commands, fix `Install`'s path with a test, and commit.

- [ ] **Step 5: Add the package map to `CLAUDE.md`**

Add a short "Code map" under the Architecture section, one line per package from this plan's File Structure. Also add the dependency rule: core packages never import `internal/agent/claude`.

- [ ] **Step 6: Commit and push**

`git add CLAUDE.md && git commit -m "Document code map; verify Claude Code integration" && git push`

- [ ] **Step 7: Report the manual checks for the owner**

These are interactive steps that can't be run headless:
1. `hopback` in the smoke project → the concierge is the default → chat, agree on a task → exit → accept "Start building".
2. In the build session, make a change, then `/park` → exit → see the "Parked…" offer.
3. `hopback` again → the brief shows the parked state.
4. Close a build session without `/park` → the next `hopback` shows the "ended without a handoff" warning.
