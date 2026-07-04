# Matches

## Overview

**Owner:** `matches`
**Status:** `draft`
**Current version:** `0.4`
**Last reviewed:** `2026-07-03`

The Matches domain owns match participation, result entry, official week-close
effects, reopening, corrections, and match-level workflow status.

## Scoresheet Entry UI (Current State)

The current scoresheet is a browser-driven entry screen with backend-authoritative
save validation and Close Week workflow. The frontend still renders live pairing
math for operator feedback, but official standings and player stats come from
backend-controlled closed-week data.

Backend stores raw `round_results`, derives `match_results` on save, and gates
official downstream views with `week_closed=1`. Handicap Review and Handicap
Apply belong to the `handicaps` domain, though this domain preserves the
handicap snapshots they depend on.

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

**Frontend validation** (`web/app.js`) remains helper UX only. It normalizes
inputs and shows live pairing outcomes, but it is not authoritative.

### Behavior

- **Errors -> HTTP 422** with `{"messages": [...]}` body (see `validation.Result`). No rows are written.
- **Warnings -> save proceeds.** Warnings are computed and later surfaced through
  Close Week review flows.
- Warning acknowledgment and Close Week finalization are implemented in later
  phases documented below.

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

## Close Week -- Phase 3B: Advance Result After Close (implemented 2026-06-24)

### Goal

Return a close result summary in the `POST /close` success response so the
admin sees a compact success view in the modal after closing a week, instead
of the modal dismissing immediately.

### Backend changes

`closeWeekHandler` now returns after a successful commit:

```json
{
  "closed": true,
  "week_number": 2,
  "acknowledgment_count": 1,
  "advance_result": {
    "message": "Week closed. Standings and player stats now include this week's results.",
    "closed_week": {
      "match_count": 3,
      "completed_count": 3,
      "closed_count": 3,
      "status": "closed"
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
      "message": "No handicap changes are applied automatically."
    }
  }
}
```

`next_week_number` and `next_week` are omitted when no further weeks are
scheduled. `advance_result` is best-effort: if the post-commit summary query
fails, the response still returns `{"closed": true, "week_number": N,
"acknowledgment_count": N}` so the close is never misreported as failed.

The data-collection logic was extracted into `buildAdvanceResult(seasonID,
weekNum int64) (models.AdvanceResult, error)`, a package-level helper called
from both `getAdvancePreview` and `closeWeekHandler`. No writes are performed
by the helper.

### Frontend changes

After a successful close, the Review & Close modal body is replaced with a
success summary built by `_renderCloseSuccess(closeData, weekNum)`. The
confirm button changes to "Done" (dismisses the modal). Schedule, standings,
and player stats are refreshed in the background as before.

The Phase 3A "Advance Preview" section still appears before close. After
close, the modal body is replaced entirely with the success view.

### Not in Phase 3B

- Automatic handicap writes
- Blank `round_results` creation
- `lineup_plans` creation or modification
- Audit tables

### Files changed

- `models/models.go` -- `AdvanceResult` struct
- `handlers/api.go` -- `buildAdvanceResult` helper extracted; `closeWeekHandler`
  returns `advance_result`; `getAdvancePreview` delegates to helper
- `handlers/api_test.go` -- 8 Phase 3B tests
- `web/app.js` -- `_renderCloseSuccess` helper; `confirmBtn.onclick` shows
  success view instead of dismissing immediately

## Close Week -- Phase 3C: Handicap Recommendation Preview (implemented 2026-06-25)

### Goal

Show read-only handicap recommendations in the Advance Preview and post-close
success summary. Recommendations are computed from closed official match data only.
No handicap values are written anywhere in this phase.

### Scope: read-only / no writes

Phase 3C does **not** write to:
- `players.handicap`
- `handicap_history`
- `lineup_plans`
- `round_results`
- Any other table beyond what Close Week already writes (Phase 1)

Because no handicap writes occur, the Reopen workflow requires no new rollback logic.

### Response shape extension

`AdvancePreviewHandicap` now carries an optional `recommendations` field:

```json
{
  "method": "game_diff_average",
  "status": "preview",
  "message": "2 players have recommended handicap changes (not yet applied).",
  "recommendations": [
    {
      "player_id": 12,
      "player_name": "John Smith",
      "current_handicap": 1.5,
      "recommended_handicap": 2.3,
      "matches_played": 4,
      "admin_hold": false,
      "skipped": false
    },
    {
      "player_id": 17,
      "player_name": "Jane Doe",
      "current_handicap": 2.0,
      "recommended_handicap": 2.0,
      "matches_played": 3,
      "admin_hold": false,
      "skipped": false,
      "reason": "no_change"
    },
    {
      "player_id": 22,
      "player_name": "Bob Lee",
      "current_handicap": 3.0,
      "recommended_handicap": 3.0,
      "matches_played": 0,
      "admin_hold": false,
      "skipped": true,
      "reason": "no_data"
    },
    {
      "player_id": 31,
      "player_name": "Alice Wu",
      "current_handicap": 2.0,
      "recommended_handicap": 2.0,
      "matches_played": 2,
      "admin_hold": true,
      "skipped": true,
      "reason": "admin_hold"
    }
  ]
}
```

`recommendations` is absent (`omitempty`) for `manual_review` and `kicker_average_preview`.

### Method routing

| `handicap_update_method` rule | Status | Recommendations |
|-------------------------------|--------|-----------------|
| `manual_review` (default) | `no_auto_apply` | absent |
| `game_diff_average` | `preview` | present |
| `kicker_average_preview` | `unsupported` | absent |

The rule is read from `season_rules`. If absent or empty, `manual_review` is assumed.

### Stable reason codes

| Code | Meaning |
|------|---------|
| `no_data` | Player is on the season roster but has no closed match data |
| `admin_hold` | Player has `admin_hold=1`; no recommendation computed |
| `no_change` | Computed recommendation equals current handicap |
| `capped` | Computed average exceeded `max_individual_handicap` and was capped |
| `unsupported_method` | Reserved for future use |

### game_diff_average formula (draft)

The `game_diff_average` recommendation is **draft preview logic, not confirmed
league policy**. It is a starting point for discussion only.

Formula: `recommended = round(avg_diff, 1)` where

```
avg_diff = SUM(match_results.diff) / COUNT(match_results)
```

across all matches in the season where `completed = 1 AND week_closed = 1`.

`round(x, 1)` rounds to the nearest 0.1 (same as `math.Round(x*10)/10`).

The `max_individual_handicap` season rule (default 4.5) caps the absolute value
of the recommendation: if `|recommended| > maxHC`, the value is capped and marked
`reason: "capped"`.

Players are sourced from `season_rosters` (managed seasons) UNION players with
closed `match_results` in the season (legacy seasons). This ensures players on the
roster with no play time appear as `skipped: true, reason: "no_data"`.

### kicker_average_preview status

`kicker_average_preview` returns `status: "unsupported"` with a plain-text message.
No recommendations are computed. The kicker average formula is deferred to a future
phase once the league defines it.

### Endpoints affected

Both endpoints populate the `handicap` field via the shared `buildHandicapPreview`
helper, which calls `buildAdvanceResult`:

| Endpoint | Trigger |
|----------|---------|
| `GET /api/seasons/{id}/weeks/{week}/advance-preview` | pre-close dry-run |
| `POST /api/seasons/{id}/weeks/{week}/close` (on success) | post-commit result |

### UI behavior

When `recommendations` is present and non-empty, a compact table is appended below
the Advance Preview summary rows:

- Columns: Player, Current, Recommended, Matches, Notes
- Skipped rows (`admin_hold` or `no_data`): muted text, lock badge or "No data"
- `no_change` rows: show same value in both columns, "No change" note
- `capped` rows: show capped value, "Capped" badge
- A warning paragraph above the table states: **"Recommendations are not applied
  automatically -- review and update manually if needed."**
- No Apply button. No checkboxes.

The same table appears in the post-close success modal under the close confirmation
header.

For `manual_review` (default), no table is rendered. The existing text-only
message ("No handicap changes are applied automatically.") is preserved.

### Not in Phase 3C

- Writing `players.handicap`
- Writing `handicap_history`
- An "Apply" button or any automatic application flow
- `kicker_average_preview` formula implementation
- `handicap_rounding` rule enforcement on recommendations
- Multi-season aggregation
- Audit table entries for recommendations

### Files changed

- `models/models.go` -- `PlayerHandicapRec` struct; `Recommendations` field on
  `AdvancePreviewHandicap`
- `handlers/api.go` -- `buildHandicapPreview`, `computeGameDiffAverageRecs`,
  `seasonHandicapUpdateMethod`, `seasonMaxIndividualHC` helpers; `buildAdvanceResult`
  delegates to `buildHandicapPreview`
- `handlers/api_test.go` -- 11 Phase 3C tests
- `web/app.js` -- `_renderHandicapRecs` helper; `_renderAdvancePreview` and
  `_renderCloseSuccess` include recommendations table when present

## Phase 3D -- Handicap Review Screen

### Goal

Dedicated read-only screen so admins can review season-wide handicap
recommendations outside the Close Week modal. Remain read-only; no apply
workflow yet.

### New endpoint

```
GET /api/seasons/{id}/handicap-recommendations
```

Returns season-wide recommendations based on all `completed=1 AND
week_closed=1` matches. Response shape:

```json
{
  "season_id": 1,
  "method":     "game_diff_average",
  "status":     "preview",
  "message":    "3 players have recommended handicap changes (not yet applied).",
  "weeks_closed": 2,
  "recommendations": [
    {
      "player_id":            1,
      "player_name":          "Alice Active",
      "team_name":            "Rack City",
      "current_handicap":     1.5,
      "recommended_handicap": 2.0,
      "change_amount":        0.5,
      "matches_played":       2,
      "admin_hold":           false,
      "skipped":              false,
      "reason":               ""
    }
  ]
}
```

**Status codes returned by method:**

| Method | status | notes |
|--------|--------|-------|
| `manual_review` (default) | `no_auto_apply` | empty recommendations |
| `kicker_average_preview` | `unsupported` | empty recommendations |
| `game_diff_average`, no closed weeks | `no_data` | empty recommendations |
| `game_diff_average`, weeks closed | `preview` | full recommendations |

**Reason codes** (same set as Phase 3C): `no_data`, `admin_hold`, `no_change`,
`capped`.

**Live recompute:** recommendations are computed fresh on every request.
Reopening a week sets `week_closed=0` on its matches; the next response
automatically excludes that data. No stored recommendation rows exist to
invalidate.

**Error behavior:** Season not found returns 404. Real DB failures return 500;
empty recommendations are never returned to mask query errors.

**Read-only contract:** No writes to `players.handicap`, `handicap_history`,
or any other table.

### New model types

`HandicapReviewRec` -- per-player row for the review screen. Adds `TeamName`
and `ChangeAmount` relative to `PlayerHandicapRec` (advance-preview only).

`HandicapReviewResponse` -- top-level response wrapping the recommendations
with method, status, message, and weeks_closed.

### Frontend

New sidebar tab "Handicap" (`data-section="handicap"`, icon `bi-graph-up-arrow`)
added after Player Stats. Section contains:

- season selector (populated from `allSeasons`)
- status/message card
- "Based on N closed week(s)" context note
- Table columns: Team, Player, Current, Recommended, Change, Matches, Notes

Table behavior: skipped rows muted; Admin Hold badge; No data text; no Apply
button; no edit controls.

### Not in Phase 3D

- Writing `players.handicap` or `handicap_history`
- Apply button or automatic application
- Per-week breakdown or drill-down
- Filter/sort controls
- Export or mark-as-reviewed workflow

### Files changed

- `models/models.go` -- `HandicapReviewRec` and `HandicapReviewResponse` structs
- `handlers/api.go` -- `computeHandicapReviewRecs`, `getHandicapRecommendations`
  handler; route registered as `GET /api/seasons/{id}/handicap-recommendations`
- `handlers/api_test.go` -- 8 Phase 3D tests
- `web/index.html` -- Handicap nav item and `#section-handicap` div
- `web/app.js` -- `loadHandicapReview`, `renderHandicapReviewTable`

## Phase 3E -- Handicap Snapshot Preservation in saveRounds (implemented 2026-06-27)

### Goal

Prevent re-saving a scoresheet from silently overwriting historical handicap
snapshots (`home_handicap_used`, `away_handicap_used`) when a player's current
handicap has changed since the original save. These columns feed the
opponent-normalized Handicap Review calculation; corrupting them would
invalidate historical rack samples.

### Behavior

`saveRounds` reads prior snapshot rows inside the active write transaction,
**before** the DELETE. On re-insert, each side's snapshot is preserved or
refreshed:

| Scenario | home_handicap_used | away_handicap_used |
|----------|-------------------|--------------------|
| Same player on same side | Preserved from prior row | Preserved from prior row |
| Player substituted | Fresh from `players.handicap` | Preserved (unchanged side) |
| Both substituted | Fresh from `players.handicap` | Fresh from `players.handicap` |
| First save (no prior row) | Fresh from `players.handicap` | Fresh from `players.handicap` |
| Prior snapshot is NULL (legacy) | Fresh baseline at re-save | Fresh baseline at re-save |

The snapshot query uses the active transaction (`tx.Query`) so it reads the
pre-DELETE state. Errors from the query propagate as HTTP 500; no partial
writes occur.

### No schema change

`home_handicap_used` and `away_handicap_used` were added via additive
migration in an earlier phase. No new columns are needed.

### Files changed

- `handlers/api.go` -- `saveRounds`: reads prior snapshots into
  `map[int]priorSnap` before `DELETE`; computes `homeHCToStore` / `awayHCToStore`
  per-round on re-insert
- `handlers/api_test.go` -- `TestSaveRounds_SnapshotPreservedOnResave`,
  `TestSaveRounds_SubstitutionPreservesUnchangedSide`

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

## Phase B1 — Matches/Close Week Extraction (implemented 2026-06-30)

### Goal

Extract the week-workflow backend (list weeks, validate, close, reopen,
acknowledgment history) into a purpose-built service/store layer without
changing routes, JSON shapes, or browser behavior.

### New files

| File | Role |
|------|------|
| `backend/domains/matches/store.go` | `WeekStore` interface + `AckEntry` type |
| `backend/domains/matches/service.go` | `WeekService`, `CloseWeekRequest`, `CloseWeekResult`, `WeekCloseErr` |
| `backend/domains/matches/service_test.go` | Unit tests with stub store |
| `backend/storage/sqlite/week_store.go` | SQLite implementation of `WeekStore` |
| `backend/storage/sqlite/week_store_test.go` | DB integration tests (~15 tests) |

### Modified files

| File | Change |
|------|--------|
| `handlers/deps.go` | `WeekManager` interface + `WeekMgr WeekManager` field on `Dependencies` |
| `handlers/api.go` | Five week handlers thinned to delegate; routes conditional on `WeekMgr != nil` |
| `handlers/api_test.go` | `testServer()` wires `WeekService` into deps |
| `handlers/api_apply_c1_test.go` | `testServerWithApplyAuth()` wires `WeekService` into deps |
| `main.go` | `sqlite.NewWeekStore` → `matches.NewWeekService` → `deps.WeekMgr` |

### Architecture

```
closeWeekHandler (handler)
  → deps.WeekMgr.CloseWeek (WeekManager interface)
      → matches.WeekService.CloseWeek (service: ack-completeness, policy)
          → ValidateWeek(s.db, ...) (package-level, temporary *sql.DB debt)
          → weekStore.CloseWeek (WeekStore interface)
              → sqlite.WeekStore.CloseWeek (TX: upsert league_weeks, update matches, insert acks)
```

### Temporary debt accepted in B1

- `WeekService` holds `*sql.DB` to call the package-level `ValidateWeek(dbConn, ...)`.
  B4 will move validation into a `WeekStore` method and remove the DB field.
- `seasonRoundConfig` stays in `handlers/api.go` (handler calls it and passes the
  result to the service via `CloseWeekRequest.Cfg`).
- `buildAdvanceResult`, `getAdvancePreview`, and all handicap preview helpers stay in
  `handlers/api.go` (B2 will extract these).
- `saveRounds` stays in `handlers/api.go` (B3 — most complex due to HC snapshot TX).

### Route registration

Week routes are conditionally registered when `deps.WeekMgr != nil`. In production
(`main.go`) `WeekMgr` is always set. Tests that don't exercise week routes (e.g. Apply
auth tests) may omit it; those test servers simply won't have week endpoints. All
existing week integration tests go through `testServer()` which wires `WeekMgr`.

The `advance-preview` route was registered outside the weekMgr block in B1 (still using
`db.DB` directly via `buildAdvanceResult`). Moved inside the block and delegated to
`WeekMgr.AdvancePreview` in B2.

### Not in B1

- `buildAdvanceResult`, `computeGameDiffAverageRecs`, standings, stats (B2)
- `saveRounds` HC snapshot TX logic (B3)
- `ValidateWeek` signature change from `*sql.DB` to a store interface (B4)
- Route/shape changes, new endpoints

## Phase B2 — Advance Preview and Close Result Extraction (implemented 2026-07-01)

### Goal

Extract advance-preview assembly and handicap preview assembly out of
`handlers/api.go` into their respective domain services. No route paths or
JSON shapes changed; browser behavior is unchanged.

### New files

| File | Role |
|------|------|
| `backend/domains/matches/advance.go` | `HandicapPreviewer` interface; `WeekAdvanceSummary` type |

### Modified files

| File | Change |
|------|--------|
| `backend/domains/matches/store.go` | Added `GetWeekAdvanceSummary` to `WeekStore` interface |
| `backend/domains/matches/service.go` | Added `hcPreview HandicapPreviewer` field; `AdvanceData`, `AdvancePreview` methods; `roundConfig` private helper |
| `backend/domains/matches/service_test.go` | Added stub `GetWeekAdvanceSummary`; added `stubHandicapPreviewer`; added `AdvanceData` and `AdvancePreview` tests |
| `backend/domains/handicaps/store.go` | Added `GameDiffAverageRow` type; `GameDiffAverageRecs` to `Store` interface |
| `backend/domains/handicaps/service.go` | Added `HandicapPreview` method; `applyGameDiffCap` helper |
| `backend/domains/handicaps/service_test.go` | Added `gameDiffRecs` field to stub; `GameDiffAverageRecs` stub; 7 `HandicapPreview` tests |
| `backend/storage/sqlite/week_store.go` | `GetWeekAdvanceSummary` implementation |
| `backend/storage/sqlite/week_store_test.go` | 5 `GetWeekAdvanceSummary` tests |
| `backend/storage/sqlite/handicap_store.go` | `GameDiffAverageRecs` implementation |
| `backend/storage/sqlite/handicap_store_test.go` | 4 `GameDiffAverageRecs` tests |
| `handlers/deps.go` | Added `AdvanceData` and `AdvancePreview` to `WeekManager` interface |
| `handlers/api.go` | Thinned `getAdvancePreview` to delegate; `closeWeekHandler` calls `mgr.AdvanceData`; deleted `buildAdvanceResult`, `buildHandicapPreview`, `computeGameDiffAverageRecs`, `seasonHandicapUpdateMethod`, `seasonMaxIndividualHC` (~270 lines removed) |
| `handlers/api_test.go` | `testServer()` updated to 3-arg `NewWeekService` |
| `handlers/api_apply_c1_test.go` | Updated to 3-arg `NewWeekService` |
| `main.go` | `NewWeekService` now passes `hcSvc` as third argument |

### Architecture

```
getAdvancePreview handler
  → deps.WeekMgr.AdvancePreview (WeekManager)
      → matches.WeekService.AdvancePreview
          → store.WeekMatchCount   (404 check)
          → ValidateWeek           (validation; B4 debt: uses s.db)
          → WeekService.AdvanceData
              → store.GetWeekAdvanceSummary (match counts, status, next week)
              → hcPreview.HandicapPreview   (HandicapPreviewer interface)
                  → handicaps.Service.HandicapPreview
                      → store.SeasonHandicapRules
                      → store.GameDiffAverageRecs (outside RunTx; read-only preview)

closeWeekHandler
  → deps.WeekMgr.CloseWeek       (unchanged from B1)
  → deps.WeekMgr.AdvanceData     (new; post-commit best-effort summary)
      → (same path as above)
```

### HandicapPreviewer interface (consumer-defines)

`HandicapPreviewer` is defined in the `matches` package (consumer) and
implemented by `handicaps.Service`. This is the standard Go consumer-defines-
interface pattern and avoids an import cycle between `matches` and `handicaps`.

### Temporary debt accepted in B2

- `WeekService.roundConfig` reads `handicap_multiplier` and `min_ball_handicap`
  from `s.db` directly. This mirrors the handler's old `seasonRoundConfig`.
  Both move to a `WeekStore` method in B4 when `ValidateWeek` is extracted.
- `WeekService.db *sql.DB` is retained for the same reason (B4 will remove it).
- `GameDiffAverageRecs` is called outside `RunTx` (read-only preview, no
  atomicity needed). Acceptable for best-effort pre-close display.
- `HandicapPreview` uses `game_diff_average` logic in the advance-preview path,
  which is the legacy formula. The Handicap Review screen uses the
  opponent-normalized formula via `Recommendations`. The preview path is display-
  only and not authoritative for Apply.

### Not in B2

- `saveRounds` HC snapshot TX logic (B3)
- `ValidateWeek` signature change (B4)
- Standings or stats extraction
- Route/shape changes

## Phase B3 — Round Save/Read, Standings, and Stats Extraction (implemented 2026-07-01)

### Goal

Extract `saveRounds`, `getRounds`, `getStandings`, `getPlayerStats`, `submitResults`,
and `clearResults` from `handlers/api.go` into a dedicated `RoundService` and
`RoundStore` domain boundary. No route paths or JSON shapes changed; browser
behavior is unchanged.

### New files

| File | Role |
|------|------|
| `backend/domains/matches/round_store.go` | `RoundStore` interface + domain types |
| `backend/domains/matches/round_service.go` | `RoundService` + `computePairingResult` |
| `backend/domains/matches/round_service_test.go` | Unit tests with stub store (17 tests) |
| `backend/storage/sqlite/round_store.go` | SQLite implementation of `RoundStore` |
| `backend/storage/sqlite/round_store_test.go` | DB integration tests (13 tests) |

### Modified files

| File | Change |
|------|--------|
| `handlers/deps.go` | `RoundManager` interface + `RoundMgr RoundManager` field on `Dependencies` |
| `handlers/api.go` | Six handlers thinned to delegate; routes conditional on `RoundMgr != nil`; removed `computePairingResult`, `txSeasonMultiplier`, `txSeasonRoundConfig`, `matchWeekClosed` (~450 lines removed) |
| `handlers/api_test.go` | `testServer()` wires `RoundService` into deps |
| `handlers/api_apply_c1_test.go` | `testServerWithApplyAuth()` wires `RoundService` into deps |
| `main.go` | `sqlite.NewRoundStore` → `matches.NewRoundService` → `deps.RoundMgr` |
| `doc/domains/matches/README.md` | This section |

### Architecture

```
saveRounds handler
  → deps.RoundMgr.SaveRounds (RoundManager interface)
      → matches.RoundService.SaveRounds
          → store.IsWeekClosed         (pre-TX guard → 409 if closed)
          → store.RunTx(fn)
              → store.LoadMatchContext  (season, home/away team IDs)
              → store.SeasonRoundConfig (handicap_multiplier + min_ball_handicap)
              → store.LoadPlayerHandicap (per unique player)
              → store.LoadPriorSnapshots (HC snapshot preservation)
              → matches.ValidateRounds  (→ RoundValidationError on error)
              → store.DeleteRoundResults / InsertRoundResult × N
              → store.DeleteMatchResults / InsertMatchResult × M
              → store.MarkMatchCompleted (if any game scored)
```

### Key design decisions (Q-B3-1/2/3)

**Q-B3-1 (include submitResults/clearResults):** Both included in B3 scope.
`SubmitResults` and `ClearResults` are thin TX operations wrapped by the store.
`SubmitMatchResults` on the store calls `DeleteMatchResults` + `InsertMatchResult` × N
+ `MarkMatchCompleted` inside its own `RunTx`.

**Q-B3-2 (config inside service):** `RoundService.SaveRounds` calls
`store.SeasonRoundConfig` inside `RunTx` — no `Cfg` passed from the handler.
This keeps the handler thin and ensures the config read is part of the same
transaction as the writes.

**Q-B3-3 (active-season fallback in handler):** `getStandings` resolves
`season_id` from `league_id` via an active-season lookup in the handler, then
passes the resolved ID to `mgr.GetStandings`. The service never sees a nil season ID.

### HC snapshot preservation

The `LoadPriorSnapshots` store method reads prior `round_results` rows inside the
active transaction (before `DeleteRoundResults`). `SaveRounds` builds a
`priorByRound` map and applies the same snapshot preservation logic as the old
handler:

| Scenario | home_handicap_used | away_handicap_used |
|----------|-------------------|--------------------|
| Same player on same side | Preserved from prior row | Preserved from prior row |
| Home substituted, away same | Fresh from `players.handicap` | Preserved |
| Both substituted | Fresh | Fresh |
| First save | Fresh | Fresh |
| Prior snapshot NULL (legacy) | Fresh at re-save | Fresh at re-save |

### Error types

| Error | HTTP mapping |
|-------|-------------|
| `*matches.RoundValidationError` | 422 with `{"messages": [...]}` |
| `domainerr.Conflict` | 409 |
| `domainerr.Unprocessable` | 422 with plain `{"error": "..."}` |
| Other errors | 500 |

### Temporary debt accepted in B3

- `seasonMultiplier` and `seasonRoundConfig` remain in `handlers/api.go` for
  `validateWeekHandler` and `closeWeekHandler` (B4 will move these to
  `WeekStore.SeasonRoundConfig`).
- `WeekService.db *sql.DB` is retained (B4 debt from B1/B2).
- The `seasons.RosterEligible` cross-domain pre-TX guard stays in the handler,
  called before `mgr.SaveRounds`. It checks `teams_managed=1` seasons only;
  legacy seasons bypass the check automatically.
- `GetRounds` falls back to `logic.Multiplier` (2.55) when `LoadMatchContext`
  or `SeasonRoundConfig` fails, matching the old handler's silent error handling.

### Not in B3

- `ValidateWeek` signature change from `*sql.DB` to store interface (B4)
- `seasonRoundConfig` / `seasonMultiplier` removal from `handlers/api.go` (B4)
- Route or JSON shape changes

## Phase B4 — Round Config via RuleStore (implemented 2026-07-02)

### Goal

Remove the last `*sql.DB` fields from `WeekService` and `RoundService` by routing
round configuration (`handicap_multiplier`, `min_ball_handicap`) through the
`rules.RuleStore` interface instead of direct DB queries. This completes the B1–B3
debt accepted earlier.

### New files

| File | Role |
|------|------|
| `backend/domains/matches/round_config.go` | `ResolveRoundConfig` pure function; `RoundConfig` type |
| `backend/domains/matches/round_config_test.go` | Unit tests via `stubRuleStore`; covers defaults, overrides, validation errors |

### Modified files

| File | Change |
|------|--------|
| `backend/domains/rules/store.go` | `GetValue(ctx, seasonID, key)` added to `RuleStore` interface |
| `backend/storage/sqlite/rule_store.go` | `GetValue` implementation |
| `backend/storage/sqlite/rule_store_test.go` | `GetValue` tests |
| `backend/domains/matches/service.go` | `ruleStore rules.RuleStore` field added; `NewWeekService` takes 3-arg; `ValidateWeek` / `CloseWeek` / `roundConfig` use `ResolveRoundConfig`; `db *sql.DB` field removed |
| `backend/domains/matches/round_service.go` | `ruleStore rules.RuleStore` field added; `NewRoundService` takes 2-arg; `SaveRounds` uses `ResolveRoundConfig`; `SeasonRoundConfig` store call removed |
| `backend/domains/matches/service_test.go` | Constructor updated to 3-arg; `stubRuleStore` wired |
| `backend/domains/matches/round_service_test.go` | Constructor updated; `stubRuleStore` wired |
| `handlers/api_test.go` | `testServer()` updated to 3-arg `NewWeekService` |
| `handlers/api_apply_c1_test.go` | `testServerWithApplyAuth()` updated to 3-arg `NewWeekService` |
| `main.go` | `NewWeekService` and `NewRoundService` updated to inject `ruleSvc` |

### Architecture after B4

```
ValidateWeek handler
  → deps.WeekMgr.ValidateWeek
      → matches.WeekService.ValidateWeek
          → matches.ResolveRoundConfig(ctx, s.ruleStore, seasonID)
              → rules.RuleStore.GetValue (handicap_multiplier, min_ball_handicap)
          → store.GetWeekValidationData

saveRounds handler
  → seasons.RosterEligible (cross-domain pre-TX guard, stays in handler)
  → deps.RoundMgr.SaveRounds
      → matches.RoundService.SaveRounds
          → matches.ResolveRoundConfig(ctx, s.ruleStore, seasonID)
              → rules.RuleStore.GetValue
```

`WeekService.db *sql.DB` is fully removed. `SeasonRoundConfig` is gone from
`WeekStore` and `RoundStore` interfaces and their SQLite implementations.

### Not in B4

- `ValidateWeek` data-loading signature change (store-interface refactor, deferred)
- The `seasons.RosterEligible` pre-TX guard in `saveRounds` handler (intentional;
  see B3 decision Q-B3-3)
- Route or JSON shape changes

## Phase A — Schedule Generation Extraction (implemented 2026-07-03)

### Goal

Extract the `generateSchedule` handler from `handlers/api.go` into a dedicated
`ScheduleService` and `ScheduleStore` domain boundary within the `matches` package.
No route path, JSON shape, or runtime behavior changed.

### New files

| File | Role |
|------|------|
| `backend/domains/matches/schedule_store.go` | `ScheduleStore` interface; `ScheduleSeasonMeta`, `MatchEntry`, `SaveScheduleRequest`, `ErrSeasonNotFound` types |
| `backend/domains/matches/schedule_service.go` | `ScheduleService`, `GenerateRequest`, `GenerateResult`, `NewScheduleService` |
| `backend/domains/matches/schedule_service_test.go` | 10 unit tests with `stubScheduleStore` |
| `backend/storage/sqlite/schedule_store.go` | SQLite implementation of `ScheduleStore` (5 methods) |
| `backend/storage/sqlite/schedule_store_test.go` | 10 integration tests |

### Modified files

| File | Change |
|------|--------|
| `handlers/deps.go` | `ScheduleManager` interface + `ScheduleMgr ScheduleManager` field on `Dependencies` |
| `handlers/api.go` | `generateSchedule` replaced with ≤10-line thin wrapper; route closure added; `mapScheduleErr` added; `logic` and `time` imports removed; ~190 lines removed |
| `handlers/api_test.go` | `noopScheduleMgr` stub; `testServer()` wires real `ScheduleService` |
| `handlers/api_apply_auth_test.go` | `noopScheduleMgr` stub; `matches` import added |
| `handlers/api_apply_c1_test.go` | `testServerWithApplyAuth()` wires real `ScheduleService` |
| `main.go` | `sqlite.NewScheduleStore` → `matches.NewScheduleService` → `deps.ScheduleMgr` |
| `models/models.go` | `GenerateScheduleRequest` removed (replaced by `matches.GenerateRequest`) |

### Architecture

```
POST /api/matches/generate
  → generateSchedule handler (parse body, delegate, map error)
      → deps.ScheduleMgr.GenerateSchedule (ScheduleManager interface)
          → matches.ScheduleService.GenerateSchedule
              → store.GetScheduleSeasonMeta  (season exists + managed flag)
              → store.LoadByeRequests        (approved byes with specific weeks)
              → store.LoadTeamIDsFromHistory (legacy from_season_id path)
              → store.LoadTeamIDsForSchedule (season_teams or league fallback)
              → logic.BlanketTemplate / SingleRoundRobin / DoubleRoundRobin /
                       SplitSeason / CustomSchedule  (pure functions)
              → store.SaveGeneratedSchedule  (TX: delete unplayed, insert, update season)
```

### Key design decisions

**ScheduleStore in `matches` package, not `seasons`:** The primary output is
`matches` records. The route is `POST /api/matches/generate`. This keeps the
domain boundary consistent with `WeekStore` and `RoundStore`.

**`MatchEntry` domain type:** The store interface uses `matches.MatchEntry` rather
than `logic.ScheduleEntry` to keep the SQLite adapter free of `logic` imports.
The service converts between the two — the conversion is trivial (identical fields).

**`ErrSeasonNotFound` sentinel:** Defined in the `matches` package (not `seasons`)
to keep packages independent. The service translates it to `domainerr.NotFound`.

**`normDate` private helper in service:** The SQLite driver sometimes returns DATE
columns as full ISO timestamps. `normDate` (same logic as handler's `normDateStr`)
is a package-private helper in `schedule_service.go`. The handler's `normDateStr`
is retained for the `listSkippedWeeks` handler which still uses it.

**Route registered conditionally (`if deps.ScheduleMgr != nil`):** Matches the
pattern established by `WeekMgr` and `RoundMgr`. Always wired in production
(`main.go`) and in the two full test servers (`testServer`, `testServerWithApplyAuth`).

**`models.GenerateScheduleRequest` removed:** Was only referenced by the old handler.
Replaced by `matches.GenerateRequest` with identical JSON field names — no API break.

### Accepted debt

- `logic` and `time` were removed from `handlers/api.go` imports as they were
  exclusively used by the old handler body.
- `nullStr` helper (previously used only by `generateSchedule`) was removed with
  the handler. The SQLite store inlines the equivalent `nil`-or-value logic directly.

### Not in Phase A

- Route or JSON shape changes
- Schedule preview / pushback workflow
- Auth changes

## Phase B — Match Read/Assign Extraction (implemented 2026-07-03)

### Goal

Extract `listMatches`, `getMatch`, and `assignMatchTeams` from `handlers/api.go`
into a `MatchService` and `MatchStore` domain boundary. No route path, JSON shape,
or runtime behavior changed. The `matchSelect` constant moved from the handler file
into the SQLite adapter where it belongs.

### New files

| File | Role |
|------|------|
| `backend/domains/matches/match_store.go` | `MatchStore` interface; `ListMatchesRequest`, `ErrMatchNotFound` |
| `backend/domains/matches/match_service.go` | `MatchService`, `NewMatchService`, `ListMatches`, `GetMatch`, `AssignMatchTeams` |
| `backend/domains/matches/match_service_test.go` | 9 unit tests with `stubMatchStore` |
| `backend/storage/sqlite/match_store.go` | SQLite implementation of `MatchStore` (3 methods); owns `matchSelect` constant |
| `backend/storage/sqlite/match_store_test.go` | 11 integration tests |

### Modified files

| File | Change |
|------|--------|
| `handlers/deps.go` | `MatchManager` interface + `MatchMgr MatchManager` field on `Dependencies` |
| `handlers/api.go` | Three handlers thinned to delegate; routes conditional on `MatchMgr != nil`; `matchSelect` constant removed; `mapMatchErr` added |
| `handlers/api_test.go` | `noopMatchMgr` stub; `testServer()` wires real `MatchService` |
| `handlers/api_apply_auth_test.go` | `noopMatchMgr` stub |
| `handlers/api_apply_c1_test.go` | `testServerWithApplyAuth()` wires real `MatchService` |
| `main.go` | `sqlite.NewMatchStore` → `matches.NewMatchService` → `deps.MatchMgr` |

### Architecture

```
GET /api/matches
  → listMatches handler (parse query params, delegate, map error)
      → deps.MatchMgr.ListMatches (MatchManager interface)
          → matches.MatchService.ListMatches
              → store.ListMatches (season_id / league_id / all variants)

GET /api/matches/{id}
  → getMatch handler
      → deps.MatchMgr.GetMatch
          → matches.MatchService.GetMatch
              → store.GetMatch  (ErrMatchNotFound → domainerr.NotFound → 404)
              → store scans match row + results rows → models.MatchDetail

PATCH /api/matches/{id}/assign
  → assignMatchTeams handler
      → deps.MatchMgr.AssignMatchTeams
          → matches.MatchService.AssignMatchTeams
              → store.AssignMatchTeams (UPDATE matches SET home_team_id=?, away_team_id=?)
```

### Key design decisions

**`MatchStore` in `matches` package:** Consistent with `ScheduleStore`, `WeekStore`,
and `RoundStore`. The match resource is this domain's primary table.

**Store returns `models.Match` and `models.MatchDetail` directly:** `models` is a
pure data package; importing it from the SQLite adapter is established practice
(several existing stores do so). The service layer adds error categorization on top;
no conversion step is needed here because `models.Match` is already the response type.

**`normMatchDatePtr` private helper in SQLite store:** The SQLite driver coerces DATE
columns to full ISO timestamps. The store applies truncation so callers receive clean
`YYYY-MM-DD` values. The handler's `normDatePtr` is retained for the `scanSeason`
helper which still uses it.

**`assignMatchTeams` preserves no-RowsAffected behavior:** The original handler
returned 200 even when the match ID did not exist (no RowsAffected check). Phase B
preserves this exactly — the store does a plain UPDATE and the service propagates
only genuine DB errors.

**Route registered conditionally (`if deps.MatchMgr != nil`):** Matches the pattern
established by `WeekMgr`, `RoundMgr`, and `ScheduleMgr`.

### Accepted debt

- `normDatePtr` remains in `handlers/api.go` for the `scanSeason` helper (seasons CRUD
  not yet extracted). `normMatchDatePtr` in the SQLite store is a private copy with the
  same logic.

### Not in Phase B

- Route or JSON shape changes
- Match CRUD beyond the three extracted handlers
- Lineup plans extraction
- Skipped-weeks or bye-request extraction

## Decision History

### 2026-06-08 - Make week close authoritative

**Status:** `accepted`

Score entry stores pending data. Official calculations and result effects are
committed only after backend Close Week validation succeeds.

### 2026-06-08 - Require transparent warning acknowledgment

**Status:** `accepted`

Errors block close. Warnings require explicit, reasoned, audited admin
acknowledgment.
