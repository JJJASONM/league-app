# Players

## Overview

**Owner:** `players`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-07-14`

Players are shared system-wide identities. They are not owned by one league or
team and are separate from authenticated user accounts.

## Participation

A player may have one home team in a season and may substitute for any team in
any league or season. Match participation records the team represented and
whether the player was rostered or substituting.

The current `players.team_id` implementation cannot represent this target
behavior and will eventually be replaced by season roster relationships.

## Quick Add

Admins can quick-add a missing player from the Players page without entering a
full profile. The minimum required fields are at least one name (first or last)
and a diff rating (defaults to 0). Player number, phone, email, and admin hold
are optional and can be completed later using the standard Edit Player modal.

Before creating the player, quick-add warns when the typed name normalizes to
the same full name as an existing player in the active league (see Duplicate
Warning below). This is advisory only; it does not block creation.

Deferred: INCOMPLETE profile status, close-week blocking for incomplete
profiles, and match-entry quick-add integration are not yet implemented.

## Duplicate Warning (Phase A, 2026-08-19)

Quick-add compares the typed first/last name against the already-loaded,
league-scoped player list on the Players page. Both the typed name and each
existing player's name are normalized (trimmed, internal whitespace collapsed,
case-folded) before comparison; a match is exact-normalized only, not fuzzy.

When a match is found, the admin sees the existing player's name and team (or
"no team" if unrostered) and can either cancel or continue and add the new
player anyway. No match means quick-add proceeds exactly as before this phase.

This is entirely client-side (`web/domains/players/players-page-component.js`):
no backend validation, DB constraint, or API change was made. Two different
real people who share a name will still see the warning every time they are
quick-added -- that is expected, not a bug, given the warn-only design.

## Safe Merge (Backend Phase A, 2026-08-19)

Admins can merge a duplicate player record into a surviving one:
`POST /api/players/{source_id}/merge` with body `{"target_id": <id>}`. Backend
and store only in this phase -- there is no merge UI yet.

Every supported reference to `source_id` is repointed to `target_id`, then the
source player row is deleted, all inside one transaction (all-or-nothing;
any failure rolls back everything):

- `match_results.player_id`
- `handicap_history.player_id`
- `round_results.home_player_id` and `round_results.away_player_id`
- `lineup_plans.player_id` and `lineup_plans.sub_for_id`
- `season_teams.captain_id`
- `season_rosters.player_id`
- `teams.captain_id`

`round_results`' handicap snapshot columns (`home_handicap_used`,
`away_handicap_used`, `handicap_pts_used`, `handicap_to`) and all game-score
columns are untouched by a merge -- only the player-ID columns move.
`handicap_history` rows from both players survive the merge and are
repointed, not rewritten or deduplicated; a merged player's history is the
union of both players' prior rows.

The merge is refused, with nothing changed, in three ways:

- **400 Bad Request** -- `source_id == target_id`.
- **404 Not Found** -- source or target player does not exist.
- **409 Conflict** -- an unsafe collision blocks the merge:
  - Source and target already both have a `season_rosters` row for the same
    season, regardless of team -- this is the general form of "source and
    target are rostered on different teams in the same season," since
    `season_rosters` has no `team_id` in its unique key.
  - Source and target already played against each other (one home, one away)
    in the same `round_results` row -- merging would make a player play
    themselves. Discovered while implementing; not in the original blocker
    list. Checked ahead of the broader participation check below so this
    more specific error takes priority when both would otherwise apply.
  - Source and target already both participate (as home player, away
    player, or both, in any row -- not necessarily the same row) in the
    same `(match_id, round_number)` of `round_results`. Covers both as
    home, both as away, and one as home in one row while the other is away
    in a different row of the same match/round -- any of these would make
    target appear more than once in one round after merging, the same
    condition Close Week rejects as `WEEK_PLAYER_DUPLICATE`. The narrower
    "both as home" case was the original implementation; broadened to full
    participation after review.
  - Source and target already both have a `lineup_plans` row for the same
    team/week/season (would violate its unique constraint). Discovered while
    implementing; not in the original blocker list.

The route is protected by the same personal-key admin mutation auth as the
rest of player CRUD (`league_admin`, `admin`, or `system_admin` role via
`clearanceAuth`).

## Deferred Player Maintenance

The following player-record maintenance items remain parked:

- Merge UI (preview, confirm) -- backend/store only so far.
- INCOMPLETE profile status and close-week blocking for incomplete profiles.
- Match-entry quick-add integration.

Duplicate detection for quick-added players and the safe merge backend are
both implemented as of the phases above; neither is in this deferred list
anymore.

## Questions

### PLAYERS-Q001 - Quick-add requirements

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-07-14`

**Context:** League night cannot stop for complete registration, but duplicate
players and unsupported handicap values must be avoided.

**Resolution (Phase 1):**
- Quick-add lives on the Players page as a simplified modal.
- Required fields: at least one of first name or last name; diff rating (default 0).
- Optional fields: team. Player number, phone, email, and admin hold are omitted
  from quick-add and completed later via Edit Player.
- Duplicate detection: deferred. No DB unique constraint on name or player number.
- INCOMPLETE profile status and close-week blocking: deferred to a later phase.
- Match-entry quick-add integration: deferred.

## Decision History

### 2026-06-08 - Share players across leagues

**Status:** `accepted`

One player record may participate in multiple nights and formats.

### 2026-06-08 - Allow any system player to substitute

**Status:** `accepted`

Substitute eligibility is not restricted to the same league or season.

### 2026-07-14 - Quick-add Phase 1: Players page only, name + handicap minimum

**Status:** `accepted`

Phase 1 quick-add lives on the Players page as a simplified modal. The minimum
field set is at least one name and a diff rating (default 0). Player number,
contact fields, and admin hold are omitted and completed later via Edit Player.
INCOMPLETE status, close-week blocking, and match-entry integration are
deferred. Resolves PLAYERS-Q001.

### 2026-08-19 - Quick-add duplicate warning Phase A

**Status:** `accepted`

Added a client-side, warn-only, normalized-exact-name duplicate check to
Players page quick-add, comparing against the already-loaded league-scoped
player list. No backend, schema, or API change. Safe merge remained deferred
at this point (see the Safe Merge Backend Phase A entry below); INCOMPLETE
profile status and match-entry quick-add remain deferred.

### 2026-08-19 - Safe merge backend Phase A

**Status:** `accepted`

Added `players.PlayerService.MergePlayers` / `PlayerStore.MergePlayers` and
`POST /api/players/{id}/merge`: repoints all nine supported player-ID
references across seven tables from a source (duplicate) player to a target
(surviving) player in one transaction, then deletes the source. Backend and
store only -- no merge UI in this phase. Statuses: 400 same player, 404
missing player, 409 for an unsafe collision (season-roster, round-results
participation, self-opponent, or lineup-plan). The round-results check
started as "both as home" only and was broadened during review to any
participation (home or away, any row) in the same match/round, since the
narrower check missed both-away and cross-role cases. The self-opponent,
round-participation, and lineup-plan blockers were all discovered during
implementation, not specified up front. Handicap snapshot columns and
handicap_history values are preserved exactly; only foreign keys move.
