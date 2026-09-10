# Admin (League Admin hub)

## Overview

**Owner:** `admin`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-09-10`

The Admin domain owns the League Admin hub: a read-only operational
command center for `league_admin`/`admin`/`system_admin` users that links
the existing operational workflows (weekly scoring, lineups/substitutes,
finances, communication, player/user, setup) together and surfaces their
current state from data those screens already expose.

It is deliberately narrow. It owns no schema, no backend routes, and no
business rules. It does not compute status, standings, readiness, or dues
decisions -- it reads fields other domains already computed and renders
them as counts and jump links. It is **not** the deferred "Admin
code-management screens" roadmap item: no controlled-code editing, label
management, display-order, or active-flag editing is part of this domain.

## League Admin Screen Phase 1 (implemented 2026-09-10)

### Why this phase exists

The app now has several useful admin screens (Weekly Summary, Financial,
Communication, Player Overview, Users, plus the older operational screens),
but the league-admin workflow is spread across many sidebar entries. This
phase adds one hub that answers "what needs attention now, and where do I
go next" without adding new business rules or a broad admin-platform
redesign.

### What Phase 1 added

- New `web/domains/admin/` frontend domain
  (`league-admin-api-service.js`, `league-admin-page-component.js`,
  `admin-domain.js`) and a new "League Admin" nav entry
  (`#nav-item-league-admin`, `d-none` by default), placed directly under
  Dashboard.
- **No new backend routes or schema.** The hub makes a small fixed set of
  existing GETs on load -- two season-level, plus (when a focus week
  exists) three focus-week/default-lineup calls -- each consumed directly
  by one card:
  - `GET /api/seasons/{id}/weeks` (Weekly Summary / Schedule) -- season-wide
    per-week match/scored/closed counts.
  - `GET /api/seasons/{id}/finances/dues` (Financial) -- rostered players'
    paid/unpaid status.
  - `GET /api/seasons/{id}/weeks/{week}/recap` (Weekly Summary) -- the
    focus week's per-match approved/processed/closed detail.
  - `GET /api/lineup-plans?season_id={id}&week_number={week}` (Lineups),
    called twice -- once for the focus week and once for `week_number=0`
    (the default lineup) -- so readiness can use the same week-specific /
    default fallback Match Entry uses.

  The `finances/dues` route is `clearanceAuth`-protected; the hub is
  already gated to those roles, and a failed call degrades that one card
  to a link. Every card degrades to a plain jump link when its data is
  unavailable -- the hub never fabricates a count.
  `league-admin-api-service.js` duplicates these as thin wrappers rather
  than importing another domain's `*-api-service.js`, matching this
  codebase's per-domain-owns-its-own-calls convention.
- `admin-nav-request` shell event (one line in `web/app.js`,
  `document.addEventListener('admin-nav-request', e => navTo(e.detail.section))`),
  a deliberate rename of the identical `dashboard-nav-request` handler so
  the hub's cross-screen jumps stay self-documenting. No other shell
  bridge method was needed -- every hub link is a plain section
  navigation.

### Nav / role gating

`#nav-item-league-admin` is toggled in `web/app.js`'s `updateIdentityUI()`
off the same `canManageFinances` (`hasFinanceAdminRole(identity)`) value
that already gates `#nav-item-finances`, `#nav-item-communications`, and
`#nav-item-player-overview`. No new permission model; `role=player` and
no-key identities see it hidden, exactly like the other three.

### Focus week

Several cards report on one "focus week": the lowest `week_number` with
`match_count > 0` whose `status` is not `closed`; if every week with
matches is closed, the highest such week; if no week has matches, there is
no focus week and those cards render as link-only.

### Cards

| Card | Shows | Data source | Links |
|------|-------|-------------|-------|
| Season / Week strip | active season name; focus week number + open/closed badge (or "no schedule generated") | shell `activeSeason` + `GET .../weeks` | Seasons (when no schedule) |
| Weekly Score Processing | season line (`N/M matches scored`, `X/Y weeks closed`); focus week ladder counts (Missing / Scored / Approved / Processed / Closed) | `GET .../weeks` + focus week `recap` | Weekly Summary, Schedule |
| Lineups & Substitutes | `ready X/Y teams` for the focus week; `substitutes in use: N` | focus week `recap` (teams playing) + focus-week and default (`week_number=0`) `lineup-plans`. A team is ready only when its resolved lineup (week-specific plans if it has >=3, else the default plans) has its first 3 rows all resolve to a real player in the full league player list -- the same path Match Entry uses (a substitute's `player_id` may not be on the team's roster). Substitutes counted from `is_sub` in those resolved first-3 slots for teams actually playing. | Lineups, Match Entry |
| Money | `unpaid dues: N/M players` | `GET .../finances/dues` | Financial |
| Communication | the three V1 message types, as a reminder | none (static) | Communication |
| Players & Users | lookup / account management prompt | none (static); Users button only when identity role is `system_admin`/`admin` | Players, Player Overview, Users |
| Jump to | flat button row | none (static); Users button conditional as above | Schedule, Weekly Summary, Financial, Communication, Players, Teams, Seasons, Handicap, Users |

The ladder-status key (Missing/Scored/Approved/Processed/Closed) is a
deliberate duplicate of `weekly-summary-page-component.js`'s private
`#matchStatus` method -- the *decision* (which state a match is in) comes
entirely from the server's `has_result`/`approved_at`/`processed_at`/
`week_closed` fields; the hub only maps those to a label and a count.

### What Phase 1 defers

All explicitly out of scope per PM decision, not oversights:

- Any write action -- the hub is read-only; it only navigates.
- Automated email/SMS/mobile notifications.
- An audit/history framework.
- Developer/system tools consolidation (Backup stays its own sidebar
  button; the hub does not link to it).
- Controlled-code / label / display-order / active-flag editing (the
  separate deferred "Admin code-management screens" item -- not this).
- New roles, permissions, sessions, passwords, JWTs, or login redesign.
- Payment editing/voiding.
- A backend aggregate endpoint -- the load is a small fixed set of
  existing GETs (season weeks, season dues, and for the focus week: its
  recap, its lineup plans, and the default lineup plans), each feeding one
  card, judged non-brittle; no new surface was needed.
- Per-week approved/processed counts for *every* week at once (only the
  focus week gets the deeper recap call, to keep the load cheap).
- Any visual redesign beyond Bootstrap cards/badges.

### Verification

Frontend-only change -- no backend code added or modified. Verified:

- `node --check` on all four new/changed JS files
  (`admin-domain.js`, `league-admin-api-service.js`,
  `league-admin-page-component.js`, `web/app.js`).
- `go test ./... -count=1` and `go build ./...` rerun for full regression
  safety per the cross-domain-screen convention -- both pass, zero
  regressions (no Go code touched).
- The hub's focus-week selection and every card's count computation were
  simulated in a standalone Python script against real staging season 6
  data (`GET .../weeks`, focus week `recap`, focus-week and default
  `lineup-plans`, `finances/dues`, and the league player list): focus
  week resolved to Week 1, the ladder counts matched the recap's
  per-match states (2 missing, rest 0), the season line matched (`8/10
  scored`, `0/5 weeks closed`), lineup readiness -- resolved via the
  week-specific/default fallback and full-player-list resolution -- was
  `4/4 teams` with `0` substitutes, and dues showed `10/12 unpaid` --
  all matching the underlying endpoint responses.
- Actual browser rendering of the hub (card layout, the jump buttons
  navigating to the right sections, the conditional Users button) remains
  **NOT VERIFIED (no browser)** in this developer's tool session.

### Correction (2026-09-10, same day): lineup readiness matched to Match Entry

**PM finding, caught before commit:** the Lineups & Substitutes card's
first draft treated a team as ready when it had at least 3 `lineup_plans`
rows for the focus week (`planCountByTeam[id] >= 3`). A row count is not
the same as a renderable scoresheet -- the same class of issue already
fixed on the Dashboard.

**Fix:** the card now resolves each playing team's lineup the same way
Match Entry does -- week-specific plans when the team has at least 3, else
the default (`week_number=0`) plans; ready only when the first 3 resolved
rows all resolve to a real player in the full league player list (not the
team roster, since a Substitute Workflow Phase 1 substitute's `player_id`
may not be on that roster). `refresh()` gained an `allPlayers` parameter
(passed from `web/app.js` as `state.allPlayers`), and `#load()` now also
fetches `GET /api/lineup-plans?season_id={id}&week_number=0`. The
substitute count is taken from `is_sub` in those resolved first-3 slots
for teams actually playing the focus week. Frontend/docs-only -- no Go
code touched. Re-simulated against real staging season 6 data: still
`4/4 teams ready`, `0 substitutes` for Week 1, now via the resolution
path rather than a raw row count.

## Decision History

### 2026-09-10 - League Admin Screen Phase 1: operational hub

**Status:** `accepted`

Added a new, admin-only League Admin hub that links the existing
operational screens together and surfaces their current state (weekly
score ladder counts, lineup readiness and substitute count, unpaid dues
count) from data those screens already expose. No new backend routes,
schema, business rules, or permission model -- gated by the same
`hasFinanceAdminRole` role set the Financial, Communication, and Player
Overview nav entries already use. Read-only: the hub navigates, it never
mutates. Not the deferred "Admin code-management screens" item. See
"League Admin Screen Phase 1" above for full detail.

### 2026-09-10 - Correction: lineup readiness matches Match Entry

**Status:** `accepted`

PM review caught the Lineups & Substitutes card treating "3+ lineup_plans
rows" as ready. Corrected to use Match Entry's own resolution path
(week-specific/default fallback, first 3 rows resolving to real players in
the full league player list). See "Correction (2026-09-10, same day)"
above.
