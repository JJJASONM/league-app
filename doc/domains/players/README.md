# Players

## Overview

**Owner:** `players`
**Status:** `draft`
**Current version:** `0.3`
**Last reviewed:** `2026-08-27`

Players are shared system-wide identities. They are not owned by one league or
team and are separate from authenticated user accounts.

## Participation

A player may have one home team in a season and may substitute for any team in
any league or season. Match participation records the team represented and
whether the player was rostered or substituting.

The current `players.team_id` implementation cannot represent this target
behavior and will eventually be replaced by season roster relationships.

## Quick Add

Admins can quick-add a missing player from the Players page without entering a
full profile. The minimum required fields are at least one name (first or last)
and a diff rating (defaults to 0). Player number, phone, email, and admin hold
are optional and can be completed later using the standard Edit Player modal.

Before creating the player, quick-add warns when the typed name normalizes to
the same full name as an existing player in the active league (see Duplicate
Warning below). This is advisory only; it does not block creation.

Deferred: INCOMPLETE profile status, close-week blocking for incomplete
profiles, and match-entry quick-add integration are not yet implemented.

## Duplicate Warning (Phase A, 2026-08-19)

Quick-add compares the typed first/last name against the already-loaded,
league-scoped player list on the Players page. Both the typed name and each
existing player's name are normalized (trimmed, internal whitespace collapsed,
case-folded) before comparison; a match is exact-normalized only, not fuzzy.

When a match is found, the admin sees the existing player's name and team (or
"no team" if unrostered) and can either cancel or continue and add the new
player anyway. No match means quick-add proceeds exactly as before this phase.

This is entirely client-side (`web/domains/players/players-page-component.js`):
no backend validation, DB constraint, or API change was made. Two different
real people who share a name will still see the warning every time they are
quick-added -- that is expected, not a bug, given the warn-only design.

## Safe Merge (Backend Phase A, 2026-08-19)

Admins can merge a duplicate player record into a surviving one:
`POST /api/players/{source_id}/merge` with body `{"target_id": <id>}`. Backend
and store only in this phase -- there is no merge UI yet.

Every supported reference to `source_id` is repointed to `target_id`, then the
source player row is deleted, all inside one transaction (all-or-nothing;
any failure rolls back everything):

- `match_results.player_id`
- `handicap_history.player_id`
- `round_results.home_player_id` and `round_results.away_player_id`
- `lineup_plans.player_id` and `lineup_plans.sub_for_id`
- `season_teams.captain_id`
- `season_rosters.player_id`
- `teams.captain_id`

`round_results`' handicap snapshot columns (`home_handicap_used`,
`away_handicap_used`, `handicap_pts_used`, `handicap_to`) and all game-score
columns are untouched by a merge -- only the player-ID columns move.
`handicap_history` rows from both players survive the merge and are
repointed, not rewritten or deduplicated; a merged player's history is the
union of both players' prior rows.

The merge is refused, with nothing changed, in three ways:

- **400 Bad Request** -- `source_id == target_id`.
- **404 Not Found** -- source or target player does not exist.
- **409 Conflict** -- an unsafe collision blocks the merge:
  - Source and target already both have a `season_rosters` row for the same
    season, regardless of team -- this is the general form of "source and
    target are rostered on different teams in the same season," since
    `season_rosters` has no `team_id` in its unique key.
  - Source and target already played against each other (one home, one away)
    in the same `round_results` row -- merging would make a player play
    themselves. Discovered while implementing; not in the original blocker
    list. Checked ahead of the broader participation check below so this
    more specific error takes priority when both would otherwise apply.
  - Source and target already both participate (as home player, away
    player, or both, in any row -- not necessarily the same row) in the
    same `(match_id, round_number)` of `round_results`. Covers both as
    home, both as away, and one as home in one row while the other is away
    in a different row of the same match/round -- any of these would make
    target appear more than once in one round after merging, the same
    condition Close Week rejects as `WEEK_PLAYER_DUPLICATE`. The narrower
    "both as home" case was the original implementation; broadened to full
    participation after review.
  - Source and target already both have a `lineup_plans` row for the same
    team/week/season (would violate its unique constraint). Discovered while
    implementing; not in the original blocker list.

The route is protected by the same personal-key admin mutation auth as the
rest of player CRUD (`league_admin`, `admin`, or `system_admin` role via
`clearanceAuth`).

## Deferred Player Maintenance

The following player-record maintenance items remain parked:

- Merge UI (preview, confirm) -- backend/store only so far.
- INCOMPLETE profile status and close-week blocking for incomplete profiles.
- Match-entry quick-add integration.

Duplicate detection for quick-added players and the safe merge backend are
both implemented as of the phases above; neither is in this deferred list
anymore.

## Player Overview Phase 1 Implementation

**Status:** `implemented`
**Date:** `2026-08-27`

### What Phase 1 added

The first "whole-app" screen for a single player: an admin-viewable,
read-only summary of one player's season -- team, schedule, stats,
current handicap, and a money placeholder -- built from a new endpoint
and a new frontend screen. No player login or self-service portal; this
is admin-viewable only, per PM decision.

- `GET /api/players/{id}/overview?season_id={id}` -- unprotected read
  (consistent with every other GET in the app; no new auth). `season_id`
  is optional: when omitted, the player's league's active season is used
  (`SeasonManager.FindActiveSeasonByLeague`, already used elsewhere).
- Handler-level composition (`handlers/api_player_overview_handler.go`),
  per explicit PM decision: no new cross-domain players-domain service
  for Phase 1. The handler calls `PlayerManager.GetPlayer`,
  `SeasonManager.GetSeason`/`GetPlayerRosterTeam`, `TeamManager.GetTeam`,
  `MatchManager.ListMatches`, and `RoundManager.GetPlayerStats` directly
  -- the same managers already injected via `handlers.Dependencies` for
  every other route. If this grows beyond Phase 1, promoting it into a
  dedicated service is a later decision, not made now.
- Team resolution prefers the season roster (`season_rosters`, the
  target per-season team model) via a new
  `SeasonManager.GetPlayerRosterTeam` method -- a one-line passthrough
  exposing an existing store lookup (`SeasonStore.GetPlayerRosterTeam`)
  that was previously internal-only, used only by `AddRosterPlayer`'s
  validation. Falls back to the player's direct `players.team_id` when
  they have no `season_rosters` entry for the season (covers
  non-roster-managed seasons and players who simply aren't rostered that
  season). When neither resolves, `team` is `null` and `schedule` is
  empty -- not an error.
- Schedule is derived, not stored: the handler fetches the season's full
  match list (`GET /api/matches`'s underlying `ListMatches` has no
  team_id or player_id filter) and filters client-side (in the handler)
  on the resolved team_id matching `home_team_id`/`away_team_id`.
  Accepted for Phase 1 at current data volumes per PM decision.
- Stats are the player's season totals from the existing
  `RoundManager.GetPlayerStats` season-scoped query, matched by
  `player_id` from the full season response (no new query). Zero-valued
  when the player has no `match_results` rows for the season.
- Money is an explicit placeholder (`{"tracked": false, "message": "..."}`)
  -- dues/payment tracking does not exist in this schema (confirmed
  during discovery: no table, column, or planned-but-deferred mention
  anywhere in the codebase or docs). Per PM decision, no schema was
  invented; the section exists so the frontend can show an honest
  "not tracked yet" message instead of omitting money entirely.
  **Update 2026-08-27:** a real financial data model now exists (see
  `doc/domains/finances/README.md`, Financial Phase 1) -- `dues_payments`
  and `payouts` tables, a new `finances` domain, and a league-admin-only
  Financial screen. Replacing this placeholder with a real per-player
  dues lookup was explicitly scoped as a small, separate Phase 2 and was
  not part of Financial Phase 1; this section still reflects the
  as-shipped Phase 1 behavior (`tracked` is still always `false` here)
  until that follow-on lands.
  **Update 2026-08-29:** that follow-on has now shipped -- see "Player
  Overview Phase 2" below. `tracked` is `true` whenever a `FinanceManager`
  is wired; it is `false` only in test-only setups that omit one.
- Frontend: new `<player-overview-page>` component
  (`web/domains/players/player-overview-page-component.js`) with a
  player-select dropdown, and a new "Player Overview" nav entry/section.
  A "View Overview" button was added to each Players list row
  (`players-page-component.js`), dispatching a `player-overview-nav-request`
  custom event that the shell (`web/app.js`) handles via a new
  `openPlayerOverview(playerId)` bridge function -- mirroring the
  existing `openMatchEntry`/`openHandicapForWeek` cross-domain
  deep-link pattern, including a new `overviewPreSelectPlayerId` field
  on `appContext` mirroring `entryPreSelectMatchId`. Frontend is
  presentation-only: it renders exactly what the endpoint returns, no
  client-side business logic beyond display formatting.

### Response shape

```json
{
  "player":   { "...": "existing Player fields" },
  "season":   { "id": 2, "name": "Spring 2026" },
  "team":     { "id": 5, "name": "Eight Is Enough" },
  "handicap": { "current": 3.4 },
  "schedule": [
    { "match_id": 33, "week_number": 2, "match_date": "2026-08-10",
      "opponent_team_name": "...", "home_or_away": "home",
      "completed": true }
  ],
  "stats": { "sets_won": 0, "sets_lost": 0, "games_won": 0,
             "games_lost": 0, "win_pct": 0 },
  "money": { "tracked": false,
             "message": "Dues and payouts are not tracked yet." }
}
```

`team` is `null` when no team resolves for the player/season
combination (schedule and stats are then empty/zero, not an error).

### What Phase 1 defers

All explicitly out of scope per PM decision, not oversights:

- Real player login, player-facing self-service portal.
- Payment entry/history, payout calculations (blocked on the money/dues
  data model not existing at all yet -- a project of its own, not a
  screen addition). **Update 2026-08-29:** the data model now exists and
  Phase 2 added read-only dues status/history to this screen (see below)
  -- payment *entry* and payout display remain out of scope here; use
  the Financial screen for those.
- Communication/notifications, mobile-specific layout.
- Handicap history/trend view (the `handicap_history` table exists but
  has no query or endpoint reading it by `player_id` anywhere in the
  codebase; current handicap value only for Phase 1).
- Multi-season player view or a season selector on this screen (active
  season only for Phase 1).
- Two incidental gaps found during discovery, explicitly deferred to
  separate follow-ups rather than bundled here:
  - `GetPlayerStats`'s `WinPct` field is never scanned/computed in the
    season-scoped query (`backend/storage/sqlite/round_store.go`) --
    always `0.0` in every response, including this endpoint's `stats`.
  - The league-scoped variant of the same query still drops
    season-roster-only players (an `INNER JOIN teams` on the legacy
    `players.team_id`); only the season-scoped branch was fixed
    2026-08-23.

### Verification

`go test ./...` and `go build ./...` pass, including five new focused
tests in `handlers/api_player_overview_test.go`: explicit `season_id`,
omitted `season_id` (defaults to active season), missing player (404),
a player not rostered for the selected season but with a direct
`players.team_id` (falls back correctly, zeroed stats), and a player
with no resolvable team at all (`team: null`, empty schedule, zeroed
stats) -- every response in every case confirms `money.tracked=false`.
`node --check` passes on all six changed/new JS files. Manually verified
end to end against a local server build with seeded demo data: explicit
and omitted `season_id` both resolved correctly, a missing player
returned 404, and the new nav entry/section/script tags all served
correctly from the running binary. Actual browser rendering of the new
screen and the "View Overview" button remain **NOT VERIFIED (no
browser)** in this developer's tool session.

## Player Overview Phase 2 Implementation

**Status:** `implemented`
**Date:** `2026-08-29`

### What Phase 2 added

Replaced the Phase 1 money placeholder with real per-player season dues
status, backed by the `finances` domain added in Financial Phase 1.
Admin-viewable only -- no player login or permissions added. Payout
display was considered and deliberately left out: payouts are team-level,
not player-level, and per PM decision were only worth adding here if
trivial; composing team-level payout data onto a player-level screen
was judged not trivial enough to bundle into this phase.

- New read method `FinanceStore.ListDuesPaymentsByPlayer(ctx, seasonID,
  playerID)` (`backend/domains/finances/store.go`,
  `backend/storage/sqlite/finances_store.go`), plus the matching
  `FinanceService`/`FinanceManager` passthrough. Added instead of
  duplicating SQL in the Player Overview handler, per PM's implementation
  guidance -- mirrors `ListDuesPayments`' query but scopes by
  `player_id` too, since Player Overview needs one player's history,
  not the full season list the Financial screen's `GET .../dues` uses.
- `getPlayerOverview` (`handlers/api_player_overview_handler.go`) gained
  two new parameters, `financeMgr FinanceManager` and `ruleMgr
  RuleManager`, and a new `playerOverviewMoney` helper that composes the
  money section: `ListDuesPaymentsByPlayer` for `paid`/`total_paid`/
  `payments`, plus `RuleManager.List` matched against the `dues_amount`
  season_rules key for display only (same convention Financial Phase 1
  established). `financeMgr` may be `nil` (the shared `testServer()`
  test helper does not wire one); when `nil`, money falls back to the
  original Phase 1 placeholder rather than erroring, so none of the six
  existing Phase 1 tests needed to change. `ruleMgr` is always non-nil
  in production (`RuleManager` is required elsewhere in `Register`), but
  is still nil-checked in the helper rather than assumed.
- Money composition uses the player's own ID directly -- it does not
  depend on team/roster resolution, so a player with no season roster
  entry (or no team at all) still gets a real dues status rather than
  silently inheriting the untracked placeholder.
- Both manager calls fail soft, matching every other optional
  composition step in this handler (team lookup, schedule, stats): a
  `FinanceManager`/`RuleManager` error degrades to an empty/zero result
  rather than failing the whole overview request.
- Route registration (`api_player_overview_routes.go`,
  `handlers/api.go`) passes `deps.FinanceMgr` and `deps.RuleMgr` through
  but does not add them to the registration gate (still `MatchMgr !=
  nil && RoundMgr != nil`) -- a missing `FinanceMgr` is a valid,
  supported state (the placeholder fallback above), not a reason to
  refuse to mount the route. **Update 2026-08-30:** the route is now
  wrapped in `clearanceAuth(deps.ApplyAuth, ...)` -- see "Privacy
  inconsistency -- resolved" below.
- Frontend (`web/domains/players/player-overview-page-component.js`):
  the money section now renders a Dues card (paid/unpaid badge, total
  paid, last payment date, dues amount when configured) instead of the
  static warning banner, falling back to the original warning banner
  only when `money.tracked` is `false`.

### Response shape

```json
"money": {
  "tracked": true,
  "paid": true,
  "total_paid": 25.5,
  "dues_amount": "25",
  "payments": [
    { "id": 1, "season_id": 1, "player_id": 1, "team_id": 2,
      "amount": 25.5, "paid_at": "2026-01-05T00:00:00Z",
      "recorded_by_user_id": 1, "note": "cash",
      "created_at": "2026-08-29T..." }
  ]
}
```

When unpaid, `paid` is `false`, `total_paid` is `0`, `payments` is `[]`,
and `message` is `"No dues payment recorded yet."` (`dues_amount` is
omitted whenever the season has no `dues_amount` rule configured, and
`message` is omitted whenever `paid` is `true`).

### Privacy inconsistency -- resolved 2026-08-30

At initial ship (2026-08-29), Financial Phase 1's ALL-four-routes-
protected convention (`clearanceAuth`, league_admin/admin/system_admin --
money data should not be public just because other domain reads are)
was not extended to Player Overview: `GET /api/players/{id}/overview`
stayed an unprotected GET even though it now surfaced the same kind of
per-player dues status (paid/unpaid, amounts, payment dates). This was
flagged as open question `PLAYERS-Q002` rather than resolved
unilaterally, since the approved scope for that phase did not include
an auth change.

**PM decision (2026-08-30):** protect the whole route with
`clearanceAuth(deps.ApplyAuth, ...)`, the same role gate Financial
Phase 1 uses -- not field-level auth on just `money`. Rationale: this
screen is admin-facing until real player login/permissions exist, and
gating the full route is simpler and clearer than trying to keep a
field-level exception legible. `registerPlayerOverviewRoute` now wraps
its handler in `clearanceAuth` exactly like `registerFinanceRoutes`
does; the nav entry (`#nav-item-player-overview`) is hidden unless the
resolved identity qualifies, reusing the same role check the Financial
nav entry already computes in `updateIdentityUI()` (`web/app.js`).
Under the shared `testServer()` test helper (no `ApplyAuth` wired),
`clearanceAuth` is a passthrough and the route stays open -- the same
behavior every other clearanceAuth-protected route already has under
that setup, not a special case introduced here. This resolves
`PLAYERS-Q002`; see `doc/roadmap.md`'s Resolved Questions table for the
recorded decision text.

**Follow-up correction (2026-08-30, same day):** the Players list's
"View Overview" row button initially still rendered unconditionally --
a non-admin viewer would see it, click it, and get the existing
401/403 toast rather than a broken page, which was safe but did not
match the admin-facing screen intent. PM asked for the row action
hidden too, matching the nav's intent exactly. Fixed by extracting the
role check in `web/app.js` into a shared top-level function,
`hasFinanceAdminRole(identity)`, used by `updateIdentityUI()` for both
nav entries *and* passed as a new third `canViewPlayerOverview`
argument to `<players-page>.refresh()`. `players-page-component.js`
stores it as a private field and only renders the "View Overview"
button when it is `true` -- no auth logic was added inside the
component itself, per PM's explicit instruction; it only renders what
the shell tells it. One known limitation, not fixed: setting or
clearing the Admin Key only calls `updateIdentityUI()` (nav
visibility), not `loadSection()`, so if a viewer is already on the
Players page when their key changes, the row button won't
appear/disappear until they navigate away and back -- this matches how
every other identity-gated nav item already behaves in this shell (none
of them force a live re-render of the currently active section either),
so it was not treated as a new gap introduced by this fix.

### What Phase 2 defers

- Payout display on Player Overview (team-level; judged not trivial
  enough to bundle in per PM's "unless trivial" guidance).
- Payment entry from Player Overview (still Financial-screen-only).
- Any change to the Financial screen or standings behavior (none made).

### Verification

`go test ./... -count=1` and `go build ./...` pass, including: 1 new
`FinanceService` delegation test and 3 new `FinanceStore` tests
(empty/newest-first/scoped-by-player) for `ListDuesPaymentsByPlayer`;
4 new handler tests in `handlers/api_player_overview_money_test.go`
(no payments -> `tracked=true paid=false`, one payment -> `tracked=true
paid=true` with correct `total_paid`, a `dues_amount` rule shown in the
response, and an unrostered player still getting a real money status);
and 7 new handler tests in `handlers/api_player_overview_auth_test.go`
(no header -> 401 with `WWW-Authenticate`, invalid token -> 403, the
static `LEAGUE_ADMIN_TOKEN` -> 403, `score_keeper` role -> 403, and
league_admin/admin/system_admin each reaching the handler). All six
pre-existing Phase 1 Player Overview tests pass unchanged (they use the
shared `testServer()` helper, which has no `ApplyAuth`/`FinanceMgr`
wired, so they continue to exercise the unauthenticated,
`tracked=false` fallback path). `node --check` passes on the changed JS
files. Manually verified end to end against a local server build:
created a league/season/team/player, confirmed the initial overview
showed `tracked:true paid:false`, recorded a real dues payment through
the Financial API, confirmed the overview updated to `paid:true` with
the correct `total_paid` and payment history, then set a `dues_amount`
season rule and confirmed it appeared in the response (this manual pass
predates the auth correction and used an unauthenticated local build;
the automated auth tests above are what verify the corrected behavior).
Actual browser rendering of the new Dues card and the corrected nav
gating remain **NOT VERIFIED (no browser)** in this developer's tool
session. The row-action follow-up (`hasFinanceAdminRole` extraction,
`players-page-component.js`'s conditional button render) added no new
Go code, so `go test ./... -count=1`/`go build ./...` were rerun for
regression safety only; `node --check` was rerun on `web/app.js`,
`web/domains/players/players-page-component.js`, and
`web/domains/players/player-overview-page-component.js`. Actual browser
confirmation that the row button appears/disappears with the resolved
identity remains **NOT VERIFIED (no browser)** as well.

## Questions

### PLAYERS-Q001 - Quick-add requirements

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-07-14`

**Context:** League night cannot stop for complete registration, but duplicate
players and unsupported handicap values must be avoided.

**Resolution (Phase 1):**
- Quick-add lives on the Players page as a simplified modal.
- Required fields: at least one of first name or last name; diff rating (default 0).
- Optional fields: team. Player number, phone, email, and admin hold are omitted
  from quick-add and completed later via Edit Player.
- Duplicate detection: deferred. No DB unique constraint on name or player number.
- INCOMPLETE profile status and close-week blocking: deferred to a later phase.
- Match-entry quick-add integration: deferred.

## Decision History

### 2026-06-08 - Share players across leagues

**Status:** `accepted`

One player record may participate in multiple nights and formats.

### 2026-06-08 - Allow any system player to substitute

**Status:** `accepted`

Substitute eligibility is not restricted to the same league or season.

### 2026-07-14 - Quick-add Phase 1: Players page only, name + handicap minimum

**Status:** `accepted`

Phase 1 quick-add lives on the Players page as a simplified modal. The minimum
field set is at least one name and a diff rating (default 0). Player number,
contact fields, and admin hold are omitted and completed later via Edit Player.
INCOMPLETE status, close-week blocking, and match-entry integration are
deferred. Resolves PLAYERS-Q001.

### 2026-08-19 - Quick-add duplicate warning Phase A

**Status:** `accepted`

Added a client-side, warn-only, normalized-exact-name duplicate check to
Players page quick-add, comparing against the already-loaded league-scoped
player list. No backend, schema, or API change. Safe merge remained deferred
at this point (see the Safe Merge Backend Phase A entry below); INCOMPLETE
profile status and match-entry quick-add remain deferred.

### 2026-08-19 - Safe merge backend Phase A

**Status:** `accepted`

Added `players.PlayerService.MergePlayers` / `PlayerStore.MergePlayers` and
`POST /api/players/{id}/merge`: repoints all nine supported player-ID
references across seven tables from a source (duplicate) player to a target
(surviving) player in one transaction, then deletes the source. Backend and
store only -- no merge UI in this phase. Statuses: 400 same player, 404
missing player, 409 for an unsafe collision (season-roster, round-results
participation, self-opponent, or lineup-plan). The round-results check
started as "both as home" only and was broadened during review to any
participation (home or away, any row) in the same match/round, since the
narrower check missed both-away and cross-role cases. The self-opponent,
round-participation, and lineup-plan blockers were all discovered during
implementation, not specified up front. Handicap snapshot columns and
handicap_history values are preserved exactly; only foreign keys move.

### 2026-08-27 - Player Overview Phase 1: first whole-app player screen

**Status:** `accepted`

Added `GET /api/players/{id}/overview` (unprotected read, handler-level
composition per PM decision -- no new players-domain service) and a new
`<player-overview-page>` frontend screen showing one player's season
team, schedule, stats, current handicap, and a money placeholder
(dues/payouts are not tracked anywhere in this schema, confirmed during
discovery). Team resolution prefers `season_rosters`, falling back to
the player's direct `team_id`. A new `SeasonManager.GetPlayerRosterTeam`
passthrough exposes an existing store lookup that was previously
internal-only. Real player login, a self-service portal, payment
entry/payouts, handicap history, and multi-season views are all
explicitly deferred, not oversights. Two incidental gaps found during
discovery (`GetPlayerStats`'s `WinPct` always zero; the league-scoped
stats query still dropping roster-only players) were deliberately not
bundled into this phase. See "Player Overview Phase 1 Implementation"
above for full detail.

### 2026-08-29 - Player Overview Phase 2: real dues status

**Status:** `accepted`

Replaced the Phase 1 money placeholder with real per-player season dues
status (paid/unpaid, total paid, payment history, configured dues
amount), backed by a new `FinanceStore.ListDuesPaymentsByPlayer` read
method added to the `finances` domain from Financial Phase 1 rather than
duplicating SQL in the handler. Admin-viewable only; no player login or
permissions added. Payout display and payment entry from this screen
were both left out -- payouts are team-level (not trivial to bundle
here per PM's "unless trivial" guidance) and payment entry stays
Financial-screen-only. `FinanceManager` is optional in the route's
parameters (falls back to the Phase 1 placeholder when nil, e.g. in the
shared test helper), so no existing Phase 1 test needed to change.
At initial ship, Player Overview's money section was reachable through
an unprotected GET even though Financial Phase 1 deliberately put the
same money data behind `clearanceAuth` -- flagged as open question
`PLAYERS-Q002` rather than resolved unilaterally. See the "Auth
correction" entry below for the resolution.

### 2026-08-30 - Player Overview Phase 2 auth correction: protect the route

**Status:** `accepted`

Resolves `PLAYERS-Q002`. `GET /api/players/{id}/overview` is now
protected by `clearanceAuth` (league_admin/admin/system_admin) -- the
same role gate Financial Phase 1 uses -- rather than field-level auth
on just `money`, per explicit PM decision: this screen is admin-facing
until real player login/permissions exist, and gating the whole route
is simpler and clearer. The nav entry is hidden unless the resolved
identity qualifies. Same-day follow-up: PM asked for the Players list's
"View Overview" row button hidden too, matching the nav's intent
exactly (it had initially been left rendering unconditionally, failing
safely with a 401/403 toast for non-admins rather than a broken page,
but not matching the admin-facing decision). Fixed by extracting the
role check into a shared `hasFinanceAdminRole(identity)` function in
`web/app.js`, used by both nav entries and passed into
`<players-page>.refresh()` as a new `canViewPlayerOverview` argument --
no auth logic was added inside the component. See "Privacy
inconsistency -- resolved 2026-08-30" in "Player Overview Phase 2
Implementation" above for full detail.
