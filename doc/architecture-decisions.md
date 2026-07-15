# League App Architecture Decisions

**Status:** Target design
**Last reviewed:** 2026-06-08

This document consolidates approved product and architecture decisions. It
describes the target model, not necessarily the schema currently implemented in
`db/db.go`. See `doc/erd.mermaid` for the current physical database.

## Design Principles

- Organize frontend and backend code by business domain.
- Use matching domain names across the screen, API, backend, and documentation.
- Keep authoritative business logic and validation on the backend.
- Use native Web Components with ES modules and no required npm build step.
- Use light DOM and shared CSS by default.
- Keep rule definitions in backend code and editable values in the database.
- Prefer stable controlled codes over free text.
- Name optional explanatory free-text fields `notes`.
- Keep one append-only system audit log.
- Preserve historical results instead of silently rewriting them.

## Competition Model

A league represents one competition track, including its playing night and
format:

```text
Monday 8-Ball
Tuesday 8-Ball
Thursday 9-Ball
```

Multiple leagues may run at the same time. Each league may have one active
season, while draft future seasons and closed historical seasons coexist.

The database does not currently support divisions. Division scope is not part
of the approved target model.

## Season Setup Workflow

The league administrator owns season setup:

```text
Create draft season and configure rules
-> add existing or new teams
-> assign players to home teams
-> enter start date
-> enter schedule options, requests, and No Play weeks
-> generate schedule
-> preview and adjust
-> approve
-> activate immediately
```

Season participation is explicit. Teams are registered in `season_teams` before
activation. The setup checklist enforces minimum participation rules and
schedule readiness before the season can go live.

Rules and team membership are locked when the season activates. Controlled
administrative workflows may alter the active schedule without silently
changing completed history.

## Rule Model

Rule definitions are developer-owned backend code. Definitions include:

- Permanent machine key
- Editable display label
- Explicit value type
- Developer-defined validation metadata
- Status: `draft`, `experimental`, or `confirmed`
- Version and explanation

Supported value types begin with:

- `number`
- `whole_number`
- `boolean`
- `text`
- `choice`
- `percentage`
- `duration`

Editable values use this inheritance:

```text
system default -> league -> season
```

The API returns the effective value and its source scope. Creating a season
produces a season rule snapshot. The snapshot is configurable during setup and
locked at activation.

Mid-season rule amendments and emergency corrections remain an open design
topic.

## Team And Player Participation

Players are shared system-wide and may participate in multiple leagues without
duplicate player records.

During season setup, a player may have one home team in that season. A player
may substitute for any team in any league or season as long as the player
exists in the system.

Target concepts:

```text
season_teams
  season_id
  team_id
  status_code
  joined_at

season_rosters
  season_id
  team_id
  player_id
  status_code

match_players
  match_id
  team_id
  player_id
  participation_code
```

These names are conceptual until schema implementation is designed. The
current `players.team_id` field does not support the approved long-term model.

## Last-Minute Players

An admin may search for an existing player or quick-add a new player during
match entry. A quick-added player:

- Becomes a real player record immediately
- May play immediately
- Receives an `INCOMPLETE` profile status
- Appears in an Admin review queue
- Prevents the league week from closing until reviewed

`PLAYERS-Q001` remains open: define minimum quick-add fields, duplicate
detection, and initial handicap assignment.

## Schedule Workflow

Schedule preview may be changed without affecting the currently active season.
The final editing controls will be defined with the schedule UI.

Planned and emergency No Play weeks use the same concept. Each stores a
controlled reason code and optional `notes`.

A schedule pushback:

- Selects a cutoff week/date
- Inserts one or more No Play league weeks
- Moves every unplayed scheduled week at or after the cutoff together
- Preserves completed match dates
- Honors existing No Play dates
- Extends the season end date
- Shows all affected matches before approval
- Creates an audit entry

## Match And Week Workflow

Match score data remains pending until the containing week closes successfully:

```text
score entry -> pending validation -> close week -> official results
```

The exact controlled match statuses are deferred to `MATCHES-Q001` and the
online-entry design in `MATCHES-Q002`. Results not committed through week close
are excluded from official standings and player statistics and shown with
match-specific status, for example:

```text
Team 1 vs Team 2 - Incomplete
Team 3 vs Team 4 - Missing player review
Team 5 vs Team 6 - Reopened
```

Scores may be entered before week close, but they do not trigger official
handicap, match outcome, standings, or player-stat calculations. Close Week
runs backend validation and commits official calculations only after the
validation requirements are satisfied.

Close Week validation looks for missing scores or players, impossible scoring
combinations, duplicate participation, incomplete player profiles, handicap or
input inconsistencies, unresolved matches, and format-specific scoring errors.

- Errors block close and cannot be overridden.
- Warnings require explicit admin acknowledgment before close.
- Acknowledgment requires admin identity, controlled reason code, optional
  `notes`, timestamp, warning details, affected records, and an audit entry.

Transparency is the default for administrative overrides. Where historical
warning acknowledgments appear in the regular UI remains open.

Weeks may be closed multiple times. To correct a closed week:

```text
select affected matches
-> reopen week
-> edit only selected matches
-> finalize corrected matches
-> close week again
```

Unaffected matches remain locked. Every reopen, change, warning acknowledgment,
and close is audited. Standings and statistics remain available throughout the
season and reflect only results committed through successful week close.

## Season Closing

Season closing is a separate Admin workflow. It does not happen automatically
when another season activates.

Closing a season:

- Verifies every match is complete or has a controlled resolution
- Calculates standings and placements
- Lets the admin review and approve the results
- Creates an immutable final standings snapshot
- Marks the season closed
- Creates an audit entry

A closed season may be reopened through an audited admin action. Corrections
require recalculation, review, and closing again.

## Controlled Codes

Statuses, reasons, categories, and participation types use stable codes.
Transactional records store only the code; current labels are resolved for
display.

Code values:

- Are permanent after creation
- Are unique within a code set
- May repeat across different code sets
- Have editable labels
- Have display order
- Have an active flag
- Remain readable when inactive but cannot be selected for new records

Developers define which code sets exist and what they mean. A future Admin
screen will manage labels, display order, and active status.

Potential code sets include:

- Season status
- Team and roster status
- Player profile status
- Match workflow status
- Week workflow status
- Match resolution
- No Play reason
- Correction and reopen reason
- Substitute participation type
- Audit action

The physical design of the code system remains pending until these sets and
their overlap are reviewed together.

**Phase 1 decision (2026-07-14):** Behavior-driving codes remain developer-owned
constants in each domain package. No DB-backed `code_sets` or `code_values`
tables are implemented. DB-backed labels, display order, and active flags are
deferred until a real admin workflow requires runtime editing without a deploy.
A read-only backend codes registry and `/api/codes/{set}` endpoint are possible
future work, not current implementation. See `doc/domains/codes/README.md` for
the full inventory and deferred items. Resolves `CODES-Q001`.

## Audit Log

Use one append-only audit log across all domains. Entries include:

- Domain and affected record
- Action code
- Previous values
- New values
- Acting user
- Timestamp
- Reason code when applicable
- Optional `notes`

Admins can filter audit history by date, user, domain, action, and affected
record. Audit history is never silently replaced or deleted.

## Users And Roles

Players and authenticated users are different concepts:

- A player participates in leagues and matches.
- A user signs in and receives roles and permissions.
- A player may exist without a user account.
- An admin user may exist without a player record.

Provisional relationship:

```text
users.player_id NULL UNIQUE -> players.id
```

Review this before authentication is implemented, especially for household
accounts, guardians, shared emails, and account transfers.

`USERS-Q001` remains open: define email invitation and account activation when
a user account is created for an existing player.

## Open Questions

| ID | Status | Question |
| --- | --- | --- |
| `RULES-Q001` | on hold | How are emergency or mid-season rule amendments handled? |
| `PLAYERS-Q001` | on hold | What fields and handicap value are required for quick-add players? |
| `USERS-Q001` | open | How does the email invitation and account-linking workflow operate? |
| `CODES-Q001` | resolved 2026-07-14 | What physical code-table design best supports all approved code sets? |
| `SCHEDULES-Q001` | open | Which manual edits are allowed during schedule preview? |
| `MATCHES-Q001` | on hold | What match status follows completed score entry before week close? |
| `MATCHES-Q002` | on hold | How will online score entry, permissions, drafts, and review work? |
| `MATCHES-Q003` | open | Where are historical warnings and acknowledgments displayed? |

## Decision History

### 2026-06-08 - Adopt domain-first architecture

**Status:** accepted

Frontend, backend, and documentation use matching business-domain names with
small public interfaces.

### 2026-06-08 - Make backend business logic authoritative

**Status:** accepted

The backend owns rules, permissions, calculations, constraints, validation, and
persistence in preparation for users and roles.

### 2026-06-08 - Approve season-centered workflow

**Status:** accepted

Season setup proceeds through rules, teams, players, dates, scheduling requests,
preview, approval, and immediate activation.

### 2026-06-08 - Separate players from users

**Status:** accepted with relationship review pending

Player participation and authenticated access are separate concerns. The
initial one-to-one foreign-key proposal must be reviewed before implementation.

### 2026-06-08 - Use controlled codes and one audit log

**Status:** accepted

Stable codes drive behavior; labels are editable display data. Administrative
changes are recorded in one append-only audit history.

### 2026-06-08 - Make Close Week the official calculation boundary

**Status:** accepted

Score entry remains pending. Backend week-close validation detects errors and
warnings before official calculations affect standings and statistics.

### 2026-06-08 - Default administrative overrides to transparency

**Status:** accepted

Errors cannot be overridden. Every warning override requires an explicit,
reasoned acknowledgment preserved in the audit log.

### 2026-06-08 - Move all system rules to backend enforcement

**Status:** accepted, on hold — implement after architecture rebuild

All season rule keys (`max_individual_handicap`, `handicap_rounding`,
`max_pairing_spot`, `max_match_spot`, `handicap_update_method`,
`lineup_players_per_team`, `games_per_pairing`, `allow_substitutes`,
`allow_bye_requests`, `require_bye_approval`) will be enforced server-side.
Currently only `handicap_multiplier` is read by the backend; the rest are
display-only in the frontend. No new frontend-only rule enforcement should be
added. Wire rules to backend as part of the domain-first migration.

### 2026-06-08 - Defer season preview/approve step

**Status:** accepted, deferred — add to backlog after architecture rebuild

The approved season setup workflow includes a preview-and-approve step before
activation. The current implementation goes directly from schedule generation
to activation. The preview/approve stage will be added once the season domain
is migrated to the target architecture.

### 2026-06-08 - Approve all domain READMEs as build targets

**Status:** accepted

All domain documents under `doc/domains/` (rules, seasons, schedules, matches,
players, teams, standings, users, codes, audit) are approved as authoritative
targets. Implementation may proceed domain by domain using these as the design
reference.

### 2026-07-14 - Defer physical code storage; adopt developer-owned constants

**Status:** accepted

Behavior-driving codes remain stable developer-owned constants in each domain
package. No DB-backed code tables are implemented in Phase 1. The `rules`
domain's Definition registry is the approved pattern analog. Admin-editable
labels and a read-only codes API are deferred until a real admin workflow
creates the need. Resolves `CODES-Q001`.
