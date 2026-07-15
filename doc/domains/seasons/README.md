# Seasons

## Overview

**Owner:** `seasons`
**Status:** `draft`
**Current version:** `0.3`
**Last reviewed:** `2026-07-04`

The Seasons domain owns setup, activation, league-week workflow, closing,
reopening, and final standings snapshots.

## Setup Workflow

```text
Draft
-> name season + set start date
-> configure rules (shown in New Season form; buffered until save)
-> save → season record created; buffered rules saved immediately
-> add participating teams (from prior season or new)
-> assign players to season roster per team; set captain per team
-> manage: schedule generation, skip weeks, bye requests
-> resolve all checklist blockers
-> activate
```

The admin owns setup. Team participation is explicit: each team must be
registered in `season_teams` before the checklist passes. Teams can be added
from a prior season (copies roster) or created new.

### New Season Rules

The New Season form shows the same rule controls as Edit Season Details inside
a collapsed "Season Rules (optional)" accordion. Rule changes made before
clicking "Save & Continue" are buffered in the `<rules-editor>` web component's
`#pendingRules` map. `flushPending(seasonId)` is called immediately after the
season record is created and saves all buffered rules via
`POST /seasons/{id}/rules`.

### Management Panel

The season management panel is the setup dashboard for managed seasons. It
shows the setup checklist, registered team count, roster counts, captain status,
and any checklist items that map directly to a team. Activation controls are
shown only for draft seasons (`active=false` and `activated_at IS NULL`) and are
disabled until the checklist returns `can_activate=true`.

For managed seasons, schedule generation uses the current season's registered
teams only. The legacy "Use Teams From" selector is hidden in the management
panel for managed seasons because the backend rejects `from_season_id` on that
path. Bye request team choices and odd/even messaging also use registered
season teams rather than all permanent league teams.

Schedules for inactive seasons are admin previews. User-facing schedule views
remain active-season focused; the management panel displays a preview note until
the season is activated.

## Active Season

One season per league may be active. Different leagues, nights, and formats may
have active seasons simultaneously.

Activation locks rules and team membership. Controlled schedule changes remain
available.

## Week Review

Scores may be entered before an admin closes the league week. Close Week runs
backend validation, presents errors and warnings, and commits official
calculations only after errors are resolved and every warning is explicitly
acknowledged.

A closed week may be reopened multiple times, but only selected affected
matches become editable. Closing again reruns validation and creates another
audited review event.

Close Week is only available for active seasons. Attempting to close a week
for a draft season returns 409 with code `WEEK_CLOSE_SEASON_DRAFT`. The
schedule page reflects this with a disabled "Review & Close" button and a
draft-season banner.

## Closing

A season cannot close while matches remain unresolved. Each match must be
completed or receive a controlled resolution. Closing calculates placements,
requires admin approval, and stores an immutable final standings snapshot.

Corrections to a closed season require audited reopening, recalculation, review,
and closing again.

## Decision History

### 2026-06-08 - Separate activation and closing

**Status:** `accepted`

Activating a new season never silently closes the previous season.

### 2026-06-08 - Make season participation explicit

**Status:** `accepted`

Season teams will be recorded directly rather than inferred from matches.

## Activation Enforcement

`POST /api/seasons/{id}/activate` runs the setup checklist before proceeding.
For managed seasons (`teams_managed=1`), all blocker checks must pass or the
handler returns `422 Unprocessable Entity` with:

```json
{
  "error": "season cannot be activated; resolve all blockers first",
  "blockers": [
    { "code": "NO_SCHEDULE", "message": "...", "team_id": 0 }
  ]
}
```

Stable blocker codes: `TEAMS_TOO_FEW`, `TEAM_NO_PLAYERS`, `TEAM_NO_CAPTAIN`,
`CAPTAIN_NOT_ON_ROSTER`, `SCHEDULE_STALE`, `NO_SCHEDULE`, `NO_END_DATE`.

Warning codes (do not block activation): `TEAM_FEW_PLAYERS`.

**Legacy bypass:** Seasons with `teams_managed=0` (the `DEFAULT` for all rows
created before Phase One) skip all checklist enforcement and always return
`can_activate=true`. This is not a zero-team check — it is an explicit column
flag. New seasons created via the API always get `teams_managed=1`.

**Setup lock:** First activation sets `activated_at=CURRENT_TIMESTAMP` on the
season row. This timestamp is never cleared. `IsDraft()` on `SeasonService`
checks `activated_at IS NULL`, so the setup lock persists even after another
season becomes active (deactivation does not re-enable editing).

## Backend Domain Boundary (Phase A)

The seasons domain is extracted from `handlers/api.go` into a layered stack:

```
handlers/api.go          — thin HTTP layer; calls SeasonManager interface
handlers/deps.go         — SeasonManager interface definition
backend/domains/seasons/ — pure domain logic; no HTTP or database imports
  service.go             — SeasonService: Activate, Checklist, PreviousSeason,
                           IsDraft, MarkStaleIfScheduled
  store.go               — SeasonStore interface + types (SeasonMeta, TeamSummary,
                           SeasonTeamEntry, PreviousSeasonResult, ChecklistBlockErr,
                           ErrNotFound)
  public.go              — package-level helpers: RosterEligible
backend/storage/sqlite/
  season_store.go        — sqlite.SeasonStore implementing seasons.SeasonStore
```

### SeasonStore interface

Methods: `IsDraft`, `GetMeta`, `GetTeamSummaries`, `GetMatchCount`, `Activate`,
`MarkStaleIfScheduled`, `FindActiveWithNoEndDate`, `FindClosestPriorByEndDate`,
`GetSeasonTeams`, `GetMatchTeams`.

`GetMeta` uses `strftime('%Y-%m-%d', col)` in the SELECT to force TEXT return from
the SQLite driver, which would otherwise convert DATE-typed columns to `time.Time`.

### SeasonService methods

- **IsDraft** — delegates to store; returns `ErrNotFound` when season absent
- **Checklist** — builds `models.SetupChecklist`; skips all checks for legacy
  seasons (`teams_managed=0`); returns `(SetupChecklist, ErrNotFound)` for absent
  seasons
- **Activate** — runs checklist; returns `*ChecklistBlockErr` when blocked;
  otherwise calls store.Activate; TODO comment marks rule-snapshot hook as deferred
- **PreviousSeason** — priority 1: active season with no end_date; priority 2:
  closest prior by end_date; team data from `season_teams` with fallback to match
  history
- **MarkStaleIfScheduled** — sets `schedule_stale=1` when unplayed matches exist

### Handler thinning (Phase A)

Logic removed from `handlers/api.go`:
- `isDraftSeason(seasonID int64) bool` — deleted; replaced by `mgr.IsDraft(ctx, id)`
- `markStaleIfScheduled(seasonID int64)` — deleted; replaced by `mgr.MarkStaleIfScheduled(ctx, id)`
- `previousSeasonResponse` and `previousTeamEntry` types — deleted; replaced by
  `seasons.PreviousSeasonResult` and `seasons.SeasonTeamEntry`

Handlers thinned: `activateSeason`, `getSeasonChecklist`, `getPreviousSeasonTeams`,
`addSeasonTeam`, `updateSeasonTeam`, `removeSeasonTeam`, `addRosterPlayer`,
`removeRosterPlayer` — all now take a `SeasonManager` parameter injected via closure.

### Backend Domain Boundary (Phase B)

Phase B moves the remaining business logic out of three team-management handlers
and two bye-request handlers into the domain layer.

**New store methods (SeasonStore interface):**

Team management (7): `GetTeamLeagueID`, `GetSeasonTeam`, `AddSeasonTeamCopy`,
`AddSeasonTeamNew`, `CheckPlayerOnSeasonRoster`, `UpdateSeasonTeamMeta`,
`RemoveSeasonTeam`.

Bye requests (7): `CountParticipatingTeams`, `CheckTeamInSeason`, `HasDuplicateBye`,
`InsertByeRequest`, `GetByeRequest`, `HasByeConflict`, `SetByeApproval`.

**New service methods (SeasonService / SeasonManager interface):**

- **AddTeam** — checks draft, validates managed constraint and prior-season
  membership, delegates to `AddSeasonTeamCopy` or `AddSeasonTeamNew`, calls
  `MarkStaleIfScheduled`, returns `GetSeasonTeam`
- **RemoveTeam** — checks draft, delegates to `RemoveSeasonTeam`, calls
  `MarkStaleIfScheduled`
- **UpdateTeam** — checks draft, validates name, validates captain on roster,
  delegates to `UpdateSeasonTeamMeta`, returns `GetSeasonTeam`
- **CreateByeRequest** — gets meta, checks odd team count, validates league/season
  membership, checks duplicate, delegates to `InsertByeRequest`
- **UpdateByeRequest** — gets existing bye, checks week-0 guard, checks conflict,
  delegates to `SetByeApproval`

**Error translation pattern:**

Store methods return typed sentinel errors (`ErrTeamAlreadyInSeason`,
`ErrTeamNotInSeason`, `ErrByeNotFound`, `ErrTeamNotInPriorSeason`). The service
translates these into `domainerr.Err` with categories (`InvalidInput`,
`Unprocessable`, `NotFound`, `Internal`). The handler calls `mapSeasonErr` which
maps `domainerr.Err` categories to HTTP status codes (400/404/422/500).

SQLite adapters (`backend/storage/sqlite/`) must NOT import `domainerr`; only
the service layer uses it.

**New SQLite files:**
- `backend/storage/sqlite/season_team_store.go` — 7 team methods
- `backend/storage/sqlite/season_bye_store.go` — 7 bye methods

**Local types removed from `handlers/api.go`:**
- `addSeasonTeamRequest` — replaced by `seasons.AddTeamRequest`
- `updateSeasonTeamRequest` — replaced by `seasons.UpdateTeamRequest`

**Handler thinning (Phase B):**

`addSeasonTeam`, `updateSeasonTeam`, `removeSeasonTeam`, `createByeRequest`,
`updateByeRequest` — each reduced to ≤10 lines: parse path/body, delegate to
`mgr`, map error, write response.

### Backend Domain Boundary — Season Roster Phase A (implemented 2026-07-03)

Phase A extracts four roster-related endpoints from `handlers/api.go` into the
seasons domain boundary.

**Endpoints extracted:**

| Route | Handler |
|-------|---------|
| `GET /api/seasons/{id}/teams/{tid}/roster` | `listSeasonRoster` |
| `POST /api/seasons/{id}/teams/{tid}/roster` | `addRosterPlayer` |
| `DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}` | `removeRosterPlayer` |
| `GET /api/seasons/{id}/players/available` | `listAvailablePlayers` |

**New sentinel error:**
- `ErrRosterEntryNotFound` — returned by `DeleteRosterPlayer` when the entry is absent

**New store methods (SeasonStore interface):**

Roster (5): `ListRoster`, `GetPlayerRosterTeam`, `InsertOrGetRosterPlayer`,
`DeleteRosterPlayer`, `ListAvailablePlayers`.

**New service methods (SeasonService / SeasonManager interface):**

- **ListRoster** — delegates to store; ensures non-nil slice
- **AddRosterPlayer** — draft check, team-in-season check, player-on-other-team
  check, idempotent insert via `InsertOrGetRosterPlayer`
- **RemoveRosterPlayer** — draft check, delegates to `DeleteRosterPlayer`; maps
  `ErrRosterEntryNotFound` → `domainerr.NotFound`
- **ListAvailablePlayers** — delegates to store; ensures non-nil slice; propagates
  `ErrNotFound` for missing seasons → `mapSeasonErr` returns 404

**New SQLite files:**
- `backend/storage/sqlite/season_roster_store.go` — 5 store methods
- `backend/storage/sqlite/season_roster_store_test.go` — 9 integration tests

**New domain test files:**
- `backend/domains/seasons/roster_service.go` — 4 service methods
- `backend/domains/seasons/roster_service_test.go` — 13 unit tests

**Behaviour note — idempotent add:** `InsertOrGetRosterPlayer` uses `INSERT OR
IGNORE` and always re-fetches by `(season_id, team_id, player_id)`. Re-adding a
player already on the same team returns the existing entry cleanly (201), which
corrects a silent-empty-body bug in the original handler.

**Accepted debt:**
- `listAvailablePlayers` previously fetched `league_id` from the seasons table but
  never used it in the player query; the new store uses it only for season existence
  verification, which preserves the original behaviour (season not found → 404).

### Backend Domain Boundary — Season Roster Phase B (implemented 2026-07-04)

Phase B extracts the remaining 6 season-adjacent handlers that still used direct
`db.DB` calls: `listSeasonTeams`, `listSkippedWeeks`, `createSkippedWeek`,
`deleteSkippedWeek`, `listByeRequests`, and `deleteByeRequest`.

**Endpoints extracted:**

| Route | Handler |
|-------|---------|
| `GET /api/seasons/{id}/teams` | `listSeasonTeams` |
| `GET /api/seasons/{id}/skipped-weeks` | `listSkippedWeeks` |
| `POST /api/seasons/{id}/skipped-weeks` | `createSkippedWeek` |
| `DELETE /api/seasons/{id}/skipped-weeks/{sid}` | `deleteSkippedWeek` |
| `GET /api/seasons/{id}/bye-requests` | `listByeRequests` |
| `DELETE /api/seasons/{id}/bye-requests/{bid}` | `deleteByeRequest` |

**New store methods (SeasonStore interface):**

- `ListSeasonTeams` — returns season teams with roster counts; uses shared
  `seasonTeamCols` + `scanSeasonTeamRow` helpers in `season_team_store.go`
- `ListSkippedWeeks`, `CreateSkippedWeek`, `DeleteSkippedWeek` — in new file
  `season_skipped_week_store.go`; `CreateSkippedWeek` uses `INSERT OR IGNORE` +
  re-query for idempotent behaviour
- `ListByeRequests`, `DeleteByeRequest` — added to `season_bye_store.go`;
  `DeleteByeRequest` returns `ErrByeNotFound` when 0 rows affected

**New service methods (SeasonService / SeasonManager interface):**

- **ListSeasonTeams** — delegates; ensures non-nil slice
- **ListSkippedWeeks** — delegates; ensures non-nil slice
- **CreateSkippedWeek** — delegates directly (no business logic currently needed)
- **DeleteSkippedWeek** — delegates directly; no error on missing row (preserves
  original handler behaviour)
- **ListByeRequests** — delegates; ensures non-nil slice
- **DeleteByeRequest** — delegates; maps `ErrByeNotFound` → `domainerr.NotFound`
  → `mapSeasonErr` → HTTP 404

**New SQLite files:**
- `backend/storage/sqlite/season_skipped_week_store.go` — 3 store methods
- `backend/storage/sqlite/season_skipped_week_store_test.go` — 7 integration tests

**Appended to existing SQLite files:**
- `backend/storage/sqlite/season_bye_store.go` — `ListByeRequests`, `DeleteByeRequest`
- `backend/storage/sqlite/season_team_store.go` — `ListSeasonTeams`

**New domain service files:**
- `backend/domains/seasons/skipped_week_service.go` — 3 service methods
- `backend/domains/seasons/skipped_week_service_test.go` — 7 unit tests
- `backend/domains/seasons/bye_service.go` — `ListByeRequests`, `DeleteByeRequest`
- `backend/domains/seasons/bye_service_test.go` — 7 unit tests + 2 `ListSeasonTeams` tests

**Behaviour notes:**
- `listByeRequests` previously used `JOIN teams` (not LEFT JOIN); new store uses
  `LEFT JOIN` with `COALESCE` for defensive robustness, matching the existing
  `InsertByeRequest`/`GetByeRequest` pattern already in the store.
- `deleteSkippedWeek` previously ignored all errors and always returned 200. The
  new handler propagates genuine DB errors as 500; missing row returns 200
  (no change in observable behaviour for callers).
- `deleteByeRequest` previously returned 404 on missing row (direct Exec check).
  The new path uses `mgr.DeleteByeRequest` → `domainerr.NotFound` → `mapSeasonErr`
  → 404. Behaviour is preserved.

### Backend Domain Boundary — Season CRUD Phase C (implemented 2026-07-04)

Phase C extracts the five season CRUD handlers (`listSeasons`, `getSeason`,
`createSeason`, `updateSeason`, `deleteSeason`) from `handlers/api.go` into the
domain layer and deletes dead handler code that accumulated over earlier phases.

**Endpoints extracted:**

| Route | Handler |
|-------|---------|
| `GET /api/seasons` | `listSeasons` |
| `GET /api/seasons/{id}` | `getSeason` |
| `POST /api/seasons` | `createSeason` |
| `PUT /api/seasons/{id}` | `updateSeason` |
| `DELETE /api/seasons/{id}` | `deleteSeason` |

**New store methods (SeasonStore interface):**

- `ListSeasons(ctx, leagueID *int64)` — optional league filter; ordered by id DESC
- `GetSeason(ctx, seasonID)` — full row; returns `ErrNotFound` (wrapped) for absent rows
- `CreateSeason(ctx, CreateSeasonInput)` — inserts with `teams_managed=1`; returns stored row
- `UpdateSeason(ctx, seasonID, UpdateSeasonInput)` — updates mutable fields; re-fetches and
  returns full stored row; returns `ErrNotFound` (via `GetSeason`) when row absent
- `DeleteSeason(ctx, seasonID)` — no error when row is absent

**Input types (seasons package):**

```go
type CreateSeasonInput struct {
    LeagueID     int64
    Name         string
    StartDate    *string
    ScheduleType string
    NumWeeks     int
}

type UpdateSeasonInput struct {
    Name         string
    StartDate    *string
    ScheduleType string
    NumWeeks     int
}
```

**New service methods (SeasonService / SeasonManager interface):**

- **ListSeasons** — delegates; ensures non-nil slice
- **GetSeason** — propagates `ErrNotFound`
- **CreateSeason** — validates name (non-empty) and `league_id` (non-zero); defaults
  `schedule_type` to `"double_rr"` when blank
- **UpdateSeason** — defaults `schedule_type` to `"double_rr"` when blank; delegates
- **DeleteSeason** — delegates directly

**Dead code deleted from `handlers/api.go`:**

- `seasonCols` const — replaced by `seasonFullCols` in `backend/storage/sqlite/season_crud_store.go`
- `scanSeason` func — replaced by `scanFullSeason` in the sqlite layer
- `normDatePtr` func — no longer needed (callers pass `*string` directly)
- `normDateStr` func — no longer needed
- `seasonTeamSelect` const — moved into `season_team_store.go` in an earlier phase
- `scanSeasonTeam` func — moved into `season_team_store.go` in an earlier phase

**Bug fixed — `updateSeason` response:**

The original handler responded with 200 and an empty body after updating. The new
handler reads back the full stored row via `mgr.UpdateSeason` and returns it as JSON,
matching the behaviour of `createSeason` and all other mutation endpoints.

**SQLite constant note (`seasonFullCols`):**

The column list is a Go string constant (compile-time concatenation with `+`) with
a leading and trailing space, so it can be concatenated as:
`"SELECT" + seasonFullCols + "FROM seasons"` without risking missing whitespace at
the boundary. The list uses `strftime('%Y-%m-%d', col)` for `start_date`, `end_date`,
and `activated_at` to prevent the driver from converting DATE text to `time.Time`.

**New SQLite files:**
- `backend/storage/sqlite/season_crud_store.go` — 5 store methods + `scanFullSeason`
- `backend/storage/sqlite/season_crud_store_test.go` — 11 integration tests

**New domain service files:**
- `backend/domains/seasons/crud_service.go` — 5 service methods
- `backend/domains/seasons/crud_service_test.go` — 13 unit tests using functional stubs

### Deferred

Rule snapshot at activation (lock `season_rules` against further changes) is marked
with a TODO comment in `SeasonService.Activate` and remains unimplemented.

## Available Players

`GET /api/seasons/{id}/players/available` returns all active system players not
already rostered in the season. This includes unassigned players (no `team_id`)
and players assigned to teams in other leagues. No league filter is applied.

## Scoresheet Roster Eligibility

`POST /api/matches/{id}/rounds` blocks scoresheet entry when either team in
the match has fewer than 3 season-roster players. Returns `422` with a
descriptive error. This check is skipped for legacy seasons with
`teams_managed=0`.

## Schedule Generation

`POST /api/matches/generate` with a nonzero `from_season_id` uses prior-season
match records to collect team IDs. For managed seasons (`teams_managed=1`) this
path is rejected (400). Managed seasons always generate exclusively from
`season_teams`; `from_season_id` must be omitted or zero.

Additional guards enforced at the service layer before generation runs:

- `SCHEDULE_HAS_CLOSED_WEEKS` (409): the season has at least one league week
  with status `closed`. Reopen the affected week before regenerating.
- `SCHEDULE_ACTIVE_HAS_COMPLETED` (409): the season is active (`active=1`)
  **and** at least one match has `completed=1`. Draft seasons are not subject
  to this guard.

## Bye Requests

`POST /api/seasons/{id}/bye-requests` validates:

1. **Participating team count** — for managed seasons the count comes from
   `season_teams` rows only. Legacy seasons fall back to all league teams when
   no `season_teams` rows exist. An even count rejects the request.
2. **League membership** — the requested team must belong to this season's league.
3. **Season membership** (managed only) — the team must also appear in
   `season_teams`. A league member that is not registered in the season is
   rejected (400) with "team is not registered in this season".

## Deferred Enhancements

### SEASONS-TODO-002 - Default lineup setup

**Status:** `deferred`

Operators may eventually want to set default lineups during season creation or
immediately after creating a season. This should remain an optional setup aid,
not a Close Week requirement, because next-week lineup readiness is
informational and last-minute substitutions are common.

### SEASONS-TODO-001 — Team-selection UI

**Status:** `deferred`

The backend API for season teams and rosters is implemented. The frontend
team-selection UI step has not been built. Currently operators must use the
backend APIs or a future admin screen to register teams before activation.

Do not build the frontend team-selection workflow until the backend APIs are
confirmed stable in production.
