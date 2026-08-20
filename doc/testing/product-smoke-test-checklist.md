# Product Smoke-Test Checklist

**Owner:** Product test readiness
**Status:** draft
**Last reviewed:** 2026-08-19

Purpose: give an admin tester a pass/fail path through the widest useful
slice of the app before more feature work is added, on staging or a local
build. This is a checklist to run, not a report of a run already performed --
none of the steps below have been executed against staging as part of this
discovery pass (no staging mutation or deploy was performed; see Scope note
at the end).

---

## Critical finding: browser cannot perform admin writes today

**This blocks nearly every write step below and should be read before running
the rest of this checklist.**

`web/lib/api-client.js`'s shared `api()` helper -- used by every domain
screen except Handicap Review -- never attaches an `Authorization` header.
Every admin mutation route (league/team/player CRUD including quick-add,
season setup CRUD, rules, skipped-weeks, bye-requests, season teams/roster,
schedule generate/pushback-apply, lineup plan save/delete, match
assign/results/rounds, week close/reopen, season close/reopen, and
`POST /api/backup`) is gated by `clearanceAuth` / `systemAdminAuth`, both of
which require a `Bearer <personal-key>` header and return 401 without one.

Verified locally (not on staging) against the running dev server:

```
POST /api/players  (no auth header) -> 401
POST /api/leagues  (no auth header) -> 401
POST /api/teams    (no auth header) -> 401
POST /api/seasons  (no auth header) -> 401
POST /api/backup   (no auth header) -> 401
```

The only exception is the **Handicap Review & Apply** screen
(`web/domains/handicaps/handicap-review-component.js`), which has its own
manually-typed "personal key" input field and attaches `Authorization`
itself, bypassing the shared `api()` helper. Every other screen has no such
field -- there is no login, session, or key-storage mechanism anywhere else
in the frontend (`grep`-confirmed: `Authorization`/`Bearer`/`api_key` appear
in exactly two files, both under `web/domains/handicaps/`).

**Practical effect:** clicking Save/Add/Delete/Generate/Close/Reopen/Backup
on any screen except Handicap Review will silently fail with a 401 toast.
GET reads (browsing leagues, players, teams, seasons, schedule, standings,
stats, dashboard) are unaffected and work normally in the browser.

**Workaround for this checklist, until fixed:** bootstrap one personal-key
user via curl (see Before You Start), then verify write behavior via curl
instead of clicking the UI button. Each checklist section below marks which
steps are browser-testable today and which need the curl workaround.

**Recommended next branch:** see Recommended Next Branches at the end. This
is the top-priority item.

---

## Before You Start

### 1. Confirm the app is running and healthy

```
curl http://localhost:8080/healthz
```
Expect `{"status":"ok"}`, 200. (Unauthenticated -- no bootstrap needed.) On
staging, substitute the staging URL; `GET /api/leagues` is what
`scripts/deploy/staging-common.ps1`'s `Wait-StagingHealth` actually polls
today, not `/healthz` -- both work, but they're not the same check
(`/healthz` also pings the DB connection; `/api/leagues` does not
distinguish "empty league list" from "DB unreachable"). Minor inconsistency,
not a blocker.

### 2. Confirm LEAGUE_ADMIN_TOKEN is configured

`POST /api/users` (creates the personal-key user needed for step 3) is
gated by the static `LEAGUE_ADMIN_TOKEN`. Locally on this machine it is
confirmed set at the Windows User level, which `Start-StagingApp` in
`scripts/deploy/staging-common.ps1` will inherit when staging is started
under the same account. Confirm the same is true wherever staging actually
runs before testing there -- if unset, `Apply route: NOT MOUNTED` is logged
at startup and `POST /api/users` (and Handicap Apply's static-token
fallback) will not work either.

### 3. Bootstrap one personal-key user (curl workaround, one-time per environment)

```
curl -X POST http://localhost:8080/api/users \
  -H "Authorization: Bearer $env:LEAGUE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"smoke-test-admin"}'
```
Every user created this way gets `role="admin"` (hardcoded in
`backend/storage/sqlite/apply_auth_store.go`) -- the backward-compatible
alias that satisfies both `league_admin`-tier routes and the stricter
`system_admin`-tier backup route, so one bootstrap user covers every
gated route in this checklist. Save the returned `api_key` (shown once,
never re-retrievable) as `$KEY` for the curl commands below.

```
curl http://localhost:8080/api/users -H "Authorization: Bearer $env:LEAGUE_ADMIN_TOKEN"
```
lists existing users (without key hashes) if you need to check whether this
step was already done.

### 4. Load data

Base seed (leagues, teams, players, rosters, rules, skipped-weeks -- **no
matches, no schedule**):
```
go run . -data ./data -seed
```

Scoresheet fixtures (self-contained 4-team league with blank/partial/
completed matches across 5 weeks, ready for match-entry and close-week
testing without generating a schedule by hand):
```
go run . -data ./data -seed-scoresheet-fixtures -fixture-weeks all
```

Both are additive (`INSERT OR IGNORE` / upsert) and safe to run together or
re-run. See Data Readiness below for what each does and does not cover.

---

## Data Readiness

**Base seed (`scripts/seed.sql`, via `-seed`):** 2 leagues (8-ball, 9-ball),
5 seasons across historical/active/draft states, 13 teams, 40 players,
season rules, skipped weeks, season teams with partial captain assignment
(draft season 3 intentionally has one team with no captain, to exercise that
UI state), season rosters (including partially-rostered draft seasons so the
"available players" picker has something to show), and 13 explicit
`handicap_history` rows for the 9-ball league. **No matches or schedule are
seeded** -- schedule generation must be done by hand (or via curl, given the
auth gap) against one of the seeded active seasons (season 2 or season 4)
to exercise match entry, close week, standings, handicap review, or recap.

**Scoresheet fixtures (`db/scoresheet_fixtures.go`, via
`-seed-scoresheet-fixtures`):** a separate, self-contained 4-team "Fixture
Scoresheet League" with lineups and matches already generated across 5
weeks (blank / partial / completed / tie-break / mixed-table examples --
see `doc/domains/matches/scoresheet-fixtures.md`). This is the fastest path
to a working match-entry / close-week / standings / recap smoke test without
touching schedule generation at all.

**Gap found:** `scripts/deploy/seed-staging.ps1` only runs `--seed`, not
`-seed-scoresheet-fixtures`. Staging's out-of-the-box data (after a fresh
seed) will have the base league/team/player/roster data but no matches --
match-entry/close-week/standings/handicap/recap testing on staging requires
either generating a schedule by hand first or adding a `-SeedFixtures`
option to `seed-staging.ps1`. See Recommended Next Branches.

**Gap found:** the Dashboard's score-entry readiness gate (Phase A, shipped
2026-08-19) has no seed data exercising its "not ready" (disabled button)
state -- every seeded/fixture lineup is complete. To see the disabled state,
temporarily delete one `lineup_plans` row for an overdue week's team via
`sqlite3` or a curl `DELETE /api/lineup-plans/{id}`, then restore it after.
Not a bug -- just nothing in current seed data demonstrates the gate
actually gating.

**Gap found:** Player safe-merge backend (Phase A, shipped 2026-08-19) has
no admin UI yet (tracked as deferred in `doc/roadmap.md`). It can only be
smoke-tested via curl today (see the Player Safe-Merge Backend section
below).

---

## Checklist

Each item lists the browser path, then a pass/fail checkpoint. Steps marked
**[AUTH-BLOCKED]** cannot be completed by clicking the UI today -- use the
paired curl command instead, with `$KEY` from Before You Start step 3.

### 1. League / Team / Player Setup

- Browser: Dashboard -> "Manage Leagues" (sidebar) opens the league modal.
  - [ ] Existing leagues (Demo Pool League, Demo 9-Ball League) list correctly.
  - [ ] **[AUTH-BLOCKED]** Creating/editing a league via the modal fails
        with a 401 toast today.
    - curl check: `curl -X POST http://localhost:8080/api/leagues -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" -d '{"name":"Smoke Test League","game_format":"8ball"}'` -> expect 201.
- Browser: Teams nav.
  - [ ] Team list renders per active league, with rosters/captains as seeded.
  - [ ] **[AUTH-BLOCKED]** Add/edit team fails with 401 in the browser.
    - curl check: `POST /api/teams` with `$KEY` -> expect 201.
- Browser: Players nav.
  - [ ] Player list renders, sortable/filterable as designed, diff/handicap
        values match seed data.
  - [ ] **[AUTH-BLOCKED]** "Add Player" (full modal) and Quick Add both fail
        with 401 in the browser.
    - curl check: `POST /api/players` with `$KEY` -> expect 201.

### 2. Player Quick-Add Duplicate Warning (Phase A)

- Browser: Players nav -> Quick Add -> type an existing player's name (e.g.
  "Rex Barlow" in Demo Pool League).
  - [ ] Warning appears naming the existing player and team, before create
        is attempted -- this check runs entirely client-side against the
        already-loaded player list, so it fires even though the eventual
        Add is auth-blocked. Confirms the warning logic itself independent
        of the auth gap.
  - [ ] Cancel closes the flow with no request sent.
  - [ ] **[AUTH-BLOCKED]** "Add Anyway" still hits the same 401 as any other
        create, once past the warning.
  - [ ] Typing a unique name shows no warning.

### 3. Player Safe-Merge Backend (Phase A, no UI)

No browser path exists yet. curl-only:
```
curl -X POST http://localhost:8080/api/players/<source_id>/merge \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"target_id": <target_id>}'
```
  - [ ] A safe merge (two players with no overlapping season/round/lineup
        data) returns 200 with `{"status":"merged",...}`.
  - [ ] An unsafe merge (e.g. two players already on rosters in the same
        season) returns 409 with a Conflict message.
  - [ ] Same-ID merge returns 400; a nonexistent player ID returns 404.
  - Backend correctness for this endpoint is already covered by 23 automated
    tests (service + SQLite integration + handler/route); this step is
    about confirming it behaves the same way against real seeded data, not
    re-proving the logic.

### 4. Season Creation

- Browser: Seasons nav -> Add Season.
  - [ ] Form renders with league/name/dates/schedule-type fields.
  - [ ] **[AUTH-BLOCKED]** Save fails with 401.
    - curl check: `POST /api/seasons` with `$KEY` -> expect 201.
  - [ ] Existing seasons list correctly with active/draft/historical status
        chips matching seed data (season 1 historical, season 2 active,
        season 3 draft, for the 8-ball league).

### 5. Teams, Rosters, Rules (Season Setup)

- Browser: Seasons nav -> open a season -> Teams tab.
  - [ ] Season 3 (draft) shows 4 of 6 teams registered, one team with no
        captain -- matches the seed data's intentional partial state.
  - [ ] **[AUTH-BLOCKED]** Add/remove season team, set captain -- all 401.
- Browser: Seasons nav -> open a season -> Roster tab.
  - [ ] "Available players" list for season 3's partially-rostered teams
        shows the specific unrostered players noted in `scripts/seed.sql`
        comments (e.g. Opal Kwan, Nina Park for Bridge Over Troubled Cues).
  - [ ] **[AUTH-BLOCKED]** Add/remove roster player -- 401.
- Browser: Seasons nav -> open a season -> Rules tab.
  - [ ] Seeded rule values render (handicap_multiplier, etc.) for season 2/4.
  - [ ] **[AUTH-BLOCKED]** Editing a rule value -- 401.

### 6. Skipped Weeks, Bye Requests, Schedule Generation

- Browser: Seasons nav -> Skipped Weeks.
  - [ ] Season 2's seeded skipped weeks (MLK Day, Memorial Day) render.
  - [ ] **[AUTH-BLOCKED]** Add/remove a skipped week -- 401.
- Browser: Seasons nav -> Bye Requests.
  - [ ] Empty state renders correctly (no bye requests are seeded).
  - [ ] **[AUTH-BLOCKED]** Creating a bye request -- 401.
- Browser: Schedule nav -> Generate Schedule (for a season with no matches
  yet, e.g. season 2 or 4, or a fresh test season).
  - [ ] **[AUTH-BLOCKED]** Generate -- 401.
    - curl check: `POST /api/matches/generate` with `$KEY` -> expect 200
      and a generated match list on `GET /api/matches?season_id=...`.

### 7. Schedule Pushback Preview / Apply

- Browser: Schedule nav -> pushback controls (visible once a schedule
  exists).
  - [ ] Preview (unauthenticated by design --
        `pushback-preview` is intentionally unprotected despite POST) shows
        the shift plan without requiring auth. This should work in the
        browser today.
  - [ ] **[AUTH-BLOCKED]** Apply Pushback -- 401.
    - curl check: `POST /api/seasons/{id}/schedule/pushback-apply` with
      `$KEY`.

### 8. Lineup Plans

- Browser: Lineup nav.
  - [ ] Fixture league (if scoresheet fixtures were loaded) shows full
        3-player lineups per team per week.
  - [ ] **[AUTH-BLOCKED]** Save/delete a lineup plan -- 401.
    - curl check: `POST /api/lineup-plans` with `$KEY`.

### 9. Dashboard Score-Entry Readiness Gate (Phase A)

- Browser: Dashboard nav, with an active league that has an overdue,
  unscored match.
  - [ ] Overdue week with a complete lineup shows an enabled "Enter Scores"
        button (this is the only state current seed/fixture data produces
        -- see Data Readiness gap above).
  - [ ] To see the disabled state: temporarily remove a lineup_plans row for
        an overdue week's team (curl `DELETE /api/lineup-plans/{id}` with
        `$KEY`, or direct sqlite3 edit locally), reload Dashboard, confirm
        the button is disabled with the "set lineups" explanatory link, then
        restore the row.
  - This whole check is read-driven (GET /api/matches, GET /api/lineup-plans)
    and not affected by the auth gap -- only the "set lineups" link target
    (Lineup Plans save) is auth-blocked, consistently with section 8.

### 10. Match Entry and Score Save

- Browser: Match Entry nav, pick a fixture-league match from week 1 (blank,
  ready for entry).
  - [ ] Scoresheet renders with correct lineup, handicaps, and game-entry
        grid.
  - [ ] **[AUTH-BLOCKED]** Save Scoresheet -- 401.
    - curl check: `POST /api/matches/{id}/rounds` with `$KEY` and a rounds
      payload -> expect 200, then confirm via `GET /api/matches/{id}/rounds`.
  - [ ] Week 3 fixture matches (pre-completed) display correctly as
        completed with correct adjusted scores and round-winner badges --
        this is read-only and browser-testable today.

### 11. Close / Reopen Week

- Browser: Schedule nav -> Review & Close on a week with all matches scored
  (e.g. fixture week 3).
  - [ ] Validation preview renders (warnings, missing-score detection).
  - [ ] **[AUTH-BLOCKED]** Confirm Close -- 401.
    - curl check: `POST /api/seasons/{id}/weeks/{week}/close` with `$KEY`.
  - [ ] **[AUTH-BLOCKED]** Reopen Week button -- 401.
    - curl check: `POST /api/seasons/{id}/weeks/{week}/reopen` with `$KEY`.

### 12. Standings and Player Stats

- Browser: Standings nav / Player Stats nav, on a season with at least one
  closed week (requires section 11's curl workaround first, since closing
  is auth-blocked).
  - [ ] Standings reflect only officially closed-week results (per
        `doc/roadmap.md`'s stated invariant).
  - [ ] Player Stats table renders per-player win/loss/diff correctly.
  - Both are read-only GET screens -- fully browser-testable today once
    at least one week is closed via the curl workaround.

### 13. Handicap Review / Apply

- Browser: Handicap nav.
  - [ ] Recommendations list renders (read-only, unauthenticated GET) for a
        season with closed-week history, e.g. the 9-ball league using its
        seeded `handicap_history` rows.
  - [ ] This is the **one screen with a working browser auth path today**:
        paste a personal key (from Before You Start step 3) into its manual
        token field, then Apply a recommendation.
  - [ ] Applied change appears in the player's handicap and in
        `handicap_history`.

### 14. Week Recap

- Browser: Schedule nav -> Recap toggle on a closed week.
  - [ ] Recap panel renders match results, missing-match count, handicap
        changes applied, and next-week readiness -- fully read-only GET,
        browser-testable today (once a week is closed via section 11's
        curl workaround).

### 15. Season Close / Reopen

- Browser: Seasons nav -> season detail -> Close Season / Reopen Season
  buttons.
  - [ ] Season 1 (already historical/closed in seed data) shows the Reopen
        affordance and season 2 (active) shows the Close affordance,
        confirming button visibility logic without needing a write.
  - [ ] **[AUTH-BLOCKED]** Close Season -- 401.
    - curl check: `POST /api/seasons/{id}/close` (exact path per
      `handlers/api_season_close_routes.go`) with `$KEY`.
  - [ ] **[AUTH-BLOCKED]** Reopen Season -- 401.

### 16. Backup and Health Endpoint

- Browser: sidebar "Backup DB" button.
  - [ ] **[AUTH-BLOCKED]** Fails with 401 -- verified locally, see the
        Critical Finding above. This button has never worked from the
        browser since the Phase 6 backup-auth rollout
        (`doc/roadmap.md`, "Then" section, Phase 6, 2026-08-08).
    - curl check: `POST /api/backup` with `$KEY` -> expect 200 and a backup
      file path in the response; confirm the file exists in the data
      directory's backup location.
- `GET /healthz`:
  - [ ] Returns `{"status":"ok"}`, 200, unauthenticated. Confirmed working
        locally (see Before You Start step 1).

---

## Known Gaps Summary

| # | Gap | Severity | Where |
|---|-----|----------|-------|
| 1 | Browser cannot perform any admin write except Handicap Apply -- shared `api()` client never sends an Authorization header, and no other screen has a way to obtain/store a personal key | **Critical** | `web/lib/api-client.js`, all domain screens except handicaps |
| 2 | `seed-staging.ps1` does not load scoresheet fixtures, so a freshly seeded staging has no matches to test match-entry/close-week/standings/handicap/recap without manual schedule generation | Medium | `scripts/deploy/seed-staging.ps1` |
| 3 | No seed/fixture data demonstrates the Dashboard readiness gate's "disabled" state | Low | seed data only; documented workaround above |
| 4 | Player safe-merge has no admin UI (already tracked as deferred in `doc/roadmap.md`) | Low (known/tracked) | `doc/roadmap.md` "Player record maintenance" |
| 5 | Staging health check in `staging-common.ps1` polls `/api/leagues`, not the dedicated `/healthz` the app already exposes | Low | `scripts/deploy/staging-common.ps1` |

## Recommended Next Branches

1. **`browser-admin-auth-bridge`** (highest priority -- unblocks nearly every
   item in this checklist). Smallest safe shape: generalize the pattern
   already proven on the Handicap Review screen -- a single place in the
   shell (e.g. a sidebar field or small settings panel) where an admin
   pastes a personal key once per browser session, stored in memory or
   `sessionStorage`, with `web/lib/api-client.js`'s shared `api()` helper
   attaching it to every request when present. This matches the roadmap's
   own framing ("browser sessions and JWTs are deferred until... a users
   management screen creates the concrete need") -- a paste-once bridge is
   not full session/JWT work, just closing the gap the existing
   `clearanceAuth` rollout left open on the frontend side.
2. **`staging-seed-fixtures-option`** -- add an opt-in switch to
   `seed-staging.ps1` to also run `-seed-scoresheet-fixtures`, so a fresh
   staging seed has ready-to-use match data without manual schedule
   generation. Small, isolated, deploy-tooling-only change.
3. Everything else discovered above (dashboard gate demo data, staging
   health-check endpoint choice, merge UI) is low severity and can stay
   backlog until the auth bridge unblocks broader testing and shows whether
   they're still worth prioritizing.

---

## Scope Note

This discovery pass inspected code, deploy/seed scripts, and the local dev
database; it made no staging deployment, no staging data mutation, and no
staging network calls. All empirical checks (the 401 confirmations,
`/healthz`, env var checks) were run against the local dev server and local
Windows environment only, per the "do not mutate staging data or deploy
unless PM explicitly asks" instruction. Running this checklist against
actual staging is the next step once PM decides how to sequence it against
the `browser-admin-auth-bridge` branch above.
