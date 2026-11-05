# Hopback MVP — Design

**Date:** 2026-11-05
**Status:** Draft for review
**Based on:** `docs/PRD.md` plus a design conversation with the project owner. Where this document and the PRD disagree, this document wins for the MVP; every deviation is listed in §13.

---

## 1. Intent

The owner works on many personal projects and returns to them after days or weeks. Today they `cd` into the repo, run `claude --resume`, pick the latest conversation and scroll it, "hoping it all comes back". The thing that needs restoring is **the developer's own memory** of what they were doing, not the agent's context.

Hopback's MVP must make that moment fast and reliable:

1. Run `hopback` in the repo and immediately read a short brief, written like a consultant's status email to a busy executive: bottom line first, then what needs a decision, then the recommended next step.
2. Pick up where you left off, either in a fresh agent session seeded from the handoff or by reopening the old conversation.
3. When a feature is finished, get a recommendation for what to build next from a **concierge**: a separate agent session whose only job is planning against the project's backlog.

The PRD's principle stands: **the agent does the thinking, Hopback maintains continuity.** Hopback itself contains no LLM. Every brief, handoff and recommendation is written by an agent and stored by Hopback. Hopback shows them back without any AI.

## 2. Goals, non-goals, anti-goals

**Goals (MVP)**
- `hopback` in a repo shows the brief instantly, with no agent launch and no LLM call.
- The brief has two shapes. **Mid-feature:** what the feature is, why we're building it, where we stopped. **Between features:** what was recently completed (a few items, not all of them) and what's left.
- Two session roles, **build** and **concierge**, both driven by Hopback through Claude Code.
- Deliberate parking (`/park`) that produces a handoff. Hopback validates it and never reports a park as successful when it wasn't.
- Agents record backlog items, decisions, task progress and architectural impact during a session via the `hopback` CLI.
- `hopback projects` gives a one-screen overview of all registered projects.

**Non-goals (MVP):** see §12 (deferred) and the PRD's out-of-scope list: no sprints, story points, deadlines, assignments or velocity.

**Anti-goals.** These would count as failure even if everything technically works:
- The brief is long, generic or reads like a changelog instead of a briefing.
- Hopback claims a session was parked when the handoff is missing or broken.
- Concierge sessions pollute "latest session" / "last worked" / resume.
- Agent-invented features silently appear in the backlog.
- Claude Code specifics leak into the core model so that adding Codex later means changing the core.
- Inactivity is presented as a problem ("overdue", "stale", red warnings).

## 3. The loop

```
hopback  (in repo)
   │  shows the BRIEF (instant, from files)
   │  menu: continue building / reopen last build conversation / talk to concierge
   ▼
CONCIERGE session ── planning only, no code
   reads backlog, state, latest handoff, what changed since
   discusses with the user → records the chosen next task (`hopback next set`)
   user exits → Hopback offers: "Start building <task>? [Y/n]"
   ▼
BUILD session ── Claude Code, objective = chosen task
   writes code; records ideas/decisions/progress via `hopback` as it goes
   user types /park → agent updates state + writes handoff → `hopback session park`
   user exits → Hopback offers: "Parked. Talk to the concierge about what's next? [Y/n]"
   ▼
back to CONCIERGE …
```

The user can also skip the concierge entirely and go straight to building.

### 3.1 `hopback` inside a project

1. Print the brief (§5).
2. Print a short menu. The default choice depends on the brief's mode:
   - **Continue building:** start a *fresh* build session seeded from the latest handoff, with the current or recommended task as its objective. This is the default when the brief is mid-feature.
   - **Reopen last build conversation:** native Claude Code resume of the most recent *build* session. Only offered when its native session ID is known.
   - **Talk to the concierge:** start a fresh concierge session. This is the default when the brief is between features, or when there's no brief yet.
   - **Quit.**
3. Launch the chosen session as a child process attached to the terminal, wait for it to exit, then run the post-session step (§6.4, §7.3).

### 3.2 `hopback` outside a project

List registered projects (as `hopback projects` does) and let the user pick one by number. Picking a project behaves like §3.1 for that project. The agent is launched with that project as its working directory; Hopback cannot change the parent shell's directory.

## 4. Concepts

### 4.1 Project
A directory containing `.hopback/`. Hopback finds the current project by walking up from the working directory. A project has a stable ID (in `project.json`) that does not depend on its path.

### 4.2 Session
One agent conversation that Hopback knows about.

- **Role:** `build` or `concierge`.
  - "Last worked", "latest session", the brief, the resume option and the default `hopback sessions` view consider **build sessions only**.
  - Concierge sessions are recorded but listed separately (`hopback sessions --concierge`) and are **never offered for resume**. Every concierge starts fresh.
- **Agent:** `claude-code` is the only driven agent in the MVP. The field exists so other agents slot in later.
- **Native session ID:** Claude Code's conversation UUID, assigned by Hopback at launch (§9).
- **Status:**
  - `active`: running.
  - `parked`: a build session that ended with a validated handoff.
  - `interrupted`: a build session that ended without a handoff. The UI shows this as "ended without handoff".
  - `completed`: a concierge session that ended. Concierge sessions don't produce handoffs.
- **Other fields:** objective, task ID, display name, start and end timestamps, handoff path.

A session left `active` whose launching Hopback process is gone (terminal closed, crash) must be marked `interrupted` the next time Hopback runs in that project. A session that is genuinely still running in another terminal must **not** be marked interrupted.

### 4.3 Backlog item
Fields:
- ID, title, description.
- Type: `idea | feature | bug | improvement | tech-debt | research | experiment`.
- Priority: `high | medium | low`.
- Status: `idea | planned | in_progress | blocked | completed | cancelled`.
- Optional `depends_on`.
- Optional `needs_review` (a reason plus the event that raised it).
- Timestamps: created, updated, completed.
- Source session and recording agent.
- `origin`: `user_requested` or `agent_suggested`, so the user-approval rule (§8) is auditable.

### 4.4 Decision
Fields:
- ID, title, status (`open | decided | superseded`), context.
- For open decisions: the options and the tradeoffs.
- For decided ones: the decision and the rationale.
- Optional `supersedes`.
- Timestamps, source session and recording agent.

**Open** decisions are how an agent asks the user to make a call. They appear under "Needs your decision" in the brief.

### 4.5 Project state (the brief's source)
The latest agent-written status, in `state.json`. Fields:
- `mode`: `mid_feature | between_features`.
- `bottom_line`: one or two sentences.
- `current_feature` (when mid-feature): task ID, why, where it stopped, next step.
- `blockers`.
- `recommended_next`: a task ID plus the reason.
- `alternatives`: two or three task IDs, each with a reason.
- `project_status`: `active | paused | blocked | completed | archived`.
- `revision`, `updated_at`, `updated_by` (session and agent).

### 4.6 Event
An append-only log line for every change Hopback makes, with a timestamp, type, session, agent and data. Events let the concierge see "what changed since you last looked" and give the history the PRD requires.

## 5. The brief

The brief is rendered by Hopback from `state.json`, `backlog.json`, `decisions.json` and the session records. No AI is involved. It is short, plain and structured like an executive status email.

```
Docslet · status brief · last worked 12 days ago · Claude Code

BOTTOM LINE
  Version history is half built. Storage works; diff view not started.

NEEDS YOUR DECISION
  D-4  Store full revisions or only diffs? (more disk vs. more complexity)
  ⚑    2 tasks may be affected by the auth change: T-9, T-11 (review)

RECOMMENDED NEXT
  T-12  Build the diff view, so restored drafts can be compared first.

BACKGROUND
  Why:         Users need to restore old drafts after bad edits.
  Stopped at:  Revision table + save hook done.
  Next step:   Diff endpoint.
```

Between features, the BACKGROUND section is replaced with:

```
  Recently done:  Auth · Document upload · Sharing links
  Still left:     7 open (3 high): Version history, Collaboration, Deployment …
```

Rules:
- "Recently done" is the last few completed backlog items by completion time. "Still left" shows the open count plus the top items by priority. Hopback computes both from the backlog.
- "Needs your decision" collects open decisions, backlog items flagged `needs_review`, and blockers.
- If the latest build session ended without a handoff, show a clear one-line warning at the top, then the last known state.
- With no state yet (a fresh project), say so and point to the concierge.
- Inactivity is shown neutrally ("last worked 12 days ago"), never as a warning.
- Exact wording, colours and widths are the builder's call. It must fit on one screen for a typical project.

`hopback status` prints the brief without the menu.

## 6. Concierge

### 6.1 Purpose
A planning-only agent session for **one project**. It does not see other projects. It reads the backlog, state, decisions, the latest build handoff, and the events since the last concierge session. It discusses with the user what to build next, and records the outcome.

Hopback puts "what changed since the last concierge session" directly into the concierge's opening prompt. This is a plain list of events since that session ended, computed without AI, so the concierge doesn't have to dig through `events.jsonl` itself.

### 6.2 Always fresh
Every concierge session is a new conversation. Its continuity comes entirely from Hopback's files. Anything worth keeping from a concierge conversation must be written down through `hopback` as it happens: priorities, decisions, reasons for postponing something.

### 6.3 What it may do
- Read everything in `.hopback/`.
- Change the backlog and decisions under the rules in §8.
- Set the recommended next task and alternatives (`hopback next set`), and update the brief's bottom line if the plan changed.
- It must **not** modify project code. Enforce this through the launch configuration if practical (for example, disallowing file-editing tools while still allowing it to run `hopback`). Otherwise enforce it through its instructions. This is the builder's call.

### 6.4 After the concierge exits
If a next task was set during the session, Hopback offers to start a build session on it immediately. Otherwise it simply returns.

## 7. Build sessions and parking

### 7.1 Starting
- **Fresh build session:** Hopback launches Claude Code with build-role instructions and an opening prompt. The prompt tells the agent to:
  - read `.hopback/HOPBACK.md`, the state, the latest handoff, and the relevant backlog items and decisions;
  - give the user a brief summary of where things stand;
  - then work on the objective (a task ID).
- **No objective available** (no current feature and no recommended next): start the session without one, and have the agent ask the user what to work on.
- **Reopen:** native Claude Code resume of the last build session's conversation (§9).

### 7.2 During the session
The agent records meaningful project-level changes as they happen, using `hopback` commands under the rules in §8: new backlog items, decisions, task status changes, and architectural changes with the tasks they may affect. Trivial code changes are not recorded.

### 7.3 Parking (the only session-ending path in the MVP)
The user types `/park` in Claude Code. The park instructions (shared with every agent through HOPBACK.md) tell the agent to:
1. Update the status of the tasks it worked on.
2. Record any decisions not yet recorded.
3. Write the new project state (`hopback state set`): mode, bottom line, current feature or recently completed, blockers, recommended next step.
4. Write the session handoff: a Markdown narrative for the next agent. Its required sections, documented in HOPBACK.md, are **Objective**, **Done**, **Stopped at**, **Unfinished**, **Gotchas** and **Next step**. A section may say "none", but it must be present.
5. Run `hopback session park`.

`hopback session park` validates that:
- the handoff exists, is non-empty, and has the required sections;
- `state.json` was updated by this session after it started.

If anything is missing, it exits with an error, lists what's missing, and leaves the session `active`. The park instructions tell the agent to fix the problem and retry, or to tell the user plainly that parking failed.

When the build session exits, Hopback checks the session record:
- **Parked:** offer the concierge (§3).
- **Not parked:** mark it `interrupted` and print a plain message saying it ended without a handoff, with a reminder to `/park` next time. **Recovery is deferred** (§12).

### 7.4 Sessions not launched by Hopback
If the user runs `claude` directly in a Hopback project, the agent still finds Hopback through the pointer in CLAUDE.md, and `/park` still works:
- When the session environment variable is absent, the park procedure first registers a session (`hopback session register` or similar). That returns an ID, which the agent passes explicitly (`--session <id>`) to the commands that follow. Park validation then works as in §7.3.
- Such a session has no native session ID, so it can be continued fresh but not reopened.
- Hopback does not try to discover these conversations.

## 8. Update rules (who may write what, without asking)

| Situation | Behaviour |
|---|---|
| User explicitly asks ("add this to the backlog", "let's go with Postgres") | Agent records it directly (`origin: user_requested`). |
| Agent comes up with a feature, refactor, idea or decision on its own | Agent asks the user first and records it only on a yes (`origin: agent_suggested`). |
| Progress on the task being worked on (started, done, blocked) | Recorded directly. |
| A change may affect other backlog items | Agent reports it with the affected task IDs. Hopback flags those items `needs_review`. Nothing is changed or cancelled automatically. |
| Agent needs the user to make a call it can't make now | Agent records an **open** decision. It shows in "Needs your decision". |

These rules apply to the concierge and build roles alike. They live in HOPBACK.md, so any agent gets them. Hopback cannot enforce "asked the user first". It records `origin` so it's visible.

## 9. Claude Code integration (the only driven agent in the MVP)

Verified against Claude Code 2.1.286:

| Need | Mechanism |
|---|---|
| Know the conversation ID without discovery | Launch with `--session-id <uuid>` generated by Hopback |
| Reopen a conversation | `--resume <uuid>` |
| Mark concierge sessions in Claude's own `/resume` picker | `--name "hopback concierge · <project>"`; build sessions get `--name "hopback build · <task>"` |
| Give the session its role | `--append-system-prompt <role instructions>` plus an opening prompt argument |
| Attribute CLI calls to the session | Hopback sets an environment variable (e.g. `HOPBACK_SESSION`) on the child process. `hopback` commands the agent runs inherit it. |
| `/park` | `hopback init` installs a project-level Claude Code command or skill named `park`, whose body points to the park procedure in HOPBACK.md. The builder should verify the current project-level command/skill location for this Claude Code version. |

Hopback passes only the flags above. It leaves the user's own Claude Code defaults alone (model, permission mode, effort). If `claude` isn't on the PATH, Hopback says so plainly instead of failing obscurely.

All Claude Code specifics live in an **agent adapter** behind an interface: build the launch, resume and park commands; set the session name; supply role instructions; install agent-specific files at init. The core (project, sessions, backlog, decisions, state, events, brief) must not import or mention Claude Code.

## 10. Storage

### 10.1 Project-local (committed to git with the project)
```
.hopback/
  HOPBACK.md        Protocol for agents: purpose, file formats, read order,
                    update rules (§8), commands, park procedure, handoff format.
                    Static text shipped with Hopback (no AI), versioned.
  project.json      ID, name, description, stack, schema_version
  state.json        Project state / brief source (§4.5)
  backlog.json      Backlog items (§4.3)
  decisions.json    Decisions (§4.4)
  events.jsonl      Append-only event log (§4.6)
  sessions/
    <id>.json       Session record (§4.2)
    <id>.md         Handoff (agent-written; build sessions only)
```
- Every JSON file carries a `schema_version`. Formats are documented, either in HOPBACK.md or in schema files next to it (builder's call), so the project stays readable without Hopback installed.
- Session IDs must be unique without coordination (two sessions may start concurrently) and sort by start time.
- Anything strictly machine-local and transient, such as process IDs used to detect dead `active` sessions or lock files, must not be committed. Keep it outside `.hopback/` or gitignore it.

### 10.2 Machine-local
`~/.hopback/projects.json` holds the list of registered projects (ID, name, path). No database in the MVP: everything about a project is in its own folder, so there is nothing to synchronise. If listing many projects ever gets slow, SQLite can be added later as a rebuildable index.

### 10.3 `hopback init`
- Create `.hopback/` with empty files and HOPBACK.md, then register the project.
- If `.hopback/` already exists (for example, a cloned repo on a new machine), just register it.
- Add a short, clearly marked pointer block to `CLAUDE.md` (creating the file if needed) saying "this project uses Hopback; read `.hopback/HOPBACK.md`".
- Append the same block to `AGENTS.md` if that file already exists.
- Install the Claude Code `/park` command (§9).
- Running it again is safe and doesn't duplicate anything.
- HOPBACK.md and the `/park` command file belong to Hopback. Any Hopback command refreshes them when the installed Hopback carries a newer template version. Files the agent writes are never touched.

### 10.4 Integrity and concurrency (applies to all writes)
- All writes go through Hopback. Agents may *read* files directly. Direct edits are tolerated, but Hopback validates files on read and reports invalid ones instead of silently fixing them.
- Every read-modify-write runs under a per-project lock. Files are replaced atomically (write a temp file, then rename). On Windows the rename can fail if another process has the file open, so retry briefly.
- Field-level operations (task update, decision add) need no revision check because the lock serialises them. Whole-document writes (`state set`) carry the revision the agent read. If it's stale, reject with a message showing what changed. Never overwrite silently.
- `events.jsonl` is append-only and written under the same lock.
- Every write records the session and agent that made it.

## 11. CLI (MVP)

Human-facing:
| Command | Purpose |
|---|---|
| `hopback` | §3.1 inside a project, §3.2 outside |
| `hopback init` | §10.3 |
| `hopback projects` | Table: name, status, last worked, brief headline / recommended next, open count, needs-review count |
| `hopback status` | The brief, no menu |
| `hopback sessions [--concierge]` | Build sessions (or concierge sessions): when, status, objective |
| `hopback resume [project] [--fresh]` | Shortcut: reopen the last build conversation, or with `--fresh`, start a fresh build session |
| `hopback concierge [project]` | Shortcut: start a concierge session |

Agent-facing. Structured flags or JSON input, `--json` output, non-zero exit with a clear message on validation failure:
| Capability | Notes |
|---|---|
| read context | Everything an agent needs at session start, in one call (optional convenience; agents may read the files directly) |
| task add / update / list | Enforces enums, records origin and attribution, emits events |
| decision add / resolve | Open or decided; resolve closes an open one |
| architecture change | Summary plus affected task IDs. Records the event and flags those tasks `needs_review`. |
| review clear | User or agent clears a `needs_review` flag after reviewing |
| state set | Whole-state write with stale-revision check (§10.4) |
| next set | Recommended task, reason, and alternatives |
| session park | Validation per §7.3 |

Exact command names and flag syntax are the builder's call. Command families like `hopback task add` are preferred, matching the PRD's examples.

## 12. Deferred (not in the MVP)

- **Recovery when a session ends without `/park`:** reopening the conversation in the background to write the handoff, recording git facts at session end, recovering with a different agent when one is out of usage.
- **Checkpoints** (saving a handoff without ending the session).
- **Driving other agents** (Codex first). They can already read the files and call `hopback`.
- MCP server.
- Interactive TUI. Plain numbered prompts for now. A GUI dashboard is not planned.
- Optional milestones and completion percentage.
- Cross-project concierge ("which project should I pick up today?").
- Git worktree handling (§14).
- Reminders and notifications.

## 13. Deviations from the PRD

| PRD | MVP | Why |
|---|---|---|
| SQLite for operational data | A JSON project list. All project data lives in `.hopback/`. | Removes the PRD's open file↔DB sync problem; nothing in the MVP needs a database. |
| `hopback park` CLI command for humans | `/park` inside the agent; `hopback session park` is the agent-facing registration | A separate terminal can't instruct a running interactive agent. |
| `hopback switch` | Folded into `hopback` outside a project and `hopback resume <project>` | Same effect, fewer commands. |
| `sessions/session-001.md` numbering | Unique, time-sortable IDs | Concurrent sessions must not collide. |
| No concierge concept | Build/concierge session roles | Owner's design decision: planning shouldn't consume the build session's context. |
| Phase 2 includes checkpoints | Deferred | Owner chose park-only for now. |
| Phase 3 includes completion estimates | Deferred | The brief's done/left already answers the owner's need. |

## 14. Known limitations (accepted for the MVP)

- **Git worktrees:** each worktree has its own copy of `.hopback/`, so their states diverge until merged.
- **Multiple machines:** sequential task and decision IDs (e.g. `T-12`) can collide if the user records items on two machines before syncing. This is rare for a solo developer, and the IDs stay human-friendly.
- **Agent discipline:** Hopback can't verify that an agent asked before recording an idea. `origin` makes it visible.
- **Claude's own `/resume` list** still shows concierge sessions. They're labelled by `--name`, not hidden.
- **Concierge sessions consume agent usage**, the same as build sessions.

## 15. Who decided what

**Owner decided:**
- The brief's two shapes and their content.
- The consultant-style briefing format.
- Concierge as a separate session per project, fresh every time, marked so it never counts as the latest session.
- Park-only for ending sessions; recovery comes later.
- The update rules in §8.
- CLI-first, at most a TUI, no dashboard.
- Claude Code only for now.
- Stack left to the builder.

**My calls (owner can override):**
- Go, as a single binary.
- No SQLite.
- `.hopback/` committed to git.
- CLI only, no MCP.
- Plain prompts, no TUI yet.
- Milestones and completion % deferred.
- The menu defaults in §3.1.
- Open decisions as the "Needs your decision" mechanism.
- Sessions not launched by Hopback get recorded at park time without a native ID.
- The required handoff sections (§7.3).
- If there's no objective, the build agent asks the user.
- Hopback computes the concierge's "what changed" list.
- Hopback keeps HOPBACK.md and `/park` up to date in each project.
- Hopback leaves the user's Claude Code defaults alone.

**Left to the builder:**
- CLI library and command syntax.
- ID formats (within §10.1's constraints).
- Lock mechanism.
- Exact JSON field names.
- Brief styling.
- How the concierge's no-code rule is enforced.
- Test strategy. A fake agent adapter is recommended, so the lifecycle can be tested without launching Claude Code.
