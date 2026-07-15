# Schedules

## Overview

**Owner:** `schedules`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-07-14`

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
- Creates an audit entry

## Deferred Schedule UI Polish

The following schedule-screen ideas are parked until the current admin workflow
has more usage:

- Collapsible week sections or accordions after scoresheets are created, so the
  current week stays easier to scan.
- Verification and improvement of Schedule page navigation into Match Entry,
  including any "Open" button behavior that does not route correctly.

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
