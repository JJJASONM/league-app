# Leagues

## Overview

**Owner:** `leagues`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-09`

A league is the top-level container. It owns teams, seasons, and players.
Each league represents one night of play, one format, and one scheduling cadence.

## Structure

| Field | Notes |
|-------|-------|
| `name` | Display name — must be unique enough for operator clarity |
| `game_format` | `8ball` \| `9ball` \| `10ball` \| `straight` — drives handicap and scoring rules |
| `day_of_week` | Optional — recorded for reference; not used in scheduling logic today |

Players and teams belong to exactly one league. A player cannot play for teams
in two different leagues without two separate player records.

## Team-Count Verification Checklist

The Manage Leagues modal displays a per-league checklist to surface common
setup gaps before schedule generation:

| Check | Condition |
|-------|-----------|
| At least 2 teams configured | `team_count >= 2` |
| Odd team count for natural bye rotation | `team_count > 0 && team_count % 2 == 1` |
| Review teams and rosters | Advisory — always shown as a reminder |

Team counts are fetched via `GET /api/teams?league_id={id}` on every open,
add, and delete so the display stays current without reopening the modal.
A newly created league shows 0 teams immediately.

The checklist uses a green check-circle for satisfied conditions and a muted
empty circle for unsatisfied ones. The team-count table badge is green when
`team_count >= 2` and amber otherwise.

## Decision History

### 2026-06-09 - Refresh checklist on every add/delete

**Status:** `accepted`

`addLeague` and `deleteLeague` call the shared `refreshLeaguesTable()` helper
after mutating the list. This keeps team-count badges and checklist counts
accurate without requiring the modal to be reopened. The alternative (caching
the previous count map and patching it locally) was rejected as error-prone.
