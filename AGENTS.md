# League App Engineering Guide

This file defines the expected architecture and working conventions for Codex
and other contributors. `CLAUDE.md` contains additional project context and
should remain compatible with these rules.

## Architecture

### Domain-First Organization

Organize frontend and backend code by business domain. Prefer matching names
across both sides so a problem visible on screen points to an obvious starting
location in the repository.

Target domains include:

- `audit`
- `codes`
- `rules`
- `schedules`
- `seasons`
- `teams`
- `players`
- `matches`
- `handicaps`
- `standings`
- `users`
- `roles`
- `auth`

The target structure is:

```text
web/domains/<domain>/
backend/domains/<domain>/
doc/domains/<domain>/
```

Major backend domains (matches, handicaps, seasons, leagues, players, teams,
and rules) are now under `backend/domains`. Continue the domain-first approach
for any new domains. Migrate one domain at a time rather than moving unrelated
code in a large mechanical refactor.

The current database schema is not the target schema. Check
`doc/architecture-decisions.md` before extending tables so approved workflow
decisions are not lost in incremental changes.

### Domain Boundaries

Each domain must expose a small public entry point. Code outside a domain should
use that public interface rather than importing internal implementation files.

Preferred entry points:

```text
web/domains/<domain>/<domain>-domain.js
backend/domains/<domain>/public.go
```

Frontend entry-point filenames must identify both the domain and their role.
Do not use `index.js` as a domain entry point. For example, use
`web/domains/handicaps/handicaps-domain.js`.

Temporary direct access is acceptable during migration, but new dependencies
should use the public interface.

### Responsibility Split

The backend is authoritative for:

- Business rules and rule interpretation
- Authorization and permissions
- Handicap and scoring calculations
- Scheduling constraints
- Authoritative validation
- Persistence and database operations

The frontend is responsible for:

- Presentation and rendering
- User interactions and form state
- Helpful, non-authoritative validation
- API communication

Do not duplicate authoritative business logic in the browser. The frontend may
explain or preview backend behavior, but the backend must validate it again.

## Frontend

### Frontend Domain Pattern

The frontend must grow toward the same domain boundaries as the backend. The
application shell owns only cross-domain coordination:

- Navigation and lightweight routing
- Shared application context such as selected league and season
- Authentication context when introduced
- Notifications and global error presentation
- Mounting domain entry points

The shell must not own domain rendering, domain API details, or domain-local
state. Do not add substantial new workflows directly to `web/app.js` or
`web/index.html`.

A frontend domain should use only the files its responsibilities require. A
typical domain may look like:

```text
web/domains/handicaps/
  handicaps-domain.js
  handicap-review-component.js
  handicap-api-service.js
```

Responsibilities:

- `<domain>-domain.js` is the domain's public entry point. It registers or
  exports the domain's supported components without exposing internals.
- `<workflow>-component.js` owns rendering, interaction, lifecycle, loading,
  empty, validation, and error states for one cohesive workflow.
- `<domain>-api-service.js` encapsulates domain API calls and response handling.
- Additional components are added only when they have a distinct responsibility
  or meaningful reuse.
- A separate state module is added only when domain state is too complex to
  remain local to its components.

Domain components communicate with the shell through explicit properties,
methods, and custom events. Local state is the default. Shared state is limited
to genuinely application-wide context such as league, season, authentication,
and user preferences.

Do not create generic abstractions merely to match a folder template. Shared
API, paging, caching, routing, or state infrastructure requires at least two
real consumers with the same confirmed semantics.

Framework adoption is a separate architecture decision. Use native Web
Components for incremental domain extraction. Evaluate Vue or another
framework only after a representative domain pilot demonstrates that custom
lifecycle, event coordination, or shared state has become unnecessarily
complex.

### Native Web Components

Use native Web Components and ES modules without requiring an npm build step.
Use light DOM by default so shared styles remain visible and easy to debug.
Reserve Shadow DOM for components that genuinely require style isolation.

Reusable components belong under:

```text
web/components/
  buttons/
  data/
  feedback/
  input/
  layout/
```

Domain-specific UI belongs under `web/domains/<domain>/`. Components should use
clear, domain-oriented custom element names such as `<rules-editor>` and
`<schedule-grid>`. Custom element names must contain a hyphen.

Avoid inline event attributes such as `onclick`. Components should register
their own listeners and communicate through explicit methods, properties, and
custom events.

Reusable components must remain domain-neutral. A shared input component must
not directly depend on a domain editor, domain API, global event bus, or hidden
global cache. Keep controlled-code data access separate from the select input
that renders those values.

### Frontend Naming

All frontend filenames must indicate what they are. Avoid generic names such as
`index.js`, `service.js`, `state.js`, `view.js`, `component.js`, `utils.js`, and
`helpers.js`.

Use names that identify the domain or reusable purpose and the responsibility:

```text
handicaps-domain.js
handicap-review-component.js
handicap-api-service.js
close-week-component.js
controlled-code-select.js
validation-message-list.js
```

The same rule applies to exported classes, functions, custom elements, events,
and CSS classes. Names should be useful in imports, searches, stack traces, and
browser developer tools.

### Shared CSS

Keep styling in shared CSS rather than embedding large style blocks in
component JavaScript. Grow toward this structure:

```text
web/styles/
  tokens.css
  base.css
  layout.css
  components.css
  forms.css
  tables.css
  themes.css
```

Use custom properties for design tokens, themes, and dynamic styling. Modern
CSS features are encouraged with sensible browser fallbacks:

- Grid, Flexbox, container queries, and subgrid
- OKLCH, relative colors, and `light-dark()`
- Popovers, scroll snap, and scrollbar styling
- `interpolate-size`, `calc-size()`, and smooth transitions

Prefer semantic HTML and stable, descriptive class names.

## File Design

- Keep `web/index.html` primarily as the application shell.
- Prefer files below roughly 300 lines.
- Review files approaching 500 lines for mixed responsibilities.
- Split by responsibility, not merely to satisfy a line count.
- Avoid vague modules such as `utils`, `helpers`, or `manager` unless their
  cross-domain responsibility is explicit and cohesive.
- Match frontend and backend terminology whenever the concepts are the same.

## Business Logic Status

Business behavior must be labeled as one of:

- `draft`: incomplete or awaiting confirmation
- `experimental`: intentionally being evaluated
- `confirmed`: approved behavior that may be relied upon

Tests should protect confirmed behavior and stable boundaries. Do not turn
draft formulas into permanent requirements through overly specific tests.
Stable boundaries include API shapes, persistence, required fields, error
handling, and confirmed rules.

Non-obvious business logic requires an explanation block:

```go
// Rule: Max individual handicap
// Status: draft
// Version: 0.1
// What: Caps a player's handicap at the configured league limit.
// Why: Prevents one handicap value from overwhelming match scoring.
// Inputs: Player handicap and league rule settings.
// Output: The capped handicap value.
// Notes: Legacy FileMaker interpretation pending confirmation.
```

Keep comments current when behavior changes. Explain the business reasoning,
not syntax that is already clear from the code.

## Domain Documentation

Each domain must have:

```text
doc/domains/<domain>/README.md
```

Start new documents from `doc/domains/_template/README.md`. Domain documentation
must describe purpose, status, terminology, inputs, outputs, business rules,
examples, assumptions, unresolved questions, and decision history.

Confirmed business-logic changes require a matching documentation update.
Update `doc/architecture-decisions.md` when a decision affects more than one
domain.

### Question Tracking

Number unresolved questions by domain:

```text
RULES-Q001
SCHEDULES-Q001
HANDICAPS-Q001
```

Keep answered questions in the document. Record their status, resolution date,
decision, reasoning, and related commit when available. Never delete a question
merely because it was answered.

Use `notes` for optional free text in database records. Prefer controlled codes
for statuses, reasons, categories, and other values used by business logic.
Business logic must use stable codes rather than display labels.

### Decision History

Record important decisions concisely. The domain document explains why a
decision was made; Git remains the detailed record of exact file changes.

## Migration

Migrate one domain at a time, beginning with `rules`. During the first pass:

1. Preserve existing appearance and behavior.
2. Establish the domain boundary and public interface.
3. Extract reusable Web Components where responsibilities are clear.
4. Move authoritative behavior to the backend.
5. Add tests for stable boundaries and confirmed behavior.
6. Add or update domain documentation.
7. Verify behavior before making separate design improvements.

## Roadmap Discipline

Use `doc/roadmap.md` to decide whether new work belongs in the current phase.
Do not pull later roadmap items forward merely because they are interesting or
adjacent to current work.

Before proposing or starting a new phase, answer these four questions:

1. Does this directly stabilize an existing admin workflow?
2. Does this reduce monolith ownership by moving logic into domain, service,
   store, adapter, or component boundaries?
3. Is this aligned with the roadmap's `Now` or `Next` sections?
4. Does this avoid pulling broader auth, audit, or platform expansion forward
   prematurely?

If the answer is no, defer the work unless the Project Manager explicitly
reprioritizes it.

Current sequencing rules:

- Prioritize restructuring and workflow completion.
- Do not let auth grow beyond the current bridge unless it becomes
  operationally necessary.
- Do not pull the shared audit/history capability forward yet.
- Do not treat browser/mobile work as platform expansion; only consider small
  workflow prototypes later.
- Keep new implementation work on named feature branches, not on `main`.

## Git And Project Log

- Use Git CLI for status, diff, staging, commits, history, and blame.
- Keep `main` stable and reviewable. Do not begin new implementation work
  directly on `main` once an accepted milestone is in place.
- Before starting a new feature or phase, the Project Manager creates a branch
  named for the active work item, for example `handicap-apply-b3`,
  `close-week-phase-3c`, or `teams-phase-8`.
- Use explicit feature or phase names for branches. Do not use vague branch
  names such as `test`, `temp`, `misc`, or `updates`.
- Reserve `main` for accepted work, reviewed commits, staging-ready
  checkpoints, and pushed milestones.
- Keep incomplete, experimental, or in-review implementation work on the active
  feature branch until it is accepted.
- The Project Manager owns branch creation and naming so invited reviewers and
  stakeholders can rely on `main` staying consistent.
- The Project Manager also owns the workflow steps that change repository or
  environment state outside normal implementation handoff:
  - create and switch feature branches
  - push branches or `main` to `origin`
  - merge approved branches back into `main`
  - deploy to staging, restart staging, and clean up merged branches
- After a feature or phase branch is accepted, merged into `main`, and pushed
  to `origin/main`, the Project Manager should delete that merged branch
  locally and on `origin` unless there is a short-term reason to retain it.
- If a merged branch is intentionally retained, record the reason in the PM
  handoff, roadmap notes, or todo notes so branch clutter does not become the
  default.
- Developers should hand off commit-ready work, verification results, and
  deployment notes to the Project Manager rather than performing those PM-owned
  steps unless explicitly directed otherwise.
- The Project Manager also owns the local project learning journal in
  `PROJECT_LOG.log`. After major accepted phases, the PM may backfill entries
  that explain what changed, why it mattered, how the code works now, what
  stayed intentionally unchanged, and what lessons or gotchas should be
  remembered later.
- `PROJECT_LOG.log` is for personal learning and project continuity. It may
  include short code examples for non-obvious patterns. It is not part of the
  normal commit flow and must remain uncommitted.
- Use the following default job split unless the Project Manager explicitly
  delegates a step:
  1. Project Manager creates or switches to the named feature branch.
  2. Developer implements, tests, documents, and reports a review-ready handoff.
  3. Project Manager reviews scope, approves or rejects, and requests any
     corrections.
  4. Developer creates the approved commit and reports the commit hash, staged
     file list, and post-commit status.
  5. Project Manager pushes the feature branch to `origin`.
  6. Project Manager merges the approved branch into `main`.
  7. Project Manager pushes `main` to `origin/main`.
  8. Project Manager deploys to staging when deployment is requested or needed
     for verification.
  9. Project Manager verifies staging health and records the result.
  10. Project Manager deletes the merged branch locally and on `origin` unless
      there is a documented reason to keep it.
- Strict PM mode is the default. "Next", "go ahead", "keep moving", or similar
  PM direction means continue only through PM-owned steps unless the Project
  Manager explicitly asks the same agent to act as the Developer too.
- Do not perform implementation, code edits, test-writing, or developer-side
  commit preparation merely because the next phase is clear. That work belongs
  to the Developer role unless explicitly delegated.
- Switch into combined PM + Developer execution only when the Project Manager
  explicitly asks for implementation, coding, or developer-side handling in the
  same session.
- Developer handoff memos should not claim PM-owned steps as complete unless the
  Project Manager explicitly delegated them for that phase.
- PM-owned steps should be requested and reported explicitly using PM wording
  such as branch creation, merge approval, push, `DEPLOY-STAGING`, and branch
  cleanup.
- PM handoff helper commands under `.claude/commands/` are approved workflow
  tools:
  - `/handoff "branch-name: next task description"` is PM-owned and writes a
    compact resume primer to `.claude/pending-task.md`.
  - `/pickup` is developer-owned when the developer session also runs in Claude
    Code and should load `.claude/pending-task.md` before implementation begins.
  - If the developer session runs in Cursor instead of Claude Code, the PM
    should still use `/handoff`; paste the saved `.claude/pending-task.md`
    block directly into the Cursor chat instead of relying on `/pickup`.
  - Do not commit `.claude/pending-task.md`.
- Include question IDs in commits when a change resolves or advances one.
- Stage only intended files; the worktree may contain unrelated user changes.
- Never commit `PROJECT_LOG.log`.
- Update `PROJECT_LOG.log` with what changed, why it mattered, and how the work
  was performed so it remains a teaching record.
