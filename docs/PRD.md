# Hopback — Product Requirements Document

**Version:** 1.0  
**Status:** Initial product specification  
**Product:** Hopback  
**Category:** AI-assisted project continuity and session management

**Tagline:** Don't just remember your sessions. Remember your projects.

---

# 1. Product Overview

## 1.1 What is Hopback?

Hopback is a project-first session manager for independent developers who use AI coding agents to build and maintain multiple projects.

Modern AI coding tools have made it significantly easier to work on several projects simultaneously. Developers can switch between projects, experiment with new ideas and delegate implementation to different AI agents.

However, this creates a new problem: maintaining continuity across all these projects.

A developer might work on five or six projects, use Claude Code for one, Codex for another, and alternate between agents within the same project.

When returning to a project after several days or weeks, they often have to reconstruct their previous mental state:

- What was I building?
- What have I already implemented?
- What remains unfinished?
- Which ideas did I postpone?
- What architectural decisions did I make?
- What was I planning to do next?
- How much of the project is actually complete?

Existing AI coding agents maintain their own conversation and session histories, but conversation history is not equivalent to project-level continuity.

Hopback addresses this problem by maintaining a unified representation of projects, their backlogs, progress, decisions and development sessions, independently of the AI agent being used.

## 1.2 Core Philosophy

**Other tools help AI agents remember. Hopback helps developers remember their projects.**

Hopback is not another coding agent, conventional project management application or AI memory engine.

Its purpose is to help developers maintain awareness and control of their projects while allowing them to continue using their preferred development tools.

The coding agent is responsible for understanding the codebase, identifying meaningful changes and generating project handoffs.

Hopback provides the infrastructure to manage sessions, persist project state and make that information accessible to the developer and other agents.

## 1.3 Product Principles

1. **Project-first:** Projects are the primary entities. Conversations and sessions belong to projects.
2. **User-centric:** Optimize for developer awareness, not agent memory.
3. **Agent-agnostic:** No dependency on a particular coding agent or harness.
4. **Workflow-friendly:** Complement existing development workflows rather than replacing them.
5. **Local-first:** Project information should remain accessible and portable.
6. **Agent-owned intelligence:** Agents generate handoffs, summaries and project updates. Hopback does not independently perform these reasoning tasks.
7. **Low maintenance:** Developers should not have to manually maintain another project management system.
8. **Abandonment tolerant:** Projects may remain inactive for weeks or months without being treated as overdue.

---

# 2. Target Audience

## 2.1 Primary Audience

Independent developers, hobbyists, solo builders and developers working on personal or side projects using AI coding agents.

Typical characteristics:

- Maintains approximately 5–20 projects.
- Frequently switches between unrelated projects.
- Uses multiple AI coding agents.
- May use different agents within the same project.
- Works on projects intermittently.
- May abandon a project for weeks or months before returning.
- Has no formal sprint or delivery schedule.
- Is responsible for deciding what to build and when.

## 2.2 The Problem We Solve

The primary problem is not coordinating development across a team.

It is helping an individual developer maintain continuity and awareness across multiple independent projects.

Hopback should help users remember:

- What they intended to build.
- What they have already built.
- What remains unfinished.
- Which ideas they postponed.
- What architectural decisions they made.
- Where they stopped working.
- What changed while they were away.
- What they should do when they return.

## 2.3 Explicitly Out of Scope

Hopback is not intended to replace organizational project management systems.

The following are outside its primary scope:

- Scrum and sprint management.
- Story points and velocity tracking.
- Team workload management.
- Employee assignments.
- Organizational reporting.
- Formal approval workflows.
- Enterprise project governance.
- Corporate task and delivery management.

## 2.4 Product Design Implications

1. No mandatory deadlines or schedules.
2. No requirement to organize work into milestones.
3. No expectation that projects are continuously active.
4. No negative notifications for project inactivity.
5. Backlogs should support unstructured ideas and unfinished work.
6. Progress should communicate project state rather than productivity.
7. Resumption should be optimized for projects that have been inactive for extended periods.

**Hopback should help developers manage the projects they want to build, not the work an organization expects them to deliver.**

---

# 3. Product Scope

## 3.1 Core Capabilities

Hopback must provide:

1. Project registration and management.
2. Cross-agent session management.
3. Native session resumption where supported.
4. Project-level backlog management.
5. Agent-driven project-state updates.
6. Session parking and handoff.
7. Project resume briefings.
8. Project progress visibility.
9. Agent-agnostic interoperability.
10. Personal project portfolio awareness.

## 3.2 Explicit Non-Goals

Hopback should NOT:

- Act as a coding agent.
- Analyze repositories independently to generate project summaries.
- Independently generate session handoffs.
- Replace existing coding agents.
- Replace IDEs or terminals.
- Store or normalize complete agent conversations.
- Become a Jira, Linear or Trello replacement.
- Require developers to use a separate application during normal coding.
- Force users to change their existing development workflow.
- Require projects to follow a formal development methodology.

---

# 4. Core Product Concepts

## 4.1 Project

A project is the primary organizational entity in Hopback.

It represents a codebase or software development effort.

A project can have multiple sessions across different agents.

Example:

**Docslet**
- Claude Code sessions: 15
- Codex sessions: 8
- Cursor sessions: 3
- Total sessions: 26

All sessions belong to the same project and contribute to its unified project history.

Projects are the primary entities. Sessions are subordinate to projects.

## 4.2 Session

A session represents an individual work period.

Sessions should be independent of the agent or harness used.

A session should contain:

- Unique session ID.
- Project ID.
- Agent identifier.
- Harness identifier, where applicable.
- Native agent session reference, where available.
- Start timestamp.
- End timestamp.
- Session status.
- Objective.
- Handoff reference.
- Checkpoint information.

Suggested session statuses:

- Active
- Parked
- Completed
- Interrupted
- Failed

Session summaries and handoffs are generated by the agent, not Hopback.

## 4.3 Backlog

Each project maintains a lightweight backlog.

Backlog items represent things the developer intends to build, investigate, fix or improve.

Possible categories:

- Ideas.
- Features.
- Bugs.
- Improvements.
- Technical debt.
- Research.
- Experiments.

Each backlog item should support:

- ID.
- Title.
- Description.
- Type.
- Priority.
- Status.
- Optional dependencies.
- Optional milestone association.
- Creation timestamp.
- Last updated timestamp.
- Source session.
- Agent attribution.

Suggested statuses:

- Idea
- Planned
- In Progress
- Blocked
- Completed
- Cancelled

The backlog is not a sprint backlog.

It is a persistent collection of intended work, postponed ideas and unfinished tasks.

There should be no mandatory deadlines, story points or sprint assignments.

## 4.4 Project State

Project state represents the latest known condition of a project.

It should include:

- Overall project status.
- Current development focus.
- Last completed work.
- Current blockers.
- Next recommended action.
- Completion estimate, where available.
- Latest session reference.
- Recent architectural changes.

Project state is maintained through agent updates.

## 4.5 Decisions

Projects should maintain a history of important technical and product decisions.

Examples:

- Switching from JWT authentication to session-based authentication.
- Choosing PostgreSQL over MongoDB.
- Deciding to use immutable document revisions.
- Changing the frontend architecture.

Decisions should have timestamps and references to the sessions that introduced them.

## 4.6 Events

Meaningful project changes should be represented as events.

Examples:

- Backlog item created.
- Backlog item completed.
- Architectural decision recorded.
- Architecture changed.
- Potential impact detected.
- Session checkpointed.
- Session parked.

Events provide historical traceability and allow Hopback to show what changed since the developer last worked on a project.

---

# 5. Product Architecture

## 5.1 Architecture Principle

Hopback must be agent-agnostic.

It must not depend on a particular coding agent's internal memory, conversation format or project management capabilities.

All agents interact with the same underlying project state.

Proposed architecture:

```text
                  HOPBACK CLI
                       |
                       |
                  HOPBACK CORE
                       |
           +-----------+-----------+
           |                       |
      Session Manager         Project Manager
           |                       |
      Agent Adapters          State / Backlog
           |                       |
           +-----------+-----------+
                       |
                Project Storage
                       |
              Shared Hopback State
                       |
        +--------------+--------------+
        |              |              |
    Claude Code      Codex          Cursor
        |              |              |
        +--------------+--------------+
```

The architecture should support new agent integrations without requiring changes to the core project model.

## 5.2 Core Components

### Hopback CLI

The primary interface for developers.

Responsibilities:

- Register projects.
- List projects.
- Display project status.
- List sessions.
- Launch agents.
- Resume sessions.
- Park sessions.
- Switch projects.
- Expose project-state operations to agents.

### Hopback Core

Responsible for:

- Project registry.
- Session registry.
- Session lifecycle.
- Agent adapters.
- State validation.
- Event processing.
- Persistence.
- Native session integration.

### Agent Integrations

Integrations allow Hopback to interact with individual coding agents.

Potential integrations:

- Claude Code plugin/hooks.
- Codex integration.
- Cursor rules or supported integrations.
- OpenCode integration.
- Gemini CLI integration.
- MCP server.

Integrations should remain thin.

They should communicate Hopback's protocol and lifecycle instructions, rather than implementing separate project management logic.

### Optional Visual Interface

A future dashboard may provide:

- Personal project overview.
- Project details.
- Session history.
- Backlog browsing.
- Project progress.
- Resume selection.

A separate desktop application is not required for the initial MVP.

---

# 6. Agent Responsibilities vs Hopback Responsibilities

This separation is fundamental.

## 6.1 Agent Responsibilities

The AI coding agent owns project understanding and knowledge generation.

It is responsible for:

- Reading project context.
- Understanding the codebase.
- Generating project descriptions.
- Creating and maintaining session handoffs.
- Identifying unfinished work.
- Identifying new backlog items.
- Updating existing tasks.
- Recording architectural decisions.
- Detecting potential architectural impact.
- Updating project completion information.
- Identifying the next recommended action.

Hopback must not independently perform these AI reasoning tasks.

## 6.2 Hopback Responsibilities

Hopback owns session orchestration and project-state infrastructure.

It is responsible for:

- Knowing which projects exist.
- Knowing which sessions belong to each project.
- Launching and resuming supported agents.
- Providing standardized prompts and instructions.
- Providing access to the Hopback protocol.
- Accepting structured agent updates.
- Validating and persisting changes.
- Maintaining session history.
- Maintaining event history.
- Displaying project and backlog information.
- Detecting missing or invalid state.
- Reporting inconsistent updates.

**The agent does the thinking. Hopback maintains continuity.**

---

# 7. Agent-to-Hopback Communication

Agents must be able to invoke Hopback independently during a session.

This is a critical requirement.

Hopback is not merely an application that invokes agents. Agents must also be able to invoke Hopback when they identify meaningful project changes.

## 7.1 Agent-Initiated Operations

Examples of operations:

| Operation | Purpose |
|---|---|
| project_get | Retrieve project information |
| task_list | Retrieve backlog items |
| task_create | Create a backlog item |
| task_update | Update an existing task |
| decision_record | Record an architectural decision |
| architecture_change | Report an architectural change |
| impact_report | Report potentially affected work |
| milestone_update | Update optional milestone progress |
| session_checkpoint | Register a checkpoint |
| session_park | Register a parked session |

These operations should be exposed through the CLI and, where useful, MCP.

Both interfaces must use the same underlying core functionality.

## 7.2 When Should Agents Invoke Hopback?

Agents should be instructed to invoke Hopback when they identify meaningful project-level changes.

| Situation | Expected action |
|---|---|
| User introduces a new feature idea | Create backlog item |
| User discusses a future improvement | Add idea to backlog |
| New architectural decision is made | Record decision |
| Architecture changes | Report architectural change |
| Existing work may be invalidated | Report potential impact |
| Significant task is completed | Update task |
| New blocker is identified | Update project state |
| User requests a checkpoint | Save checkpoint |
| User parks a project | Generate handoff and park session |

Avoid unnecessary updates for trivial code changes.

Agents should use judgement about what constitutes a meaningful project-level event.

## 7.3 Architectural Impact Tracking

This is an important capability.

Example:

1. An agent changes the authentication architecture.
2. The agent identifies that existing API implementation tasks may be affected.
3. The agent invokes Hopback with the architectural change.
4. Hopback records the change.
5. Hopback associates the change with potentially affected backlog items.
6. The developer is informed that those items may need review.

Hopback should not independently decide that an existing task is obsolete.

Potentially affected tasks should be flagged for review unless the user has enabled an appropriate automatic update policy.

---

# 8. Hopback Project Protocol

The Hopback protocol is the foundation of interoperability.

It should use a documented, agent-independent format.

The initial implementation should use project-local files.

Proposed structure:

```text
my-project/
├── src/
├── tests/
├── README.md
└── .hopback/
    ├── HOPBACK.md
    ├── project.json
    ├── state.json
    ├── backlog.json
    ├── decisions.json
    ├── events.jsonl
    └── sessions/
        ├── session-001.md
        ├── session-002.md
        └── session-003.md
```

## 8.1 HOPBACK.md

The universal entry point for agents.

It should describe:

- Hopback's purpose.
- Project-state format.
- Where project information is stored.
- How agents should read project context.
- When agents should invoke Hopback.
- How backlog updates work.
- How architectural changes should be reported.
- How session handoffs are generated.
- How projects are parked.

## 8.2 Other Files

**project.json**

Project identity and configuration.

**state.json**

Latest known project state.

**backlog.json**

Structured backlog and optional milestones.

**decisions.json**

Persistent technical and product decisions.

**events.jsonl**

Append-only project event history.

**sessions/**

Session history and handoff documents.

## 8.3 Important Storage Principle

The agent generates and maintains the handoff content.

Hopback does not generate summaries or handoffs.

Hopback may validate that expected files exist and contain valid data, but must not silently replace missing content with AI-generated content.

## 8.4 Interoperability Requirements

- Any agent capable of reading and writing files should be able to use the protocol.
- Avoid vendor-specific formats.
- Use documented JSON schemas for machine-readable state.
- Use Markdown for human-readable documentation.
- Maintain schema versions.
- Include timestamps and source-agent attribution.
- Support multiple agents working on the same project.
- Detect stale revisions and conflicting updates.
- Preserve historical records.

The project should remain understandable and usable without Hopback installed.

---

# 9. Session Lifecycle

## 9.1 Starting a Session

The developer invokes Hopback.

Hopback allows the developer to:

- Select a project.
- Select an agent.
- Select an existing session to resume.
- Start a new session.

When launching an agent, Hopback supplies standardized instructions.

The agent should:

1. Read HOPBACK.md.
2. Read the current project state.
3. Review the latest session handoff.
4. Review relevant backlog items.
5. Review relevant architectural decisions.
6. Summarize its understanding to the developer.
7. Continue normal development.

Hopback does not independently generate the resume briefing.

The agent generates it using project-local information.

## 9.2 Native Session Resumption

Where supported, Hopback should be able to resume an existing native agent conversation.

The session registry should preserve native session identifiers and necessary metadata.

Two kinds of resumption must be supported:

**Conversation resumption**

Continue the original native agent conversation.

**Project resumption**

Start a fresh conversation using the latest project state and handoff.

These are separate actions.

Hopback must not assume all agents support equivalent native resume functionality.

## 9.3 Parking a Session

Parking is a first-class lifecycle operation.

When the developer wants to stop working:

1. Hopback instructs the agent to park the project.
2. The agent reviews the current work.
3. The agent updates the project state.
4. The agent updates the backlog if necessary.
5. The agent records important decisions.
6. The agent identifies blockers.
7. The agent records the next recommended action.
8. The agent creates a session handoff.
9. Hopback validates the expected output.
10. Hopback registers the session as parked.

If the agent fails to complete the handoff, Hopback must report that parking is incomplete.

It must not falsely report a successful checkpoint.

## 9.4 Interrupted Sessions

The system should support sessions that terminate unexpectedly.

Hopback should distinguish between:

- Properly parked sessions.
- Sessions that were interrupted.
- Sessions that are still active.
- Sessions whose native state is unavailable.

An interrupted session should not be assumed to have a valid final handoff.

The latest successful checkpoint should remain accessible.

---

# 10. Project Portfolio Awareness

This is the primary user-facing differentiator.

Hopback should help developers understand the state of their entire project portfolio.

The developer should not have to open individual agents to discover what remains unfinished.

## 10.1 Project Overview

Each project should display:

- Project name.
- Project description.
- Technology stack.
- Project status.
- Completion estimate, where available.
- Last worked timestamp.
- Latest session.
- Outstanding task count.
- Blocked task count.
- Next recommended action.

Suggested project statuses:

- Active
- Paused
- Blocked
- Completed
- Archived

Project inactivity should not automatically mean a project is blocked or overdue.

## 10.2 Resume Briefing

Before resuming a project, the user should be able to see:

- What was accomplished in the previous session.
- What remains unfinished.
- What the current project focus is.
- What architectural changes occurred.
- What blockers exist.
- What should happen next.

The briefing should be generated by the coding agent using stored project state.

## 10.3 Backlog

The backlog should be lightweight and flexible.

Suggested categories:

- Ideas
- Planned
- In Progress
- Blocked
- Completed
- Cancelled

Support:

- Priority.
- Optional dependencies.
- Descriptions.
- Optional milestones.
- Source sessions.
- Creation and modification history.

There should be no mandatory deadlines, sprint assignments or story points.

## 10.4 Project Completion

Project completion must not be calculated from the number of completed tasks alone.

Optional weighted milestones may be used.

Example:

| Milestone | Weight | Completion |
|---|---:|---:|
| Authentication | 20% | 100% |
| Document management | 25% | 100% |
| Version history | 20% | 50% |
| Collaboration | 20% | 0% |
| Deployment | 15% | 0% |

Completion:

P = Σ (milestone weight × completion fraction)

Milestones and weights should be optional.

The agent may propose progress updates, but the developer must be able to review and override them.

Display project completion separately from backlog health.

Do not present completion estimates as objective measurements of the entire software project's quality.

---

# 11. CLI Requirements

The CLI is the primary interface for the initial product.

It should support:

```bash
hopback
```

Open an interactive project/session selector.

```bash
hopback init
```

Initialize Hopback in a repository.

```bash
hopback projects
```

List registered projects.

```bash
hopback status
```

Display the current project's state.

```bash
hopback sessions
```

Display session history.

```bash
hopback resume
```

Open the resume selector.

```bash
hopback resume <project>
```

Resume the latest relevant session.

```bash
hopback resume <project> --agent <agent>
```

Start or resume the project using a selected agent.

```bash
hopback park
```

Initiate the parking workflow.

```bash
hopback switch
```

Switch projects.

Agent-facing operations:

```bash
hopback task add
hopback task update
hopback task list

hopback decision add

hopback architecture change
hopback impact report

hopback checkpoint
```

Exact command syntax is subject to CLI design.

Agent-facing commands should support structured input and machine-readable output.

The CLI must be usable both interactively by humans and programmatically by agents.

---

# 12. Technical Direction

Proposed initial stack:

| Component | Technology |
|---|---|
| Core | Go |
| CLI | Go |
| Local operational database | SQLite |
| Project state | Markdown + JSON |
| Agent communication | CLI + MCP |
| Desktop interface | Tauri + React (future) |
| Version control integration | Git |

These are proposed implementation choices, not mandatory requirements.

## 12.1 Local-First

The initial product should be local-first.

No cloud account should be required.

Project state should be portable and version-controllable.

Cloud synchronization and team collaboration are future possibilities, not MVP requirements.

## 12.2 Persistence

SQLite should manage operational information such as:

- Registered projects.
- Session registry.
- Agent metadata.
- Event indexing.
- Native session references.

Project-local files should contain portable project knowledge and handoffs.

The system must define how SQLite and project files remain consistent.

---

# 13. Reliability and Data Integrity

Important architectural requirements:

- Use atomic writes for state changes.
- Preserve append-only event history.
- Use schema versioning.
- Detect stale revisions.
- Detect conflicting concurrent updates.
- Maintain session identifiers.
- Record agent attribution.
- Avoid silent overwrites.
- Validate agent-provided structured updates.
- Preserve historical checkpoints.
- Never claim a handoff was completed when it failed.

Multiple agents may operate on the same project, so concurrency must be considered from the beginning.

---

# 14. MVP Scope

The MVP should prove one central hypothesis:

**Can a developer reliably return to any project after days or weeks and immediately understand its current state and resume work?**

## Phase 1 — Foundation

- Go-based CLI.
- Project initialization.
- Project registry.
- Local project-state structure.
- Basic session registry.
- HOPBACK.md protocol instructions.
- Structured backlog.

## Phase 2 — Claude Code Integration

Claude Code is the first integration because it is the initial development and testing environment.

Implement:

- Agent launch.
- Native session resumption where supported.
- Resume prompt.
- Parking prompt.
- Agent-initiated backlog updates.
- Agent-initiated architectural change reports.
- Checkpoint registration.

## Phase 3 — Personal Project Awareness

- Project listing.
- Project status.
- Backlog overview.
- Session history.
- Optional project completion estimates.
- Resume briefing.
- Recently worked on projects.

## Phase 4 — Interoperability

Integrate a second agent, preferably Codex.

Validate:

- Shared project state.
- Cross-agent session history.
- Cross-agent handoffs.
- Agent-initiated updates.
- Native session references.
- Concurrent updates.

## Phase 5 — Future Enhancements

- Optional desktop dashboard.
- Additional agent integrations.
- Cloud synchronization.
- Team collaboration, only if there is a clear future need.
- Advanced project dependency tracking.
- Personal project prioritization.

---

# 15. MVP Acceptance Criteria

The MVP should be considered successful when:

1. A developer can register multiple repositories.
2. Hopback can list all registered projects.
3. A developer can launch a supported coding agent from Hopback.
4. The agent can read the shared project protocol.
5. The agent can generate a project handoff.
6. Hopback can register and display sessions.
7. The agent can create backlog items through Hopback.
8. The agent can update existing tasks.
9. The agent can report architectural changes.
10. Hopback can flag potentially affected tasks.
11. A developer can park a session.
12. A developer can resume a project using a fresh agent conversation.
13. Native session resumption works where the agent supports it.
14. A second agent can consume and update the same project state.
15. Project state remains understandable without Hopback installed.
16. A developer can return to an inactive project and understand its status without manually reconstructing previous conversations.

---

# 16. Unresolved Product Decisions

These decisions require further investigation before finalizing implementation.

| Topic | Open question |
|---|---|
| CLI experience | Interactive TUI or conventional CLI initially? |
| Agent launching | Which agents should be supported first? |
| Session discovery | Can native sessions be discovered automatically? |
| Native resume | Which agent APIs and CLI commands are available? |
| Project identity | How should multiple Git worktrees map to projects and sessions? |
| Storage | How should SQLite and project-local files synchronize? |
| Backlog updates | Which updates require user confirmation? |
| Progress | How should optional milestone weights be established? |
| Concurrent agents | What conflict-resolution strategy should be used? |
| MCP | Should MCP be included in the first release or follow the CLI? |
| Dashboard | Is a TUI sufficient for the first version? |
| Notifications | Should Hopback offer optional reminders for projects the user wants to revisit? |

These should be investigated rather than prematurely decided.

---

# 17. Product Positioning

**Product name:** Hopback

**Tagline:** Don't just remember your sessions. Remember your projects.

**Product category:** AI-assisted project continuity and session management.

**Core USP:**

Hopback is a user-centric project continuity layer that helps independent developers manage multiple projects and AI coding sessions without losing track of what they are building, what remains unfinished and where they should resume.

Unlike agent-centric memory systems, Hopback makes the developer's project portfolio the primary experience.

**Primary entities:** Projects, sessions, backlog items and optional milestones.

**Primary interface:** CLI.

**Core integration:** Agent adapters and shared project protocol.

**Fundamental design principle:**

The agent does the thinking. Hopback maintains continuity.

The developer should be able to use their preferred coding agents and existing development workflows without being forced into another application.

---

# 18. Instructions for the Implementing Agent

Treat this document as the initial product specification.

Before implementing:

1. Review the requirements.
2. Identify technical ambiguities.
3. Investigate existing agent integration mechanisms.
4. Validate native session resume capabilities.
5. Propose the initial data model.
6. Design the Hopback protocol and JSON schemas.
7. Identify concurrency and persistence concerns.
8. Propose an MVP implementation plan.

Do not start by implementing a dashboard.

Do not introduce an LLM into the Hopback core.

Do not make Claude Code-specific assumptions in the core architecture.

Do not introduce Scrum or enterprise project management features into the initial implementation.

Prioritize the shared protocol, session lifecycle and CLI.

Preserve the distinction between confirmed requirements and proposed implementation details.

**The ultimate goal is to make returning to any project as easy as starting a new one.**
