# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- Build: `go build ./...` (binary: `go build -o hopback.exe ./cmd/hopback`; install: `go install ./cmd/hopback`)
- Test all: `go test ./...`
- Single test: `go test ./internal/store -run TestTaskAdd_AssignsIDRankAndDefaults -v`
- Vet: `go vet ./...`

The MVP design (`docs/superpowers/specs/2026-11-05-hopback-mvp-design.md`) refines the PRD and wins where they differ (e.g. no SQLite). The implementation plan is `docs/superpowers/plans/2026-11-05-hopback-mvp.md`. Section numbers below (§) refer to `docs/PRD.md`.

Hopback is a local-first CLI that helps an independent developer keep track of many personal projects across AI coding agents such as Claude Code, Codex and Cursor. Projects are the primary entity. Sessions, backlog, decisions and events belong to a project.

## Non-negotiable constraints

These come from the spec. They are not style preferences.

- **No LLM in Hopback core.** Hopback never writes summaries, handoffs, resume briefings, project descriptions or repository analysis. The agent does all of that reasoning; Hopback validates, persists and displays it (§6). The spec puts it as "The agent does the thinking. Hopback maintains continuity."
- **Never fake success.** If an agent's handoff or park output is missing or invalid, report parking as incomplete. Never fill in missing content, and never mark the session parked (§8.3, §9.3).
- **Agent-agnostic core.** Agent-specific behaviour (launch commands, native session IDs, resume flags, plugin/hook wiring) lives in thin adapters. The core project model must not change when a new agent is added. Claude Code is the first integration, but the core must make no Claude Code-specific assumptions (§5.1, §18).
- **One core, many interfaces.** CLI commands and any MCP tools must call the same core functions; neither interface gets its own logic (§7.1). Agent-facing commands take structured input and return machine-readable output, while human-facing commands stay interactive (§11).
- **Portable project files.** Everything in `.hopback/` must stay understandable and usable without Hopback installed: documented JSON schemas, Markdown for prose, schema versions, timestamps and agent attribution (§8.4).
- **Out of scope:** sprints, story points, deadlines, assignments, velocity, and any negative signal for inactivity. An inactive project is not overdue or blocked (§2.3, §2.4).
- **No dashboard first.** The order of work is protocol, then session lifecycle, then CLI (§18).

## Architecture (as specified)

**Two storage tiers.** How they stay consistent is still an open design question (§12.2, §16).
- Project-local `.hopback/` holds portable, version-controllable knowledge: `HOPBACK.md` (the entry point every agent reads first), `project.json`, `state.json`, `backlog.json`, `decisions.json`, append-only `events.jsonl`, and `sessions/*.md` handoffs (§8).
- A global SQLite database holds operational data: the project registry, session registry, agent metadata, native session references and the event index.

**Two-way communication.** Hopback launches and resumes agents. Agents also call Hopback mid-session, for example `task_create`, `task_update`, `decision_record`, `architecture_change`, `impact_report`, `milestone_update`, `session_checkpoint` and `session_park` (§7.1). The full operation set is in §7.1.

**Two kinds of resume, kept as separate actions (§9.2).**
- *Conversation resumption* continues the agent's native session, and only for agents that support it.
- *Project resumption* starts a fresh conversation seeded from the project state and the latest handoff.

**Session statuses** are Active, Parked, Completed, Interrupted and Failed. An interrupted session has no valid final handoff, but its last successful checkpoint must stay accessible (§9.4).

**Impact tracking.** An `architecture_change` links to the backlog items it may affect, and those items are *flagged for review*. Hopback never decides on its own that a task is obsolete (§7.3).

**Completion** is optional: P = Σ(milestone weight × completion fraction). It is never derived from task counts, the user can override it, and it is shown separately from backlog health (§10.4).

## Data integrity requirements (§13)

Multiple agents may write to the same project at the same time, so design for concurrency from the start:
- atomic writes
- stale-revision and conflict detection
- no silent overwrites
- append-only events
- schema validation of every agent-supplied update
- agent attribution on every write

Development happens on Windows, so the file-replace and locking strategy must work there as well as on Unix.

## Working on this repo

- §18 sets the order of work before implementing: identify ambiguities, investigate agent integration mechanisms and native-resume support, propose the data model, design the protocol and JSON schemas, settle concurrency and persistence, then plan the MVP. MVP phases are in §14; acceptance criteria are in §15.
- Keep confirmed requirements separate from proposals. Go, SQLite, MCP and Tauri/React are *proposed* (§12), not mandated.
- The open questions in §16 (TUI vs CLI, worktree to project mapping, SQLite/file sync, conflict resolution, MCP timing and others) should be investigated and brought to the user, not quietly decided in code.
