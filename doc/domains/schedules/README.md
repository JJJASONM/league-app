# Schedules

## Overview

**Owner:** `schedules`
**Status:** `draft`
**Current version:** `0.5`
**Last reviewed:** `2026-07-19`

The Schedules domain generates, previews, adjusts, and shifts season schedules.
It applies scheduling rules but does not define their meaning.

## Bye Requests

A bye request lets a team pre-register to take the natural sit-out slot in a
specific league week. They apply only to seasons with an **odd number of teams**
(the round-robin rotation always has exactly one team with no opponent each week).

### Rules enforced by the backend

| Rule | Behavior |
|------|----------|
| Even team count | `POST` rejected with 400 — no natural bye exists |
| Team from another league | `POST` rejected with 400 |
| Duplicate (same team + season + week) | `POST` rejected with 400 |
| Week 0 (TBD) approval | `PUT` rejected with 400 — a specific week is required |
| Conflict (two approved byes for the same week) | `PUT` rejected with 400; existing approval unchanged |
| Wrong season URL on update/delete | `PUT`/`DELETE` returns 404 — requests are scoped by season |

### Lifecycle

1. Admin records a request. **Week must be set at creation time** — no editing
   workflow exists. If created with week 0 (TBD), the request must be deleted
   and recreated with a specific week before it can be approved.
2. Admin approves — backend validates no conflict and week ≠ 0.
3. At schedule generation, `applyByeRequests` swaps week numbers so the
   approved team's natural bye falls in the requested week.
4. Only one approved bye per season+week is enforced to match the single
   natural bye slot. A second request may be recorded but cannot be approved
   until the existing approval is removed.

### Schedule application

`logic.ScheduleOptions.ByeByWeek` (map[week→teamID]) carries approved byes
into `assignDates`. `applyByeRequests` builds a full week permutation so that
every approved request is honored simultaneously, not sequentially. Requests are
sorted by week before processing to produce deterministic output regardless of
Go map iteration order. The algorithm:

1. Builds `naturalByeWeek` (teamID → week) from the generated schedule.
2. Sorts all approved requests by week number.
3. For each request, maps the team's natural bye week (`srcToNew[naturalWeek] = requestedWeek`),
   marking the target week as used.
4. Pairs remaining unclaimed source weeks with remaining unclaimed target weeks
   (both sorted) to form a complete bijection.
5. Applies the permutation to every `ScheduleEntry.WeekNumber`.

Requests that cannot be satisfied (team has no natural bye, or the target week
is already claimed by an earlier request) are silently skipped.

## Schedule Visibility

The user-facing **Schedule** page shows only the active season's schedule. No
season selector is shown; when no season is active a clear empty-state message
is displayed instead.

Draft-season schedules remain accessible through **Seasons → Manage**. Admins
can generate, preview, and adjust a draft schedule from there before activating
the season. When navigating from Seasons → Manage to the Schedule view (e.g.,
via "Go to Schedule"), `loadForSeason(previewSeasonId)` is called on the
`<schedule-page>` component so the admin sees that specific season, even if it
is not yet active.

This separation keeps the public-facing schedule clean while allowing full
pre-activation workflow on draft seasons.

## No Play Weeks

Planned holidays and later emergencies use the same concept. Store a controlled
reason code and optional `notes`. The UI may display labels such as Holiday,
Weather, or Location Closure, but database records store stable codes.

Current confirmed behavior:

- A skipped date applies only to its season.
- Schedule generation omits skipped dates and shifts later league weeks forward.
- Consecutive skipped dates are supported.
- For draft seasons (and active seasons with no completed matches), regeneration
  deletes unplayed matches only. Active seasons with any completed match block
  regeneration entirely (see Schedule Regeneration Guards below).

## Date Contract

API and form-control dates use `YYYY-MM-DD`. The backend normalizes SQLite DATE
values and accepts legacy ISO timestamps when reading skip dates. User-visible
dates use the shared frontend `displayDate()` formatter; compact poster dates
remain a deliberate print-layout exception.

## Schedule Regeneration Guards

Backend guards enforced at the service layer before any schedule generation runs:

| Code | Status | Condition |
|------|--------|-----------|
| `SCHEDULE_HAS_CLOSED_WEEKS` | 409 | The season has any league week with status `closed`. Reopen the affected week before regenerating. |
| `SCHEDULE_ACTIVE_HAS_COMPLETED` | 409 | The season is active (`active=1`) **and** at least one match has `completed=1`. Draft seasons are not subject to this guard. |

The `SCHEDULE_ACTIVE_HAS_COMPLETED` guard fires after `SCHEDULE_HAS_CLOSED_WEEKS`.
A season with closed weeks hits the first guard regardless of active status.

## Pushback

A pushback inserts one or more complete No Play league weeks at a selected
cutoff. It moves all unplayed weeks at or after the cutoff together.

The operation:

- Never changes completed match dates
- Honors existing No Play dates
- Extends the season end date
- Previews every affected match before applying
- Applies atomically
- Audit write deferred until the audit system is implemented

### Phase M — Preview endpoint (accepted 2026-07-15)

`POST /api/seasons/{id}/schedule/pushback-preview` is a read-only preview.
It returns which unplayed matches would shift and which completed matches at
or after the cutoff would be preserved, along with the projected new end date.
No data is written.

Request:
```json
{ "cutoff_week": 5, "weeks_to_add": 1 }
```

Response `shifted` contains unplayed matches at or after the cutoff with their
new week numbers and shifted dates. Response `preserved` contains completed
matches at or after the cutoff that will not move. Matches before the cutoff
are outside the preview range and are omitted from both lists.

Validation codes:
- `PUSHBACK_INVALID_CUTOFF` (400) — `cutoff_week < 1`
- `PUSHBACK_INVALID_WEEKS_TO_ADD` (400) — `weeks_to_add < 1`
- `PUSHBACK_HAS_CLOSED_WEEKS` (409) — closed week at or after the cutoff
- `PUSHBACK_SEASON_NOT_FOUND` (404) — season not found

**Not mutated by the preview:** `skipped_weeks`, `bye_requests`, any match row,
any season column.

**Audit write deferred:** The approved apply workflow requires an audit entry.
This will be added when the audit system is implemented.

**Apply endpoint implemented in Phase N** (see Phase N section below).

### Phase N -- Apply endpoint (accepted 2026-07-15)

`POST /api/seasons/{id}/schedule/pushback-apply` applies the shift atomically
and returns the same response shape as the preview endpoint.

Request:
```json
{ "cutoff_week": 5, "weeks_to_add": 1 }
```

Behavior:
- Validation and closed-week guard are the same as the preview endpoint.
- Completed matches at or after the cutoff are preserved: `week_number`,
  `match_date`, `completed`, `week_closed`, results, rounds, and history
  are never modified.
- Unplayed matches at or after the cutoff are shifted atomically:
  `week_number += weeks_to_add` and `match_date += weeks_to_add * 7 days`
  when non-null (null stays null).
- Unplayed matches before the cutoff are not touched.
- `seasons.end_date` is recomputed to `MAX(match_date)` after the shift.
- `seasons.schedule_stale` is cleared to 0.
- `skipped_weeks` and `bye_requests` are not mutated.
- The response `shifted` and `preserved` arrays mirror what the preview
  would have returned for the same request.

**Audit write deferred:** An audit entry will be added when the audit system
is implemented. The apply currently writes no audit/history row.

### Phase O -- Schedule page admin UI (accepted 2026-07-15)

The Schedule page contains a "Pushback No-Play Week" panel above the week list.
The panel is visible only when a season is selected. Admin workflow:

1. Enter Cutoff Week and Weeks to Add.
2. Click Preview. The panel shows shifted matches, preserved completed matches,
   and the projected new end date.
3. Apply is enabled only after a successful preview run.
4. If Cutoff Week or Weeks to Add changes after a preview, Apply is disabled
   until Preview is run again.
5. Click Apply Pushback. A confirm dialog appears before the apply call is made.
6. On success: the schedule list refreshes and a success toast shows the shifted
   match count.
7. On error: a toast shows the error message. For the closed-week backend message,
   the UI remaps it to: "Closed weeks exist at or after the cutoff. Reopen those
   weeks before pushing back."

**schedule-data-changed is not dispatched after apply.** Pushback shifts only
unplayed future matches and does not change completed match data, standings, or
player stats. The schedule list is refreshed via a direct reload.

**Audit write deferred** until the audit system is implemented.

## Schedule Navigation and Accordion

### Match Entry navigation

Score Entry buttons and Close Week modal match-error links both navigate to
Match Entry via the shell's openMatchEntry bridge in web/app.js. openMatchEntry
calls appContext.setEntryPreselect(seasonId, matchId) then navTo('entry'), so
the shell preselects the correct season and match when Match Entry loads.

The Close Week modal is dismissed before navigation
(bootstrap.Modal.getInstance(...).hide()). This prevents the modal from
remaining visible behind the Match Entry section.

### Collapsible week cards

Week cards on the Schedule page have a chevron toggle button in each card
header. Clicking it collapses or expands the card body (match table and
prior-ack section). The header - status chip, ack-history button,
Reopen / Review & Close - remains always visible.

Default collapse state at season load:
- Closed weeks: auto-collapsed.
- Open weeks: expanded.

Collapse state persists across same-season refreshes (after close, reopen, or
assign). State resets to defaults when the season selector changes or when a
different season is loaded via loadForSeason().

## Week-End Recap UI (implemented 2026-07-19)

### Goal

Let admins review a week-end recap from the Schedule page without calling the
API directly. Consumes the read-only Phase A backend endpoint:
`GET /api/seasons/{id}/weeks/{week}/recap`

### Behavior

Each week card header gains a **Recap** button (icon: `bi-clipboard2-data`).
The button is available for all weeks (both open and closed) since the endpoint
supports both states.

Clicking **Recap**:

1. If the week card body is collapsed, it is expanded first.
2. A recap panel is toggled inside the card body, below the match table and
   prior-ack section.
3. The panel is loaded on first open and cached for subsequent toggles within
   the same schedule render.
4. Clicking **Recap** again collapses the panel without re-fetching.

The panel shows:

| Section | Content |
|---------|---------|
| Status badge + closed_at | Week open/closed state and formatted close date when present |
| Match summaries table | Home / sets / away / games columns; "No result" badge for matches with `has_result=false` |
| Missing-result warning | Alert when `missing_count > 0` |
| Acknowledgments note | Count of warnings acknowledged at close |
| Next-week readiness | Week number, match count, unassigned/missing-lineup warnings or "Ready" |
| Handicap note | `handicap.message` from the recap response |

Close Week behavior, the Reopen button, and all other week-card controls are
unchanged.

### Implementation

| File | Change |
|------|--------|
| `web/domains/schedules/schedule-api-service.js` | `fetchWeekRecap(seasonId, weekNum)` added |
| `web/domains/schedules/schedule-page-component.js` | Import `fetchWeekRecap`; `data-action="view-week-recap"` case in click delegation; `#toggleWeekRecap` and `#renderRecapPanel` private methods; Recap button and recap panel added to `#renderWeekCard` |

### Deferred

- Player-level stat deltas in the recap panel
- Handicap changes applied (from `handicap_history`)
- Recap panel accessible from outside the Schedule page (e.g., a dedicated recap route)
- Print/export of the recap panel

## Questions

### SCHEDULES-Q001 - Preview editing controls

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-07-13`

**Context:** The admin must be able to review a generated schedule before
activation.

**Resolution:** Schedule preview policy finalized across Phases F-H:
- Draft seasons may generate or regenerate freely; unplayed matches are replaced,
  completed matches are preserved.
- Active seasons with no completed matches may still regenerate.
- Active seasons with any completed match block regeneration entirely
  (`SCHEDULE_ACTIVE_HAS_COMPLETED`, 409).
- Close Week is only available for active seasons; draft seasons return
  `WEEK_CLOSE_SEASON_DRAFT` (409).
- The schedule page shows a draft-season banner and a disabled "Review & Close"
  button for open weeks on draft seasons. The seasons panel preview note and
  generate info text reflect the active-season lock.

## Decision History

### 2026-06-09 - One approved bye per season + week

**Status:** `accepted`

Only one team may hold an approved bye request for a given season and week,
matching the single natural bye slot in the round-robin. A second request may
be recorded but approval is rejected until the existing one is withdrawn.
Week 0 (TBD) requests cannot be approved — a specific week is required before
schedule generation can honor the request. Scope (season_id) is enforced on
both update and delete to prevent cross-season mutations.

### 2026-06-10 - Schedule page shows active season only

**Status:** `accepted`

The user-facing Schedule page is filtered to the active season. Draft seasons
are managed through Seasons → Manage. Admin preview of a draft schedule is
available via the Seasons → Manage detail panel.

### 2026-06-10 - Full permutation for bye requests

**Status:** `accepted`

`applyByeRequests` uses a bijective week permutation instead of pairwise swaps.
This ensures all compatible approved bye requests are honored when multiple
requests affect different weeks in the same season. Requests are sorted before
processing to guarantee deterministic output.

### 2026-06-08 - Shift entire league weeks

**Status:** `accepted`

Pushback means every unplayed scheduled week moves together rather than moving
individual matches independently.

### 2026-07-13 - Schedule preview policy and enforcement

**Status:** `accepted`

Draft seasons may generate or regenerate freely. Active seasons with no
completed matches may also regenerate. Once an active season has any completed
match the regeneration endpoint returns 409 (`SCHEDULE_ACTIVE_HAS_COMPLETED`).
Close Week returns 409 (`WEEK_CLOSE_SEASON_DRAFT`) for draft seasons; activation
is required before any week can be officially closed. The schedule page enforces
these constraints in the UX with a draft banner and a disabled close-week button.
