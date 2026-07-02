# Seasons

## Overview

**Owner:** `seasons`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-17`

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

### SEASONS-TODO-001 — Team-selection UI

**Status:** `deferred`

The backend API for season teams and rosters is implemented. The frontend
team-selection UI step has not been built. Currently operators must use the
backend APIs or a future admin screen to register teams before activation.

Do not build the frontend team-selection workflow until the backend APIs are
confirmed stable in production.
