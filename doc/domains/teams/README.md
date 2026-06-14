# Teams

## Overview

**Owner:** `teams`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

Teams belong to a league. Their participation and player roster are selected
for each season rather than permanently stored on the team or player.

## Season Participation

Team participation is explicit and per-season. Before a draft season can
activate, its participating teams must be registered in `season_teams`.

Two tables implement explicit participation:

- **`season_teams`** — one row per (season, team). Stores a season-specific
  name snapshot (`season_name`) and the team's captain for that season
  (`captain_id`, must reference a player on the season roster).
- **`season_rosters`** — one row per (season, player). UNIQUE on
  `(season_id, player_id)` enforces one team per player per season.

The setup checklist (`GET /api/seasons/{id}/checklist`) verifies:
- At least 2 teams in season_teams
- Each team has ≥ 1 player on season roster
- Each team has a captain assigned and that captain is on the roster
- A schedule has been generated (not stale)

Seasons with `teams_managed=0` (the `DEFAULT` for rows created before Phase
One) are automatically exempt from checklist enforcement (legacy bypass).
New seasons created via the API receive `teams_managed=1` and are subject to
all checklist rules, including `TEAMS_TOO_FEW` when the season has no teams.

### Adding Teams to a Season

`POST /api/seasons/{id}/teams` accepts two mutually exclusive paths:

- `name` — creates a brand-new team in the league and registers it. This is
  the only path allowed for a managed season's first team addition.
- `from_team_id` + `from_season_id` — registers an existing team and copies
  its roster from the specified prior season. For managed seasons both fields
  are required; `from_season_id` must equal the immediately previous season
  (as returned by `PreviousSeason`). If the previous season was also managed,
  the team must appear in that season's `season_teams`.

**Legacy seasons** (`teams_managed=0`): `from_team_id` without `from_season_id`
is still accepted and falls back to copying `players WHERE team_id=?`.

Modifying teams marks `schedule_stale = 1` on the season when unplayed
matches already exist.

## Player Assignment

A player may have one home team per season. Match-level substitute
participation does not change the player's home team.

When assigning an existing player to a team via `PUT /api/players/{id}`, the
request body must include all fields the handler reads (`first_name`,
`last_name`, `phone`, `email`, `handicap`, `admin_hold`, `team_id`). Omitting
name fields causes them to be blanked — the API performs a full replacement,
not a patch. The frontend `confirmAssign` function sends the full player record
to prevent accidental data loss.

## Decision History

### 2026-06-08 - Make membership season-specific

**Status:** `accepted`

Explicit season participation supports teams sitting out and players changing
teams between seasons without rewriting history.
