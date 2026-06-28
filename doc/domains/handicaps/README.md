# Handicaps

## Overview

**Owner:** `handicaps`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-27`

The Handicaps domain owns the opponent-normalized rack formula, the read-only
Handicap Review endpoint, and the pure-Go calculation package. It does not
own rule definitions (see `rules` domain) or score storage (see `matches` domain).

## Public Interface

```text
backend/domains/handicaps/public.go
```

No DB access in this package. Pure calculation only.

## Opponent-Normalized Rack Formula (Phase 3E)

### Design rationale

The prior `game_diff_average` formula averaged `match_results.diff` (total
pairing margin per match night). That approach conflated sample quality with
sample quantity: a player facing a weak opponent produced an artificially low
implied handicap.

The opponent-normalized formula corrects for opponent strength by computing
an implied handicap **per rack** (individual game slot), using the opponent's
snapshot handicap as the baseline.

### Formula

For each eligible rack played by the reviewed player:

```
per_rack = opponent_hc + rack_diff / 0.85
```

where:
- `opponent_hc`: the opponent's handicap at the time the match was saved
  (`home_handicap_used` when the reviewed player was AWAY;
   `away_handicap_used` when the reviewed player was HOME)
- `rack_diff`: reviewed player's game score minus opponent's game score
  (positive = player won the game, negative = player lost)
- `0.85`: per-game handicap factor from the FileMaker formula

The implied handicap is the arithmetic mean of all per-rack values, rounded
to the nearest 0.01:

```
implied = math.Round(sum(per_rack) / N * 100) / 100
```

Full float64 precision is retained during accumulation; rounding is applied
only to the final average.

### Lifecycle of a rack sample

A game slot is **score-eligible** when exactly one player scores 10 (the
winner) and the other scores 0-7. Incomplete or impossible scores are excluded.

A score-eligible rack is **snapshot-eligible** (included in calculation) when
the opponent's `home_handicap_used` or `away_handicap_used` column is non-NULL.
Racks with a NULL opponent snapshot are counted in `missing_snapshot_racks` and
excluded from `included_racks`; they do **not** count toward the eligibility
threshold.

A rack is **ordered** most-recent-first: matches ordered by `match_date DESC,
match_id DESC`; within a row, game slots iterated as game3, game2, game1.
This stable ordering ensures the window slice always covers the most recent
`window_size` eligible racks.

### Window vs. lifetime

| Field | Description |
|-------|-------------|
| `lifetime_racks` | All included racks across all eligible matches |
| `lifetime_hc` | Implied handicap across all lifetime racks |
| `window_racks` | `min(lifetime_racks, window_size)` most recent racks |
| `window_hc` | Implied handicap across the window slice |

`lifetime_hc` is always populated when `included_racks > 0`, regardless of
admin hold or threshold status. This allows non-actionable players to show
provisional calculations as context for the operator.

### Eligibility threshold

A recommendation is only generated (`recommended_hc` non-nil) when
`window_racks >= eligibility_threshold`. Players below the threshold have
`reason = "below_threshold"` and nil `recommended_hc`/`change_amount`.

Both `window_size` and `eligibility_threshold` are read from season rules
(`handicap_current_game_window` and `handicap_min_games_for_recommendation`).
Missing or blank stored values default to 15 with no error. A stored
zero, negative, or non-integer value returns HTTP 500 so the invalid
configuration can be corrected via the Rules tab.

### Reason priority

| Priority | Code | Condition |
|----------|------|-----------|
| 1 | `no_data` | `included_racks == 0` |
| 2 | `admin_hold` | `players.admin_hold = 1` |
| 3 | `below_threshold` | `window_racks < threshold` |
| 4 | `capped` | `window_hc` exceeded `max_individual_handicap` |
| 5 | `no_change` | `recommended_hc == assigned_hc` (normalized to 0.01) |
| 6 | `""` | Normal recommendation |

### Roster source

Review population: players registered in `season_rosters` with their team in
`season_teams` for the target season. `season_teams.season_name` is used as
the display team name (season snapshot, not the permanent `teams.name`).

Historical rack data is cross-league and cross-season by `players.id`. A player
who moved teams or appeared in a prior season contributes those racks to their
lifetime calculation. Only 8-ball (`leagues.game_format = '8ball'`) matches with
`completed = 1 AND week_closed = 1` are included.

## Pure-Go Package (`backend/domains/handicaps`)

```go
type RackSample struct {
    OpponentHC float64
    RackDiff   float64
}

type CalcResult struct {
    LifetimeImplied float64
    LifetimeRacks   int
    WindowImplied   float64
    WindowRacks     int
}

func ComputeImpliedHandicap(samples []RackSample, window int) CalcResult
```

`samples` is ordered most-recent-first by the caller (handlers/api.go).
`ComputeImpliedHandicap` takes the first `min(len, window)` samples as the
window slice; the caller does not need to pre-slice.

Returns zero-value `CalcResult` when `samples` is nil or empty.

## Handicap Review Endpoint

```
GET /api/seasons/{id}/handicap-recommendations
```

Read-only. No writes to `players.handicap`, `handicap_history`, or any other
table. Recommendations recompute live on every request.

### Response shape

```json
{
  "season_id": 1,
  "method": "game_diff_average",
  "status": "preview",
  "message": "2 players have recommended handicap changes (not yet applied).",
  "weeks_closed": 3,
  "recommendations": [
    {
      "player_id": 12,
      "player_name": "John Smith",
      "team_name": "Rack City",
      "admin_hold": false,
      "assigned_hc": 1.5,
      "score_eligible_racks": 15,
      "missing_snapshot_racks": 2,
      "included_racks": 13,
      "window_size": 15,
      "eligibility_threshold": 15,
      "lifetime_hc": 2.47,
      "lifetime_racks": 13,
      "window_hc": 2.47,
      "window_racks": 13,
      "recommended_hc": 2.47,
      "change_amount": 0.97,
      "reason": ""
    }
  ]
}
```

`recommended_hc` and `change_amount` are `null` for non-actionable players
(`no_data`, `admin_hold`, `below_threshold`). `lifetime_hc` and `window_hc`
are `null` only when `included_racks == 0`.

### Method routing

| `handicap_update_method` | Status | Recommendations |
|--------------------------|--------|-----------------|
| `manual_review` (default) | `no_auto_apply` | empty array |
| `game_diff_average` | `preview` | full array |
| `kicker_average_preview` | `unsupported` | empty array |

### Error behavior

- Season not found: HTTP 404
- Invalid rule value (`window` or `threshold` stored as 0 or non-integer): HTTP 500
- Real DB query failure: HTTP 500

Empty recommendations are never returned to mask errors.

## Snapshot Preservation (Phase 3E)

See `doc/domains/matches/README.md` Phase 3E for the `saveRounds` snapshot
preservation implementation. In summary: re-saving a scoresheet with the same
player on a side preserves that player's prior `home_handicap_used` or
`away_handicap_used`. A substituted player receives a fresh snapshot from
`players.handicap` at save time. Legacy NULL snapshots are replaced with a
fresh baseline on the next save.

## Decision History

### 2026-06-27 - Opponent-normalized rack formula replaces game_diff_average

**Status:** `accepted`

The `game_diff_average` formula (averaging `match_results.diff`) gave misleading
recommendations when a player's opponents were consistently above or below their
skill level. The opponent-normalized per-rack formula removes that bias by
treating each game slot as an independent estimate of the player's implied
handicap, adjusted for the opponent's strength.

The prior formula remains in `computeGameDiffAverageRecs` (used by the
advance-preview / close-week path via `buildAdvanceResult`). The Handicap Review
screen (`getHandicapRecommendations`) uses the new formula exclusively.

### 2026-06-27 - Excluded racks do not count toward threshold

**Status:** `accepted`

NULL-snapshot racks are excluded from both the calculation and the threshold
count. Counting them toward the threshold would give false confidence: a player
could appear eligible but have no calculable value if all their racks lacked
opponent snapshots.
