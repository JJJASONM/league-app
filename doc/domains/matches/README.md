# Matches

## Overview

**Owner:** `matches`
**Status:** `draft`
**Current version:** `0.9`
**Last reviewed:** `2026-08-27`

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
does not make their calculations official. The match status transition after
score entry is `approved` -> `processed`, added in Weekly Score Processing
Phase 1A (see MATCHES-Q001, resolved, and the Phase 1A section below).

Official match outcomes, standings, and player statistics are still applied
only when the admin successfully closes the week -- Close Week itself is
unchanged by Phase 1A. Handicap recommendation *eligibility* is the one
exception: a processed match counts toward it immediately, before its week
closes (see "Handicap eligibility: Phase 1A compatibility behavior" below).
Results that have not passed week close still do not contribute to official
standings/player-stats totals.

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
- Match-level status codes (MATCHES-Q001) (**admin-attested approved/processed states implemented in Weekly Score Processing Phase 1A; real captain/player-side approval remains deferred**)
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
      "included_racks": 4,
      "admin_hold": false,
      "skipped": false
    },
    {
      "player_id": 17,
      "player_name": "Jane Doe",
      "current_handicap": 2.0,
      "recommended_handicap": 2.0,
      "included_racks": 3,
      "admin_hold": false,
      "skipped": false,
      "reason": "no_change"
    },
    {
      "player_id": 22,
      "player_name": "Bob Lee",
      "current_handicap": 3.0,
      "recommended_handicap": 3.0,
      "included_racks": 0,
      "admin_hold": false,
      "skipped": true,
      "reason": "no_data"
    },
    {
      "player_id": 31,
      "player_name": "Alice Wu",
      "current_handicap": 2.0,
      "recommended_handicap": 2.0,
      "included_racks": 2,
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

**Superseded 2026-06-27 by the Phase 3E opponent-normalized rack formula, and
field names below are historical only.** This response shape (and the
`match_results`-based `matches_played` field shown above) reflects
`GET /api/seasons/{id}/handicap-recommendations` as it existed at the end of
Phase 3D, before the rack-windowed engine replaced it. The endpoint's actual
current response shape -- `assigned_hc`, `score_eligible_racks`,
`included_racks`, `window_size`, `lifetime_hc`, `window_hc`, etc., with no
`skipped` field -- is documented in `doc/domains/handicaps/README.md` under
"Handicap Review Endpoint". Do not use the JSON block above as a reference
for today's response.

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

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-08-25`
**Related commit:** `weekly-score-processing-phase-1a-backend-foundation`

**Context:** Scores are entered before week close, but additional calculations
and validation still need to occur.

**Resolution:** Completed score entry does not become a review status by
itself. Two new, independent, admin-attested match-level states were added
underneath Close Week: `approved` (admin records that the team/captain
approved the scores; blocks further score edits until unapproved) and
`processed` (admin records that an approved match's results are official
enough to count toward handicap recommendation eligibility, even before
its week closes). See "Weekly Score Processing Phase 1A" above for full
detail. Real captain/player login approval was explicitly deferred, not
folded into this resolution.

### MATCHES-Q002 - Online score entry

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Online entry affects drafts, permissions, competing edits,
validation, approval, and the Close Week preview.

**Resolution:** Design the online score-entry workflow before finalizing match
statuses or calculation-preview behavior. Current direction: only rostered
players assigned to the match should be able to submit scores for that match,
with admin override. Processing individual matchups before the whole night is
finished is a likely direction, but requires more research.

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

  **Update 2026-08-24:** this accepted debt caused a real staging finding --
  the preview and `Recommendations` could show conflicting eligibility and
  values for the same players. `HandicapPreview` now delegates to
  `Recommendations` for `game_diff_average` instead of running the legacy
  formula, so the preview path and the Handicap Review screen share one
  computation. See `doc/domains/handicaps/README.md` Decision History,
  "HandicapPreview unified with Recommendations." One shape note for the
  JSON examples elsewhere in this file: `PlayerHandicapRec`'s
  `matches_played` field is renamed to `included_racks` as part of that fix.

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
  see "RosterEligible ownership decision" below, not B3 decision Q-B3-3, which
  covers `getStandings`' active-season fallback instead)
- Route or JSON shape changes

### RosterEligible ownership decision (2026-08-10)

RosterEligible remains a handler-level cross-domain pre-TX guard for now; no
workflow layer is introduced. This records the decision for the roadmap's
"Clarify ownership for multi-domain workflows" item.

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

## Phase C — Lineup Plans Extraction (implemented 2026-07-03)

### Goal

Extract the three `lineup-plans` endpoints out of the monolithic handler into
a domain boundary within the `matches` package, making `handlers/api.go` a
thin delegation layer for this resource.

### Architecture

```
GET/POST /api/lineup-plans
DELETE   /api/lineup-plans/{id}
  → thin handlers (parse, validate, delegate, mapLineupErr)
      → handlers.LineupManager interface
          → matches.LineupService
              → matches.LineupStore interface
                  ← sqlite.LineupStore
```

### New Files

| File | Purpose |
|------|---------|
| `backend/domains/matches/lineup_store.go` | `LineupStore` interface, `ListLineupPlansRequest`, `SaveLineupRequest` |
| `backend/domains/matches/lineup_service.go` | `LineupService`, `NewLineupService`; wraps store errors into `domainerr` |
| `backend/domains/matches/lineup_service_test.go` | 9 service unit tests using `stubLineupStore` |
| `backend/storage/sqlite/lineup_store.go` | SQLite implementation; owns dynamic query building and save transaction |
| `backend/storage/sqlite/lineup_store_test.go` | 9 SQLite integration tests |

### Modified Files

| File | Change |
|------|--------|
| `handlers/deps.go` | `LineupManager` interface + `LineupMgr` field on `Dependencies` |
| `handlers/api.go` | Three handlers accept `LineupManager`; route block conditional on `LineupMgr != nil`; `mapLineupErr` added |
| `handlers/api_test.go` | `noopLineupMgr` stub; `testServer` wires real `LineupService` |
| `handlers/api_apply_auth_test.go` | `noopLineupMgr` stub |
| `handlers/api_apply_c1_test.go` | `testServerWithApplyAuth` wires real `LineupService` |
| `main.go` | `sqlite.NewLineupStore` → `matches.NewLineupService` → `deps.LineupMgr` |

### Logic Moved Out of `handlers/api.go`

- Dynamic query building for season/week/team filter combinations
- `DELETE … INSERT OR IGNORE` transaction for atomic lineup replacement
- Zero-player-ID skip logic

### Key Design Decisions

**`SaveLineupRequest` carries `int64` for `WeekNumber`:** Handler's
`models.SaveTeamLineupRequest.WeekNumber` is `int`; the conversion to `int64`
happens at the handler boundary before calling the service. The DB column and
SQLite driver handle both seamlessly.

**`DeleteLineupPlan` does not error on non-existent ID:** The original handler
called `db.DB.Exec` and discarded all results, always returning 200. The store
passes through genuine DB errors (e.g., connection failure) but does not return
a `ErrNotFound` for missing rows — preserving the original behavior for the
normal delete-then-confirm flow while surfacing real failures.

**`mapLineupErr` mirrors `mapMatchErr`/`mapScheduleErr`:** Consistent error
translation pattern across all three match-family managers.

### Accepted Debt

| Item | Disposition |
|------|-------------|
| Skipped-weeks still direct `db.DB` | Belongs to seasons domain; separate extraction pass |
| `listByeRequests`/`deleteByeRequest` still direct `db.DB` | Same; complete bye-request extraction is a seasons-domain task |
| Lineup plans `WeekNumber` type mismatch (`int` vs `int64`) | Cast at handler boundary; no model change needed |

### Not in Phase C

- Route or JSON shape changes
- Skipped-weeks or bye-request extraction
- Leagues/players/teams CRUD extraction

## Next-Week Preparation Workflow

Next-week readiness is informational. The Close Week transaction does not mutate
next-week data. The advance-preview and advance-result responses report current
readiness only; no automatic preparation steps occur.

### What advance-preview reports

`GET /api/seasons/{id}/weeks/{week}/advance-preview` includes `next_week_number`
and `next_week` when a further scheduled week exists. Both are omitted for the
final week. The same fields appear in `advance_result` embedded in the POST close
response immediately after a successful close.

`next_week` fields:

| Field | Description |
|---|---|
| `match_count` | Total scheduled matches in next week |
| `assigned_count` | Matches where both home and away team IDs are set |
| `unassigned_count` | Matches missing a team assignment |
| `lineup_plan_count` | Total lineup_plans rows across all teams for next week |
| `missing_lineup_team_ids` | Team IDs with zero lineup plan entries for next week |

### What Close Week does not do

Close Week does not:
- Create lineup plans for next week
- Assign teams to next-week matches
- Initialize any next-week records
- Block closure based on missing next-week lineup plans or team assignments

Close validation checks only current-week matches (unassigned teams, no game
winners, player duplicates). Next-week gaps are surfaced as informational signals
only.

### Admin workflow for preparing next week

1. **Before season start** -- Generate the schedule. Standard round-robin formats
   produce match rows with home and away teams already set. Blanket and custom
   formats produce unassigned match slots that require manual team assignment.

2. **Assigning teams (blanket/custom only)** -- The week card in the Schedule page
   shows an "Assign" button for each unassigned match. The admin selects home and
   away teams via PATCH /api/matches/{id}/assign. Completed matches cannot be
   reassigned (MATCH_ALREADY_COMPLETED, 409).

3. **Entering lineup plans** -- Admins enter player slot assignments before each
   match night using the match entry screen. Lineup plans are stored per team,
   week, and season in lineup_plans.

4. **Checking readiness** -- The advance-preview modal (opened from "Review &
   Close") shows next-week match count, unassigned count, and lineup status. After
   close, the success panel repeats these signals in advance_result.

5. **Correcting gaps** -- Missing team assignments and lineup plans are fixed from
   the Schedule page and match entry screen. The close modal provides no in-place
   fix actions.

### Deferred

- Blocking close on missing next-week lineup plans or unassigned matches. The
  non-blocking design is intentional: legitimate workflows (substitutes, last-minute
  changes) mean lineups are often not finalized until match night.
- Auto-creating or pre-populating lineup plans.
- Adding next_week_date to the advance-preview response (the API returns
  next_week_number only; the calendar date is visible on the schedule view).
- A dedicated next-week preparation checklist page.
- Captain-facing lineup submission (requires auth and the online score-entry workflow).

## Week-End Clearance And Recap

Close Week remains the official week-clearance action. There is no separate
`cleared` state at this time; the state model stays `open` and `closed`.

A week may close even when some scheduled matches have no result. Missing
matches are recorded in the recap with `has_result = false` and excluded from
standings and player stats until they are later resolved. The recap panel makes
this visible so an admin can move to the next week without losing visibility
into what was not played.

The week-end recap is available immediately after Close Week succeeds and from
any closed or open week card on the Schedule page. Implemented sections are
marked with the phase that delivered them; remaining items are deferred.

- Match results summary *(Phase A)*
- Missing or no-result matches *(Phase A)*
- Player statistics changes *(Phase C)*
- Handicap changes actually applied *(Phase D2)*
- Warning acknowledgments *(Phase A)*
- Next-week readiness signals *(Phase A)*
- Team record changes *(deferred)*
- Team statistics *(deferred)*
- Handicap recommendation changes *(deferred -- visible in close-week advance preview, not in recap endpoint)*
- Links to official standings and player-stats views from the recap panel *(deferred)*

Handicap application should remain a separate explicit admin step, but it is
part of the week-end recap flow. Updated handicaps should be applied for
completed matches before the next week's scoresheets are printed or used.
Future online scoring may allow processing individual matchups before the full
night is finished, but that requires additional workflow research.

Reopening a week after handicap changes were applied should be allowed only
through an explicit admin action. The exact audit, reversal, and recalculation
policy is deferred until the broader audit/reopen design exists.

## Week-End Recap Phase A -- Read-Only Recap Endpoint (implemented 2026-07-18)

### Goal

Provide a single read-only endpoint that assembles the full week-end recap
without requiring multiple client round-trips. The recap is available
immediately after Close Week and from any closed (or open) week card.

### Endpoint

```
GET /api/seasons/{id}/weeks/{week}/recap
```

- Read-only; no rows are inserted, updated, or deleted.
- Returns 404 when no matches exist for the season/week.
- Returns 200 with the recap object for both open and closed weeks.

### Response shape

```jsonc
{
  "season_id":    3,
  "week_number":  1,
  "status":       "closed",
  "closed_at":    "2026-07-14T10:00:00Z",
  "matches": [
    {
      "match_id":       42,
      "home_team_id":   1,
      "home_team_name": "Team A",
      "away_team_id":   2,
      "away_team_name": "Team B",
      "match_date":     "2026-07-14",
      "has_result":     true,
      "home_sets_won":  3,
      "away_sets_won":  0,
      "home_games_won": 9,
      "away_games_won": 4
    }
  ],
  "missing_count":     0,
  "player_stats": [
    {
      "player_id":   7,
      "player_name": "Jane Doe",
      "team_name":   "Team A",  // omitted when empty
      "sets_won":    2,
      "sets_lost":   1,
      "games_won":   6,
      "games_lost":  3,
      "diff":        1.5
    }
  ],
  "handicap_changes": [
    {
      "player_name":  "Jane Doe",
      "old_handicap": 1.5,
      "new_handicap": 2.0
    }
  ],
  "acknowledgments":   [...],    // same shape as /acknowledgments endpoint
  "next_week_number":  2,
  "next_week":         { ... },  // same shape as advance-preview next_week
  "handicap":          { ... }   // same shape as advance-preview handicap
}
```

`closed_at` is omitted when the week is open. `next_week_number` and `next_week`
are omitted when no further weeks are scheduled. Team IDs are null for unassigned
matches. Team names prefer `season_teams.season_name`; fall back to `teams.name`
for legacy seasons.

### Response sections

| Section | Description |
|---------|-------------|
| `status` / `closed_at` | Week open/closed state and timestamp |
| `matches` | One entry per scheduled match with home/away set and game counts |
| `has_result` | True when `matches.completed = 1` (scores were entered and saved) |
| `missing_count` | Count of match rows where `has_result = false` |
| `player_stats` | Per-player stat totals (sets, games) derived from match_results; empty array when no results recorded |
| `handicap_changes` | Handicap changes applied during this week, matched by `handicap_history.week_number` (added Phase D1); empty array when none recorded or when applies omitted the week number |
| `acknowledgments` | All `week_close_acknowledgments` rows for this week (same as `/acknowledgments` endpoint) |
| `next_week_number` / `next_week` | Next-week readiness signals (same as advance-preview) |
| `handicap` | Handicap method, status, and recommendations (same as advance-preview) |

### Backend placement

`WeekRecap` is a method on `WeekService` (not a separate manager). This is
consistent with `AdvancePreview`, which also assembles a multi-section read-only
response from existing store calls.

`GetWeekRecapData` is a new `WeekStore` method that runs a single GROUP BY
query joining `matches`, `match_results`, `season_teams`, and `teams`. It
aggregates home/away set and game counts per match in one pass.

The response reuses three existing store calls (`WeekMatchCount`,
`GetWeekAdvanceSummary`, `ListAcknowledgments`) and the `HandicapPreviewer`
interface -- no new network requests are needed from the client.

### Files changed

| File | Change |
|------|--------|
| `models/models.go` | `RecapMatchRow`, `WeekRecap` structs |
| `backend/domains/matches/store.go` | `WeekRecapData` type; `GetWeekRecapData` added to `WeekStore` interface |
| `backend/domains/matches/service.go` | `WeekRecap` method on `WeekService` |
| `backend/domains/matches/service_test.go` | `GetWeekRecapData` stub; 4 `WeekService_WeekRecap` tests |
| `backend/storage/sqlite/week_store.go` | `GetWeekRecapData` SQLite implementation |
| `backend/storage/sqlite/week_store_test.go` | 4 `GetWeekRecapData` integration tests |
| `handlers/deps.go` | `WeekRecap` added to `WeekManager` interface |
| `handlers/api.go` | Route `GET /api/seasons/{id}/weeks/{week}/recap`; `recapWeekHandler` |
| `handlers/api_weeks_test.go` | `weekGetRecap` helper; 3 `TestRecapWeek_*` tests |

### Deferred (not in Phase A)

- Player-level stat deltas: implemented in Phase C (RecapPlayerStat, GetWeekPlayerStats)
- Handicap changes actually applied: implemented in Phase D1 (week_number column on handicap_history) and Phase D2 (GetWeekHandicapChanges, recap wiring, Review & Apply deep-link)
- Frontend recap UI: implemented in Phase B (schedules domain, schedule-page-component.js)
- Persisted recap snapshots
- Audit writes for recap views

## Week-End Recap Phase C -- Player Stat Deltas (implemented 2026-07-21)

### Goal

Enrich the week-end recap response with per-player stat totals (sets won/lost,
games won/lost) derived from match_results for the week. No schema changes or
new API routes are required.

### Data source

`match_results` (player_id, team_id, sets_won, sets_lost, games_won, games_lost,
diff) is joined to `matches` (season_id, week_number) via match_id. A GROUP BY
player_id + team_id aggregates multi-match players. Player names come from the
players table; team names prefer season_teams.season_name and fall back to
teams.name for legacy seasons.

### Files changed

| File | Change |
|------|--------|
| `models/models.go` | `RecapPlayerStat` struct added; `WeekRecap.PlayerStats` field added |
| `backend/domains/matches/store.go` | `GetWeekPlayerStats` added to `WeekStore` interface |
| `backend/domains/matches/service.go` | `GetWeekPlayerStats` call and nil-guard in `WeekRecap` |
| `backend/domains/matches/service_test.go` | `GetWeekPlayerStats` stub; 2 `WeekRecap` player-stats tests |
| `backend/storage/sqlite/week_store.go` | `GetWeekPlayerStats` SQLite implementation |
| `backend/storage/sqlite/week_store_test.go` | 2 `GetWeekPlayerStats` integration tests |
| `web/domains/schedules/schedule-page-component.js` | Player-stats table in `#renderRecapPanel` |
| `doc/domains/matches/README.md` | Phase A deferred list updated; Phase C section added |
| `doc/domains/schedules/README.md` | Week-End Recap UI deferred list updated |

### Deferred (not in Phase C)

- Handicap changes actually applied: implemented in Phase D1 and Phase D2 (see below)
- Recap panel accessible from outside the Schedule page
- Print/export of the recap panel

## Week-End Recap Phase D1 -- Handicap History Week Linkage (implemented 2026-07-21)

### Goal

Add a nullable `week_number` column to `handicap_history` so that Apply batches
can be linked to a recap week. Without this column the read path (D2) would have
no way to match apply rows to the week they belong to. No effective-date
heuristic is used; the caller decides when to populate the field.

### Schema change

`handicap_history.week_number INTEGER` -- added via additive migration; NULL for
all pre-D1 rows and for any apply request that omits the field. All rows in one
Apply batch share the same `week_number` value.

### Flow

`applyRequestDTO.WeekNumber *int` (JSON: `week_number`, omitempty) is decoded by
the handler and forwarded through `ApplyRequest.WeekNumber` to
`HandicapHistoryRow.WeekNumber`. The SQLite adapter's 16-column INSERT includes
the column. Server-side inference from effective_date is not performed.

### Files changed

| File | Change |
|------|--------|
| `db/db.go` | `week_number INTEGER` in CREATE TABLE + additive migration |
| `backend/domains/handicaps/store.go` | `WeekNumber *int` on `HandicapHistoryRow` |
| `backend/storage/sqlite/handicap_apply_store.go` | 16-column INSERT includes `week_number` |
| `backend/domains/handicaps/apply.go` | `WeekNumber *int` on `ApplyRequest`; passed to each row |
| `handlers/api.go` | `WeekNumber *int json:"week_number,omitempty"` on `applyRequestDTO` |

## Week-End Recap Phase D2 -- Applied Handicap Changes in Recap (implemented 2026-07-25)

### Goal

Surface applied handicap changes in the week-end recap and wire the Handicap
Review component so that applies from the recap context are linked back to the
recap week.

### Recap read path

`GetWeekHandicapChanges` is a new `WeekStore` method that queries
`handicap_history WHERE season_id = ? AND week_number = ?` ordered by
`player_name_snapshot`. The result is a `[]models.RecapHandicapChange`
(player_name, old_handicap, new_handicap). `WeekService.WeekRecap` calls it and
populates `WeekRecap.HandicapChanges`. An empty array is returned -- never nil --
when no rows exist.

### Apply write path

The `<handicap-review>` Web Component gains a `#weekNumber` private field.
`setWeekContext(weekNum)` (public method) sets it and shows a visible banner:
"Week context: applying from Week N recap. Applied rows will be linked to Week N."
`loadSeason()` and `reset()` clear the field and hide the banner.

When `#weekNumber` is non-null, the Apply request body includes
`"week_number": N`, which flows through the existing D1 path into
`handicap_history.week_number`.

### Schedule recap panel

`#renderHandicapChangesSection` in `schedule-page-component.js` renders the
applied-changes table whenever `season_id` and `week_number` are available.
Empty state shows "No handicap changes have been recorded for Week N yet." with
a "Review & Apply" deep-link button. The button fires
`data-action="open-handicap-for-week"`, which calls the shell bridge
`openHandicapForWeek(seasonId, weekNum)`.

### Shell bridge and <handicaps-page>

`openHandicapForWeek(seasonId, weekNum)` in `app.js` calls `navTo('handicap')`
then delegates to `document.querySelector('handicaps-page')?.openForWeek(...)`.

`<handicaps-page>.openForWeek(seasonId, weekNum)` sets the season selector,
awaits `widget.loadSeason(seasonId)`, then calls `widget.setWeekContext(weekNum)`.
The await ensures `setWeekContext` runs after `loadSeason` clears `#weekNumber`.

### NULL gap

Pre-D1 applies and any apply that omits `week_number` store NULL in
`handicap_history.week_number`. `GetWeekHandicapChanges` never returns these
rows; the section is hidden when `handicap_changes` is empty.

### Files changed

| File | Change |
|------|--------|
| `models/models.go` | `RecapHandicapChange` struct; `WeekRecap.HandicapChanges []RecapHandicapChange` |
| `backend/domains/matches/store.go` | `GetWeekHandicapChanges` added to `WeekStore` interface |
| `backend/domains/matches/service.go` | `WeekRecap` calls `GetWeekHandicapChanges`; nil coerced to empty slice |
| `backend/domains/matches/service_test.go` | stub field, stub method, 2 new `WeekRecap` tests |
| `backend/storage/sqlite/week_store.go` | `GetWeekHandicapChanges` SQLite implementation |
| `handlers/api_weeks_test.go` | 2 new integration tests: matching row present, mismatched week absent |
| `web/domains/handicaps/handicap-review-component.js` | `#weekNumber` field; `setWeekContext()`; week context banner; `loadSeason`/`reset` clear banner; Apply body conditionally includes `week_number` |
| `web/domains/handicaps/handicaps-domain.js` | `openForWeek(seasonId, weekNum)` public method |
| `web/app.js` | `openHandicapForWeek` shell bridge delegates to `<handicaps-page>` |
| `web/domains/schedules/schedule-page-component.js` | `#renderHandicapChangesSection`; `open-handicap-for-week` click delegation |
| `doc/domains/handicaps/README.md` | Phase D1 and D2 decision entries |
| `doc/domains/schedules/README.md` | Recap panel table updated; handicap-changes deferred bullet removed |

### Deferred (not in Phase D2)

- Recap panel accessible from outside the Schedule page
- Print/export of the recap panel
- Persisted recap snapshots

## Weekly Score Processing Phase 1A -- Match-Level Approval/Processing (implemented 2026-08-25)

### Goal

Replace the physical signed-scoresheet process with an admin-attested,
match-level approval and processing state that sits *underneath* Close
Week, so a match's results can start counting toward handicap
recommendation eligibility before every match in its week is complete.
This is backend-only: no frontend buttons or status badges yet, and Close
Week's own behavior is unchanged (see "What Phase 1A deliberately does not
change" below). This closes MATCHES-Q001 at the design level for the
score-entry-to-processed lifecycle; team-side (real captain/player login)
approval remains a separate, deferred question.

### Lifecycle

    scheduled -> scores entered -> approved -> processed -> week closed

`scheduled` and `scores entered` are the existing `completed` field.
`approved` and `processed` are two new, independent match-level states.
`week closed` is the existing `week_closed` field, unchanged in Phase 1A.

### Schema (additive, matches `matches`)

    approved_at          DATETIME   -- NULL = not approved
    approved_by_user_id  INTEGER    -- nullable; admin-attested approval does not require a personal-key user
    approval_note        TEXT NOT NULL DEFAULT ''
    processed_at         DATETIME   -- NULL = not processed
    processed_by_user_id INTEGER    -- nullable, same reason

Non-null `*_at` is the single source of truth for each state (no separate
boolean flag), matching how `seasons.activated_at` already works. Exposed
on `models.Match` (`approved_at`, `approved_by_user_id`, `approval_note`,
`processed_at`, `processed_by_user_id`, all `omitempty`) so a later
frontend phase can render status without a new endpoint.

### New endpoints

    POST /api/matches/{id}/approve     body: {"note": "optional string"}
    POST /api/matches/{id}/process     bodyless
    POST /api/matches/{id}/unapprove   bodyless
    POST /api/matches/{id}/unprocess   bodyless

All four use the same `clearanceAuth` chain as `/results` and `/rounds`
(personal-key Bearer auth; `league_admin`, `admin`, `system_admin` allowed;
`score_keeper` rejected).

### Validation

**Approve:** match must exist (404) -> season not closed (409) -> week not
closed (409) -> match completed (422 `MATCH_NOT_SCORED`) -> not already
processed (409 `MATCH_ALREADY_PROCESSED`). Re-approving an
approved-but-unprocessed match is idempotent (updates note/timestamp).

**Process:** match must exist (404) -> season not closed (409) -> week not
closed (409) -> `approved_at IS NOT NULL` (422 `MATCH_NOT_APPROVED`). Sets
`processed_at`/`processed_by_user_id` only -- does **not** write
`handicap_history` and does not itself change any player's handicap.
Handicap Apply remains the only writer of `handicap_history`; Process only
changes what counts as eligible input data for it (see below).

**Unapprove:** match must exist (404) -> season not closed (409) -> week
not closed (409) -> not already processed (409
`MATCH_ALREADY_PROCESSED` -- unprocess first). Clears `approved_at`,
`approved_by_user_id`, `approval_note`.

**Unprocess:** match must exist (404) -> season not closed (409) -> week
not closed (409). Clears `processed_at`/`processed_by_user_id` only --
approval is deliberately left intact; the admin can separately unapprove
afterward if the correction requires editing scores.

### Score edits blocked after approval/processing

`SaveRounds`, `SubmitResults`, and `ClearResults` now reject with 409 when
the match is approved (`MATCH_APPROVED`) or processed (`MATCH_PROCESSED`),
checked via a shared `checkMatchEditable` helper
(`backend/domains/matches/approval_service.go`). There is no separate
team/admin credential in Phase 1A -- the same personal key that entered
scores also approves them -- so this is a workflow guard against
accidental post-signoff edits, not a non-repudiation guarantee. The
correction path is explicit: unprocess (if processed) -> unapprove (if
approved) -> edit scores -> approve again -> process again.

### Handicap eligibility: Phase 1A compatibility behavior

`backend/storage/sqlite/handicap_store.go`'s `EligibleRacks` and
`ClosedWeekCount` now gate on:

    m.completed = 1 AND (m.processed_at IS NOT NULL OR m.week_closed = 1)

A processed-but-open-week match counts immediately. A legacy closed-week
match with `processed_at` still NULL (every match closed before this phase
existed, and every match closed by today's unchanged Close Week) continues
to count exactly as before -- this OR is what makes Phase 1A additive
rather than a breaking change to existing recommendations. `ClosedWeekCount`
keeps its name despite no longer checking `week_closed` alone, to avoid
renaming its interface method, stub, and every doc reference across two
files for a naming-only concern; see
`doc/domains/handicaps/README.md` for the mirrored explanation there.
`GameDiffAverageRecs` (dead code since the `handicap-preview-parity` phase
removed its only caller) was not touched.

### What Phase 1A deliberately does not change

- Close Week's own validation, warning-acknowledgment, and error behavior
  are completely unchanged. Close Week does not yet require matches to be
  approved/processed, and does not auto-process approved matches. That
  alignment is **Phase 1B**.
- No frontend buttons or status badges. `models.Match` exposes the new
  fields so Phase 1C can render them, but nothing in `web/` reads them yet.
- No real captain/player login approval -- "approved" means
  admin-attested in Phase 1A, full stop.
- Handicap Apply's own mechanism, auth, and tests are untouched.

### Files changed

- `db/db.go` -- five additive columns on `matches`
- `models/models.go` -- five new `Match` fields
- `backend/domains/matches/round_store.go` -- `MatchApprovalState`,
  `GetMatchApprovalState`/`ApproveMatch`/`ProcessMatch`/`UnapproveMatch`/
  `UnprocessMatch` added to the `RoundStore` interface
- `backend/domains/matches/approval_service.go` (new) --
  `RoundService.ApproveMatch`/`ProcessMatch`/`UnapproveMatch`/
  `UnprocessMatch`, `checkMatchEditable`, and the six new error codes
- `backend/domains/matches/round_service.go` -- `checkMatchEditable` wired
  into `SaveRounds`/`SubmitResults`/`ClearResults`
- `backend/storage/sqlite/round_store.go` -- SQL implementations of the
  five new `RoundStore` methods
- `backend/storage/sqlite/match_store.go` -- `matchSelect` and its two scan
  sites (`ListMatches`, `GetMatch`) extended with the five new columns
- `backend/storage/sqlite/handicap_store.go` -- `EligibleRacks`/
  `ClosedWeekCount` compatibility gate
- `handlers/deps.go` -- four new `RoundManager` methods
- `handlers/api_match_results_routes.go` -- four new routes
- `handlers/api_match_results_handlers.go` -- four new handlers,
  `approvingUserID`, `writeMatchApprovalErr`

### Tests

Service-layer (`backend/domains/matches/approval_service_test.go`): every
validation branch for all four actions plus the score-edit-blocked
regressions and a proof that an ordinary, never-approved match is
unaffected. Store-layer
(`backend/storage/sqlite/round_store_test.go`,
`backend/storage/sqlite/handicap_store_test.go`): field persistence,
approval-preserved-after-unprocess, and the eligibility compatibility
matrix (processed-but-open included, legacy-closed-with-null-processed-at
still included, approved-but-unprocessed excluded, plain open excluded).
Handler-level (`handlers/api_match_approval_test.go`,
`handlers/api_match_auth_test.go`): full HTTP round trip for all four
endpoints, the score-edit conflicts, auth gating matching the existing
match-mutation routes, and an end-to-end proof that
`GET /handicap-recommendations` reflects a processed-but-open-week match's
data (`TestHandicapRecommendations_IncludeProcessedButOpenMatch`).

### Deferred (Phase 1B and later)

- ~~Close Week requiring all matches processed, or auto-processing remaining
  approved matches at close time~~ (auto-processing implemented in Phase
  1B below; Close Week requiring approval was considered and explicitly
  not implemented -- see that section)
- Frontend: Match Entry approve/process buttons, Schedule week-card status
  badges, any Week Recap UI change
- Real captain/player login approval (Player Portal)
- Bulk unapprove/unprocess across multiple matches

## Weekly Score Processing Phase 1B -- Close Week Auto-Processing (implemented 2026-08-25)

### Goal

Make Close Week auto-process every match that is approved but not yet
individually processed, so an admin who approved matches throughout the
week doesn't have to click Process on each one separately before closing.
Close Week's own requirements (every match must have a saved game winner)
are otherwise completely unchanged.

### Design decision: no new approval requirement on Close Week

The Phase 1A discovery memo had proposed that Close Week also *require*
every scored match to be approved first (a new `WEEK_MATCH_NOT_APPROVED`
hard error), on top of auto-processing. That combination was implemented
first, then deliberately reverted after it broke roughly 25 existing
Close Week tests whose purpose has nothing to do with approval -- they
exist to test warning acknowledgment, reopen, standings-after-close, and
similar behavior, and all of them seed a scored-but-never-approved match
by design. Requiring approval to close would have been a real, disruptive
behavior change to every admin's existing Close Week workflow for a
requirement PM's actual Phase 1B scope did not clearly ask for (re-reading
the request: "incomplete or unapproved matches should not silently count
as processed" is satisfied by *skipping* them in the auto-process step,
not by blocking the close). Close Week's validation
(`validateWeekData`/`CodeWeekMatchNoScores`) is therefore **unchanged** in
Phase 1B -- confirmed by the fact that `closeweek.go` has no net diff in
this phase's commit.

### Behavior

`WeekStore.CloseWeek` gained a `processedByUserID *int64` parameter and
now returns `(processedCount int, err error)`. Inside the same transaction
that upserts `league_weeks` and sets `matches.week_closed=1`, it also runs:

    UPDATE matches SET processed_at=CURRENT_TIMESTAMP, processed_by_user_id=?
    WHERE season_id=? AND week_number=? AND completed=1
      AND approved_at IS NOT NULL AND processed_at IS NULL

  - A match that is approved but not yet processed gets processed as part
    of the close, atomically with the rest of the close transaction.
  - A match that was never approved (even if scored) is **skipped** --
    it is not silently processed. The week still closes around it; the
    match's handicap eligibility is unaffected because it now qualifies
    through the pre-existing `week_closed = 1` compatibility path from
    Phase 1A instead of the `processed_at` path.
  - A match that is already processed is left untouched (`processed_at`
    is not overwritten with a new timestamp).
  - The `completed=1` clause is defensive -- `approved_at` can only be set
    on a completed match by `ApproveMatch` -- so this statement can never
    process an incomplete match even if that invariant were ever violated
    elsewhere.

`CloseWeekRequest` gained `ProcessedByUserID *int64`; `CloseWeekResult`
gained `ProcessedCount int`. The close-week HTTP handler resolves the
acting personal-key user the same way Phase 1A's approve/process handlers
do (`approvingUserID(r)`) and surfaces `processed_count` in the close
response JSON alongside the existing `acknowledgment_count`.

### Reopen and correction (requirements 5/6)

`ReopenWeek` is completely unchanged -- it only clears `week_closed` and
`league_weeks.status`. Approval and processing state (including anything
Phase 1B auto-processed) survive a reopen untouched. The existing
Phase 1A correction path (unprocess, then unapprove, then edit scores,
then re-approve, then re-process) works exactly the same after a reopen
that followed an auto-process, confirmed by a dedicated regression test.

### Files changed

- `backend/domains/matches/store.go` -- `WeekStore.CloseWeek` signature
- `backend/domains/matches/service.go` -- `CloseWeekRequest.ProcessedByUserID`,
  `CloseWeekResult.ProcessedCount`, pass-through in `WeekService.CloseWeek`
- `backend/domains/matches/service_test.go` -- `stubWeekStore` extended;
  new pass-through test
- `backend/storage/sqlite/week_store.go` -- the auto-process `UPDATE`
  inside `CloseWeek`'s existing transaction
- `backend/storage/sqlite/week_store_test.go` -- three new store tests
  (auto-processes approved, skips unapproved, does not reprocess already-processed)
  plus signature fixes for existing `CloseWeek` call sites
- `handlers/api_week_handlers.go` -- `ProcessedByUserID` wired into the
  request, `processed_count` added to the response
- `handlers/api_match_approval_test.go` -- four new integration tests
  covering the close-time auto-process, the skip-unapproved case, the
  legacy-eligibility-path composition, and the reopen/correction path

### Tests

13 new tests total: 3 SQLite store-level (auto-process, skip-unapproved,
no-reprocess), 1 service-level (actor pass-through and count propagation),
4 handler-level integration tests, plus confirmation that all pre-existing
Close Week, Reopen, and standings/player-stats tests still pass unchanged
-- the closest thing to a full regression guarantee available without a
literal no-op diff.

### Deferred

- Close Week requiring approval before it can close (considered, reverted
  -- see "Design decision" above)
- ~~Any UI surfacing of `processed_count` or an "N matches will be
  auto-processed" preview~~ (implemented in Phase 1C below)
- ~~Frontend approve/process buttons and status badges~~ (implemented in
  Phase 1C below)

## Weekly Score Processing Phase 1C -- Frontend Approval/Processing UI (implemented 2026-08-26)

### Goal

Make the Phase 1A/1B approve/process/unprocess/unapprove workflow visible
and usable in the browser, on top of the existing Admin Key auth path.
UI-only per PM's constraints: no business-rule, route, auth, or schema
behavior changes. (One small API-shape addition was needed for the
closed-week correction below -- `week_closed` is now serialized on
`models.Match`; this is an existing column exposed for the first time,
not a new column, migration, or behavior change.)

### Match Entry page

`web/domains/matches/match-entry-page-component.js`'s scoresheet toolbar
(the same header row that already showed the Completed/Pending and Season
Closed badges) now also shows:

- A status badge: "Approved" (`bg-info`) when `approved_at` is set and
  `processed_at` is not; "Processed" (`bg-primary`) when `processed_at` is
  set. Both read directly from the match fields already returned by
  `GET /api/matches/{id}` since Phase 1A -- no new endpoint needed.
- Action buttons, shown only when valid for the current state (mirroring
  the backend's own guards so an admin is never shown a button that would
  just 409/422): Approve (completed, not yet approved), Process (approved,
  not yet processed), Unprocess (processed), Unapprove (approved, not yet
  processed). All four are additionally hidden when the match's own week
  is closed, not just when the season is closed -- see "Phase 1C
  correction" below.
- Save/Clear are hidden once a match is approved or processed, once its
  week is closed, or once its season is closed
  (`canEditScores = !locked && !isApproved && !isProcessed`, where
  `locked = seasonClosed || weekClosed`), replaced by an inline hint naming
  the exact correction path ("Unprocess, then unapprove, to edit scores
  again" / "Unapprove to edit scores again" / "Reopen the week first, then
  ...") -- this is the "make score-edit blocking understandable"
  requirement. The backend's own 409 conflict message is still the fallback
  for anyone who reaches the API directly.
- No note-entry UI for Approve in this phase -- approve is called with an
  empty note. The backend's `approval_note` field exists and is exercised
  by Phase 1A/1B's own tests; adding a note-entry modal was judged
  unnecessary scope for the first UI pass and was not requested.
- No real captain/player-side approval -- every action is still
  admin-attested, unchanged from Phase 1A.

### Schedule page

`web/domains/schedules/schedule-page-component.js`:

- Each match row in a week card now shows the same Approved/Processed
  badge (reusing the `approved_at`/`processed_at` fields already present
  on the match list response) next to the existing Done/Pending badge.
- The Close Week review modal now shows an `alert-info` note ("N approved
  matches will be auto-processed when this week closes") computed
  client-side from the season's already-fetched match list, filtered to
  the target week -- one additional `fetchSeasonMatches` call added to the
  existing `Promise.all` in `#reviewCloseWeek`, no new backend endpoint.
- The post-close success panel now shows a new "Auto-processed" row using
  the close response's existing `processed_count` field (added in Phase 1B).

### API service layer

Four new functions added to `web/domains/matches/match-entry-api-service.js`
only (`approveMatch`, `processMatch`, `unprocessMatch`, `unapproveMatch`) --
not duplicated into `schedule-api-service.js`, since the Schedule page only
needs to *read* fields already present on data it fetches for other reasons
(the match list, the close response) and does not perform any of these
actions itself. Per PM's constraint against adding shared abstractions
before two real consumers exist with matching behavior, this stayed a
single-domain addition. All four use the shared `api()` client, which
already attaches the Admin Key the same way every other domain's mutations
do -- no new auth wiring.

### Phase 1C correction (2026-08-26): closed-week button gating + wording

PM review found that Match Entry's toolbar gated Approve / Process /
Unprocess / Unapprove (and Save/Clear) on `seasonClosed` only, but the
backend's approval service and `SaveRounds`/`SubmitResults`/`ClearResults`
all independently reject those actions when the match's own **week** is
closed (`IsWeekClosed`), which is a real, common state distinct from the
season being closed. The UI could show a button that would 409.

Fix:

- `models.Match` gained `WeekClosed bool` (`json:"week_closed"`), mirroring
  the existing `matches.week_closed` column that was not previously
  serialized. `backend/storage/sqlite/match_store.go`'s `matchSelect`,
  `ListMatches`, and `GetMatch` now select and scan it. No new column, no
  migration, no route or auth change -- an existing value made visible on
  a response that already exists.
- Three focused tests added in `match_store_test.go`:
  `TestMatchStore_GetMatch_WeekClosedFalseByDefault`,
  `TestMatchStore_GetMatch_WeekClosedTrueAfterSet`,
  `TestMatchStore_ListMatches_WeekClosedReflectsColumn`.
- `match-entry-page-component.js`'s toolbar now computes
  `locked = seasonClosed || weekClosed` and uses `locked` everywhere
  `seasonClosed` alone was previously used to gate `canEditScores`,
  `approveBtn`, `processBtn`, `unprocessBtn`, and `unapproveBtn`.
- A new `alert-warning` hint (`weekClosedHint`) is shown whenever the week
  is closed but the season is not, telling the admin to reopen the week on
  the Schedule page first, before the existing unprocess/unapprove/edit
  correction path applies. A "Week Closed" badge was added next to the
  existing "Season Closed" badge (suppressed when the season badge is
  already showing, since season-closed is the stronger condition).
- Also corrected this section's own wording: it previously said
  "Backend-only per PM's constraints," which was wrong for a frontend UI
  phase. Now reads "UI-only per PM's constraints: no business-rule, route,
  auth, or schema behavior changes," with the `week_closed` exposure
  called out explicitly as a small API-shape addition, not a business-rule
  change.

The Schedule page needed no change for this correction -- it only reads
`approved_at`/`processed_at` for read-only badges and never renders the
approve/process/unprocess/unapprove buttons that trigger the 409.

### Verification

`node --check` on all changed JS files (clean). Go files changed this
round (`models/models.go`, `backend/storage/sqlite/match_store.go`, and
its test file), so `go test ./...` and `go build ./...` were both re-run
and pass, including the three new `WeekClosed` tests. Manually
smoke-tested the full approve -> process -> (blocked edit) -> unprocess ->
unapprove cycle against local dev data via curl (bootstrapping a local
personal-key user the same way staging passes do), confirming the exact
JSON shapes the new UI code reads are actually returned by the running
server -- not a browser test, since none is available in this environment,
but the closest practical substitute. Actual button rendering and click
behavior in a real browser remain **NOT VERIFIED (no browser)**.

### Deferred

- Note-entry UI for Approve
- Bulk approve/process across multiple matches from the Schedule page
  (partially addressed by Weekly Summary Phase 1's client-side "Process
  Approved Scores" loop below -- still no real bulk backend endpoint)
- ~~Any change to Week Recap UI~~ (addressed by Weekly Summary Phase 1
  below -- `RecapMatchRow` now carries approval/processing status)
- Real captain/player-side approval (Player Portal)

## Weekly Summary Phase 1 -- Standalone Weekly Admin Screen (implemented 2026-08-27)

### Goal

Give the league admin one screen to see where a week stands -- match
scoring/approval/processing status, what can be processed now, handicap
changes, and next-week readiness -- without requiring every match in the
week to be complete first. Builds directly on Week Recap and Weekly
Score Processing (both already shipped) rather than introducing a new
aggregate endpoint or a new cross-domain service.

### What Phase 1 added

- `models.RecapMatchRow` gained three fields: `ApprovedAt *string`,
  `ProcessedAt *string`, `WeekClosed bool` -- the same three fields
  already on `models.Match`, now also on each Week Recap match row. This
  is an API-shape addition only (existing columns, no migration, no new
  route) -- the same pattern used for `week_closed`'s addition to
  `models.Match` in Weekly Score Processing Phase 1C.
- `backend/storage/sqlite/week_store.go`'s `GetWeekRecapData` query now
  selects `m.approved_at, m.processed_at, m.week_closed` alongside the
  existing per-match columns and scans them the same way
  `match_store.go` already does for `GET /api/matches`.
- No new endpoint: `GET /api/seasons/{id}/weeks/{week}/recap` is the
  same route, now returning three more fields per match. No new auth --
  it was already an unprotected read.
- New `web/domains/weekly-summary/` frontend domain: a season+week
  selector, a per-match status ladder (Unscored / Scored / Approved /
  Processed / Closed, computed client-side from the three new fields
  plus the existing `has_result`), an incomplete-week-safe note (shown
  whenever `missing_count > 0`, explicitly stating the handicap/stats
  sections reflect data entered so far, not final results), a "Process
  Approved Scores" button, a next-week readiness card (reusing the
  existing `next_week`/`next_week_number` fields verbatim), and a
  handicap section showing both the week's recorded `handicap_changes`
  and the season-wide recommendations preview (`handicap.message` /
  `handicap.recommendations`) already embedded in Week Recap.
- "Process Approved Scores" is a client-side loop over the existing
  `POST /api/matches/{id}/process` endpoint for every row where
  `approved_at` is set and `processed_at`/`week_closed` are not -- no new
  bulk-processing backend endpoint, per PM decision. Each match is
  processed with its own existing validation; a per-match failure is
  toasted individually and does not stop the rest of the loop.
- Close Week itself is untouched and not reachable from this screen
  beyond a status badge (Week Open / Week Closed) and an "Open in
  Schedule" button that dispatches the same `season-nav-request` event
  the Seasons domain already uses to jump to the Schedule page with a
  season preselected -- no Close Week UI or logic was duplicated here.
- Reuses existing global shell functions directly (`openMatchEntry`,
  `openHandicapForWeek`) rather than introducing new cross-domain
  events for those two actions, consistent with how
  `schedule-page-component.js` already calls them.

### Response shape addition

```json
{
  "matches": [
    {
      "match_id": 1,
      "...": "existing fields unchanged",
      "approved_at": "2026-08-27T22:04:53Z",
      "processed_at": null,
      "week_closed": false
    }
  ]
}
```

### What Phase 1 defers

All explicitly out of scope per PM decision, not oversights:

- Substitute creation/lineup workflows (`lineup_plans.is_sub`/
  `sub_for_id` remain read-only in the write path -- unrelated,
  discovered during Weekly Summary discovery, not touched here).
- Payment/financial schema of any kind.
- A real atomic bulk-process-without-closing backend endpoint (V1 uses
  the client-side loop above; worth reconsidering only if it becomes a
  real pain point).
- Auth changes -- Week Recap remains unprotected, same as before.
- `GetPlayerStats`'s `WinPct`-always-zero bug and the league-scoped
  roster-only gap (both already tracked as separate follow-ups from
  Player Overview Phase 1 discovery, unrelated to this phase).
- Schedule undo and other low-severity polish items.

### Verification

`go test ./... -count=1` and `go build ./...` pass, including two new
focused store tests in `backend/storage/sqlite/week_store_test.go`:
`TestWeekStore_GetWeekRecapData_ApprovalFieldsDefaultUnset` (a fresh
unscored match reports no approval fields and `week_closed=false`) and
`TestWeekStore_GetWeekRecapData_ApprovalFieldsReflectMatchState` (two
matches in the same week side by side -- one approved-only, one
approved+processed+closed -- confirming the scan is correct for both
states independently). `node --check` passes on all four changed/new JS
files. Manually verified end to end against a local server build with a
real generated schedule: approved and processed a real match via curl,
confirmed the recap's per-match fields updated correctly at each step,
confirmed `missing_count`, `next_week` readiness (including missing
lineups on every team, since none were seeded), and the season-wide
`handicap.message` (`manual_review`, "No handicap changes are applied
automatically") all rendered exactly as the frontend expects. Actual
browser rendering of the new screen remains **NOT VERIFIED (no
browser)** in this developer's tool session.

## Decision History

### 2026-08-27 - Weekly Summary Phase 1: standalone weekly admin screen

**Status:** `accepted`

Added a new admin-facing "Weekly Summary" screen showing a week's match
status ladder (unscored/scored/approved/processed/closed), a
client-side "Process Approved Scores" action looping the existing
per-match process endpoint, handicap changes/recommendations, and
next-week readiness. No new endpoint or auth -- built entirely on Week
Recap (`GET /api/seasons/{id}/weeks/{week}/recap`), which gained three
API-shape-only fields (`approved_at`, `processed_at`, `week_closed` on
each `RecapMatchRow`). Close Week stays untouched and separate, linked
to only via an "Open in Schedule" button. Substitute workflows, a real
bulk-process backend endpoint, and payment/financial schema all remain
explicitly deferred. See "Weekly Summary Phase 1" above for full detail.

### 2026-08-26 - Weekly Score Processing Phase 1C: frontend approval/processing UI

**Status:** `accepted`

Match Entry now shows approval/processing status and admin action buttons
(approve/process/unprocess/unapprove) gated the same way the backend gates
them, including the closed-week case (see the "Phase 1C correction"
subsection above); Schedule shows the same status per match row plus
`processed_count` after a close and a pre-close "N will auto-process"
note. No business-rule, route, or auth changes -- everything reads fields
already returned by existing endpoints since Phase 1A/1B, plus one small
API-shape addition (`week_closed` now serialized on `models.Match`). See
"Weekly Score Processing Phase 1C" above for full detail.

### 2026-08-25 - Weekly Score Processing Phase 1B: Close Week auto-processes approved matches

**Status:** `accepted`

Close Week now auto-processes every approved-but-unprocessed match as part
of its existing close transaction, so admins don't need to click Process
on each match individually. Close Week's own validation is deliberately
**unchanged** -- a proposal to also require every scored match be approved
before the week could close was implemented, then reverted after it broke
~25 existing tests whose purpose is unrelated to approval, and because
PM's actual scope only asked that unapproved matches not be *silently
processed*, not that they block the close. See "Weekly Score Processing
Phase 1B" above for full detail.

### 2026-08-25 - Weekly Score Processing Phase 1A: match-level approval/processing

**Status:** `accepted`

See the full section above. Summary: added admin-attested `approved_at`/
`processed_at` state on `matches`, four new endpoints, score-edit blocking
after approval/processing, and a compatibility-preserving handicap
eligibility gate (`processed_at IS NOT NULL OR week_closed = 1`). Close
Week itself is unchanged; wiring it to the new states is Phase 1B.

### 2026-07-18 - Week-end clearance uses Close Week plus recap

**Status:** `accepted`

The app keeps a simple week state model: `open` and `closed`. Closing a week can
move operations forward even when some matches have no result, as long as those
matches are recorded in the recap and excluded from standings/player stats until
resolved. Handicap Apply remains an explicit admin step in the recap flow and
should happen before next-week scoresheets are used.

**Update 2026-08-25:** the *week*-level model (`open`/`closed`) is
unchanged, but individual *matches* now also carry independent
`approved`/`processed` state underneath it -- see "Weekly Score Processing
Phase 1A" above. A match can be processed (and count toward handicap
eligibility) before its week closes; Close Week itself still works exactly
as described here until Phase 1B.

### 2026-07-14 - Next-week readiness is informational, not a close blocker

**Status:** `accepted`

Close Week validation checks only the current week's matches. Next-week readiness
(unassigned team slots, missing lineup plans) is reported in advance-preview and
advance-result as an informational signal. The admin fixes gaps from the Schedule
page and match entry screen; the close modal does not block or act on next-week
state. Blocking close on missing lineup plans would prevent legitimate last-minute
substitution workflows.

### 2026-06-08 - Make week close authoritative

**Status:** `accepted`

Score entry stores pending data. Official calculations and result effects are
committed only after backend Close Week validation succeeds.

### 2026-06-08 - Require transparent warning acknowledgment

**Status:** `accepted`

Errors block close. Warnings require explicit, reasoned, audited admin
acknowledgment.
