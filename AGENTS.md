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

The current backend is not yet under `backend/domains`. Migrate it one domain
at a time instead of moving unrelated code in a large mechanical refactor.
`rules` is the first reference domain.

The current database schema is not the target schema. Check
`doc/architecture-decisions.md` before extending tables so approved workflow
decisions are not lost in incremental changes.

### Domain Boundaries

Each domain must expose a small public entry point. Code outside a domain should
use that public interface rather than importing internal implementation files.

Preferred entry points:

```text
web/domains/<domain>/index.js
backend/domains/<domain>/public.go
```

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

### Native Web Components

Use native Web Components and ES modules without requiring an npm build step.
Use light DOM by default so shared styles remain visible and easy to debug.
Reserve Shadow DOM for components that genuinely require style isolation.

Reusable components belong under:

```text
web/components/
  buttons/
  data/
  input/
  layout/
```

Domain-specific UI belongs under `web/domains/<domain>/`. Components should use
clear, domain-oriented custom element names such as `<rules-editor>` and
`<schedule-grid>`. Custom element names must contain a hyphen.

Avoid inline event attributes such as `onclick`. Components should register
their own listeners and communicate through explicit methods, properties, and
custom events.

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

## Git And Project Log

- Use Git CLI for status, diff, staging, commits, history, and blame.
- Include question IDs in commits when a change resolves or advances one.
- Stage only intended files; the worktree may contain unrelated user changes.
- Never commit `PROJECT_LOG.log`.
- Update `PROJECT_LOG.log` with what changed, why it mattered, and how the work
  was performed so it remains a teaching record.
