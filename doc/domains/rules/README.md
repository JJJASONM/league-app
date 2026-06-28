# Rules

## Overview

**Owner:** `rules`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

The Rules domain defines configurable league behavior, validates editable rule
values, resolves inherited values, and creates a locked season snapshot.
Handicaps, schedules, and matches apply those values within their own domains.

## Public Interface

Target entry points:

```text
web/domains/rules/index.js
backend/domains/rules/public.go
```

The backend returns rule definitions, editable values, validation errors, and
effective values with their source scope.

## Rule Definition

Each developer-defined rule has:

- Permanent key
- Editable label
- Explicit type
- Validation metadata
- Default value
- Status and version
- What/why/input/output explanation

Rule keys cannot be changed after creation. Labels may change because business
logic uses keys, not labels.

## Scope And Snapshot

```text
system default -> league -> season
```

The most specific configured value wins. The API identifies the source of the
effective value. Season creation snapshots effective values; season activation
locks the snapshot.

## Developer-Defined Rules Reference

### min_ball_handicap

Key: `min_ball_handicap` | Type: integer, min 0, default 0 | Group: Handicap Settings

**Status:** draft — frontend-enforced only (scoresheet calculation)

**Behavior:** threshold cutoff, not a floor.

- If computed spot (raw balls from multiplier formula) is below this value, **no spot is given** (0).
- If computed spot equals or exceeds this value, the computed spot is used unchanged.
- Equal-rated players always receive 0 regardless of this setting.
- Value 0 disables the threshold entirely (use computed spot only).

**Examples with min_ball_handicap = 2:**

| Computed spot | Result |
|---------------|--------|
| 0 (equal)     | 0 — equal-rated players always 0 |
| 1             | 0 — below threshold, no spot given |
| 2             | 2 — meets threshold, computed spot applies |
| 5             | 5 — above threshold, computed spot applies |

**Where applied:** `calcHandicap()` in `web/app.js`. Not yet backend-authoritative. Stored in `season_rules.rule_key = 'min_ball_handicap'` via the existing rules tab; read at match-entry time from `/api/seasons/{id}/rules`.

### handicap_current_game_window

Key: `handicap_current_game_window` | Type: integer, min 1, default 15 | Group: Handicap Settings | Order: 70

**Status:** draft: backend-enforced (read by `seasonHandicapWindowConfig` in `handlers/api.go`)

**Behavior:** Controls the rolling window for the opponent-normalized rack calculation.

- The most-recent `N` eligible racks (ordered by match date DESC, then game slot DESC) form the "current window".
- `window_hc` is the implied handicap across those `N` racks; `lifetime_hc` uses all racks regardless of this setting.
- When `lifetime_racks < window_size`, all lifetime racks are used for both values.
- Missing/blank stored value defaults to 15 with no error.
- Stored zero, negative, or non-integer value returns HTTP 500 from the recommendations endpoint.

**Where applied:** `GET /api/seasons/{id}/handicap-recommendations` via `seasonHandicapWindowConfig`.

### handicap_min_games_for_recommendation

Key: `handicap_min_games_for_recommendation` | Type: integer, min 1, default 15 | Group: Handicap Settings | Order: 80

**Status:** draft: backend-enforced (read by `seasonHandicapWindowConfig` in `handlers/api.go`)

**Behavior:** Minimum number of included racks required before a recommendation is generated.

- "Included racks" excludes racks with a NULL opponent snapshot; those racks are counted in `missing_snapshot_racks` and do **not** count toward this threshold.
- Players with `window_racks < threshold` receive `reason = "below_threshold"` and nil `recommended_hc`/`change_amount`.
- `lifetime_hc` and `window_hc` are still populated for below-threshold players when `included_racks > 0` (provisional context only).
- Missing/blank stored value defaults to 15 with no error.
- Stored zero, negative, or non-integer value returns HTTP 500 from the recommendations endpoint.

**Where applied:** `GET /api/seasons/{id}/handicap-recommendations` via `seasonHandicapWindowConfig`.

## Questions

### RULES-Q001 - Mid-season amendments

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Locations, emergencies, or mistakes may require an active season
to change.

**Resolution:** Define whether amendments create a dated version, require
special authorization, or apply only to unplayed matches.

## Decision History

### 2026-06-08 - Developer-owned definitions

**Status:** `accepted`

Developers control meaning, types, and validation. Authorized users edit
permitted values only.

### 2026-06-08 - Lock season snapshot at activation

**Status:** `accepted`

A season keeps a stable ruleset after approval. Amendment details remain open.
