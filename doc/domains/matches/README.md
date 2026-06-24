# Matches

## Overview

**Owner:** `matches`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-06-08`

The Matches domain owns match participation, result entry, finalization,
reopening, corrections, and match-level workflow status.

## Scoresheet Entry UI (Current State)

The current scoresheet is a **frontend-calculated** entry screen. Handicap application and pairing outcomes are computed in the browser using `web/app.js`. Backend stores raw round data only. All calculations described below are draft/frontend-only until a backend Close Week validation pass is implemented.

### Numeric score inputs

Each game slot has two numeric inputs: one for the home player and one for the visiting player. Inputs accept values 0-10.

- All inputs are **normalized to 0-10 immediately** on change (`normalizeScoreInput`): values above 10 are clamped to 10, negative values to 0, non-numeric entries to blank. The input element is updated in place so the visible value always matches what will be saved.
- Enter **10** in a player's cell to mark that player as the game winner (10 points = 7 object balls + 8-ball).
- Once a winner is known, the loser's stored value is further **clamped to 0-7** and written back to the input.
- If both cells show 10, the **last-edited side** wins; the other input is immediately set to 0.
- Tab order within a round: H G1 -> V G1 -> H G2 -> V G2 -> H G3 -> V G3, then next round.

### Pairing winner determination

A pairing winner is declared once the opponent **cannot catch up** even if they win every remaining game for maximum points (10 per game). The leader does not win early just by being ahead; the math must confirm the opponent's maximum possible final adjusted score is still lower.

**Early-stop rule (fewer than 3 games entered):**

```
home wins early  if  adjH > adjA + (remaining * 10)
away wins early  if  adjA > adjH + (remaining * 10)
```

where `remaining = 3 - games_played`, and `adjusted = raw score + ball HC spot` (if applicable).

**Full-completion rule (all 3 games entered, remaining = 0):**

1. Higher adjusted score wins.
2. If adjusted scores are tied, more games won in the pairing wins.
3. If both are tied, no winner (true mathematical tie).

**Examples:**

| Situation | adjH | adjA | remaining | Result |
|-----------|------|------|-----------|--------|
| H wins G1 10-0, no HC | 10 | 0 | 2 | No winner (V can still score 20) |
| H wins G1+G2 10-0, 10-0, no HC | 20 | 0 | 1 | H wins (V can score only 10) |
| H leads adjusted 21-5 after 2 games | 21 | 5 | 1 | H wins (V max = 15 < 21) |
| H leads adjusted 18-10 after 2 games | 18 | 10 | 1 | No winner (V max = 20 > 18) |

**Handicap alone never determines a winner.** If no games have been entered for a pairing, the winner is `''` regardless of handicap difference. The `hasScore` guard (`g1w`, `g2w`, or `g3w` non-empty) is required before any winner logic runs.

### Ball HC column

The Ball HC column appears on the scoring table between Rating and Adj Score. It spans both rows (home and visiting) for a pairing and displays the computed spot as a plain integer:

- `0` -- no spot (equal ratings, or computed spot suppressed by `min_ball_handicap` threshold)
- `N` (e.g. `2`, `5`) -- N balls spotted to the lower-rated player; the direction (home vs. visitor) is shown in the Adj Score column via the `ss-adj-win` highlight, not in this column

The column is populated immediately on render from player ratings, before any game scores are entered.

**Handicap calculation is frontend-only (draft debt).** The formula reads `handicap_multiplier` and `min_ball_handicap` from `scoresheetSeasonRules` (fetched at match-entry load time from `/api/seasons/{id}/rules`). The `min_ball_handicap` rule is a cutoff: a computed spot below the threshold is treated as 0, not raised to the threshold value. See `doc/domains/rules/README.md` for examples.

### Winner highlight in adjusted score

The adjusted score cell for the pairing winner receives the `ss-adj-win` CSS class, rendering it with a distinct background. The Ball HC column makes the applied spot visible, so no separate annotation appears in the winner cell.

### Page 2 -- Rounds Won

The scorekeeper summary page (page 2) shows Rounds Won for each team. A round is won by the team that first reaches 2 mathematically-determined pairing wins in that round. A pairing contributes once its winner is locked by the early-stop rule above; all 3 games in the pairing do not need to be finished, and all 3 pairings in the round do not need to be played.

If no scores have been entered anywhere on the sheet, the field shows a blank line. Once any score is entered, the live count is shown.

## Backend Scoresheet Validation

**Package:** `backend/domains/matches` -- `ValidateRounds`

Backend validation is now authoritative for 8-ball scoresheet round submissions. The
validator runs inside `saveRounds` before any DB write. It uses `backend/validation`
for structured result types.

**Frontend validation** (`web/app.js`) remains helper UX only -- it normalises inputs
and shows live pairing outcomes, but does not duplicate the backend validator.

### Behavior

- **Errors -> HTTP 422** with `{"messages": [...]}` body (see `validation.Result`). No rows are written.
- **Warnings -> save proceeds.** Warnings are computed and available for future Close Week
  use but are not currently returned to the frontend.
- Warning acknowledgment and Close Week finalization remain future work.

### Validation codes

| Code | Level | Condition |
|------|-------|-----------|
| `SCORESHEET_NO_SCORES` | warning | No game on the sheet has a winner |
| `SCORESHEET_GAME_BOTH_WINNERS` | error | Both home and away score 10 in one game |
| `SCORESHEET_GAME_SCORE_RANGE` | error | A score falls outside 0-10 |
| `SCORESHEET_LOSER_SCORE_RANGE` | error | Loser's score exceeds 7 when a winner exists |
| `SCORESHEET_GAME_INCOMPLETE` | warning | Non-zero scores but no declared winner |
| `SCORESHEET_PAIRING_UNDETERMINED` | -- | Reserved -- Close Week finalization |
| `SCORESHEET_ROUND_INCOMPLETE` | -- | Reserved -- Close Week finalization |

### Pairing winner determination (mirrors frontend early-stop)

- `hasScore` guard: handicap alone never determines a winner
- Early stop: `adjLead > adjTrail + remaining * 10`
- Full completion: higher adjusted score wins; games-won tiebreak if tied; no winner on true tie

### Round winner tracking

`ScoresheetResult.RoundWinners` maps round numbers to the winning side once a team
has 2 determined pairing wins in that round. Currently informational only.

## Score Entry And Workflow

Scores may be entered and saved before the league week closes. Entering scores
does not make their calculations official. The exact match status transition
after score entry remains open (see MATCHES-Q001).

Official handicap adjustments, match outcomes, standings, and player
statistics are applied when the admin successfully closes the week. Results
that have not passed week close do not contribute to official totals.

## Close Week -- Phase 1 (implemented 2026-06-21)

**Package:** `backend/domains/matches` -- `ValidateWeek`

The Close Week workflow is implemented in Phase 1 with the following scope.

### Schema

- `league_weeks` table: tracks per-week status (`open` | `closed`) per season.
  A row is created on first close; absence implies `open`.
- `matches.week_closed INTEGER NOT NULL DEFAULT 0`: set to 1 on all matches in a
  week when the week is officially closed. Standings filter on this column.

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/seasons/{id}/weeks` | List all weeks with status, match counts |
| `GET` | `/api/seasons/{id}/weeks/{week}/validate` | Dry-run validation (no write) |
| `POST` | `/api/seasons/{id}/weeks/{week}/close` | Validate + commit close |

`validate` and `close` return the same `validation.Result` JSON body on errors.
`close` returns HTTP 422 when errors exist; 200 `{"closed":true}` on success.

### Standings Gate

`getStandings` filters on `completed=1 AND week_closed=1`. Matches with saved
scores that have not been through week close are excluded from official totals.

**Deploy note:** Existing seasons with saved scores will show empty standings
until their weeks are explicitly closed via the new API.

**Player stats gate:** `getPlayerStats` (season scope) applies the same filter --
only `match_results` from `completed=1 AND week_closed=1` matches count. The
league-scope stats path has no season concept and is unchanged.

### Phase 1 Validation Codes

| Code | Level | Condition |
|------|-------|-----------|
| `WEEK_MATCH_NO_SCORES` | error | No `round_results` row with a game winner (score of 10) |
| `WEEK_MATCH_UNASSIGNED` | error | `home_team_id` or `away_team_id` is NULL |
| `SCORESHEET_GAME_BOTH_WINNERS` | error | Re-run from `ValidateRounds` on saved data |
| `SCORESHEET_GAME_SCORE_RANGE` | error | Re-run from `ValidateRounds` on saved data |
| `SCORESHEET_LOSER_SCORE_RANGE` | error | Re-run from `ValidateRounds` on saved data |
| `SCORESHEET_NO_SCORES` | warning | Re-run from `ValidateRounds` on saved data |
| `SCORESHEET_GAME_INCOMPLETE` | warning | Re-run from `ValidateRounds` on saved data |

In Phase 1, warnings are surfaced in the UI but do not block close.

### Deferred (not in Phase 1)

- Warning acknowledgment storage and audited admin override (**implemented in Phase 2A**)
- Reopen workflow (`POST /api/seasons/{id}/weeks/{week}/reopen`) (Phase 2B)
- Handicap update suggestions at close time
- Duplicate player participation check (`WEEK_PLAYER_DUPLICATE`)
- `SCORESHEET_PAIRING_UNDETERMINED` and `SCORESHEET_ROUND_INCOMPLETE` codes
- `sets_won` / `sets_lost` population
- Match-level status codes (MATCHES-Q001)
- Audit log table

### UI Placement

Close Week controls appear in the Schedule tab, in each week's card header:
- **Closed** badge (green) for closed weeks
- **Open** badge (grey) + "Review & Close" button for open weeks
- The button opens a validation summary modal; confirm button is disabled
  when errors are present; warnings are shown but do not block confirm

## Close Week -- Phase 2A: Warning Acknowledgment (implemented 2026-06-21)

### MatchID on validation messages

`validation.Message` gained an optional `MatchID *int64` field (`json:"match_id,omitempty"`).
`ValidateWeek` stamps `MatchID` on every message it emits, including messages forwarded
from `ValidateRounds`. The compound key `(match_id, warning_code, field)` uniquely
identifies a warning for acknowledgment purposes.

`ValidateRounds` used directly by `saveRounds` does not set `MatchID` (nil). Existing
callers are unaffected; the field serializes with `omitempty`.

### Acknowledgment gate

POST `/close` now accepts an optional request body:

```json
{
  "acknowledgments": [
    {
      "match_id":     5,
      "warning_code": "SCORESHEET_GAME_INCOMPLETE",
      "field":        "rounds[0].game2",
      "notes":        "Optional free-text note"
    }
  ]
}
```

Behavior:
- Error-level messages still block close exactly as Phase 1.
- The close handler re-runs `ValidateWeek` before writing. The current warning
  set at close time may differ from the set shown to the user at validate time.
- Every current warning must be acknowledged. The match is: `(match_id, warning_code, field)` all equal.
- Stale/extra acknowledgments (no matching current warning) are silently ignored.
- If any current warning is unacknowledged, close returns HTTP 422 with the unacknowledged
  warnings promoted to error-level messages.
- Missing body and empty `acknowledgments` array are equivalent (no acks submitted).
- When no warnings exist, a missing body still succeeds (Phase 1 behavior unchanged).

### Acknowledgment storage

Each acknowledged warning is stored as one row in `week_close_acknowledgments` within
the same transaction as the week close.

| Column | Notes |
|--------|-------|
| `season_id` | Foreign key to seasons; ON DELETE CASCADE |
| `week_number` | Matches the closed week |
| `match_id` | Foreign key to matches; ON DELETE SET NULL (history survives match deletion) |
| `warning_code` | The warning code (e.g. `SCORESHEET_GAME_INCOMPLETE`) |
| `field` | The warning field; empty string for non-field warnings |
| `notes` | Optional free-text note from admin; empty string if none |
| `acknowledged_at` | Timestamp set by database default |

Rows from prior close operations are retained across reopens. A new set of rows
is inserted on each re-close.

### Deferred (not in Phase 2A)

- Actor/user identity on acknowledgments (deferred to auth phase)
- Required controlled reason codes (deferred to CODES-Q001)
- Warning invalidation history on reopen
- Reopen workflow (**implemented in Phase 2B**)
- Audit log module
- `sets_won` / `sets_lost` population
- Handicap update suggestions

### UI behavior

The Review & Close modal gains acknowledgment checkboxes when warnings are present
and no errors block close:

- One checkbox and optional notes input per warning
- Match badge, code, field, and message text are shown per warning
- Confirm Close is disabled until every checkbox is checked
- On confirm, all acknowledgments are collected and sent in the POST body
- If the backend returns 422 (stale/missing acks), a toast shows the error messages

When errors are present, warnings are shown as read-only context (no checkboxes).
When no warnings exist, behavior is unchanged from Phase 1.

## Close Week -- Phase 2B: Reopen Closed Week (implemented 2026-06-21)

### Endpoint

`POST /api/seasons/{id}/weeks/{week}/reopen`

### Behavior

- Requires the week to be currently closed (`league_weeks.status = 'closed'`). Returns
  HTTP 409 Conflict if the week is open or has no `league_weeks` row.
- Requires at least one match to exist for the season and week number. Returns HTTP 404
  if no matches are found (the week does not exist as a schedulable entity).
- Within a single transaction:
  - Sets `league_weeks.status = 'open'` and `league_weeks.closed_at = NULL`.
  - Sets `matches.week_closed = 0` for all matches in the season/week.
- Returns HTTP 200 `{"reopened": true, "week_number": <n>}` on success.

### Data preserved

- `round_results` rows are not touched.
- `match_results` rows are not touched.
- `week_close_acknowledgments` rows from prior close operations are retained.
  A new set of acknowledgment rows is inserted on the next re-close.

### Standings and player stats impact

Both `getStandings` and `getPlayerStats` (season scope) filter on
`completed=1 AND week_closed=1`. Setting `week_closed=0` on reopen immediately
excludes the week's matches from official standings and player stats without any
additional query changes.

### UI behavior

Closed week cards in the Schedule tab show a yellow **Reopen** button in place of the
**Review & Close** button. Clicking Reopen opens a confirmation modal with the message:

> This week will be removed from standings until it is closed again. Saved scores will remain.

On successful reopen:
- The schedule refreshes (week card shows Open badge + Review & Close button).
- Standings refresh.
- Player stats refresh (if a season is selected).
- A success toast is shown.

On failure, the backend error message is shown in a danger toast.

### Deferred (not in Phase 2B)

- `reopen_count` / `last_reopened_at` tracking on `league_weeks`
- Warning invalidation history (clearing stale acks on reopen)
- Actor/user identity on reopen
- Audit log entry for reopen operations
- Per-match selective reopen (currently reopens the whole week)

## Close Week -- Phase 2D: Sets, Validation, and Navigation (implemented 2026-06-23)

### sets_won / sets_lost in saveRounds

`saveRounds` now populates `sets_won` and `sets_lost` in `match_results` using
`ScoresheetResult.RoundWinners` returned by `ValidateRounds`.

- A player on the winning side of a round gets `sets_won += 1`.
- A player on the losing side of a round gets `sets_lost += 1`.
- A "round winner" requires the team to win 2 or more pairings in that round (`roundHomeWins[rn] >= 2` or `roundAwayWins[rn] >= 2`).
- Rounds with no determined winner (e.g. 1-1 pairing split or undetermined pairings) contribute 0 sets to either side.
- `saveRounds` already deletes and re-inserts `match_results` on every save; sets are recomputed automatically on resave.
- The `week_closed=1` gate on `getPlayerStats` ensures sets do not appear in official stats until after Close Week.
- **No schema change.** `sets_won` and `sets_lost` columns exist in `match_results` and were previously always written as 0 by this path.
- **No backfill.** Existing rows only update when the match is re-saved.

### WEEK_PLAYER_DUPLICATE validation

`ValidateWeek` now detects when a player appears more than once in a single round
within the same match. This is an **error** that blocks close.

**Code:** `WEEK_PLAYER_DUPLICATE`

**Trigger:** For each round number in a match's `round_results`, a player ID must
appear at most once across all home and away player slots. If any player ID is seen
twice in the same round, the error is emitted for that match and the match is
skipped for further validation.

The `UNIQUE(match_id, round_number, home_player_id)` DB constraint prevents a player
from appearing as HomePlayerID twice in the same round but does not prevent a player
from appearing as both HomePlayerID in one pairing and AwayPlayerID in another pairing
of the same round. `WEEK_PLAYER_DUPLICATE` catches this case.

### Schedule-to-match-entry navigation

Open-week match rows in the Schedule tab now show a **Score Entry** button alongside
the existing Assign button.

- Clicking **Score Entry** hides any open modal, pre-selects the match in the Match
  Entry tab, and navigates there directly.
- The button is not shown on closed-week match rows (the backend blocks saves on closed
  weeks regardless).
- In the Review & Close modal, per-match error group headers display the Match badge as
  a clickable button. Clicking it dismisses the modal and opens Match Entry for that match.
- Navigation is wired via `data-action="open-match-entry"` delegation; no inline event
  attributes are used for the new buttons.

### Deferred (not in Phase 2D)

- `SCORESHEET_PAIRING_UNDETERMINED` - valid outcome; design decision pending
- `SCORESHEET_ROUND_INCOMPLETE` - definition of "incomplete" vs legal 1-1-1 split pending
- Audit log module, actor identity, reopen reason codes

## Close Week -- Phase 2E: Acknowledgment History Visibility (implemented 2026-06-23)

### Goal

Surface prior Close Week warning acknowledgments to authorized admins without
building the full application-wide audit module. Resolves MATCHES-Q003.

### New endpoint

`GET /api/seasons/{id}/weeks/{week}/acknowledgments`

- Returns all `week_close_acknowledgments` rows for the season/week, ordered
  by `acknowledged_at DESC`.
- Returns `[]` (empty array) when the week exists but has no acknowledgments.
- Returns 404 when no matches exist for the season/week.
- No paging in this phase; operational volumes are small.

Response shape:

```json
[
  {
    "id": 12,
    "season_id": 3,
    "week_number": 2,
    "match_id": 7,
    "warning_code": "SCORESHEET_GAME_INCOMPLETE",
    "field": "rounds[1].game3",
    "notes": "Admin note",
    "acknowledged_at": "2026-06-23 10:30:00"
  }
]
```

`match_id`, `field`, and `notes` are omitted from the response when empty/null.

### `ack_count` on WeekSummary

`GET /api/seasons/{id}/weeks` now includes `ack_count` per week. This is the
total number of acknowledgment rows ever written for that season/week (accumulated
across all close cycles). It remains > 0 after reopen because rows are never
deleted.

`ack_count` is 0 for weeks that were closed cleanly with no warnings.

### Schedule card history indicator

When `ack_count > 0` for a week (open or closed), the schedule card header
shows a small "N prior acks" toggle button. Clicking it fetches the new endpoint
on first expand and renders a compact list of ack rows inline under the match
table. Subsequent clicks toggle without re-fetching.

The indicator appears on both open and closed weeks. On an open week with
`ack_count > 0`, the acks are historical (from a previous close cycle).

### Review & Close modal prior history notice

When `reviewCloseWeek` opens for a week whose `ack_count > 0` (i.e. the week
was previously closed and has been reopened), a collapsible notice appears at
the top of the modal body, before current errors/warnings. The notice shows
the count and a "View" button that loads the ack rows inline.

If `ack_count === 0`, the modal behavior is unchanged.

### Files changed

- `models/models.go` -- `WeekSummary.AckCount int`; new `CloseAck` struct
- `handlers/api.go` -- `listWeeks` ack count aggregate; `getWeekAcknowledgments`
  handler; route registration
- `handlers/api_test.go` -- 6 new Phase 2E tests
- `web/app.js` -- `loadWeekAcknowledgments`; schedule card ack toggle;
  Review & Close converted to data-action delegation with `data-ack-count`;
  prior history notice in close modal

### Not in Phase 2E

- Actor/user identity on acknowledgment rows
- `reopen_count` / `last_reopened_at` on `league_weeks`
- Controlled reopen reason codes
- Global audit log table or audit module
- Grouping acknowledgments by close cycle
- `SCORESHEET_PAIRING_UNDETERMINED` / `SCORESHEET_ROUND_INCOMPLETE` codes

## Close Week -- Phase 3A: Advance Week Preview (implemented 2026-06-23)

### Goal

Show what advancing the week would mean -- close readiness, next week
readiness, and handicap update status -- without modifying any data.

### New endpoint

`GET /api/seasons/{id}/weeks/{week}/advance-preview`

- Read-only; no rows are inserted, updated, or deleted.
- Returns 404 when no matches exist for the season/week.
- Returns 200 with a preview object even when the week has validation errors.

Response shape:

```json
{
  "season_id": 3,
  "week_number": 2,
  "can_close": true,
  "validation_messages": [...],
  "current_week": {
    "match_count": 3,
    "completed_count": 3,
    "closed_count": 0,
    "status": "open"
  },
  "next_week_number": 3,
  "next_week": {
    "match_count": 3,
    "assigned_count": 2,
    "unassigned_count": 1,
    "lineup_plan_count": 4,
    "missing_lineup_team_ids": [7]
  },
  "handicap": {
    "method": "manual_review",
    "status": "preview_only",
    "message": "No handicap changes are applied automatically. Phase 3A preview is read-only."
  }
}
```

`next_week_number` and `next_week` are omitted when no further weeks are
scheduled. `validation_messages` mirrors `validation.Result.Messages`.
Use `can_close` to determine close eligibility without parsing the list.

### Review & Close modal Advance Preview section

`reviewCloseWeek` fetches the validate and advance-preview endpoints in
parallel (`Promise.all`). A compact "Advance Preview" table is appended to
the modal body showing:

- **This week** -- scored matches / total and a Ready / Has errors badge
- **Next week** -- match count, unassigned slots, lineup plan status
- **Handicap** -- read-only status message

The section is always shown when the endpoint succeeds. If the endpoint
fails (e.g. network error), the section is silently omitted. The existing
close / warning acknowledgment flow is unchanged.

### Not in Phase 3A

- Automatic handicap writes
- Blank `round_results` creation
- `lineup_plans` creation or modification
- Changes to the Close Week transaction
- Audit tables
- Reopen count or last-reopened tracking

### Files changed

- `models/models.go` -- `AdvancePreview`, `AdvancePreviewMessage`,
  `AdvancePreviewWeekSummary`, `AdvancePreviewNextWeek`, `AdvancePreviewHandicap`
- `handlers/api.go` -- `getAdvancePreview` handler; route registration
- `handlers/api_test.go` -- 6 Phase 3A tests
- `web/app.js` -- `_renderAdvancePreview` helper; `reviewCloseWeek` uses
  `Promise.all` and appends advance preview section to modal body

## Close Week Validation (full target -- future phases)

The backend validates the week's score data before official calculations are
committed. Validation includes:

- Missing scores or players
- Impossible scoring combinations
- Duplicate player participation
- Incomplete player profiles
- Handicap or input inconsistencies
- Unresolved matches
- Format-specific scoring errors

Validation results have two severities:

- **Error:** blocks week close and cannot be overridden.
- **Warning:** may allow close only after explicit admin acknowledgment.

Every warning acknowledgment records the warning details, affected records,
admin identity, controlled reason code, optional `notes`, and timestamp in the
shared audit log. Transparency is the default.

## Corrections

An admin reopens the containing week and selects only the affected matches.
Unaffected finalized matches remain locked. Corrected matches are finalized and
the week is closed again.

All corrections record old values, new values, actor, reason, and timestamp in
the shared audit log.

## Questions

### MATCHES-Q001 - Status after score entry

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Scores are entered before week close, but additional calculations
and validation still need to occur.

**Resolution:** Decide whether completed score entry creates a review status,
remains draft, or uses another controlled status.

### MATCHES-Q002 - Online score entry

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Online entry affects drafts, permissions, competing edits,
validation, approval, and the Close Week preview.

**Resolution:** Design the online score-entry workflow before finalizing match
statuses or calculation-preview behavior.

### MATCHES-Q003 - Historical warning display

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-06-23`
**Related commit:** `Phase 2E`

**Context:** Warning acknowledgments are audited, but their placement on
historical match and week screens is not decided.

**Resolution:** Phase 2E. Acknowledgments are read back via
`GET /api/seasons/{id}/weeks/{week}/acknowledgments`. The schedule card shows
a count badge and expandable history section when `ack_count > 0`. The Review
& Close modal shows a prior history notice when re-closing a reopened week.
No actor identity or audit module is required at this level.

## Decision History

### 2026-06-08 - Make week close authoritative

**Status:** `accepted`

Score entry stores pending data. Official calculations and result effects are
committed only after backend Close Week validation succeeds.

### 2026-06-08 - Require transparent warning acknowledgment

**Status:** `accepted`

Errors block close. Warnings require explicit, reasoned, audited admin
acknowledgment.
