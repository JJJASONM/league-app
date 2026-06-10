# Teams

## Overview

**Owner:** `teams`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

Teams belong to a league. Their participation and player roster are selected
for each season rather than permanently stored on the team or player.

## Season Participation

When an admin creates a season, all active league teams are selected by
default. The admin may remove teams or create new teams before activation.

Target relationships:

```text
season_teams
season_rosters
```

These are conceptual and not implemented. The current database infers season
participation from matches and stores a single `players.team_id`.

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
