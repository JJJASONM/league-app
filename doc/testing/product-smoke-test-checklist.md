# Product Smoke-Test Checklist

**Owner:** Product test readiness
**Status:** draft
**Last reviewed:** 2026-08-20

Purpose: give an admin tester a pass/fail path through the widest useful
slice of the app before more feature work is added, on staging or a local
build. This is a checklist to run, not a report of a run already performed --
none of the steps below have been executed against staging (no staging
mutation or deploy was performed; see Scope Note at the end).

---

## Admin Key setup (resolved 2026-08-20)

Every admin mutation route (league/team/player CRUD including quick-add,
season setup, rules, skipped-weeks, bye-requests, roster, schedule
generate/pushback-apply, lineup plans, match assign/results/rounds, week
close/reopen, season close/reopen, `POST /api/backup`) requires a
`Bearer <personal-key>` header. Until 2026-08-20 nothing in the browser
attached one -- see the branch history below if you need the original
finding. `browser-admin-auth-bridge` closes that gap:

- A new **Admin Key** button in the sidebar opens a small modal to paste a
  personal API key (created via `POST /api/users` -- see Before You Start
  step 3).
- The key is stored in `sessionStorage` for that browser tab only (never
  `localStorage`) -- gone when the tab closes, or immediately via the
  modal's Clear button.
- `web/lib/api-client.js`'s shared `api()` helper now attaches
  `Authorization: Bearer <key>` to every request when a key is set. Every
  domain screen that uses the shared client is covered automatically --
  no per-screen changes were needed.
- A 401 (no/expired key) or 403 (wrong role) now surfaces as a specific,
  actionable toast ("Admin key required..." / "Admin key was rejected...")
  instead of a generic error.
- The static `LEAGUE_ADMIN_TOKEN` still never appears in browser code --
  it's used once, server-side/via curl, to bootstrap the personal-key user.
- **Handicap Review & Apply** (`web/domains/handicaps/handicap-review-component.js`)
  keeps its own separate, already-working manual token field and
  session-memory-only handling, unchanged -- it was not migrated to the
  shared bridge (see the branch handoff for why).

**What to do before testing:** open the app, click **Admin Key** in the
sidebar, paste the key from Before You Start step 3, click Save. That one
action now unblocks every write step in this checklist except Handicap
Review, which still uses its own separate token field the first time you
Apply a recommendation.

---

## Before You Start

### 1. Confirm the app is running and healthy

```
curl http://localhost:8080/healthz
```
Expect `{"status":"ok"}`, 200. (Unauthenticated -- no key needed.) On
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

### 3. Bootstrap one personal-key user

```
curl -X POST http://localhost:8080/api/users \
  -H "Authorization: Bearer $env:LEAGUE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"smoke-test-admin"}'
```
Every user created this way gets `role="admin"` (hardcoded in
`backend/storage/sqlite/apply_auth_store.go`) -- the backward-compatible
alias that satisfies both `league_admin`-tier routes and the stricter
`system_admin`-tier backup route, so one bootstrap user covers every gated
route in this checklist. Save the returned `api_key` (shown once, never
re-retrievable) -- paste it into the browser's **Admin Key** sidebar button
(see above) for UI testing, and/or keep it as `$KEY` for any curl checks
below.

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
seeded** -- schedule generation must be done by hand against one of the
seeded active seasons (season 2 or season 4) to exercise match entry, close
week, standings, handicap review, or recap.

**Scoresheet fixtures (`db/scoresheet_fixtures.go`, via
`-seed-scoresheet-fixtures`):** a separate, self-contained 4-team "Fixture
Scoresheet League" with lineups and matches already generated across 5
weeks (blank / partial / completed / tie-break / mixed-table examples --
see `doc/domains/matches/scoresheet-fixtures.md`). This is the fastest path
to a working match-entry / close-week / standings / recap smoke test without
touching schedule generation at all.

**Resolved 2026-08-23** by `staging-seed-fixtures-option`:
`scripts/deploy/seed-staging.ps1` now accepts an opt-in `-SeedFixtures`
switch. Default behavior (no switch) is unchanged -- base seed only. With
the switch, it also runs `--seed-scoresheet-fixtures --fixture-weeks all`
against the same staging executable and data directory immediately after
the base seed succeeds, and verifies the fixture league appears via the API
before reporting success:

```powershell
.\scripts\deploy\seed-staging.ps1 -ConfirmSeed SEED-STAGING -SeedFixtures
```

A fixture-seed failure rolls back the same way a base-seed failure already
did (restore the pre-seed backup, restart staging on the old data). See
`QUICKSTART.md`'s Staging section for the one-line usage.

**Gap found:** the Dashboard's score-entry readiness gate (Phase A, shipped
2026-08-19) has no seed data exercising its "not ready" (disabled button)
state -- every seeded/fixture lineup is complete. To see the disabled state,
temporarily delete one `lineup_plans` row for an overdue week's team via
`sqlite3` or a `DELETE /api/lineup-plans/{id}` (browser, with Admin Key set,
or curl), then restore it after. Not a bug -- just nothing in current seed
data demonstrates the gate actually gating.

**Gap found:** Player safe-merge backend (Phase A, shipped 2026-08-19) has
no admin UI yet (tracked as deferred in `doc/roadmap.md`). It can only be
smoke-tested via curl today (see the Player Safe-Merge Backend section
below) -- the Admin Key bridge doesn't change this, since there's no browser
screen to test regardless of auth.

---

## Checklist

Each item lists the browser path, then a pass/fail checkpoint. Write steps
assume the Admin Key is already set (see above); a curl equivalent is given
for anyone who prefers verifying the backend directly with `$KEY`.

### 1. League / Team / Player Setup

- Browser: Dashboard -> "Manage Leagues" (sidebar) opens the league modal.
  - [ ] Existing leagues (Demo Pool League, Demo 9-Ball League) list correctly.
  - [ ] Creating/editing a league via the modal succeeds with Admin Key set.
    - curl check: `curl -X POST http://localhost:8080/api/leagues -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" -d '{"name":"Smoke Test League","game_format":"8ball"}'` -> expect 201.
- Browser: Teams nav.
  - [ ] Team list renders per active league, with rosters/captains as seeded.
  - [ ] Add/edit team succeeds with Admin Key set.
    - curl check: `POST /api/teams` with `$KEY` -> expect 201.
- Browser: Players nav.
  - [ ] Player list renders, sortable/filterable as designed, diff/handicap
        values match seed data.
  - [ ] "Add Player" (full modal) and Quick Add both succeed with Admin Key
        set.
    - curl check: `POST /api/players` with `$KEY` -> expect 201.

### 2. Player Quick-Add Duplicate Warning (Phase A)

- Browser: Players nav -> Quick Add -> type an existing player's name (e.g.
  "Rex Barlow" in Demo Pool League).
  - [ ] Warning appears naming the existing player and team, before create
        is attempted -- this check runs entirely client-side against the
        already-loaded player list, independent of the Admin Key.
  - [ ] Cancel closes the flow with no request sent.
  - [ ] "Add Anyway" succeeds with Admin Key set, once past the warning.
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
  - [ ] Save succeeds with Admin Key set.
    - curl check: `POST /api/seasons` with `$KEY` -> expect 201.
  - [ ] Existing seasons list correctly with active/draft/historical status
        chips matching seed data (season 1 historical, season 2 active,
        season 3 draft, for the 8-ball league).

### 5. Teams, Rosters, Rules (Season Setup)

- Browser: Seasons nav -> open a season -> Teams tab.
  - [ ] Season 3 (draft) shows 4 of 6 teams registered, one team with no
        captain -- matches the seed data's intentional partial state.
  - [ ] Add/remove season team, set captain -- all succeed with Admin Key set.
- Browser: Seasons nav -> open a season -> Roster tab.
  - [ ] "Available players" list for season 3's partially-rostered teams
        shows the specific unrostered players noted in `scripts/seed.sql`
        comments (e.g. Opal Kwan, Nina Park for Bridge Over Troubled Cues).
  - [ ] Add/remove roster player succeeds with Admin Key set.
- Browser: Seasons nav -> open a season -> Rules tab.
  - [ ] Seeded rule values render (handicap_multiplier, etc.) for season 2/4.
  - [ ] Editing a rule value succeeds with Admin Key set.

### 6. Skipped Weeks, Bye Requests, Schedule Generation

- Browser: Seasons nav -> Skipped Weeks.
  - [ ] Season 2's seeded skipped weeks (MLK Day, Memorial Day) render.
  - [ ] Add/remove a skipped week succeeds with Admin Key set.
- Browser: Seasons nav -> Bye Requests.
  - [ ] Empty state renders correctly (no bye requests are seeded).
  - [ ] Creating a bye request succeeds with Admin Key set.
- Browser: Schedule nav -> Generate Schedule (for a season with no matches
  yet, e.g. season 2 or 4, or a fresh test season).
  - [ ] Generate succeeds with Admin Key set.
    - curl check: `POST /api/matches/generate` with `$KEY` -> expect 200
      and a generated match list on `GET /api/matches?season_id=...`.

### 7. Schedule Pushback Preview / Apply

- Browser: Schedule nav -> pushback controls (visible once a schedule
  exists).
  - [ ] Preview (unauthenticated by design -- `pushback-preview` is
        intentionally unprotected despite POST) shows the shift plan with or
        without an Admin Key set.
  - [ ] Apply Pushback succeeds with Admin Key set.
    - curl check: `POST /api/seasons/{id}/schedule/pushback-apply` with
      `$KEY`.

### 8. Lineup Plans

- Browser: Lineup nav.
  - [ ] Fixture league (if scoresheet fixtures were loaded) shows full
        3-player lineups per team per week.
  - [ ] Save/delete a lineup plan succeeds with Admin Key set.
    - curl check: `POST /api/lineup-plans` with `$KEY`.

### 9. Dashboard Score-Entry Readiness Gate (Phase A)

- Browser: Dashboard nav, with an active league that has an overdue,
  unscored match.
  - [ ] Overdue week with a complete lineup shows an enabled "Enter Scores"
        button (this is the only state current seed/fixture data produces
        -- see Data Readiness gap above).
  - [ ] To see the disabled state: temporarily remove a lineup_plans row for
        an overdue week's team (browser with Admin Key set, curl, or direct
        sqlite3 edit), reload Dashboard, confirm the button is disabled with
        the "set lineups" explanatory link, then restore the row.
  - This whole check is read-driven (GET /api/matches, GET /api/lineup-plans)
    and unaffected by the Admin Key either way -- only the "set lineups"
    link target (Lineup Plans save) needs it, consistently with section 8.

### 10. Match Entry and Score Save

- Browser: Match Entry nav, pick a fixture-league match from week 1 (blank,
  ready for entry).
  - [ ] Scoresheet renders with correct lineup, handicaps, and game-entry
        grid.
  - [ ] Save Scoresheet succeeds with Admin Key set.
    - curl check: `POST /api/matches/{id}/rounds` with `$KEY` and a rounds
      payload -> expect 200, then confirm via `GET /api/matches/{id}/rounds`.
  - [ ] Week 3 fixture matches (pre-completed) display correctly as
        completed with correct adjusted scores and round-winner badges --
        this is read-only and browser-testable regardless of the key.

### 11. Close / Reopen Week

- Browser: Schedule nav -> Review & Close on a week with all matches scored
  (e.g. fixture week 3).
  - [ ] Validation preview renders (warnings, missing-score detection).
  - [ ] Confirm Close succeeds with Admin Key set.
    - curl check: `POST /api/seasons/{id}/weeks/{week}/close` with `$KEY`.
  - [ ] Reopen Week button succeeds with Admin Key set.
    - curl check: `POST /api/seasons/{id}/weeks/{week}/reopen` with `$KEY`.

### 12. Standings and Player Stats

- Browser: Standings nav / Player Stats nav, on a season with at least one
  closed week (section 11).
  - [ ] Standings reflect only officially closed-week results (per
        `doc/roadmap.md`'s stated invariant).
  - [ ] Player Stats table renders per-player win/loss/diff correctly.
  - Both are read-only GET screens -- browser-testable regardless of the key,
    once at least one week is closed.

### 13. Handicap Review / Apply

- Browser: Handicap nav.
  - [ ] Recommendations list renders (read-only, unauthenticated GET) for a
        season with closed-week history, e.g. the 9-ball league using its
        seeded `handicap_history` rows.
  - [ ] This screen still uses its own, separate token field (unchanged by
        the Admin Key bridge -- see the Admin Key setup section above):
        paste a personal key into its manual token field the first time you
        Apply a recommendation.
  - [ ] Applied change appears in the player's handicap and in
        `handicap_history`.

### 14. Week Recap

- Browser: Schedule nav -> Recap toggle on a closed week.
  - [ ] Recap panel renders match results, missing-match count, handicap
        changes applied, and next-week readiness -- fully read-only GET,
        browser-testable regardless of the key (once a week is closed via
        section 11).

### 15. Season Close / Reopen

- Browser: Seasons nav -> season detail -> Close Season / Reopen Season
  buttons.
  - [ ] Season 1 (already historical/closed in seed data) shows the Reopen
        affordance and season 2 (active) shows the Close affordance,
        confirming button visibility logic without needing a write.
  - [ ] Close Season succeeds with Admin Key set.
    - curl check: `POST /api/seasons/{id}/close` (exact path per
      `handlers/api_season_close_routes.go`) with `$KEY`.
  - [ ] Reopen Season succeeds with Admin Key set.

### 16. Backup and Health Endpoint

- Browser: sidebar "Backup DB" button.
  - [ ] Succeeds with Admin Key set (`role=admin` from bootstrap satisfies
        the stricter system-admin-tier check this route uses). Confirmed
        locally via a real browser-equivalent smoke test -- see the branch
        handoff for detail; this button had never worked from the browser
        since the Phase 6 backup-auth rollout
        (`doc/roadmap.md`, "Then" section, Phase 6, 2026-08-08) until now.
    - curl check: `POST /api/backup` with `$KEY` -> expect 200 and a backup
      file path in the response; confirm the file exists in the data
      directory's backup location.
- `GET /healthz`:
  - [ ] Returns `{"status":"ok"}`, 200, unauthenticated. Confirmed working
        locally (see Before You Start step 1).

---

## Known Gaps Summary

| # | Gap | Severity | Where | Status |
|---|-----|----------|-------|--------|
| 1 | Browser could not perform any admin write except Handicap Apply | ~~Critical~~ | `web/lib/api-client.js` | **Resolved 2026-08-20** by `browser-admin-auth-bridge` -- see Admin Key setup above |
| 2 | `seed-staging.ps1` does not load scoresheet fixtures, so a freshly seeded staging has no matches to test match-entry/close-week/standings/handicap/recap without manual schedule generation | ~~Medium~~ | `scripts/deploy/seed-staging.ps1` | **Resolved 2026-08-23** by `staging-seed-fixtures-option` -- pass `-SeedFixtures` |
| 3 | No seed/fixture data demonstrates the Dashboard readiness gate's "disabled" state | Low | seed data only; documented workaround above | Open |
| 4 | Player safe-merge has no admin UI (already tracked as deferred in `doc/roadmap.md`) | Low (known/tracked) | `doc/roadmap.md` "Player record maintenance" | Open |
| 5 | Staging health check in `staging-common.ps1` polls `/api/leagues`, not the dedicated `/healthz` the app already exposes | Low | `scripts/deploy/staging-common.ps1` | Open |

## Recommended Next Branches

1. Now that both the Admin Key bridge and the staging fixture-seed option
   are done, the next step is simply to run this checklist against actual
   staging end to end (base seed + `-SeedFixtures`) and record real
   pass/fail results here -- everything above is still a checklist to run,
   not a completed run.
2. Everything else discovered above (dashboard gate demo data, staging
   health-check endpoint choice, merge UI) is low severity and can stay
   backlog until that staging pass shows whether they're still worth
   prioritizing.
3. Separately discovered, out of scope for this checklist: the
   `.codex/skills/deploy-staging/scripts/` copies of these same staging
   scripts have drifted out of sync with `scripts/deploy/` independent of
   this checklist's work (older `staging-common.ps1` missing the WAL/SHM
   backup handling and `LEAGUE_ADMIN_TOKEN` resolution, and now also missing
   `-SeedFixtures`). Worth a small dedicated sync branch if both copies need
   to stay usable.

---

## Scope Note

This document's data/gap findings came from inspecting code, deploy/seed
scripts, and the local dev database -- no staging deployment, staging data
mutation, or staging network calls were made at any point. The Admin Key
bridge (2026-08-20) was verified locally: `node --check` on all changed/new
JS, the full Go test suite, and a real functional smoke test that loaded
the actual shipped `web/lib/admin-key-store.js` and `web/lib/api-client.js`
source into a sandboxed Node context and drove the real `api()` function
against the local dev server -- confirming the friendly 401 message with no
key, a successful `POST /api/players` after `setAdminKey()`, a return to
the 401 after `clearAdminKey()`, and unaffected GET reads throughout. Also
curl-verified `POST /api/leagues`, `/api/teams`, `/api/seasons`, and
`/api/backup` directly against the real key. All test data created during
that smoke test was cleaned up afterward. Running this checklist against
actual staging is still the next step.
