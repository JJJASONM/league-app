# Matches

## Overview

**Owner:** `matches`
**Status:** `draft`
**Current version:** `0.22`
**Last reviewed:** `2026-10-13`

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

**Update 2026-09-27:** Player Score Approval Phase 1A implements the
backend data model and domain/store primitives this question needs --
per-side team-approval state, optimistic score-revision control,
correction requests, append-only history, and eligibility resolution
(see "Player Score Approval Phase 1A" above). This **advances** the
question; it does not resolve it. Still open: HTTP routes, session/API-key
authorization, any frontend/UI, the read-authorization matrix, and
staging/browser verification -- all planned for Phases 1B/1C/1D.

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
  **Update 2026-09-02:** implemented by Substitute Workflow Phase 1 -- see
  that section below.
- Payment/financial schema of any kind.
- A real atomic bulk-process-without-closing backend endpoint (V1 uses
  the client-side loop above; worth reconsidering only if it becomes a
  real pain point).
- Auth changes -- Week Recap remains unprotected, same as before.
- `GetPlayerStats`'s `WinPct`-always-zero bug and the league-scoped
  roster-only gap (both already tracked as separate follow-ups from
  Player Overview Phase 1 discovery, unrelated to this phase). **Update
  2026-09-01:** both closed by `player-stats-winpct-roster-scope-fix`
  -- `WinPct` was a discovery-time misdiagnosis (already computed
  correctly by `RoundService.GetPlayerStats`), and the league-scoped
  roster/lineup gap was real and is now fixed. See `doc/roadmap.md` for
  full detail.
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

## Substitute Workflow Phase 1 -- Admin Substitute Support (implemented 2026-09-02)

### Goal

Let a league admin replace a planned lineup player with a substitute for a
specific match/week, and have that substitute used consistently in Match
Entry, before approval/processing/week/season locks take over. This is a
working-app V1 item -- last-minute substitutes are a normal part of a real
league night, and score entry was not realistic without this.

### What Phase 1 added

`lineup_plans.is_sub`/`sub_for_id` already existed in the schema but were
read-only in every write path (`SaveTeamLineup` hardcoded `is_sub=0`,
never set `sub_for_id`) -- confirmed sufficient during discovery, so no
schema change was made. A substitution replaces an existing lineup slot in
place: the row's `player_id` becomes the substitute, `sub_for_id` records
the original player, `is_sub` becomes `true`. There is no separate
"substitute" row alongside the original -- doing it this way keeps exactly
one row per slot, matching what Match Entry's lineup auto-fill already
expects (exactly 3 rows per team/week).

- Two new endpoints, both `clearanceAuth`-gated (league_admin/admin/
  system_admin, the same role set every other admin mutation uses):
  - `POST /api/lineup-plans/{id}/substitute` -- body
    `{ substitute_player_id }`. `{id}` is the *lineup_plans row id* being
    substituted, not a player id.
  - `DELETE /api/lineup-plans/{id}/substitute` -- reverts the slot back to
    its original player (`is_sub=false`, `sub_for_id=null`).
- New `LineupStore` methods (`backend/domains/matches/lineup_store.go`,
  SQLite impl `backend/storage/sqlite/lineup_store.go`): `GetLineupPlan`
  (fetch one row by id), `FindMatchID` (look up the match for a
  team/season/week, used for lock checks below), `SetSubstitute`,
  `ClearSubstitute`.
- `LineupService` (`backend/domains/matches/lineup_service.go`) gained a
  second constructor dependency, `MatchLockChecker` -- a narrow,
  purpose-built interface (`IsSeasonClosedForMatch`, `IsWeekClosed`,
  `GetMatchApprovalState`) that `RoundStore` already satisfies
  structurally. Callers pass the *same* `RoundStore` instance already
  constructed for `RoundService` rather than building a second one
  (`matches.NewLineupService(lineupStore, roundStore)` in `main.go` and
  every test-server helper).
- **Lock enforcement (per PM decision, all four required):** a
  substitute change is rejected with `409 Conflict` when the team's match
  for that season/week is season-closed, week-closed, approved, or
  processed -- the exact same lock set `RoundService` enforces for score
  edits, since a substitute swap changes who is credited for a match just
  as much as a score edit would. When no match has been scheduled yet for
  that team/week, there is nothing to lock against and the change is
  allowed. Validation errors (`400`): missing `substitute_player_id`,
  substitute same as the current player, clearing a slot that isn't
  currently substituted. `409 SUB_ALREADY_IN_LINEUP` when the chosen
  substitute is already in this team's lineup under a different slot
  (mapped from the table's own `UNIQUE(season_id, team_id, week_number,
  player_id)` constraint). **Update 2026-09-02 (same-match duplicate-player
  guard):** `409 SUB_PLAYER_ALREADY_IN_MATCH` ("that player is already in
  this match") when the chosen substitute already has a `lineup_plans` row
  for the same season/week under *either* the match's home or away
  `team_id` -- fixes a staging finding where a player could be selected as
  a substitute on one team while already a starter on the other, resulting
  in the same player occupying two slots in one match. Checked via a new
  `LineupStore.PlayerInMatchLineup` method, using the match's home/away
  team IDs resolved from `MatchLockChecker.LoadMatchContext` (a new method
  added to that interface -- `RoundStore` already satisfied it
  structurally, so no new wiring was needed). Not applied to
  `ClearSubstitute`: reverting a slot to the player who already held it
  cannot introduce a new duplicate that wasn't already possible before the
  substitution existed.
- **Match Entry** (`web/domains/matches/match-entry-page-component.js`):
  - Player resolution (existing round results and lineup-plan auto-fill)
    now looks players up against the *full* player list instead of the
    team-filtered roster arrays -- a substitute's `player_id` may not
    carry the team's `team_id`, so the old team-filtered lookups would
    have silently dropped them and broken auto-fill.
  - The manual "Confirm Tonight's Lineup" picker's player dropdowns now
    offer every player in the league (grouped "This Team" /
    "Other Players (Substitute)"), not just the team's own roster, so a
    true substitute can be picked there too.
  - The scoresheet's roster table shows a small "Sub" button next to each
    of the 3 home/away players when that slot has a known `lineup_plans`
    row and scores are still editable. Clicking it opens a single shared
    modal to pick the substitute (from the full player list, *not*
    grouped -- the "This Team" / "Other Players (Substitute)" `<optgroup>`s
    belong only to the manual picker described above) and calls `POST
    .../substitute`, then reloads Match Entry so the scoresheet picks up
    the new player_id, name, and handicap automatically. Once
    substituted, the slot shows a "Sub for X" badge with an "Undo" link
    (calls `DELETE .../substitute`) while scores remain editable.
    **Update 2026-09-02:** the modal's player list now also excludes
    every player already occupying one of the match's 6 slots (both
    teams), not just the current slot's own player -- see the same-match
    duplicate-player guard note above.
  - The Sub/Undo controls are only shown for slots resolved from a saved
    `lineup_plans` row (the common case: a team already has a planned
    lineup). A slot resolved only from already-scored round results (no
    known `lineup_plans` row tracked for it) does not show a Sub control
    in this phase -- see "What Phase 1 defers" below.
- **Weekly Summary:** `GetWeekPlayerStats`
  (`backend/storage/sqlite/week_store.go`) gained a `LEFT JOIN
  lineup_plans` on the same season/team/week/player key the query already
  groups by, plus a `LEFT JOIN players` to resolve `sub_for_id`'s name.
  `models.RecapPlayerStat` gained `IsSub`/`SubForName` (omitempty). This
  makes substitute status available on the existing Week Recap endpoint's
  `player_stats` array without any larger redesign. **Not done:** the
  Weekly Summary frontend does not currently render `player_stats` at all
  (pre-existing -- it never has, since Weekly Summary Phase 1 only shows
  match rows and handicap changes); adding a substitute-status *display*
  would mean building a new player-stats table in that screen, which is
  new UI construction, not "showing status when data is already
  available." Left for a future phase if/when Weekly Summary gets a
  per-player stats section.
- **Player Overview:** verified, not changed. Season-scoped
  `GetPlayerStats` (which Player Overview's stats section already calls)
  joins `match_results` to a player purely by `player_id` with no team
  filter, so a substitute's results from a match played for someone
  else's team already count correctly toward their own totals -- added a
  new regression test
  (`TestRoundStore_GetPlayerStats_SubstitutePlayer_StatsCountTowardOwnTeam`)
  to confirm this explicitly rather than relying on it being true by
  accident. **Known, accepted limitation:** Player Overview's *schedule*
  section resolves matches via the player's own team for the season
  (`season_rosters`/direct `team_id`), so a one-off match they subbed
  for a different team will not appear in their own schedule list. Fixing
  this would mean walking a player's `lineup_plans`/`match_results` across
  every team, not just their own -- exactly the "big player-history
  redesign" this phase was told not to force. Documented, not fixed.

### Routes and auth

| Route | Method | Auth |
|-------|--------|------|
| `/api/lineup-plans/{id}/substitute` | POST | `clearanceAuth` (league_admin/admin/system_admin) |
| `/api/lineup-plans/{id}/substitute` | DELETE | `clearanceAuth` |

The three original `/api/lineup-plans` routes are unchanged (GET
unprotected, POST/DELETE `clearanceAuth`-gated, same as before this
phase).

### Response shape

```json
// POST /api/lineup-plans/{id}/substitute -> models.LineupPlan
{
  "id": 1, "season_id": 1, "team_id": 1, "team_name": "Home Team",
  "player_id": 2, "player_name": "Sub Player", "handicap": 3.0,
  "week_number": 1, "is_sub": true, "sub_for_id": 1
}
```
`DELETE .../substitute` returns the same shape with `player_id` reverted
to the original, `is_sub: false`, `sub_for_id` omitted.

### What Phase 1 defers

All explicitly out of scope per PM decision, not oversights:

- A Sub control for slots resolved only from already-scored round results
  (no known `lineup_plans` row) -- retroactively substituting a played
  match raises different questions (rewriting history vs. recording a
  new entry) this phase does not answer.
- A Weekly Summary UI section displaying the new `is_sub`/`sub_for_name`
  fields (data is available on the API; no display surface exists yet).
- Fixing Player Overview's schedule section to show a substitute's
  one-off matches for other teams (documented limitation, not fixed).
- Automated notifications, player-facing substitute requests, mobile
  flow, full audit/history, substitute payments/fees, broader lineup
  redesign, schedule regeneration, new standings formulas, new finance
  behavior.
- Live re-render of an already-loaded Match Entry lineup on identity/key
  change -- not applicable here (this is an auth-role feature, not an
  identity-gating one), noted only because it mirrors a limitation
  already documented for Player Overview Phase 2's row-button gating.

### Verification

`go test ./... -count=1` and `go build ./...` pass, including: 12 new
`LineupService` unit tests (`backend/domains/matches/lineup_service_test.go`,
using a new `stubMatchLockChecker`) covering validation, all four lock
checks (season closed, week closed, approved, processed), the
no-match-scheduled-yet allow path, the UNIQUE-constraint-to-409 mapping,
and clear-substitute delegation; 8 new SQLite store tests
(`backend/storage/sqlite/lineup_store_test.go`) covering
`GetLineupPlan`/`FindMatchID` found/not-found, `SetSubstitute` (updates
the row, records the original player, rejects a duplicate-in-lineup
substitute), and `ClearSubstitute` (reverts, rejects a slot that was
never substituted); 13 new handler tests
(`handlers/api_lineup_substitute_test.go`) covering 401/403/409/400
across auth, all four locks, and league_admin/admin/system_admin
success, plus set-then-clear round-tripping through real HTTP; 1 new
store test for Weekly Summary's substitute-status join
(`TestWeekStore_GetWeekPlayerStats_ShowsSubstituteStatus`); 1 new store
test for the Player Overview stats verification above. `node --check`
passes on both changed/new JS files
(`web/domains/matches/match-entry-page-component.js`,
`web/domains/matches/match-entry-api-service.js`). Manually verified end
to end against a local server build: saved a real lineup through `POST
/api/lineup-plans`, called the new substitute endpoint, confirmed `GET
/api/lineup-plans` showed the substitute's `player_id`/`is_sub`/
`sub_for_id` correctly, then cleared it and confirmed it reverted
exactly to the original row. Actual browser rendering of the Match Entry
Sub/Undo controls and the roster picker's "Other Players (Substitute)"
group remain **NOT VERIFIED (no browser)** in this developer's tool
session.

## Default Lineup Week-Filter Fix + Setup Checklist Warning (implemented 2026-09-16)

### Goal

Season setup polish discovery (roadmap "Season setup polish -- default
lineup setup") found that a full "Default Lineup" editor already exists
on the Lineups screen (`week_number=0`, used consistently by Dashboard,
Match Entry, and the League Admin hub as the fallback when no
week-specific lineup exists) -- no new screen was needed. It also found a
real, live correctness bug in how that data is read back, and a missing
nudge during season setup. This phase fixes the bug and adds the nudge;
it does not add any new default-lineup screen or UI.

### The bug: `week_number=0` was ambiguous with "no week filter"

`ListLineupPlansRequest`'s doc comment previously said "WeekNumber and
TeamID of 0 mean no filter," and `backend/storage/sqlite/lineup_store.go`
implemented that literally (`if req.WeekNumber != 0 { ... }`). Since
`week_number=0` is also the real, meaningful "Default Lineup" value, a
request for the default lineup silently returned **every week's**
lineup_plans rows for that season once a team had both default and
week-specific rows saved -- exactly the state a "set defaults, override
occasionally" workflow produces. No caller in the codebase (Dashboard,
Match Entry, the League Admin hub, or the Lineups screen itself) ever
relied on "all weeks in one call"; every one always passes an explicit
real week number or `0` for the default. This was confirmed against real
staging data during discovery: a `week_number=0` request for a
5-week season returned 60 rows (`5 weeks x 4 teams x 3 players`) --
exactly every week combined, not a genuine ~12-row default set.

**Fix:** `ListLineupPlans` now always applies `AND lp.week_number = ?` --
the `WeekNumber != 0` guard is removed. `week_number=0` is now an exact,
unambiguous filter for the Default Lineup rows, same as any other week
number. `TeamID` keeps its existing `0` = "no team filter" convention
unchanged -- team ids are never `0`, so that half was never ambiguous.
No schema change, no API-shape change (same query params, same response
fields), and no frontend caller needed to change, since every real caller
already passed an explicit week number and gets the same correct result
now that the filter is honored. This also retroactively fixes Dashboard's
and the League Admin hub's lineup-readiness checks and Match Entry's
week-specific/default fallback resolution, all of which read through this
same method.

### New: non-blocking setup checklist warning

`GET /api/seasons/{id}/checklist` now includes a `TEAM_NO_DEFAULT_LINEUP`
warning (in the existing `warnings` array, using the existing
`ChecklistItem{code, message, team_id}` shape -- no new response field)
for any season team with fewer than 3 default-lineup (`week_number=0`)
rows, distinguishing "no default lineup set" (0 rows) from "an incomplete
default lineup (N/3 players set)" (1-2 rows). This is composed at the
**handler** layer (`appendDefaultLineupWarnings` in
`handlers/api_season_teams_handlers.go`), not inside
`SeasonService.computeChecklist` -- the seasons domain has no dependency
on lineups, matching how Financial/Player Overview/the League Admin hub
already compose cross-domain reads at the handler layer instead of
reaching into another domain's service. `registerSeasonSetupRoutes`
gained a `lineupMgr LineupManager` parameter (optional, nil-guarded, same
convention as `FinanceMgr`) threaded from `deps.LineupMgr` through to
`getSeasonChecklist`; every other route in that function is unaffected.

**This is a warning only.** It is appended to `c.Warnings`, never
`c.Blockers`, after `SeasonService.Checklist` has already computed
`CanActivate` (which depends only on `Blockers`) -- so it cannot affect
season activation. Close Week, schedule generation, and activation still
do not depend on future lineups, per the explicit PM constraint. The
existing Seasons screen checklist renderer (`web/domains/seasons/
seasons-domain.js`'s `#renderChecklist`) already displays any `{code,
message, team_id}` warning item generically, so **no frontend change was
needed** -- the new warning renders automatically alongside the existing
ones (e.g. `TEAM_FEW_PLAYERS`).

### What this phase defers

- No new "Default Lineup" screen -- the existing Lineups screen editor
  is unchanged and sufficient.
- No link/button from the warning to the Lineups screen -- judged not
  tiny enough to include without drifting into UI polish; the warning
  text names the team, and Lineup Planning is already one click away in
  the nav.
- No enforcement of "exactly 3 players" in `SaveTeamLineup` itself
  (write-side stays permissive, same as before) -- only the read-side
  readiness checks (here and elsewhere) require exactly 3.
- No guard on `SetSubstitute`/`ClearSubstitute` against `week_number=0`
  rows -- noted as a low-risk edge case during discovery (no UI exposes
  it today), not addressed here.
- Consolidating the three independent `resolvePlans`-style
  week-specific/default fallback implementations (Dashboard, Match
  Entry, the League Admin hub) into one shared helper -- a real
  code-quality opportunity, not part of this phase's scope.
- Any auth/roles/session change. Any schema change.

### Verification

`go test ./... -count=1` and `go build ./...` pass, including: 1 new
SQLite store test guarding the exact bug
(`TestLineupStore_ListLineupPlans_WeekZeroNotMixedWithOtherWeeks` --
seeds week-0 and week-1 rows for the same season/team, confirms each
`WeekNumber` filter returns only its own week); 2 existing store tests
(`TestLineupStore_ListLineupPlans_BySeason`,
`TestLineupStore_ListLineupPlans_ByTeam`) updated to pass an explicit
`WeekNumber`, since they previously (and unintentionally) relied on the
now-removed "no filter" behavior; 1 new handler test
(`TestSeasonChecklist_DefaultLineupWarning_AppearsThenClears` --
confirms the warning appears for a team with no default lineup, confirms
it is never present in `blockers`, saves a full 3-player default lineup,
confirms the warning disappears, and confirms `can_activate` is
unaffected by the lineup-state change in either direction). All four
pre-existing checklist tests (`TestSeasonChecklist_LegacySeason_
CanActivate`, `_ManagedNoTeams_BlocksTooFew`, `_TwoTeamsNoSchedule_
Blocked`, `_AllGood_CanActivate`) continue to pass unchanged -- none of
them assert an exact `warnings` array, only `blockers`/`can_activate`,
so the new warning does not disturb them. Manual/staging verification
against a disposable season remains **NOT VERIFIED** in this developer's
tool session pending a staging pass.

## Player Score Approval Phase 1A -- Data Model and Backend Foundation (implemented 2026-09-27)

### Business status and confirmed rules

This phase begins replacing the physical signed scoresheet with online
player approval, advancing `MATCHES-Q002` -- it does **not** resolve it.
Routes, authorization, session enforcement, and any UI are explicitly
Phase 1B/1C/1D work, not part of this phase. Confirmed PM product rules
this phase implements at the data-model and domain-service level:

- A match ultimately requires one approval from each team side (home and
  away), independent of the existing admin-attested approval.
- Approval is not captain-only.
- A player is eligible to act for a team side when either they are
  currently rostered to that team for the match's season
  (`season_rosters`), or they actually participated for that side in the
  match (`round_results`), including as a substitute. Either condition
  alone is sufficient.
- Player records never require a user account; a linked account is only
  required for the authenticated player actions Phase 1B adds.
- Each side has exactly one current state: `pending`, `approved`, or
  `correction_requested`. A correction request requires a note and cannot
  simultaneously count as approval for that side.
- Any score, participating-player, or substitute change affecting the
  scoresheet increments the match's score revision, resets both sides back
  to `pending`, and records history for each non-pending side cleared.
  Reopening a week, by itself, does **not** clear team approvals -- only an
  actual scoresheet-affecting mutation does.
- Team approvals and correction requests always apply to one exact score
  revision.
- The existing admin-attested `approved_at` (acceptance), `processed_at`
  (handicap-eligibility/processing), and `week_closed` states are
  completely unchanged in meaning and behavior by this phase.
- No captain role is introduced.

### Schema (additive)

Fifteen new columns on `matches`, plus a new `match_approval_events`
table (see `db/db.go`):

```
score_revision                     INTEGER NOT NULL DEFAULT 0

home_approval_state                TEXT NOT NULL DEFAULT 'pending'
                                    CHECK (IN 'pending'|'approved'|'correction_requested')
home_approval_state_at             DATETIME
home_approval_actor_user_id        INTEGER   -- no FK
home_approval_actor_player_id      INTEGER   -- no FK, repointed on player merge
home_approval_actor_name_snapshot  TEXT NOT NULL DEFAULT ''
home_approval_state_score_revision INTEGER   -- set for 'approved' AND 'correction_requested'
home_correction_note               TEXT NOT NULL DEFAULT ''

away_approval_state / away_approval_state_at / away_approval_actor_user_id /
away_approval_actor_player_id / away_approval_actor_name_snapshot /
away_approval_state_score_revision / away_correction_note   -- mirror of the above
```

**PM correction (2026-09-28):** the revision field is named
`*_approval_state_score_revision`, not `*_approval_score_revision` --
both `approved` and `correction_requested` are states that describe one
exact revision, so the field is generic to "state," not "approval"
specifically. `RequestCorrection` sets it to the revision being
disputed, exactly like `ApproveTeamSide` sets it to the revision being
approved; only `pending` clears it to NULL. The Go field is
`TeamSideApprovalState.StateScoreRevision`.

`home`/`away` naming (rather than `approval`-only naming) is deliberate:
both `approved` and `correction_requested` have an actor, so the actor/
note/revision fields describe the side's current state generically, not
an approval specifically. Flat columns on `matches` were chosen over a
normalized `(match_id, team_side)` current-state table because the
cardinality is fixed at exactly two forever, and every other piece of
this domain's match-level lifecycle state (`completed`, `week_closed`,
`approved_at`, `processed_at`) already lives directly on `matches` --
introducing a side table here would be the only inconsistent case.

```sql
CREATE TABLE match_approval_events (
    id, match_id REFERENCES matches(id) ON DELETE CASCADE,
    event_scope TEXT CHECK (IN 'home'|'away'|'match'),
    event_type  TEXT CHECK (IN 'approved'|'withdrawn'|'correction_requested'|
                              'cleared_by_edit'|'admin_accepted'|'admin_override'),
    score_revision INTEGER NOT NULL,
    actor_user_id INTEGER,      -- no FK
    actor_player_id INTEGER,    -- no FK, NEVER repointed by merge (see below)
    actor_name_snapshot TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    home_state_snapshot / home_note_snapshot / away_state_snapshot / away_note_snapshot,
    created_at
)
```

`event_scope = 'match'` is used only for admin-level `admin_accepted`/
`admin_override` events -- an admin's acceptance decision does not belong
to either side alone, so it is never forced into a `home`/`away` scope.
The four `*_snapshot` columns give those two event types a structured,
typed record of both sides' state and note at the moment of the admin's
decision, rather than serializing that into the free-text `note` column.

### Score revision and stale-write purpose

Multi-user entry (a player and an admin, or two players, potentially
editing concurrently) makes stale writes a real risk this feature
introduces, unlike the existing admin-only Match Entry flow. `score_revision`
is the authoritative counter: every successful scoresheet-affecting
mutation increments it, atomically, in the same transaction as the
mutation. A team-side approval, withdrawal, or correction request always
names the exact revision it applies to and is rejected (with zero writes)
if that revision no longer matches. **Existing admin score-entry clients
need no changes**: `SaveRounds`/`SubmitResults`/`ClearResults` keep their
exact existing signatures and behavior when no expected revision is
supplied (the unmodified admin UI never supplies one); new parallel
methods (`SaveRoundsWithRevision`, `SubmitResultsWithRevision`,
`ClearResultsWithRevision`) return the resulting revision for Phase 1B's
player-facing writer, which will always be required to supply one.

**PM correction (2026-09-28) -- the enforcement point moved from a
pre-transaction check to a transaction-scoped conditional write.** The
original implementation validated season/week/approved/processed state
via a read BEFORE opening the transaction that performed the score
write -- a real race window, since any of that state (including a
concurrent admin acceptance or week close, neither of which touches
`score_revision`) could change between the read and the write. The
corrected design moves this entirely inside the transaction: see
"Atomic clearing" below for `AdvanceScoreRevisionForEdit`, the single
conditional `UPDATE` that is now the sole authority for this decision.
`checkMatchEditable`, the old pre-check, is removed.

### Team-side current states and correction requests

State lives in three explicit, schema-enforced values -- never inferred
from timestamp presence, since `correction_requested` cannot be
represented unambiguously that way. `pending` is the default and the
state a side returns to whenever the scoresheet changes. `approved`
records who approved and for which revision. `correction_requested`
requires a non-blank note (enforced in the domain service,
`RequestCorrection`), supersedes an existing approval for that **same**
side only (the other side's state and revision are untouched), and
blocks that side from simultaneously counting as approved. Any currently
eligible player for a side -- not only the original approver -- may
withdraw that side's approval or file a correction request; the approval
belongs to the team side, not to one individual, which keeps the rule
understandable for a real team sharing one decision.

### Append-only history

`match_approval_events` never rewrites or deletes a row (matches'
`ON DELETE CASCADE` only fires when the whole match is deleted). Every
approve, withdraw, correction request, edit-triggered clear, and admin
accept/override writes exactly one event. `actor_user_id`/
`actor_player_id` carry no foreign key -- this schema's established
attribution convention (`matches.approved_by_user_id`,
`handicap_history.applied_by_user_id`) -- so a historical event can never
block a player merge or a future account removal.
`actor_name_snapshot` keeps history human-readable regardless of later
renames or merges, the same purpose `handicap_history.player_name_snapshot`
serves; like that column, it is deliberately never rewritten by a merge --
it is a record of who acted at the time, not a live label.

### Eligibility

`RoundStore.ResolveApprovalEligibility(matchID, side, playerID)` returns
true when either: the player is on `season_rosters` for that side's team
for the match's season, or the player appears as `home_player_id`/
`away_player_id` in any `round_results` row for that match on that side.
This inclusive-OR (not a fallback order) directly satisfies "not
captain-only," handles a genuine last-minute substitute with no roster
row automatically, and remains correct for a player who changed teams
mid-season (still eligible for a match they actually played, via the
round-participant branch, even after their roster row moves).

### Atomic clearing and atomic lock enforcement

**PM correction (2026-09-28):** this section describes the corrected,
fully atomic design -- the original `bumpScoreRevisionAndClearApprovals`
only bumped/cleared; it never rejected a locked match. It is replaced by
`AdvanceScoreRevisionForEdit` (`backend/storage/sqlite/round_store.go`),
a single conditional `UPDATE` that is both the atomic lock-enforcement
gate AND the revision-advance/approval-clear step:

```sql
UPDATE matches SET score_revision = score_revision + 1
WHERE id = ? AND (? IS NULL OR score_revision = ?)
  AND approved_at IS NULL AND processed_at IS NULL AND week_closed = 0
  AND EXISTS (SELECT 1 FROM seasons se WHERE se.id = matches.season_id AND se.closed_at IS NULL)
```

Called as the **first statement** inside the same transaction as the
destructive mutation that follows it -- `SaveRounds` (service-level
`RunTx`), `SubmitMatchResults`/`ClearMatchResults` (the guard is now
acquired by the *service* layer's outer `RunTx`, wrapping the store
method, which no longer touches `score_revision` itself), and the
SQLite `LineupStore`'s `SetSubstitute`/`ClearSubstitute` (the guard runs
**before** the `lineup_plans` update, so a locked match's substitute
change is rejected before that row is ever touched, and rolls back
cleanly if it was). If the `UPDATE` matches zero rows, a read-only
follow-up query (`loadMatchGuardSnapshot`) classifies exactly why --
match not found, stale revision, season closed, week closed, admin-
accepted, or processed -- and the sentinel error is returned with zero
writes; if later validation or persistence in the same transaction
fails for any other reason, the transaction's rollback undoes the
revision advance and approval-clear along with everything else. On
success, it increments `score_revision`, resets any side that is not
already `pending` back to `pending`, and inserts one `cleared_by_edit`
event per side actually reset (a side already `pending` generates no
event).

`ApproveTeamSide`/`WithdrawTeamApproval`/`RequestCorrection` use the
same pattern for their own conditional `UPDATE`s, additionally
re-verifying atomically, at write time: match exists, expected revision
matches, `completed=1` (Approve/RequestCorrection only), season open,
week open, not admin-accepted, not processed, and (Withdraw only) the
side's current state is still `approved`. **This closes the gap the
original design left**: since admin acceptance and week close do not
themselves change `score_revision`, a team action racing either of
those could previously commit after the match should already have been
locked -- the conditional `UPDATE`'s own `WHERE` clause is now what
prevents that, not a service-layer pre-check (which is removed).

### Merge and attribution behavior

The existing safe player-merge transaction
(`backend/storage/sqlite/player_store.go`'s `mergeRepointStmts`) gained
two entries repointing `matches.home_approval_actor_player_id` /
`away_approval_actor_player_id` -- current, live attribution follows the
surviving player exactly like `round_results`/`season_rosters` already
do. `match_approval_events.actor_player_id` is deliberately **not**
repointed and carries no foreign key, so a merge (or a future account
deletion) can never fail because of historical approval history; that
history stays attributed to the pre-merge identity, readable via its own
`actor_name_snapshot`, exactly as history is expected to work.

### Existing admin compatibility and the atomic admin-acceptance primitive

`POST /api/matches/{id}/approve` and its `RoundService.ApproveMatch`
method are **completely unchanged** -- an admin can still accept a match
with zero team approvals, exactly as before. A new, separate method,
`RoundService.AdminAcceptMatch(ctx, matchID, approvedByUserID, note,
override, expectedRevision)`, implements the Phase 1B-ready accept-or-
override primitive; it is not called from any route yet.

**PM correction (2026-09-28):** the original version read team states
and revision, then wrote the event and called the existing (unconditional)
`ApproveMatch` store method -- with no re-verification at write time, so
a race between the read and the write (a score edit, a team-state
change, a process, or a week/season close) could let a decision based on
stale data still commit. The corrected design adds a required
`expectedRevision` parameter and a new store method,
`RoundStore.GuardedAdminAccept(matchID, expectedRevision, pinnedHome,
pinnedAway, approvedByUserID, note)`, whose conditional `UPDATE` pins
the exact revision and the exact home/away states the accept-vs-
override decision was based on, alongside the same match-editable
conditions `AdvanceScoreRevisionForEdit` re-verifies.

**Second PM correction (2026-09-28, same day):** the first correction's
`pinnedHome`/`pinnedAway` only pinned the state **label** (`approved`/
`correction_requested`/`pending`), which is not the complete decision
snapshot -- a correction request can be rewritten at the same score
revision, with the same label, but a different note, and a label-only
guard cannot detect that. The corrected version pins and atomically
re-verifies each side's **complete** snapshot: `pinnedHome`/`pinnedAway`
are now full `TeamSideApprovalState` values (state label, state-score-
revision -- including a `nil` state-score-revision, compared NULL-safely
via SQL's `IS` operator -- and correction note), and `AdminAcceptMatch`'s
`bothApproved` decision itself now uses the same `sideApprovedAtRevision`
rule `BothSidesApproved` already applied: a side counts as approved only
when its label is `approved` **and** its state-score-revision is
non-nil **and** equals `expectedRevision`. A side labeled `approved`
with a nil or stale state-score-revision no longer lets normal
acceptance proceed -- override is required, exactly as if that side had
never approved. New code `APPROVAL_TEAM_STATE_CHANGED` now also fires
when the pinned note or state-score-revision (not just the label) no
longer matches. The event recorded after a successful guarded write uses
these SAME pinned values, so the guarded write and its event always
describe one consistent snapshot.

`AdminAcceptMatch` now: reads state and decides normal-accept vs.
override-required from the corrected `bothApproved` rule (fast-fail,
informs the decision only) -> calls `GuardedAdminAccept`, which
re-verifies the complete snapshot atomically and performs the
`approved_at`/`approved_by_user_id`/`approval_note` write only if
nothing has changed -> on success, records the `admin_accepted` or
`admin_override` event (built from the same pinned snapshot) in the
same transaction. A stale revision, a team-state change, or a
lock-state change between the decision and the write is rejected with
zero writes; a failure at either the guarded write or the event write
rolls back the other, since both happen in the one transaction
`RoundService.AdminAcceptMatch` opens via `RunTx`.

### Explicit Phase 1A exclusions

No HTTP routes, no `auth.Action`/scope resolvers, no session/API-key
enforcement, no frontend or UI change, no captain role, no notifications,
no mobile work, no handicap formula change, and no fix for the
pre-existing, unrelated `GameDiffAverageRecs` processed-open-week
eligibility inconsistency (recommended separately as its own future
correctness candidate, not part of this phase).

### Phase 1B status

Implemented 2026-09-30 -- see "Player Score Approval Phase 1B" below for
the full routes/authorization/read-matrix design. Admin/UI integration
(Phase 1D), player UI (Phase 1C), and staging verification remain for
`MATCHES-Q002`'s full resolution.

### Verification

`go build ./...` and `go test ./... -count=1` both pass with zero
regressions to any existing test, including every Weekly Score
Processing Phase 1A/1B/1C test and every Substitute Workflow Phase 1
test. `go vet ./...` was run and is **not** fully clean -- it reports
four pre-existing warnings in unchanged handler test files
(`handlers/api_season_teams_test.go:455,1216`,
`handlers/api_weeks_test.go:1271,1313`), all "using ... before checking
for errors" findings that predate this branch and are outside this
phase's scope. This branch introduces no new `go vet` finding. New
tests: schema/migration (fresh-DB columns and defaults, CHECK
constraint rejection, pre-existing-DB upgrade without data loss,
idempotent re-init, cascade delete, no-FK-blocker on player delete);
eligibility (roster and round-participant branches, opposing/unrelated/
cross-season/cross-league rejection); revision and transitions (current
vs. stale revision, invalid transition, note-required, correction
retaining its revision, any-eligible-teammate withdrawal); atomic
clearing (`SaveRounds`, `SubmitResults`, `ClearResults`, substitute swap
on a scored vs. unscored match, no redundant clear events); history
(event content and match-scope snapshot fields, cascade on match
delete); merge (active attribution repointed, historical events and
name snapshots left untouched); and compatibility (`ApproveMatch` still
works with zero team approvals).

**PM correction round (2026-09-28), atomicity regression tests added:**
real-`RunTx`, real-SQLite tests (not stubs) proving the conditional-
write guards actually reject a race, not just a value passed in already
stale before the call begins -- `SaveRounds` rejected atomically when the
season is closed, the week is closed, the match is admin-accepted, or
the match is processed, each with `score_revision` left unchanged;
`ApproveTeamSide`/`WithdrawTeamApproval`/`RequestCorrection` rejected
under the same four lock conditions with zero events written;
`GuardedAdminAccept` rejected on a stale revision, on a team-state
change between the decision and the write, and on a lock-state change,
each with zero writes, plus two positive tests proving a successful
accept-or-override commits the acceptance and its event together;
`SetSubstitute`/`ClearSubstitute` rejected under all four lock
conditions with the `lineup_plans` row proven left exactly as it was
(not partially updated) via a full roll-back check.

**Second PM correction round (2026-09-28), same day:** 2 new real-
SQLite tests prove the admin guard now catches a state-score-revision
or correction-note change independent of the label
(`TestGuardedAdminAccept_TeamStateRevisionChanged_RealGuard_NoWrites`,
`TestGuardedAdminAccept_CorrectionNoteChanged_RealGuard_NoWrites`,
constructed via direct SQL since neither scenario is reachable through
the public write API in one step); 3 new stub-based service tests prove
`bothApproved`'s corrected decision rule
(`TestAdminAcceptMatch_ApprovedLabelWithNilRevision_RequiresOverride`,
`_ApprovedLabelWithStaleRevision_RequiresOverride`,
`_BothApprovedAtNonZeroRevision_Succeeds`); and 13 new
`LineupService`-level tests prove every guard sentinel
(`ErrGuardSeasonClosed`/`WeekClosed`/`Processed`/`AdminAccepted`/
`ErrApprovalRevisionStale`/`ErrGuardMatchNotFound`) becomes the correct
`domainerr.Conflict` (409) for both `SetSubstitute` and `ClearSubstitute`,
while an unrelated genuine storage error still falls through to the
existing generic `Internal` (500) mapping unchanged. Staging deployment
and browser verification are out of scope for this backend-only phase
and remain for Phase 1D.

## Player Score Approval Phase 1B -- Routes, Session Authorization, and Protected Reads (implemented 2026-09-30)

Phase 1B makes Phase 1A's data model and atomic guards reachable through
HTTP with the correct authorization, without adding any frontend approval
UI (that remains Phase 1C) or admin UI integration (Phase 1D). Status:
`accepted`.

### Authentication method

`auth.Identity` gained an `AuthMethod` field (`AuthMethodSession`,
`AuthMethodAPIKey`, `AuthMethodStaticToken`) -- a stable controlled code
naming the credential that actually authenticated the request, not who the
identity is. `handlers.resolveIdentity` sets it in both branches
(`AuthMethodSession` on a resolved session cookie, `AuthMethodAPIKey`
inside `buildIdentityFromLegacyUser` for a resolved Bearer key).
`AuthMethodStaticToken` is defined for completeness but never actually
attached to an `Identity` -- the static `LEAGUE_ADMIN_TOKEN` bootstrap
credential remains a genuinely separate bypass (`staticTokenBypass`) that
never becomes an `Identity`, unchanged from before this phase. Credential
precedence (session beats a stored Bearer key) is unchanged.

### New `auth.Action`s

- `ActionMatchTeamApproval` -- guards the four player-scoped team-approval
  mutation routes below. `Authorize`'s case checks only the credential
  shape: `identity.AuthMethod == AuthMethodSession && identity.IsPlayer()`.
  There is deliberately no scope/eligibility check inside `Authorize`
  itself (per-match, per-side eligibility is a domain fact
  (`ResolveApprovalEligibility`) a pure, database-free policy function
  cannot evaluate), and **no admin bypass of any kind, not even the
  otherwise-unconditional `system_admin` shortcut every other action
  gets** -- `Authorize` evaluates this action BEFORE that shortcut (PM
  correction: an earlier version let `system_admin` through first, so a
  system-admin identity could approve a team side without ever being a
  real, session-authenticated, eligible player). A `system_admin` (or any
  other identity) may use these routes only by being signed in through an
  actual browser session AND holding a linked, eligible player -- at that
  point they are authorized as that player, not as an administrator. An
  administrator who needs to act without both team approvals uses
  `AdminAcceptMatch`'s own override contract on the existing acceptance
  route instead of a masquerade path onto these player-only routes.
- `ActionMatchApprovalRead` -- guards the protected approval snapshot/
  history reads and retrofits the pre-existing public match/round routes'
  draft-detail gate (see Read-authorization matrix below). League-scoped
  exactly like `ActionMatchApproval`/`ActionMatchScoreMutate`
  (`scope.LeagueID != nil && identity.IsLeagueAdminFor(*scope.LeagueID)`).

Eligibility itself (season-roster membership or round-participation on the
requested side) is resolved via `RoundService.ResolveApprovalEligibility`
at the HTTP layer, in new purpose-built gates
(`handlers/api_match_approval_auth.go`) rather than inside `auth.Authorize`
or duplicated in a handler -- see that file's doc comments for the full
reasoning on why these gates could not reuse the existing
`scopeFn`-based `requireAction` pattern unmodified (the existing `scopeFn`
signature has no way to see the caller's own identity during scope
resolution, which per-match eligibility genuinely requires).

### Team-approval routes

All four require `expected_revision` and are exclusively player-scoped
(gated by `requireMatchTeamApprovalAction`: an active password session
with a linked player; a personal API key or the static token is always
rejected, `403`, even when the key's user has a linked player). Each
returns the resulting `MatchApprovalSnapshot` (see below) on success, and
delegates ALL eligibility/transition/lock logic to the unchanged Phase 1A
service methods -- handlers never re-implement it.

| Route | Body | Notes |
|---|---|---|
| `POST /api/matches/{id}/approve-home` | `{"expected_revision": N}` | side fixed by the route |
| `POST /api/matches/{id}/approve-away` | `{"expected_revision": N}` | side fixed by the route |
| `POST /api/matches/{id}/withdraw-approval` | `{"side": "home"\|"away", "expected_revision": N}` | side not in the path |
| `POST /api/matches/{id}/request-correction` | `{"side": "home"\|"away", "expected_revision": N, "note": "..."}` | note required, nonblank |

Status mapping (via `writeMatchApprovalErr`, extended this phase with an
`InvalidInput` -> 400 case it was previously missing -- see "Collateral
fixes" below): invalid body/side/note -> `400`; unauthenticated -> `401`;
wrong authentication method or no linked player -> `403`; match not
found -> `404`; stale revision, invalid transition, or a locked match
(season/week closed, processed, admin-accepted) -> `409`; ineligible
player or an unscored match -> `422` (the existing Phase 1A domain status,
preserved as-is per the routes/auth design's explicit instruction not to
convert a domain status into an authorization code).

### `MatchApprovalSnapshot` response contract

Every team-approval mutation route and `GET /api/matches/{id}/approval`
return this shape (`models.MatchApprovalSnapshot` /
`models.TeamSideApprovalSnapshot`):

```json
{
  "match_id": 42,
  "score_revision": 0,
  "completed": true,
  "home": {
    "state": "approved",
    "state_at": "2026-09-30T12:00:00Z",
    "actor_player_id": 7,
    "actor_name_snapshot": "Home Player",
    "state_score_revision": 0
  },
  "away": { "state": "pending" }
}
```

Every field below the top level, plus `approved_at`/`processed_at`, is
`omitempty` -- a nil `*string`/`*int64`/`*int` or a zero-value `string` is
OMITTED from the response entirely, never serialized as `null` or `""`.
The example above reflects that exactly: `approved_at`/`processed_at` are
absent (the match has not been admin-accepted), and `home.correction_note`
is absent (empty). `away` shows only `{"state": "pending"}` because every
other field on a never-touched side is either a nil pointer or an empty
string. `ActorUserID` is deliberately not part of this shape at all (an
internal id with no display value to a player-facing client) --
`ActorPlayerID`/`ActorNameSnapshot` are what a UI needs. This response is
never served through any public route.

### Admin acceptance contract

`POST /api/matches/{id}/approve` is rewired onto `AdminAcceptMatch`. This
is a deliberate, **breaking** contract change for this one route:

```json
{ "note": "optional", "override": false, "expected_revision": 0 }
```

`expected_revision` is now **mandatory** (`AdminAcceptMatch` has no
optional-revision variant, unlike the score-write routes below) -- a
missing value returns `400`. Behavior: both sides approved at the current
revision -> normal acceptance succeeds, records `admin_accepted`; one or
both missing/stale -> `override:false` (or omitted) returns `409`
`APPROVAL_OVERRIDE_REQUIRED`, `override:true` succeeds and records
`admin_override`; a stale `expected_revision` or a team-state/note change
since the decision was read -> `409` with zero writes (unchanged Phase 1A
atomic guard). Authorization is unchanged (`ActionMatchApproval`,
admin-only, scoped via match -> season -> league).

**The existing frontend Approve button does not understand this new
contract yet** (it posts only `{"note": "..."}"`, with no
`expected_revision`) -- admin UI integration is Phase 1D. This backend
branch should not be deployed alone if that would interrupt the current
button; see Roadmap.

### Player-authenticated score writes

`POST /api/matches/{id}/rounds`, `POST /api/matches/{id}/results`, and
`DELETE /api/matches/{id}/results` now accept EITHER the unchanged admin
path (`ActionMatchScoreMutate`, `expected_revision` remains optional, same
response shape plus one additive `score_revision` field the existing
admin frontend does not read) OR an eligible linked player's password
session for **at least one side** of the match (not necessarily their own
side -- a player entering the full scoresheet needs the match-wide
version of eligibility, not the single-side version team-approval actions
use). For a player-authenticated write, `expected_revision` is mandatory
(`400` if absent); a stale value returns `409` with zero writes. Every
successful write returns `score_revision` in its response body
(`{"status":"saved","score_revision":N}` /
`{"saved":N,"score_revision":N}`). The gate
(`requireAdminOrEligiblePlayerScoreWrite`) tries the admin authorization
first (unchanged `auth.Authorize` call) and only evaluates player
eligibility if that fails -- exactly one path is ever taken per request.
Any accepted score mutation clears stale team approvals via Phase 1A's
existing atomic guard, unchanged by this phase.

### Read-authorization matrix

| Caller | Public schedule (`GET /api/matches[/{id}]`) | Draft/in-progress rounds & results | Final (`completed && week_closed`) rounds | Approval snapshot/history (`GET .../approval[-events]`) |
|---|---|---|---|---|
| Unauthenticated | Yes (schedule fields only) | No | Yes | No |
| Unrelated/cross-league player | Yes (schedule fields only) | No | Yes | No |
| Eligible linked player (session) | Yes (schedule fields only in the LIST route; full in the single-match detail route -- see below) | Yes, for their own match | Yes | Yes, for their own match |
| Scoped league_admin / system_admin | Yes, including approval metadata | Yes | Yes | Yes |

Admin-attested approval metadata (`approved_by_user_id`, `approval_note`,
`processed_by_user_id`) was already leaking to every public caller via
`GET /api/matches` and `GET /api/matches/{id}` **before this phase** (a
real pre-existing gap from Weekly Score Processing Phase 1A, not something
Phase 1B introduces) -- both routes now strip these fields unless the
caller is authorized. Two deliberately different precision levels, as the
smallest consistent contract for each route's existing shape:

- **`GET /api/matches/{id}`** (single match): precise per-match check --
  admin scoped to the match's own league, OR an eligible session player
  for either side. Two INDEPENDENT visibility rules apply (PM correction:
  an earlier version stripped both together, guarded only by "not yet
  final," which let a final closed match leak approval metadata to every
  public caller):
  - `Results` (draft score content) is emptied unless the match is
    already final (`completed && week_closed`) OR the caller is
    authorized.
  - Approval metadata is stripped UNCONDITIONALLY unless the caller is
    authorized, regardless of whether the match is final -- a closed
    match's final results are public; who approved it, and any
    correction note, are never public.
  Schedule fields (teams, date, week, `completed`) are never hidden.
- **`GET /api/matches`** (list, potentially spanning leagues): scoped to
  the request's EFFECTIVE `season_id`/`league_id` filter, mirroring
  `MatchStore.ListMatches`'s own precedence (`season_id` wins when both
  are supplied; neither means unfiltered) -- `system_admin` sees metadata
  unconditionally; a `league_admin` sees it only when the effective filter
  resolves to a league they administer; an unfiltered request exposes
  metadata to nobody but `system_admin` (PM correction: an earlier version
  granted ANY authenticated `league_admin` visibility into every league's
  metadata on an unfiltered request, including leagues they do not
  administer). An eligible session player never sees metadata via this
  route, even for their own match -- resolving per-match player
  eligibility for a route that can return an unbounded number of matches
  was rejected as disproportionate; a player needing full detail uses the
  single-match route or the dedicated approval read instead. See
  `canSeeApprovalMetadataInList`/`effectiveListLeagueID`'s doc comments.
- **`GET /api/matches/{id}/rounds`** stays fully public and always `200`
  (never gated outright, to preserve "public schedule information remains
  public") -- it returns an empty list instead of the real round data for
  an unauthorized caller on a draft/in-progress match (not yet
  `completed && week_closed`), and the full list once final or for an
  authorized caller (same admin-or-eligible-player check as the detail
  route, via `canSeeMatchDraftDetailWithLeague`). `RoundResult` carries no
  approval-metadata fields at all, so there is no analogous metadata-vs-
  finality split to make here.
- **`GET /api/matches/{id}/approval`** and **`GET .../approval-events`**
  are new, genuinely protected routes (`401`/`403`/`404`, not a graceful
  degrade) -- admin scoped to the league, or an eligible session player
  for either side of that specific match. History remains append-only;
  no generic audit infrastructure was added.

### Collateral fixes found during this phase

1. **`approveMatch`'s acting-user attribution was silently broken in every
   real deployment.** `approvingUserID` read a context key
   (`clearanceUserKey`) only `requirePersonalKeyAuth` ever sets, but this
   route is gated by `guardedLeagueAdminAction`, which routes through
   `requireAction` (a different context key) whenever any real auth is
   wired -- meaning `approved_by_user_id` was never actually populated via
   this route outside of legacy-only test fixtures. Fixed with a new
   `acceptingUserID` that prefers `identityFromContext`, falling back to
   the old behavior only for those legacy fixtures.
2. **`writeMatchApprovalErr` had no `domainerr.InvalidInput` case**,
   silently mapping it to `500` -- never exercised before this phase since
   no route wired to it previously produced that category. Added the
   `400` case; this also fixes the new team-approval routes'
   `CodeApprovalInvalidSide`/`CodeApprovalNoteRequired` mapping.
3. **`submitResults`/`clearResults`/`saveRounds` previously mapped any
   non-`Conflict` `domainerr` (e.g. a genuine `NotFound`) to a generic
   `500`.** Switching these three handlers to the revision-aware service
   methods and `writeMatchApprovalErr` is a strict improvement (a
   not-found match now correctly returns `404`), not a behavior change
   any existing test depended on.

### Files changed

`backend/domains/auth/{identity,authz}.go` (AuthMethod, new Actions);
`handlers/api_auth_middleware.go` (AuthMethod population);
`handlers/deps.go` (RoundManager interface extended with the Phase 1B-
relevant Phase 1A methods); `models/models.go`
(`MatchApprovalSnapshot`/`TeamSideApprovalSnapshot`, `ExpectedRevision` on
`SaveRoundsRequest`/`SubmitResultsRequest`, new `ClearResultsRequest`);
`handlers/api_match_approval_auth.go` (new -- the player/admin
authorization gates); `handlers/api_match_approval_{routes,handlers}.go`
(new -- the four team-approval routes plus the two protected reads);
`handlers/api_match_results_{routes,handlers}.go` (rewired
results/rounds/approve routes); `handlers/api_match_{routes,handlers}.go`
(list/detail field stripping and draft-round gating); plus test-file
updates for the new `RoundManager` methods and the `/approve` contract
change (`api_clearance_auth_test.go`, `api_match_approval_test.go`,
`api_match_auth_test.go`) and one new integration test file
(`api_match_approval_phase1b_test.go`, 29 tests).

### PM correction round (2026-09-30)

PM review of the initial Phase 1B implementation above found three
authorization/privacy defects, all corrected in place in the sections
above:

1. **`ActionMatchTeamApproval` inherited the unconditional `system_admin`
   bypass**, so a `system_admin` identity could approve/withdraw/request-
   correction for a team side without ever being a real, session-
   authenticated, eligible player -- exactly the masquerade path Phase 1B
   was designed to prevent. Fixed by evaluating `ActionMatchTeamApproval`
   in `auth.Authorize` BEFORE the `system_admin` shortcut, not as a case
   inside the switch the shortcut already bypasses -- see "New
   `auth.Action`s" above and `Authorize`'s own doc comment.
2. **`canSeeApprovalMetadataInList` (`GET /api/matches`) granted ANY
   authenticated `league_admin` visibility into every league's approval
   metadata**, including leagues they do not administer, whenever the
   request carried no `season_id`/`league_id` filter. Fixed by scoping the
   check to the request's EFFECTIVE filter (mirroring
   `MatchStore.ListMatches`'s own season-id-wins-over-league-id
   precedence via the new `effectiveListLeagueID` helper) -- an unfiltered
   request now exposes metadata only to `system_admin`.
3. **`GET /api/matches/{id}` stripped approval metadata and draft results
   together, guarded only by "not yet final,"** so a final
   (`completed && week_closed`) match leaked `approved_by_user_id`/
   `approval_note`/`processed_by_user_id` to every public caller. Fixed by
   splitting the two checks: `Results` still follows the final-vs-draft
   rule, but approval metadata is now stripped whenever the caller is
   unauthorized, regardless of finality.

A fourth, documentation-only correction fixed the `MatchApprovalSnapshot`
JSON example above, which showed `approved_at`/`processed_at` as `null`
and `correction_note` as `""` -- both fields are `omitempty` on their Go
types, so a nil/zero value is OMITTED from the response, never serialized
as `null` or `""`. The example now matches the actual wire shape.

New tests added for all three authorization/privacy corrections -- see
Verification below.

### Verification

`go build ./...` and `go test ./... -count=1` pass with zero regressions.
`go vet ./...` was run and is **not** fully clean -- it reports the same
four pre-existing warnings from Phase 1A
(`handlers/api_season_teams_test.go:455,1216`,
`handlers/api_weeks_test.go:1271,1313`); this branch introduces no new
`go vet` finding. `git diff --check` and an ASCII byte-scan on every
added/changed line are both clean.

### Explicit Phase 1B exclusions

No frontend approval controls or forms, no captain role, no self-
registration/email verification, no notifications, no mobile work, no
generic audit framework, no handicap formula change. Player UI (Phase
1C), admin UI integration for the new `/approve` contract (Phase 1D), and
staging/browser verification all remain outstanding. `MATCHES-Q002` is
advanced further, not resolved -- see Roadmap.

## Player Score Approval Phase 1C -- Player-Facing Score Entry and Team Approval UI (accepted 2026-10-02)

**Status: accepted as of 2026-10-02.** The implementation below went
through two PM correction rounds (2026-09-30: five player-mode state/scope
defects; a follow-up inspection pass the same round: score inputs
themselves not disabled when read-only) -- see "PM correction round
(2026-09-30)" below for exactly what was wrong and how it was fixed. Both
rounds were reviewed and accepted. **This phase's core browser workflow
was subsequently verified on staging on 2026-10-09** (branch
`staging-player-score-approval-verification`): player navigation to "My
Overview"/"My Matches" and nowhere else, eligible-match loading scoped to
the signed-in player, score entry/save, approval for both the home and
away sides, a correction request displaying its note and attribution, an
approval visibly resetting after an edit, and a stale player-revision
reload after a concurrent admin edit all passed. Player-mode score
clearing and the Withdraw control's own browser-click path were NOT
separately verified in that pass -- see Verification.

**Why this branch exists:** Phase 1A built the authoritative score-revision
and team-approval model; Phase 1B exposed it over HTTP with session
authorization, but players still had no way to reach any of it through the
application -- Player View exposed only My Overview. This phase makes the
approved player workflow usable through the browser, reusing the existing
admin Match Entry scoresheet component rather than building a second one.

**Stop condition:** a signed-in linked player can open Player View, reach
their eligible matches via a new "My Matches" nav entry, open an eligible
match in Match Entry, enter/save/clear scores with the current
`score_revision`, see both teams' approval states, approve/withdraw/request-
correction for their own eligible side(s), recover cleanly from a stale-
revision conflict, and never see admin-only Approve/Process/Unprocess/
Unapprove controls. The implementation below meets that stop condition and
has been accepted after two PM correction rounds (see "PM correction
round" below). Its core browser workflow was verified on staging on
2026-10-09: player navigation, eligible-match loading, score entry/save,
approvals for both sides, correction display, approval reset after
editing, and stale player-revision reload all passed. Player-mode score
clearing and the Withdraw control's own browser-click path were not
separately verified in that pass and remain open acceptance items -- see
Verification. Phase 1D remains responsible for the admin `/approve`
contract integration and Weekly Summary/admin presentation, both of which
are themselves implemented and staging-verified at the core-workflow
level -- see "Player Score Approval Phase 1D" below.

### Discovery findings

- The shell already has a `workspace` concept (`admin`/`player`,
  `web/app.js`'s `applyWorkspace`) fully independent of role, and a linked
  player's own league/season context is NEVER `appContext.activeLeague` --
  that reflects whatever league an admin last selected in this browser tab.
  `player-overview-page-component.js` already established the correct
  pattern for a player-owned read: pass `season_id=null` and let the
  backend resolve the player's own league's active season server-side
  (`FindActiveSeasonByLeague` + `GetPlayerRosterTeam`) -- this phase follows
  the same pattern for the new eligible-matches read.
- The existing `<match-entry-page>` component had no mode/read-only concept
  at all, no team-approval-panel rendering, and never sent
  `expected_revision` on `saveRounds`/`clearMatchResults` -- Phase 1B's
  player-write contract was entirely unreachable from the browser before
  this phase.
- `RoundStore.ResolveApprovalEligibility`'s per-side rule (current
  season-roster membership OR actual `round_results` participation) has no
  existing batched/list-shaped equivalent: `MatchStore.ListMatches` only
  filters by `season_id`/`league_id`, never by player. A new backend read
  was required to avoid either an N+1 per-match eligibility check in the
  browser or a frontend-side approximation from `player.team_id` alone
  (explicitly disallowed -- it would hide a valid substitute's match).
- `models.RoundResult` and `models.LineupPlan` already carry each player's
  name and handicap denormalized on the row (`home_player_name`,
  `home_handicap`, etc.) -- the two most common Match Entry load paths
  (existing rounds; a pre-set lineup) never actually needed the shell's
  league-wide `allPlayers` list for player identity, only the inline
  fallback lineup-picker (offering a full roster/substitute choice) does.

### Additive backend support: `GET /api/players/{id}/eligible-matches`

A new protected read, added because no existing endpoint could answer "which
matches is this player eligible to act on" without either an N+1 backend
call per match or a frontend eligibility approximation:

- `backend/domains/matches/round_store.go`: new `RoundStore` interface
  method `ListEligibleMatchesForPlayer(ctx, seasonID, playerID) ([]PlayerEligibleMatch, error)`
  and new `PlayerEligibleMatch` domain type (match summary + independent
  `EligibleHome`/`EligibleAway` booleans + the same `MatchApprovalState`
  every other approval read uses).
- `backend/storage/sqlite/round_store.go`: implementation is a single SQL
  query per season (not per match) that mirrors
  `ResolveApprovalEligibility`'s exact per-side rule via
  `EXISTS(season_rosters ...) OR EXISTS(round_results ...)` for each side,
  joined with the match's live approval-state columns.
- `backend/domains/matches/team_approval_service.go`: unchanged passthrough
  at the service layer, matching the existing pattern for
  `ResolveApprovalEligibility`.
- `handlers/deps.go`: `RoundManager` interface extended with the same
  method (`noopRoundMgr` in `handlers/api_clearance_auth_test.go` and
  `stubRoundStore` in `backend/domains/matches/round_service_test.go`
  updated to satisfy their interfaces).
- `models/models.go`: new `PlayerEligibleMatch` JSON response type, reusing
  `TeamSideApprovalSnapshot` for `home_approval`/`away_approval` so a "My
  Matches" list row and the dedicated `GET .../approval` read render
  identically on the frontend.
- `handlers/api_player_eligible_matches_routes.go` (new),
  `handlers/api_player_eligible_matches_handler.go` (new): route and
  handler for `GET /api/players/{id}/eligible-matches`. Access control
  reuses `playerOverviewAuth` and `ActionPlayerOverviewOwn` completely
  unchanged -- the ownership model is identical to Player Overview
  (`system_admin` any player, `league_admin` only players in a league they
  administer, a linked player only their own list). No new `auth.Action`
  was needed. `season_id` is optional; when omitted, the player's own
  league's active season is resolved server-side, exactly like Player
  Overview. A player with no derivable league (teamless, or their league
  has no active season) gets an empty list, not an error.

No change was made to any existing route's request/response contract.
`GET /api/matches/{id}` and `GET /api/matches/{id}/rounds` already served
an eligible player correctly for Match Entry's own reads (Phase 1B's
`canSeeMatchDraftDetailWithLeague`), so no additional backend read was
needed for those.

### Frontend implementation

- **Player View navigation:** a new `My Matches` nav entry
  (`#nav-item-my-matches`, `web/index.html`) gated exactly like `My
  Overview` -- visible only in Player View for a linked player
  (`web/app.js`'s `applyWorkspace`) -- pointing at a new
  `<my-matches-page>` custom element and `#section-my-matches`.
- **`web/domains/matches/my-matches-page-component.js`** (new): fetches
  `GET /players/{id}/eligible-matches` for `currentIdentity.player_id`
  (never `appContext.activeSeason`), and renders week/opponent/home-or-away/
  score-status/both sides' approval-state badges per eligible match, plus
  loading and empty states. Opening a match dispatches a bubbling
  `my-match-open-request` event (never navigates directly), carrying that
  row's own `eligible_home`/`eligible_away` flags.
- **`web/app.js`**: handles `my-match-open-request` by calling a new
  `openPlayerMatchEntry(...)`, which extends the existing admin
  `entryPreSelect*` mechanism (`web/lib/app-context.js`'s
  `setEntryPreselect`/`consumeEntryPreselect` gained an optional
  `playerContext` field, `{playerId, eligibleHome, eligibleAway}`, additive
  and `null` for every ordinary admin navigation) so the shared
  `entry`-section `loadSection` case can tell the two apart and call either
  `match-entry-page`'s existing `refresh(...)` (admin) or its new
  `refreshForPlayer(...)` (player) accordingly.
- **`web/domains/matches/match-entry-page-component.js`**: reused, not
  duplicated, exactly per scope. Added:
  - `refreshForPlayer(playerId, matchId, seasonId, eligibleHome, eligibleAway)`:
    hides the admin season/match picker row, resets all mode/approval state
    (`#reset()`), and loads the one match directly.
  - `#loadMatchEntryForPlayer(matchId)`: mirrors the admin loader but
    resolves the match's OWN league (via its season's `league_id`) for the
    game-format check and league name, fetches the protected approval
    snapshot (`GET .../approval`), and only fetches the full league-wide
    player list lazily, if the inline lineup-picker fallback is actually
    reached (the two more common paths -- existing rounds, a pre-set
    lineup -- resolve player identity from each row's own embedded
    name/handicap via a new `#resolvePlayer` helper, used by both the admin
    and player loaders identically).
  - A **Team Approval panel**, rendered only in player mode, showing both
    sides' state (Pending/Approved/Correction Requested), actor name,
    correction note, and the revision that state applies to; mutation
    controls (Approve, Withdraw, Request Correction with a required note)
    are rendered only for the side(s) `eligibleHome`/`eligibleAway` mark
    true. Every mutation call passes the CURRENT `score_revision` from the
    last-fetched snapshot and re-renders the panel in place from the
    mutation's own response (every team-approval route already returns the
    fresh snapshot) -- no extra read is needed after an action succeeds.
  - The four admin-only match-level buttons (Approve/Process/Unprocess/
    Unapprove -- the Weekly Score Processing Phase 1A/1B workflow, distinct
    from team-side approval) are now gated on `#mode === 'admin'` first,
    before their own existing state checks, so they can never render in
    player mode.
  - `saveRounds`/`clearMatchResults` now send `expected_revision` in player
    mode (sourced from the last-fetched approval snapshot); the admin path
    is untouched (the parameter is omitted from the request body when not
    given). A `409` response in player mode is never retried or silently
    overwritten -- it triggers a toast explaining the scores changed
    elsewhere and a full reload of match/rounds/approval state for the
    player to review before trying again.
  - `web/lib/api-client.js`'s shared `api()` now attaches `.status` to
    every thrown `Error` (purely additive -- every existing caller only
    ever read `.message`), which is what lets Match Entry distinguish a
    409 conflict from any other failure without parsing message text.

### Tests added

Backend (17 new tests, all passing): 7 storage-layer tests in
`backend/storage/sqlite/round_store_approval_test.go` for
`ListEligibleMatchesForPlayer` (roster-home-only, roster-away-only, actual-
participant-substitute-with-no-roster-row, unrelated-player-excluded,
different-season-excluded, live-approval-snapshot-included, ordered-by-
week-then-id), and 9 new HTTP integration tests in the new
`handlers/api_player_eligible_matches_test.go` covering: a linked player
sees their own eligible match; an unrelated player sees an empty list; a
linked player cannot view another player's list (403); a same-league
`league_admin` can view it; a different-league `league_admin` cannot (403);
`system_admin` can view any player's list; an actual-participant substitute
with no season-roster row is still listed; a teamless player with no
derivable league gets an empty list, not an error; an explicit `season_id`
override is honored. Frontend changes were verified with `node --check` on
every changed/added JavaScript file (syntax only -- no automated frontend
test harness exists in this repo; see Verification below for what was and
was not exercised).

### PM correction round (2026-09-30)

PM review of the initial Phase 1C implementation above found five
player-mode state/scope defects, all corrected in place in the sections
above (not described as separate new sections, since each is a targeted
fix to behavior already documented above):

1. **`GET /api/players/{id}/eligible-matches` accepted an arbitrary
   explicit `season_id`** with no check that it belonged to the requested
   player's own league, risking exposure of a different league's protected
   match/approval data. Fixed by validating the explicit season exactly
   like Player Overview does: `seasonMgr.GetSeason` (mapped through the
   same `mapSeasonErr` Player Overview uses) followed by the identical
   unconditional league-consistency check (404 "player is not in this
   season's league" whenever the player resolves to a real league that
   disagrees with the season's) -- applied regardless of caller identity,
   so `system_admin` and a same-league `league_admin` get the same
   contract as everyone else, not a second policy.
2. **Player-mode Save/Clear rendered as available even when the approval
   snapshot failed to load**, which meant a write could be sent with no
   `expected_revision` at all (the backend correctly rejects it, but the
   UI should never offer a control it knows will fail). Fixed: a
   successfully-loaded approval snapshot is now a hard prerequisite for
   `canEditScores` in player mode; when it is null, the scoresheet renders
   read-only with a "could not be loaded" message and a Retry action
   (`#loadMatchEntryForPlayer` re-fetches everything cleanly). Also fixed
   in the same pass: `#reset()` and every player-mode load now clear/
   repopulate `#allSeasons` with ONLY the match's own season (fetched
   fresh every time), so the shared `seasonClosed` lock calculation can no
   longer read a stale admin-selected league's season list left over from
   a prior Admin View visit.
3. **The team-approval panel offered Approve/Request Correction before
   the match was scored, and kept offering every control through season-
   closed/week-closed/admin-accepted/processed locks.** Fixed by mirroring
   the backend's own guard conditions in presentation: Approve and Request
   Correction now require a completed, unlocked match; Withdraw requires
   the side to already be approved and the match to remain unlocked; no
   mutation control renders at all once the match is locked by any of
   those four conditions, replaced by a short explanation that never
   implies the player could perform the admin-only action that would
   unlock it.
4. **The dedicated Substitute/Undo controls (`ActionLineupMutate`-gated,
   admin-only routes) rendered in player mode**, where they would 403.
   Fixed by gating `#subControlHtml`'s Sub/Undo buttons to admin mode
   only (player mode shows the existing "Sub for X" attribution as
   read-only text when relevant, nothing else); the inline lineup
   picker's own admin-Lineup-page link is replaced with plain text in
   player mode (that admin section is hidden entirely in Player View);
   and the player-mode toolbar's "Change Lineup" button (which only ever
   reloaded the same match) is replaced with "Back to My Matches." The
   inline picker itself -- which is local player selection feeding a
   score write, not a lineup-management API call -- is unchanged and
   still available in both modes. No player session is authorized for any
   lineup-management/substitute route by this fix or any other part of
   this phase.
5. **Player-mode lock messages reused admin instructions** ("Reopen the
   week," "Unprocess, then unapprove") that a player cannot act on. Fixed
   with player-appropriate wording ("This week is closed. Contact a
   league administrator." / "This match has been accepted or processed
   and is locked. Contact a league administrator for corrections."),
   gated on mode; Admin View's existing detailed instructions are
   unchanged.
6. **A follow-up PM inspection pass of this same correction round found a
   sixth defect the first pass missed:** `#ssGameInput()` still rendered
   every score input enabled regardless of `#scoresheetEditable` -- a
   read-only scoresheet (player-mode approval snapshot unavailable,
   closed season, closed week, approved, or processed) hid Save/Clear and
   the Sub/Undo controls, but a player could still type directly into the
   game-score inputs themselves. Fixed by adding the HTML `disabled`
   attribute to each input whenever `#scoresheetEditable` is false,
   reading that same existing flag directly (no second editability
   policy); each input's displayed value is computed independently of
   this and is unchanged either way.

New backend tests were added for the first correction (season-league
validation); the other five are frontend-only presentation/state
corrections with no new backend surface, verified by `node --check` and
manual code review of the exact guard conditions mirrored from the
backend (no automated frontend test harness exists in this repo -- see
Verification).

### Verification

`go build ./...`, `go test ./... -count=1` (all packages, zero failures),
and `go vet ./...` (same four pre-existing warnings as every prior phase,
zero new findings) all pass. `git diff --check` is clean. An ASCII
byte-scan of every added/changed line (tracked diff and new untracked
files) found zero non-ASCII bytes. `node --check` passed on every changed
JavaScript file. Browser/manual verification of the player workflow had
NOT been performed at the time of this acceptance, including after both
correction rounds -- Phase 1D was planned to cover staging/browser
verification per the roadmap's phase split, and this phase's acceptance
did not depend on that verification having happened yet.

**Update 2026-10-09:** this phase's core browser workflow was
subsequently verified on staging (branch
`staging-player-score-approval-verification`) -- player navigation,
eligible-match loading, score entry/save, approvals for both sides,
correction display, approval reset after editing, and stale
player-revision reload all passed. Player-mode score clearing and the
Withdraw control's own browser-click path were NOT separately verified in
that pass and remain open acceptance items. See
`doc/testing/product-smoke-test-checklist.md`'s "Player Score Approval
Phases 1A-1D -- staging verification (2026-10-09)" for the full record.
`MATCHES-Q002` remains open pending verification, or explicit PM
disposition, of all remaining unchecked acceptance items (not only the
two items named here) -- see "Player Score Approval Phase 1D" below.

### Explicit Phase 1C exclusions

No captain role, no email notifications, no mobile work, no generic audit
infrastructure, no new handicap behavior, no automated week close, no admin
acceptance UI changes, no broad visual redesign, no second scoresheet
implementation, no frontend-authoritative eligibility rule. A player using
the inline lineup-picker fallback's "Sub for X" badge may see a generic
"original player" label instead of that player's actual name when
`#allPlayers` was never populated (the two more common load paths don't
need it) -- a minor, pre-existing display fallback already in the code,
not a new defect. Phase 1D (admin `/approve` contract integration, Weekly
Summary/admin presentation, staging/browser verification) remains
outstanding; `MATCHES-Q002` is advanced further, not resolved.

## Player Score Approval Phase 1D -- Admin Integration and Weekly Summary (accepted 2026-10-08)

**Status: accepted as of 2026-10-08.** The implementation below, including
the privacy correction documented in its own subsection, was reviewed and
accepted after one PM correction round. **The core end-to-end Phase 1A-1D
staging workflow passed on 2026-10-09** (branch
`staging-player-score-approval-verification`, source checkpoint
`8d7ac0c`, already deployed to staging to perform this verification) --
see `doc/testing/product-smoke-test-checklist.md`'s "Player Score
Approval Phases 1A-1D -- staging verification (2026-10-09)" for the full
record. Two specifically identified browser edge checks remain
unverified and must not be treated as passed: explicitly declining the
admin override confirmation (proving no acceptance event is written),
and forcing the approval-snapshot load to fail (proving the Retry path
renders and recovers) -- both are covered by existing code and automated
tests, but neither has received a complete browser interaction. Several
other checklist interactions were also not separately exercised or
reconfirmed during that pass: player-mode score clearing, the Withdraw
control's own browser-click path, accepting after the admin approval
snapshot becomes deliberately stale, and the Weekly Summary Open-button
navigation. See Verification.

**Why this branch exists:** Phases 1A-1C are accepted, but the admin
Match Entry Approve action still sent the pre-Phase-1B request shape (no
`expected_revision`, no `override`) -- every click would 400 against the
rewired `AdminAcceptMatch` contract. This phase completes the admin side of
the workflow and surfaces team-approval readiness in Weekly Summary, so the
combined Phase 1A-1D chain is reviewable as a deployable whole.

**Stop condition:** admin Match Entry's Accept action sends
`expected_revision` (sourced from the last-fetched approval snapshot) and an
explicit `override`, never silently converting a failed normal accept into
an override; Accept is disabled with a Retry path whenever the approval
snapshot failed to load; a 409 response (stale revision, changed team
state, or override-required) reloads the authoritative match/approval state
instead of being retried or overwritten; admin Match Entry shows both
sides' approval state (current vs. stale revision, correction notes) and,
once accepted, whether acceptance was normal or an override; Weekly Summary
shows each scored match's team-approval readiness with a path to the
relevant Match Entry screen; process/unprocess/unapprove/reopen/close-week-
auto-process/substitute/lock behavior are all unchanged. The implementation
below meets this; the core workflow was browser-verified 2026-10-09, with
two specifically identified edge checks and several additional
not-separately-reconfirmed interactions still outstanding (see
Verification).

### Discovery findings

- `AdminAcceptMatch` (`backend/domains/matches/approval_service.go`) was
  already fully implemented and tested in Phase 1B -- `override` only
  bypasses the both-sides-approved-at-current-revision requirement, never
  the match-level guards (not found, stale revision, season/week closed,
  not scored, already processed, already accepted), and a failed
  non-override call writes nothing. This phase needed no backend change to
  `AdminAcceptMatch` itself, only to the pieces around it.
- `POST /api/matches/{id}/approve` (`handlers/api_match_results_handlers.go`)
  already required `expected_revision` unconditionally and decoded
  `override`/`note` -- the gap was entirely that nothing in the browser sent
  that shape yet. The route's own response is `{"status":"approved"}` only
  (no snapshot), so the admin UI must separately reload the match and
  approval snapshot after a successful accept to see the result.
- The Phase 1C approval-snapshot (`GET /matches/{id}/approval`) and events
  (`GET /matches/{id}/approval-events`) reads were already admin-accessible
  and required no change; Match Entry's admin loader simply never called
  them before this phase.
- `RecapMatchRow` (`models/models.go`), backing Weekly Summary's week recap,
  carried `approved_at`/`processed_at`/`week_closed` but nothing about
  per-side team-approval state -- Weekly Summary had no way to show
  approval readiness before a match reaches admin acceptance.

### Admin Match Entry integration

`web/domains/matches/match-entry-page-component.js`:

- The admin loader (`#loadMatchEntry`) now fetches the match's approval
  snapshot (`GET .../approval`) unconditionally, same as the player loader
  already did -- a failed fetch leaves `#approval` null. When the match is
  already `approved_at` and the snapshot loaded, it additionally fetches
  `GET .../approval-events` and records the most recent `event_scope:
  "match"` event's `event_type` (`admin_accepted` or `admin_override`) in a
  new `#lastAcceptEvent` field, purely for display. Both fields are cleared
  by `#reset()` and re-populated on every load, never left over from a
  previous match.
- The old Approve button/`#approveMatch` is replaced by an Accept button
  (`data-action="accept-match"`) and `#acceptMatch`. When the snapshot
  failed to load, no Accept button renders at all -- a Retry button
  (`data-action="retry-admin-approval"`) does instead, and `#acceptMatch`
  itself also refuses to proceed if `#approval` is somehow null, so an
  accept can never go out with guessed state.
- `#acceptMatch` sources `expected_revision` from `this.#approval.score_revision`
  (never from the match row directly) and computes `override` as follows:
  if both sides are `"approved"` at that exact revision (`#sideReadyAtRevision`,
  mirroring `matches.sideApprovedAtRevision`'s equality check for display),
  `override: false` and the request is sent immediately. Otherwise, a native
  `confirm()` dialog names each unsatisfied side and why (not yet approved,
  correction requested with its note, or approved at a stale revision) and
  asks for explicit confirmation; declining sends nothing at all (no
  override, no accept), and only confirming sends `override: true`. There is
  no path from a declined or failed normal accept to an automatic override
  retry.
- Every outcome of `#acceptMatch` -- success, any error status, including a
  409 for a stale revision or changed team state -- ends in `#reload()`,
  which re-fetches the match, rounds/lineup, and approval snapshot fresh.
  This is deliberately unconditional: a conflict must never leave the UI
  showing pre-click state the backend no longer agrees with, and a success
  must immediately reflect the real badges/available actions rather than
  whatever the client assumed would happen.
- A new `#renderAdminApprovalPanel()` (admin-mode analog of the existing
  player-mode `#renderTeamApprovalPanel`, but read-only -- an administrator
  never acts for a team side) shows, whenever the match is completed: the
  current score revision, each side's state (`Pending` / `Approved
  (current)` / `Approved (stale revision)` / `Correction Requested`) with
  actor and correction-note attribution when present, a "ready to accept"
  vs. "will require override" readiness line (before acceptance), and --
  once accepted -- whether acceptance was normal or an administrator
  override, from `#lastAcceptEvent`. Rendered only in admin mode, alongside
  (not replacing) the existing player-mode panel, which is unchanged.
- `web/domains/matches/match-entry-api-service.js`'s `approveMatch` now
  takes `(matchId, expectedRevision, override, note)` and always sends
  `expected_revision`/`override` in the body; `note` is included only when
  given. No other admin action's request shape changed.

### Weekly Summary integration: smallest necessary backend extension

**`GET /api/seasons/{id}/weeks/{week}/recap` remains a PUBLIC route, and
every pre-existing recap field (teams, dates, set/game counts,
`approved_at`/`processed_at`/`week_closed`, missing-match count, player
stats, handicap changes, acknowledgments, next-week readiness) remains
public exactly as before this phase -- unchanged for every caller,
authenticated or not.** Weekly Summary's existing `has_result`/
`approved_at`/`processed_at`/`week_closed` status ladder already covers
Unscored/Scored/Admin-Approved/Processed/Closed -- re-encoding those same
four states a second time would duplicate existing data for no benefit.
The gap was specifically the previously-invisible Phase 1B/1C
team-approval states a scored-but-not-yet-accepted match sits in.
`models.RecapMatchRow` (`models/models.go`) gains four **pointer** fields,
computed at full fidelity in `backend/storage/sqlite/week_store.go`'s
`GetWeekRecapData` (new `approvalStatusLabel` helper) by adding
`m.score_revision`/`m.home_approval_state`/`m.home_approval_state_score_revision`/
`m.away_approval_state`/`m.away_approval_state_score_revision` to the
existing single per-week query -- no new query, no N+1:

- `score_revision` (`*int`).
- `home_approval_status` / `away_approval_status` (`*string`): one of
  `"pending"`, `"approved_current"`, `"approved_stale"`, or
  `"correction_requested"`. `"approved_stale"` means the side approved a
  now-superseded revision (invalidated by a later score edit) -- the
  exact distinction `matches.sideApprovedAtRevision` enforces
  authoritatively, computed once here so no consumer re-derives it.
- `approval_ready` (`*bool`): true only when both sides are
  `"approved_current"` -- mirrors `RoundService.BothSidesApproved` exactly,
  i.e. whether a normal (non-override) admin accept would succeed right
  now.

**These four fields are protected approval metadata, the same facts
Player Score Approval Phase 1B's `GET .../approval` already restricts --
they are never universally returned by this public route.** See "Privacy
correction" immediately below for the full policy and mechanism (this was
a PM-flagged correction to the initial Phase 1D handoff, applied before
any commit).

`web/domains/weekly-summary/weekly-summary-page-component.js` adds a "Team
Approval" column, populated by a new `#teamApprovalStatus(m)` that returns
no badge at all when `home_approval_status` is absent (either because the
match is unscored, or because this session's request was redacted -- the
two cases are indistinguishable on the wire by design, and both correctly
render as "nothing to show" rather than a guessed label) and otherwise
picks one display label (`Ready` / `Pending` / `Correction Requested` /
`Stale Revision`) by priority among the backend-computed fields -- it
performs no revision-equality comparison itself, only prioritizes among
finished enum values and the `approval_ready` boolean, per the "do not
duplicate authoritative acceptance rules in JavaScript" requirement.
Weekly Summary's admin UI receives the real fields because it always runs
under an authenticated administrator's own session request, which
satisfies the policy below like any other caller would. The existing
"Open" button (already present, linking to Match Entry for that match)
needed no change -- it already provides the required path from Weekly
Summary to the relevant Match Entry screen.

### Privacy correction: approval metadata redaction for non-admin callers

**PM review of the initial Phase 1D handoff found a privacy regression**:
the four fields above were being returned unconditionally by the public
recap route to every caller, including unauthenticated callers, linked
players, and administrators of an unrelated league -- the same protected
approval metadata Phase 1B deliberately restricted on `GET .../approval`
would have leaked through this second, pre-existing public route. Fixed
before any staging or commit, using the EXISTING Phase 1B approval-metadata
visibility policy and season-to-league scope resolution rather than
inventing a broader role check or a second protected endpoint:

- `handlers/api_match_approval_auth.go`: new `canSeeApprovalMetadataInRecap`,
  mirroring the already-existing `canSeeApprovalMetadataInList` (the same
  check `GET /api/matches` uses to scope admin-attested approval metadata
  visibility) exactly: `system_admin` unconditional; a `league_admin` only
  for the league that owns the REQUESTED season (resolved directly from
  the recap route's own season-id path parameter -- no filter ambiguity,
  since a recap is always for exactly one season). Deliberately **no
  eligible-player path** -- a linked player already has a protected
  per-match read (`GET .../approval`) for matches where they are eligible,
  and must not additionally receive every OTHER match's approval state for
  the whole week through this public, unrelated-player-reachable recap.
- `handlers/api_week_handlers.go`'s `recapWeekHandler`: builds the recap
  normally (full fidelity, via the unchanged `WeekManager.WeekRecap`),
  then -- only when `canSeeApprovalMetadataInRecap` is false -- sets all
  four pointer fields to `nil` on every match row before responding. The
  route registration (`handlers/api_week_routes.go`) now passes `deps` and
  `seasonMgr` through to the handler for this check; the route itself
  stays registered exactly as before (no new auth gate wrapping it, no
  change to who can call it at all).
- `models.RecapMatchRow`'s four fields were changed from plain
  `int`/`string`/`bool` to `*int`/`*string`/`*bool` specifically so a
  redacted field is **absent** from the JSON response, never present with
  a placeholder `0`/`""`/`false` that would be indistinguishable from a
  legitimate value for an authorized caller. `nil` now serves double duty
  by design: "not yet scored" (the store never computes these before
  `HasResult`) and "redacted for this caller" produce the identical
  absent-key wire shape, which is intentional -- a consumer has no
  legitimate reason to tell the two apart, and no caller's own redaction
  state is itself sensitive information worth distinguishing.
- `backend/storage/sqlite/week_store.go`'s `GetWeekRecapData` is
  otherwise unchanged by this correction: it always computes the full,
  authoritative values when `HasResult`, regardless of caller -- the
  redaction is a separate, later projection over the *HTTP response*, not
  a concern of the store or the `WeekManager`/`WeekService` layer. Every
  other consumer of `models.WeekRecap`/`RecapMatchRow` in Go code (there
  are none besides this one route today) is unaffected in behavior, only
  in that the four fields are now pointers rather than values.

This did not touch the accepted admin Match Entry Accept/override
behavior at all -- Match Entry sources approval state exclusively from the
dedicated, already-protected `GET .../approval`/`GET .../approval-events`
reads (unaffected by this correction), never from the recap route.

### Tests added

5 SQLite-layer tests in `backend/storage/sqlite/week_store_test.go`
covering `GetWeekRecapData`'s new fields (calculation correctness, store
layer, unaffected by the privacy correction -- the store always computes
full fidelity): nil/absent when not scored, `"pending"`/`"pending"` with
`approval_ready=false` when scored with no approvals, `"approved_current"`/
`approval_ready=true` when both sides approved at the current revision,
`"approved_stale"` for a side approved at a since-superseded revision
(`approval_ready=false`), and `"correction_requested"` for a side with a
pending correction request.

6 new HTTP-level tests in the new `handlers/api_week_recap_privacy_test.go`
prove the visibility boundary itself, against the real sqlite-backed auth
stack (`testServerWithAuth`/`seedApprovalFixture`, the same infrastructure
Phase 1B's read-privacy tests use): an unauthenticated caller receives the
existing recap data but none of the four fields
(`TestWeekRecapPrivacy_Unauthenticated_NoApprovalFields`); a linked player
(including one eligible for the match itself) receives the existing data
but none of the four fields (`..._LinkedPlayer_NoApprovalFields`); a
`league_admin` for a DIFFERENT league does not receive them
(`..._DifferentLeagueAdmin_NoApprovalFields`); a `league_admin` scoped to
the season's own league receives the correct fields
(`..._SameLeagueAdmin_SeesApprovalFields`); a `system_admin` receives them
(`..._SystemAdmin_SeesApprovalFields`); and every pre-existing recap field
remains present and correct for an unauthenticated caller, proving the
correction redacts only the four new fields and nothing else
(`..._ExistingPublicFieldsUnchanged`).

`AdminAcceptMatch`'s normal-accept/explicit-override/no-silent-override/
missing-snapshot/stale-revision/changed-team-state behavior was already
fully covered at the service layer (`backend/domains/matches/
team_approval_service_test.go`) and HTTP layer (`handlers/
api_match_approval_phase1b_test.go`) by Phase 1B, since this phase changed
none of that logic -- only the browser's use of the existing contract, the
new recap fields, and their visibility. No new backend tests were needed
for `AdminAcceptMatch` itself. The admin Match Entry accept/override/
reload flow is frontend logic with no automated test harness in this repo
(per the established Phase 1C precedent) -- verified by `node --check` and
manual code review of the guard conditions mirrored from the backend;
browser verification remains outstanding (see Verification).

### Verification

`go build ./...` and `go test ./... -count=1` (all packages, zero
failures) pass. `go vet ./...` reports the same four pre-existing warnings
as every prior phase (`handlers/api_season_teams_test.go:455,1216`,
`handlers/api_weeks_test.go:1271,1313` -- all in files untouched by this
phase), zero new findings. `git diff --check` is clean. An ASCII byte-scan
of every added/changed line found zero non-ASCII bytes. `node --check`
passed on every changed JavaScript file
(`web/domains/matches/match-entry-api-service.js`,
`web/domains/matches/match-entry-page-component.js`,
`web/domains/weekly-summary/weekly-summary-page-component.js`).
**Update 2026-10-09:** browser/staging verification of the admin
accept/override flow and Weekly Summary's new column was subsequently
performed -- the core end-to-end workflow (admin approval panel display,
normal acceptance with no prompt, the confirm-and-override path recording
an `admin_override` event, Process/Unprocess/Unapprove, the Weekly
Summary "Ready" badge, and the recap privacy boundary in both directions)
passed on staging on 2026-10-09. Two specifically identified browser edge
checks remain unverified: explicitly declining the override confirmation
(proving no acceptance event is written), and forcing the approval-
snapshot load to fail (proving the Retry path renders and recovers) --
both are covered by existing code and automated tests but have not
received a complete browser interaction. Several other checklist
interactions were also not separately exercised or reconfirmed during
that pass: player-mode score clearing, the Withdraw control's own
browser-click path, accepting after the admin approval snapshot becomes
deliberately stale, and the Weekly Summary Open-button navigation.
Neither this phase's acceptance nor Phase 1C's depends on any of these
remaining items; `MATCHES-Q002` stays open pending verification, or
explicit PM disposition, of all of them. See
`doc/testing/product-smoke-test-checklist.md`'s "Player Score Approval
Phases 1A-1D -- staging verification (2026-10-09)" for the full record.

### Explicit Phase 1D exclusions

No captain role, no new login/session behavior, no email or mobile
notifications, no generic audit infrastructure, no handicap formula
changes, no visual redesign, no unrelated cleanup, no new acceptance
endpoint, no second public approval endpoint, no broader role check than
the existing Phase 1B approval-metadata visibility policy, and no global
authentication requirement added to the recap route. `AdminAcceptMatch`
remains the only path that sets `approved_at`, and this phase added no
second copy of its decision logic anywhere (frontend presentation mirrors
it for display only; the backend re-verifies independently and
authoritatively at write time regardless of what the UI shows).
`MATCHES-Q002` remains open until staging and browser verification of the
full chain are complete.

## Decision History

### 2026-10-09 - Player Score Approval Phases 1A-1D: staging verification (core workflow)

**Status:** `staging-verified (core workflow); acceptance items outstanding`

Ran on branch `staging-player-score-approval-verification` (source
checkpoint `8d7ac0c`), documentation-only -- no application code or test
changes. Confirmed the full end-to-end Phase 1A-1D chain on staging:
player-only navigation and eligible-match scoping; player score entry and
save, advancing the score revision; player approval of the current
revision for both the home and away sides; a correction request with its
note and attribution; a concurrent admin edit correctly staling and
reloading a player's approval (revision 2 to 3); the admin approval
panel's revision/both-sides display; a normal admin accept succeeding
with no override prompt when both sides were current; an override accept
(one side withdrawn) correctly recording an `admin_override` event,
confirmed both via the browser and via a clean API replay
(`override=false` -> 409 missing-approval, `override=true` -> 200, final
event `admin_override`, resulting snapshot home-approved/away-pending at
revision 4); Process/Unprocess/Unapprove; Weekly Summary's "Ready" badge
while the week stayed Open; and the recap privacy boundary in both
directions (omitted for unauthenticated/player/cross-league callers,
present for a correctly scoped administrator). Two specifically
identified browser edge checks remain unverified, not exercised this
pass: explicitly declining the admin override confirmation (proving no
acceptance event is written), and forcing the approval-snapshot load to
fail (proving the "Retry Approval Status" path renders and recovers) --
both are already covered by existing code and automated tests. Several
other checklist interactions were also not separately exercised or
reconfirmed this pass: player-mode score clearing, the Withdraw
control's own browser-click path, accepting after the admin approval
snapshot becomes deliberately stale, and the Weekly Summary Open-button
navigation. Disposable staging fixture data was deleted via the API
afterward; the shared "Fixture Scoresheet League" was not touched. See
`doc/testing/product-smoke-test-checklist.md`'s "Player Score Approval
Phases 1A-1D -- staging verification (2026-10-09)" for the full record.
`MATCHES-Q002` stays open pending verification, or explicit PM
disposition, of all of these remaining unchecked acceptance items.

### 2026-10-08 - Player Score Approval Phase 1D: PM correction round (approval metadata privacy), accepted

**Status:** `accepted`

PM review of the Phase 1D entry below found a privacy regression: the new
`score_revision`/`home_approval_status`/`away_approval_status`/
`approval_ready` fields were being returned unconditionally by the PUBLIC
`GET /api/seasons/{id}/weeks/{week}/recap` route to every caller --
unauthenticated, linked player, or an administrator of an unrelated
league -- leaking the same protected approval metadata Phase 1B
deliberately restricted on `GET .../approval`. Fixed using the EXISTING
Phase 1B approval-metadata visibility policy (a new
`canSeeApprovalMetadataInRecap`, mirroring the already-existing
`canSeeApprovalMetadataInList` exactly: `system_admin` unconditional,
`league_admin` only for the season's own league, deliberately no
eligible-player path) rather than a broader role check or a second
protected endpoint. `models.RecapMatchRow`'s four fields became pointers
so a redacted field is absent from the JSON response rather than present
with a placeholder zero/false/empty value. The recap route itself stays
public; every pre-existing recap field remains public and unchanged. Added
6 new HTTP-level tests (`handlers/api_week_recap_privacy_test.go`) proving
the visibility boundary for all five caller types the PM required plus
unchanged pre-existing-field behavior. No change to the accepted admin
Match Entry Accept/override behavior -- it sources approval state
exclusively from the already-protected per-match reads, never from the
recap route. See "Privacy correction: approval metadata redaction for
non-admin callers" under "Player Score Approval Phase 1D" above for full
detail. Accepted 2026-10-08. Staging/browser verification remains
outstanding, not a condition of this acceptance; `MATCHES-Q002` stays
open.

### 2026-10-08 - Player Score Approval Phase 1D: admin integration and Weekly Summary, accepted

**Status:** `accepted`

Rewired admin Match Entry's Accept action onto the Phase 1B
`AdminAcceptMatch` contract (`expected_revision` sourced from the
authoritative approval snapshot, explicit `override` only after a
confirm()-based admin acknowledgment of exactly which side(s) are not
satisfied, never a silent override), added a read-only admin approval
status panel (current vs. stale revision per side, correction notes,
normal-vs-override acceptance history), and added the smallest necessary
backend extension (`score_revision`/`home_approval_status`/
`away_approval_status`/`approval_ready` on `RecapMatchRow`) so Weekly
Summary can show team-approval readiness without duplicating any
acceptance rule in JavaScript. No change to `AdminAcceptMatch`,
`ApproveTeamSide`/`WithdrawTeamApproval`/`RequestCorrection`, or any other
existing workflow -- all were already correct from Phase 1B/1C and fully
tested; this phase only wired the browser onto the existing contract. See
"Player Score Approval Phase 1D" above for full detail. Implemented and
accepted 2026-10-08 (after the privacy correction round above);
`MATCHES-Q002` stays open pending staging/browser verification of the
full Phase 1A-1D chain.

### 2026-10-02 - Player Score Approval Phase 1C: player-facing score entry and team approval UI, accepted

**Status:** `accepted`

Added a player-facing "My Matches" list and extended the existing admin
Match Entry component with a player mode (score entry/clear with mandatory
`expected_revision`, a team-approval panel, stale-revision recovery) rather
than building a second scoresheet UI. Required one small additive backend
read, `GET /api/players/{id}/eligible-matches`, reusing the existing
`ActionPlayerOverviewOwn` ownership model rather than a new `auth.Action`.
Two PM correction rounds were required before acceptance: the first found
five player-mode state/scope defects (cross-league season validation, a
missing approval-snapshot prerequisite for writes, ungated team-approval
controls, admin-only substitute controls leaking into player mode, and
admin-worded lock messages); a follow-up inspection pass in the same round
found a sixth (score inputs themselves stayed enabled on a read-only
scoresheet). All six are corrected -- see "Player Score Approval Phase 1C"
above, specifically its "PM correction round" subsection, for full detail.
Accepted 2026-10-02. Still not approved for standalone deployment on its
own, and still not browser-verified -- both remain Phase 1D's
responsibility, not conditions of this acceptance; see that section's
Verification and `doc/roadmap.md`'s Now section.

### 2026-09-30 - Player Score Approval Phase 1B: PM correction round (authorization and privacy)

**Status:** `accepted`

PM review of the 2026-09-30 Phase 1B entry below found three
authorization/privacy defects, all stemming from the same class of
mistake: a check that was too broad (an unconditional role bypass, an
unscoped admin check, a finality guard covering two unrelated
concerns). Corrected:

1. `ActionMatchTeamApproval` no longer inherits the unconditional
   `system_admin` bypass in `auth.Authorize` -- team approval requires an
   actual browser session and a linked, eligible player, with no role-only
   shortcut for any identity, `system_admin` included.
2. `GET /api/matches`'s approval-metadata visibility is now scoped to the
   request's effective `season_id`/`league_id` filter (mirroring the
   store's own filter precedence), not granted to any authenticated
   league_admin regardless of which league(s) they actually administer.
3. `GET /api/matches/{id}` now strips approval metadata independently of
   whether the match is final -- a closed match's final results remain
   public; who approved it does not.

See "PM correction round (2026-09-30)" under "Player Score Approval Phase
1B" above for full detail, and its Verification subsection for the new
regression tests.

### 2026-09-30 - Player Score Approval Phase 1B: routes, session authorization, and protected reads

**Status:** `accepted`

Made Phase 1A's data model and atomic guards reachable through HTTP:
four player-scoped team-approval mutation routes (session-only, no
personal-API-key or static-token masquerade), the `/approve` route
rewired onto `AdminAcceptMatch`'s explicit `expected_revision`/`override`
contract (a deliberate breaking change for that one route, since the
existing frontend button does not send it yet), player-authenticated
score writes on the existing rounds/results routes with mandatory
`expected_revision`, two new protected reads
(`GET .../approval[-events]`), and a read-authorization matrix that also
closes a real pre-existing gap (admin approval metadata was public via
`GET /api/matches[/{id}]` since Weekly Score Processing Phase 1A). See
"Player Score Approval Phase 1B" above for full detail. Advances
`MATCHES-Q002`; does not resolve it -- player UI, admin UI integration,
and staging verification remain.

### 2026-09-28 - Player Score Approval Phase 1A: complete-snapshot admin guard and live substitute error mapping

**Status:** `accepted`

PM review of the 2026-09-28 atomicity-corrections entry below found two
further issues:

1. **`AdminAcceptMatch`'s `bothApproved` check used the state label
   alone**, not the same current-revision rule `BothSidesApproved`
   already applied -- a side labeled `approved` with a nil or stale
   `StateScoreRevision` could incorrectly let normal acceptance proceed.
   Fixed by extracting a shared `sideApprovedAtRevision` helper both
   methods now use, and by extending `GuardedAdminAccept`'s pinned/
   re-verified snapshot from state labels alone to the complete side
   snapshot (label, state-score-revision including `nil`, and
   correction note) -- a correction note rewritten at the same revision
   with the same label is now caught too. See "Existing admin
   compatibility and the atomic admin-acceptance primitive" above.
2. **The live substitute routes' atomic-guard sentinels were not
   mapped by `LineupService`**, so a genuine lock race on
   `POST/DELETE /api/lineup-plans/{id}/substitute` surfaced as a
   generic HTTP 500 instead of a 409. Fixed with a new
   `mapSubstituteGuardErr` in `lineup_service.go`, recognizing each
   guard sentinel via `errors.Is` and mapping it to the same stable
   conflict codes `checkEditable`'s existing pre-check already uses.
   This is a live-route fix, not deferred to Phase 1B.

Both are corrected in place in this document; see the sections above for
full detail and the Verification subsection for the new regression
tests.

### 2026-09-28 - Player Score Approval Phase 1A: atomicity corrections

**Status:** `accepted`

PM review of the 2026-09-27 entry below found four correctness gaps, all
stemming from the same root cause: several checks were performed as a
read *before* the transaction that acted on them, leaving a real race
window between the check and the write. Corrected:

1. **Score-revision/lock enforcement is now a single transaction-scoped
   conditional `UPDATE`** (`AdvanceScoreRevisionForEdit`), acquired as
   the first statement inside the same transaction as the destructive
   score/result write, replacing the removed pre-transaction
   `checkMatchEditable`.
2. **`*_approval_score_revision` renamed to
   `*_approval_state_score_revision`** and `RequestCorrection` now sets
   it (previously left `NULL`) -- both `approved` and
   `correction_requested` describe one exact revision.
3. **`AdminAcceptMatch` gained a required `expectedRevision` parameter**
   and a new atomic store method, `GuardedAdminAccept`, whose
   conditional `UPDATE` pins the exact revision and team states the
   accept-vs-override decision was based on, closing the race between
   that decision and the write.
4. **Team-action (`ApproveTeamSide`/`WithdrawTeamApproval`/
   `RequestCorrection`) and substitute-swap (`SetSubstitute`/
   `ClearSubstitute`) writes now atomically re-verify every lock
   condition** (season/week/admin-accepted/processed), not just
   `score_revision` -- since admin acceptance and week close do not
   themselves change the revision, a team action or substitute swap
   could previously race past a lock that should have blocked it.

See "Player Score Approval Phase 1A" above (updated in place) for the
corrected design in full, and its Verification subsection for the new
real-transaction regression tests proving each of the four races is
actually closed, not just checked against an already-stale value.

### 2026-09-27 - Player Score Approval Phase 1A: data model and backend foundation

**Status:** `accepted`

Added `matches.score_revision` and seven per-side team-approval columns
(state/timestamp/actor/note, mirrored for home and away), a new
append-only `match_approval_events` table, and the domain/store
primitives for team approval, withdrawal, correction requests, optimistic
score-revision control, atomic score-edit/substitute-swap invalidation,
and a Phase 1B-ready admin accept-or-override method -- all backend-only,
with the existing admin approval route and its behavior completely
unchanged. Advances `MATCHES-Q002`; does not resolve it. See "Player
Score Approval Phase 1A" above for full detail (updated 2026-09-28 with
atomicity corrections -- see the entry above).

### 2026-09-16 - Default lineup week-filter fix; setup checklist warning, not a new screen

**Status:** `accepted`

Season setup polish discovery found the Default Lineup editor already
existed on the Lineups screen, so no new screen was built. It also found
`ListLineupPlansRequest`'s `WeekNumber=0` "no filter" convention
collided with `week_number=0`'s real meaning (the Default Lineup),
silently mixing week-specific rows into "default" reads once both
existed for a team -- confirmed against real staging data, not just
theoretical. Fixed by always filtering on `WeekNumber`; no caller ever
needed "all weeks in one call." Added one non-blocking `TEAM_NO_DEFAULT_
LINEUP` checklist warning, composed at the handler layer to keep
`SeasonService` free of a lineups-domain dependency, using the existing
`Warnings` array shape so no frontend change was needed. Season
activation, Close Week, and schedule generation remain independent of
lineup completeness. See "Default Lineup Week-Filter Fix + Setup
Checklist Warning" above for full detail.

### 2026-09-02 - Substitute Workflow same-match duplicate-player guard

**Status:** `accepted`

Fixes a staging finding from PM's browser verification of Substitute
Workflow Phase 1: a player could be selected as a substitute on one team
while already a starter on the other team in the same match, resulting
in the same player occupying two slots in one match at once (observed on
staging with a real fixture; PM's Undo immediately reverted the bad
state, no lasting data corruption). `LineupService.SetSubstitute` now
rejects with `409 SUB_PLAYER_ALREADY_IN_MATCH` ("that player is already
in this match") when the candidate substitute already has a
`lineup_plans` row for the same season/week under either the match's
home or away `team_id`, via a new `LineupStore.PlayerInMatchLineup`
method and a new `LoadMatchContext` method on `MatchLockChecker`
(`RoundStore` already satisfied it structurally). The frontend
substitute modal now excludes every player already in one of the
match's 6 slots, not just the current slot's own player, as a
client-side convenience on top of the authoritative backend check. A
substitute may still come from any team or league in general -- only
already being in *this specific match* is blocked. Also corrected a doc
inaccuracy found while fixing this: the smoke checklist had wrongly
implied the substitute modal (not just the separate manual "Confirm
Tonight's Lineup" picker) has "This Team" / "Other Players (Substitute)"
grouping -- it does not; corrected in place rather than adding grouping
to the modal, per this branch's scope guard. See "Substitute Workflow
Phase 1" above for full detail.

### 2026-09-02 - Substitute Workflow Phase 1: admin substitute support

**Status:** `accepted`

`lineup_plans.is_sub`/`sub_for_id` (already in the schema, previously
read-only) can now be set/cleared via two new `clearanceAuth`-gated
endpoints, `POST`/`DELETE /api/lineup-plans/{id}/substitute`. A
substitution replaces the slot's `player_id` in place (one row per slot,
matching what Match Entry's auto-fill already expects) rather than
adding a parallel row. Rejected with 409 when the team's match for that
season/week is season-closed, week-closed, approved, or processed --
the same lock set score edits respect, via a new narrow
`MatchLockChecker` interface `RoundStore` already satisfies structurally
(no second store built). Match Entry's player resolution now looks
players up against the full player list instead of the team roster (a
substitute may not carry the team's `team_id`), the manual lineup picker
now offers every league player grouped by roster/substitute, and the
scoresheet's roster table gained a Sub/Undo control per slot. Weekly
Summary's `GetWeekPlayerStats` gained `is_sub`/`sub_for_name` fields
(data only -- no new UI section, since Weekly Summary doesn't render
`player_stats` at all yet). Player Overview's stats section was verified
(not changed) to already count a substitute's results correctly; its
schedule section still won't show a substitute's one-off match for
another team, an accepted, documented limitation rather than the "big
player-history redesign" this phase was told not to force. See
"Substitute Workflow Phase 1" above for full detail.

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
