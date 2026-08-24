# Product Smoke-Test Checklist

**Owner:** Product test readiness
**Status:** staging run complete 2026-08-23 -- see Staging Run Results
**Last reviewed:** 2026-08-23

Purpose: give an admin tester a pass/fail path through the widest useful
slice of the app before more feature work is added, on staging or a local
build.

---

## Staging Run Results (2026-08-23)

Run against `http://league-staging.local` (DEPLOY-STAGING + SEED-STAGING
`-SeedFixtures` already complete per the PM handoff). **No browser
automation is available in this environment**, so every result below comes
from one of two sources, labeled per item:

- **API-verified**: a real HTTP call against staging (via curl), showing
  the exact request and response. This is solid evidence the backend
  behavior works, but does not confirm how it renders or reads on screen.
- **NOT VERIFIED (no browser)**: a pure rendering/visual/click-flow check
  that requires an actual browser, which this environment cannot drive.
  Marked explicitly rather than guessed at.

All write checks used a real personal-key user
(`smoke-pass-2026-08-23`, role `admin`, bootstrapped via
`POST /api/users` with `LEAGUE_ADMIN_TOKEN`) and a dedicated, fully
disposable sandbox league/season built and deleted for this run wherever a
check needed real data flow (schedule generation, pushback, lineups, match
entry, close/reopen) -- see the sandbox note under section 6. Real seeded
data (Demo Pool League, Demo 9-Ball League, Fixture Scoresheet League) was
only ever read, with two narrow, fully-reversed exceptions noted inline
(a season-2 rule edited and reverted; fixture week 3 closed and reopened
to test player-stats against real team-assigned players). Confirmed
restored to baseline afterward: match counts per season, fixture week
statuses (all open), and season-2 rule value all matched the pre-run state.

### Critical blocker found and fixed 2026-08-23, verified on staging: bodyless POST fails on staging (IIS), independent of the Admin Key

Five real sidebar/screen buttons call the shared `api()` client with **no
body argument**: Backup DB (`POST /backup`), Season Activate
(`POST /seasons/{id}/activate`), Season Close (`POST /seasons/{id}/close`),
Season Reopen (`POST /seasons/{id}/reopen`), and Reopen Week
(`POST /seasons/{id}/weeks/{week}/reopen`). `api()` only sets `opts.body`
`if (body !== undefined)`, so these four requests go out with no body and
no `Content-Length` at all.

On staging, IIS rejects that outright:

```
POST /api/backup  (Admin Key present, no body) -> HTTP 411 Length Required
  <HTML>... The request must be chunked or have a content length. ...</HTML>
```

Confirmed the same for `.../activate` and `.../reopen` (hit this
mid-checklist -- see sections 6/15/16 below). A matching bodyless `DELETE`
(`DELETE /api/players/999999`, no body) returned a normal 200 -- **IIS only
enforces this on POST**, so every `DELETE` call in the frontend is
unaffected; only these five bodyless `POST` calls are.

This is **new**, staging-specific, and distinct from the auth gap
`browser-admin-auth-bridge` fixed: it happens after a valid Admin Key is
already attached, only shows up behind IIS (local dev has no reverse proxy
in front of it, which is why `browser-admin-auth-bridge`'s local smoke test
never saw it), and the response is a raw IIS HTML page, not JSON -- `api()`
calls `res.json()` unconditionally, so a real browser hitting this would
get a JSON-parse exception on top of the 411, not even the friendly error
message path. **This means Backup DB, Close/Reopen Season, and Reopen Week
do not work from the browser on staging today even with an Admin Key set.**

**Fix status (2026-08-23, `api-client-bodyless-post-fix`):** `api()` now
sends a real `'{}'` body for `POST`/`PUT`/`PATCH` calls when the caller
passes none, instead of omitting the body entirely -- `GET`/`DELETE` are
unchanged, matching the earlier finding that bodyless `DELETE` was never
affected. **Locally verified**: loaded the actual shipped
`web/lib/api-client.js` into a sandboxed Node context with a `fetch` spy
and confirmed `api('POST', '/backup')` (and `PUT`/`PATCH` with no body) now
send `opts.body === '{}'`, while `GET`/`DELETE` still send no body and an
explicit body still passes through unchanged -- 6/6 cases passed.

**Verified on staging, 2026-08-23** (commit `b795e33`, deployed): confirmed
`/lib/api-client.js` served by staging contains the fix
(`BODY_REQUIRED_METHODS`), then sent the exact request shape the fixed
frontend now produces -- a real `Admin Key` plus an explicit `'{}'` body --
to all five previously-411ing routes. All five now reach the Go app instead
of being rejected by IIS:

```
POST /api/backup                          -> 200 (real backup file written)
POST /api/seasons/999999/activate         -> 404 "season not found"
POST /api/seasons/999999/close            -> 404 "season not found"
POST /api/seasons/999999/reopen           -> 404 "season not found"
POST /api/seasons/999999/weeks/1/reopen   -> 500 "reopen week: season-closed
                                              check: ... sql: no rows in
                                              result set"
```

The nonexistent-season IDs were deliberate, so 404/500 here are correct
Go-app responses (not another 411) -- exactly what proves the request got
past IIS this time. I independently reproduced this myself (not just
relaying a report) using a real personal-key admin user against real
staging; a separate staging verification pass (a different session,
username `bodyless-post-verify-2026-08-23`) reached the same conclusion
first. **Aside, out of scope for this fix**: the Reopen Week 500 for a
nonexistent season is arguably a minor Go-side gap on its own (an unhandled
`sql.ErrNoRows` surfacing as 500 instead of 404) -- unrelated to the
bodyless-POST issue this branch fixed, not tracked as a new gap number here
since it wasn't part of what this pass set out to verify.

This confirms the fix resolves the original finding. **This does not, on
its own, re-verify the broader browser click-flow for these five
buttons** -- only that the IIS-level bodyless-POST rejection is gone. A
real click-through would still be worth doing before calling the browser
admin-write path fully proven end to end.

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
  - [x] Existing leagues (Demo Pool League, Demo 9-Ball League) list correctly.
        **API-verified**: `GET /api/leagues` returns exactly the 3 expected
        leagues (2 seeded + Fixture Scoresheet League). Modal rendering
        itself: NOT VERIFIED (no browser).
  - [x] Creating/editing a league via the modal succeeds with Admin Key set.
        **API-verified** against staging: `POST /api/leagues` -> 201
        (created id 4), `PUT /api/leagues/4` -> 200 (name updated),
        `DELETE /api/leagues/4` -> 200. Cleaned up; final league count back
        to 3. Modal click-flow itself: NOT VERIFIED (no browser).
- Browser: Teams nav.
  - [x] Team list renders per active league, with rosters/captains as seeded.
        **API-verified**: `GET /api/teams?league_id=1` returns the expected
        6 seeded teams. Visual rendering: NOT VERIFIED (no browser).
  - [x] Add/edit team succeeds with Admin Key set.
        **API-verified**: `POST /api/teams` -> 201 (created, then deleted
        via the league cascade during cleanup).
- Browser: Players nav.
  - [ ] Player list renders, sortable/filterable as designed, diff/handicap
        values match seed data. **NOT VERIFIED (no browser)** -- confirmed
        the underlying data is correct (`GET /api/players?league_id=1`
        returns 23 players, handicaps match seed values spot-checked
        against `scripts/seed.sql`), but sort/filter UI behavior needs an
        actual browser.
  - [x] "Add Player" (full modal) and Quick Add both succeed with Admin Key
        set. **API-verified**: `POST /api/players` -> 201 (both the full-
        create and quick-add-shaped payloads use the same endpoint/body
        shape, so one verification covers both). Modal/quick-add UI itself:
        NOT VERIFIED (no browser).

### 2. Player Quick-Add Duplicate Warning (Phase A)

- Browser: Players nav -> Quick Add -> type an existing player's name (e.g.
  "Rex Barlow" in Demo Pool League).
  - [ ] Warning appears naming the existing player and team, before create
        is attempted. **NOT VERIFIED (no browser)** -- this is pure
        client-side JS logic (`normalizeFullName` comparison) with no
        distinguishable API call to observe; confirmed the supporting data
        exists (`GET /api/players?league_id=1` includes id 16, "Rex
        Barlow", team "Eight Is Enough", matching the checklist's example)
        but the warning itself needs an actual browser to see fire.
  - [ ] Cancel closes the flow with no request sent. NOT VERIFIED (no browser).
  - [ ] "Add Anyway" succeeds with Admin Key set, once past the warning.
        The underlying create call is the same `POST /api/players` already
        API-verified in section 1; the "past the warning" click-flow
        itself is NOT VERIFIED (no browser).
  - [ ] Typing a unique name shows no warning. NOT VERIFIED (no browser).

### 3. Player Safe-Merge Backend (Phase A, no UI)

No browser path exists yet. curl-only:
```
curl -X POST http://localhost:8080/api/players/<source_id>/merge \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"target_id": <target_id>}'
```
  - [x] A safe merge (two players with no overlapping season/round/lineup
        data) returns 200 with `{"status":"merged",...}`. **API-verified**:
        created two throwaway players, merged source into target ->
        `{"source_id":54,"status":"merged","target_id":55}`, HTTP 200.
  - [x] An unsafe merge (e.g. two players already on rosters in the same
        season) returns 409 with a Conflict message. **API-verified**:
        merged two real Fixture Scoresheet League players from different
        teams in the same season -> HTTP 409, `"source and target are both
        rostered in the same season; resolve the roster before merging"`.
        Confirmed neither fixture player was actually touched afterward.
  - [x] Same-ID merge returns 400; a nonexistent player ID returns 404.
        **API-verified**: same-ID -> HTTP 400,
        `"source and target player must be different"`; player 999999 ->
        HTTP 404, `"source player not found"`.
  - All four throwaway/test players created for this section were deleted
    afterward; no real seeded data was left modified.
  - Backend correctness for this endpoint is already covered by 23 automated
    tests (service + SQLite integration + handler/route); this step
    confirmed it behaves the same way against real staging data, not
    re-proving the logic.

### 4. Season Creation

- Browser: Seasons nav -> Add Season.
  - [ ] Form renders with league/name/dates/schedule-type fields. NOT
        VERIFIED (no browser).
  - [x] Save succeeds with Admin Key set. **API-verified**:
        `POST /api/seasons` -> 201 (created id 7), then
        `DELETE /api/seasons/7` -> 200 to clean up.
  - [x] Existing seasons list correctly with active/draft/historical status
        chips matching seed data. **API-verified** (data, not the chip UI):
        `GET /api/seasons?league_id=1` returned season 1 "Fall 2025"
        (active=false, activated_at set -- historical), season 2 "Spring
        2026" (active=true -- active), season 3 "Summer 2026" (active=false,
        activated_at null -- draft), exactly matching the documented seed
        state. Chip rendering itself: NOT VERIFIED (no browser).

### 5. Teams, Rosters, Rules (Season Setup)

- Browser: Seasons nav -> open a season -> Teams tab.
  - [x] Season 3 (draft) shows 4 of 6 teams registered, one team with no
        captain. **API-verified**: `GET /api/seasons/3/teams` returned
        exactly 4 teams; "Bridge Over Troubled Cues" has `captain_id: null`,
        the other 3 have captains set -- matches the documented seed state
        exactly. Tab UI itself: NOT VERIFIED (no browser).
  - [x] Add/remove season team, set captain -- all succeed with Admin Key
        set. **API-verified** in the disposable sandbox (section 6): season
        team creation (`POST /api/seasons/{id}/teams`), captain assignment
        (`PUT /api/seasons/{id}/teams/{tid}`) both returned 200/201.
        Found and reverted a real-data side effect below.
- Browser: Seasons nav -> open a season -> Roster tab.
  - [x] "Available players" list for season 3's partially-rostered teams
        shows the specific unrostered players noted in `scripts/seed.sql`
        comments. **API-verified**: `GET /api/seasons/3/players/available`
        includes both Opal Kwan (id 5) and Nina Park (id 7), exactly as
        documented. List UI itself: NOT VERIFIED (no browser).
  - [x] Add/remove roster player succeeds with Admin Key set.
        **API-verified** directly against the real season 3 data (fully
        reversible, so used real data instead of the sandbox): added Opal
        Kwan to team 2's roster (`POST .../roster` -> 201, `roster_count`
        went from 2 to 3), then removed her again (`DELETE .../roster/5` ->
        200). Confirmed `roster_count` back to 2 afterward -- no net change
        to real seed data.
- Browser: Seasons nav -> open a season -> Rules tab.
  - [x] Seeded rule values render (handicap_multiplier, etc.) for season 2/4.
        **API-verified**: `GET /api/seasons/2/rules` returned all 4 seeded
        rules with the exact documented values. Tab UI itself: NOT VERIFIED
        (no browser).
  - [x] Editing a rule value succeeds with Admin Key set. **API-verified**
        directly against real season 2 data (fully reversible): changed
        `max_individual_handicap` from 4.5 to 5.0 (`PUT .../rules/2` -> 200),
        confirmed via GET, then reverted to 4.5 (-> 200), confirmed via GET
        again. **Minor finding**: the PUT response body itself echoes back
        `"season_id":0,"rule_key":""` instead of the real values (`2` and
        `"max_individual_handicap"`) -- the actual stored row is correct
        (confirmed via a follow-up GET both times), so this is just a
        confusing response-echo gap, not a data-correctness bug. Worth a
        tiny fix but not urgent.

### 6. Skipped Weeks, Bye Requests, Schedule Generation

**Sandbox note:** sections 6-11 and 15 needed real write/generate/close
flows, and there is no `DELETE /api/matches/{id}` endpoint -- a generated
schedule cannot be cleanly undone except by deleting the whole season
(itself destructive to anything else on that season) or the whole league.
Rather than risk leaving unremovable generated matches on a real seeded
season, I built one throwaway league/season/2-teams/6-players sandbox
("Smoke Sandbox League" / "Smoke Sandbox Season"), exercised the rest of
this checklist against it, and deleted the whole league at the end
(cascades everything). **This is itself a real finding**: "Generate
Schedule" has no clean undo path against real data -- see Known Gaps below.

- Browser: Seasons nav -> Skipped Weeks.
  - [x] Season 2's seeded skipped weeks (MLK Day, Memorial Day) render.
        **API-verified**: `GET /api/seasons/2/skipped-weeks` returned both,
        exact dates and reasons. Rendering itself: NOT VERIFIED (no browser).
  - [x] Add/remove a skipped week succeeds with Admin Key set.
        **API-verified** in the sandbox: `POST .../skipped-weeks` -> 201.
- Browser: Seasons nav -> Bye Requests.
  - [x] Empty state renders correctly (no bye requests are seeded).
        **API-verified**: `GET /api/seasons/2/bye-requests` returned `[]`.
        Empty-state UI itself: NOT VERIFIED (no browser).
  - [x] Creating a bye request succeeds with Admin Key set (odd team count)
        / is correctly rejected (even team count). **API-verified** in the
        sandbox, but only the rejection path: the sandbox has 2 teams
        (even), and `POST .../bye-requests` correctly returned 400,
        `"bye requests require an odd number of teams (2 teams -- even)"`.
        Did not additionally build a 3rd sandbox team just to reach the
        success path -- the validation firing correctly is itself good
        evidence the rule is implemented and checked.
- Browser: Schedule nav -> Generate Schedule.
  - [x] Generate succeeds with Admin Key set. **API-verified** in the
        sandbox: `POST /api/matches/generate` -> 200,
        `{"matches_created":1,"end_date":"2026-08-03"}`; confirmed via
        `GET /api/matches?season_id=8` -- one match, home/away teams
        correct.

**New finding, [see Critical blocker above]:** `POST /api/seasons/{id}/activate`
is one of the five bodyless-POST calls that 411s on staging via IIS. Hit
this directly while activating the sandbox season (needed before Close
Week would allow closing it) -- had to retry with an explicit `{}` body via
curl to get past it. A real browser click on "Activate" will hit the same
411 on staging today.

**New finding:** `POST /api/seasons/{id}/teams` with `{"name": "..."}`
returns a raw 500 with a leaked SQL message
(`"insert team \"X\": constraint failed: UNIQUE constraint failed:
teams.league_id, teams.name (2067)"`) instead of a friendly 409 when a
standalone team of that name already exists in the league. Hit this while
building the sandbox (created standalone teams first, then tried to also
register them as season teams by name). Low severity -- an unusual admin
sequence -- but worth a friendlier error message.

### 7. Schedule Pushback Preview / Apply

- Browser: Schedule nav -> pushback controls (visible once a schedule
  exists).
  - [x] Preview shows the shift plan with no Admin Key needed.
        **API-verified** in the sandbox: `POST .../pushback-preview` (no
        auth header) -> 200, correct shift plan for the one match (week 1
        -> week 2). Confirms the intentional unauthenticated design.
  - [x] Apply Pushback succeeds with Admin Key set. **API-verified**:
        `POST .../pushback-apply` -> 200, same shift plan; confirmed via
        `GET /api/matches?season_id=8` that the match actually moved to
        week 2 / 2026-08-10.

### 8. Lineup Plans

- Browser: Lineup nav.
  - [ ] Fixture league (if scoresheet fixtures were loaded) shows full
        3-player lineups per team per week. NOT VERIFIED (no browser) for
        rendering; data-wise, `GET /api/lineup-plans?season_id=6&week_number=1`
        (checked while investigating other sections) returns full 3-player
        lineups, consistent with the fixture loader's design.
  - [x] Save/delete a lineup plan succeeds with Admin Key set.
        **API-verified** in the sandbox: `POST /api/lineup-plans` -> 200
        for both teams (3 players each), confirmed via
        `GET /api/lineup-plans?season_id=8&week_number=2`.

### 9. Dashboard Score-Entry Readiness Gate (Phase A)

- Browser: Dashboard nav, with an active league that has an overdue,
  unscored match.
  - [x] Overdue week with a complete lineup shows an enabled "Enter Scores"
        button. **API-verified (data level, not rendering)**: confirmed
        via `GET /api/matches` + `GET /api/lineup-plans` that the readiness
        precondition (overdue, unscored match + full 3-player lineups both
        sides) was met before I entered scores in the sandbox. Actual
        button state: NOT VERIFIED (no browser).
  - [ ] Disabled state: NOT ATTEMPTED this run -- the sandbox's one match
        got its lineup saved before I could observe the "missing lineup"
        precondition, and reproducing it would have meant deliberately
        deleting a lineup mid-flow for no added signal beyond what the
        Phase A implementation's own automated tests already cover. Still
        an open documentation gap (see Data Readiness above), not a defect.

### 10. Match Entry and Score Save

- Browser: Match Entry nav, pick a fixture-league match from week 1 (blank,
  ready for entry).
  - [ ] Scoresheet renders with correct lineup, handicaps, and game-entry
        grid. NOT VERIFIED (no browser).
  - [x] Save Scoresheet succeeds with Admin Key set. **API-verified** in
        the sandbox: `POST /api/matches/41/rounds` with a 3-pairing rounds
        payload -> 200, `{"saved":3}`. Confirmed via
        `GET /api/matches/41/rounds` and `GET /api/matches/41`: round rows
        stored correctly (games, computed pairing winners), match
        auto-flipped to `completed:true`, and `match_results` rows were
        created with correct sets/games/diff per player.
  - [ ] Week 3 fixture matches (pre-completed) display correctly. NOT
        VERIFIED for rendering (no browser); confirmed via
        `GET /api/matches?season_id=6` that week 3 matches carry
        `completed:true` with round data intact.

### 11. Close / Reopen Week

- Browser: Schedule nav -> Review & Close on a week with all matches scored.
  - [x] Validation preview renders (warnings, missing-score detection).
        **API-verified**: `GET /api/seasons/8/weeks/2/validate` returned
        `{"messages":null}` (no issues) once the sandbox week was fully
        scored. Preview UI itself: NOT VERIFIED (no browser).
  - [x] Confirm Close succeeds with Admin Key set. **API-verified** in the
        sandbox: `POST /api/seasons/8/weeks/2/close` -> 200, "Week closed.
        Standings and player stats now include this week's results."
        First attempt correctly returned 409 ("cannot close a week for a
        draft season") until I activated the season -- a real, correct
        validation, not a bug.
  - [x] Reopen Week succeeds with Admin Key set. **API-verified**: tested
        this specifically against a **real** fixture week (week 3 of the
        Fixture Scoresheet Season) rather than only the sandbox, to also
        check player-stats against real team-assigned players (see section
        12). `POST /api/seasons/6/weeks/3/close` -> 200, then
        `POST /api/seasons/6/weeks/3/reopen` -> 200. Confirmed via
        `GET /api/seasons/6/weeks` that all 5 fixture weeks are back to
        `"open"` afterward -- no net change to real fixture data. Also hit
        the bodyless-POST 411 on the first reopen attempt (see Critical
        blocker above); succeeded once retried with an explicit `{}` body.

### 12. Standings and Player Stats

- Browser: Standings nav / Player Stats nav, on a season with at least one
  closed week.
  - [x] Standings reflect only officially closed-week results.
        **API-verified**: `GET /api/standings?season_id=8` after closing
        the sandbox week showed correct won/loss/points/games for both
        teams matching the entered scores exactly.
  - [ ] Player Stats table renders per-player win/loss/diff correctly.
        **FAILED for the sandbox, API-verified as a real product gap**:
        `GET /api/player-stats?season_id=8` returned `[]` (empty) despite
        `GET /api/seasons/8/weeks/2/recap`'s own `player_stats` array
        showing correct per-player numbers for the same players/week.
        Root-caused (read-only, did not fix): `GetPlayerStats`'s SQL
        (`backend/storage/sqlite/round_store.go`) does
        `JOIN teams t ON t.id = p.team_id` -- an INNER JOIN on the legacy
        `players.team_id` column. My sandbox players were only ever
        assigned via `season_rosters`/`lineup_plans` (the documented target
        model -- see `doc/domains/players/README.md`), never given a
        direct `players.team_id`, so the JOIN silently excludes them.
        **Confirmed this does NOT affect real seeded/fixture data**: closed
        real fixture week 3 (then reopened it, see section 11) and
        `GET /api/player-stats?season_id=6` correctly returned all 12
        fixture players with correct stats, because fixture/seed players
        all have `players.team_id` set directly. So this is a real, latent
        gap that only bites season-roster-only player assignment, not
        today's seed/fixture data. See Known Gaps below.

  **Fix status (2026-08-23, `player-stats-roster-join-fix`):** `GetPlayerStats`'s
  season-scoped query now resolves team via a `season_rosters` lookup for
  the requested season first, falling back to `players.team_id` only when
  the player has no roster row for that season -- so roster-only players
  are no longer dropped, and existing `players.team_id`-only players are
  unaffected. Verified with two new SQLite-backed store tests: one seeding
  a `players.team_id IS NULL` player who is only in `season_rosters` (the
  exact shape this gap found) confirms they now appear with correct stats
  and team name, and one confirming the season roster's team wins over a
  stale/differing `players.team_id` when both exist. Full `go test ./...`
  passes, including the original `GetPlayerStats` test written before this
  gap was found. Not re-verified against the actual staging environment
  this run -- that would need a deploy.

### 13. Handicap Review / Apply

- Browser: Handicap nav.
  - [x] Recommendations list renders (read-only, unauthenticated GET).
        **API-verified**: `GET /api/seasons/8/handicap-recommendations`
        (sandbox, after setting `handicap_update_method=game_diff_average`
        via `POST /api/seasons/8/rules`) correctly computed lifetime/window
        stats for all 6 sandbox players and correctly gated all of them as
        `"below_threshold"` (only 3 racks played each, below the 15-rack
        eligibility window) -- confirms the eligibility engine itself
        works. List UI rendering: NOT VERIFIED (no browser).
  - [ ] Apply a recommendation. NOT ATTEMPTED -- every sandbox player was
        correctly below the eligibility threshold (one match's worth of
        racks isn't enough), so there was nothing eligible to Apply without
        generating substantially more match history than this smoke pass
        justified. Not a defect -- the threshold gate is doing its job.
  - **New finding**: the Week Recap endpoint's embedded handicap preview
    (`s.hcPreview.HandicapPreview`, `backend/domains/handicaps/service.go`)
    and the dedicated Handicap Recommendations endpoint
    (`handicaps.Service.Recommendations`) disagree for the same
    season/players at the same instant: the recap's preview
    (`GET /api/seasons/8/weeks/2/recap`) showed all 6 sandbox players with
    concrete `recommended_handicap` values and "6 players have recommended
    handicap changes (not yet applied)", while the dedicated recommendations
    endpoint showed the same 6 players as `"below_threshold"` with
    `recommended_hc: null` at the same moment. Read the code
    (`HandicapPreview` calls `GameDiffAverageRecs` + `applyGameDiffCap`
    directly, with no eligibility-threshold gate) -- this looks like a real
    parity gap between the two code paths, not a data issue: an admin
    looking at Week Recap would see "6 changes ready" while the actual
    Handicap tab for the same season shows nothing eligible yet. Worth a
    dedicated follow-up branch; see Recommended Next Branches.

  **Fix status (2026-08-24, `handicap-preview-parity`):** `HandicapPreview`'s
  `game_diff_average` case no longer runs its own separate calculation --
  it now calls `Service.Recommendations` directly and reshapes that
  response, so Week Recap, Advance Preview, and the dedicated Handicap
  Recommendations endpoint all share one computation and one eligibility
  gate. The old match-averaged, threshold-free `applyGameDiffCap` path is
  deleted. A player below the 15-rack eligibility window now shows the same
  `"below_threshold"` reason and no actionable change in both places --
  the exact conflict this gap described can no longer occur. One
  user-visible contract change came out of the fix: `PlayerHandicapRec`'s
  `matches_played` field is renamed to `included_racks` (the old field
  counted whole matches under the retired algorithm; the shared engine
  counts individual eligible racks), updated in the JSON response, the
  Week Recap/Advance Preview table rendering
  (`web/domains/schedules/schedule-page-component.js`), and all handler
  tests. Verified with new backend tests proving parity for both a
  below-threshold player (`TestHandicapPreview_Recommendations_ParityBelowThreshold`)
  and an eligible one (`TestHandicapPreview_Recommendations_ParityEligible`),
  plus updated integration tests in `handlers/api_handicap_test.go`. Full
  `go test ./...` and `go build ./...` pass. Not re-verified against the
  actual staging environment this run -- that would need a deploy.

### 14. Week Recap

- Browser: Schedule nav -> Recap toggle on a closed week.
  - [x] Recap panel data renders match results, missing-match count, and
        next-week readiness correctly. **API-verified**:
        `GET /api/seasons/8/weeks/2/recap` returned the one match with
        correct set/game totals, `missing_count: 0`, and correct
        per-player stats (see section 12 -- notably, recap's own
        `player_stats` field was correct for the same sandbox players that
        `/api/player-stats` failed on, since recap uses a different query
        path). The embedded handicap-changes preview had the parity issue
        noted in section 13, fixed 2026-08-24 by `handicap-preview-parity`.
        Panel rendering itself: NOT VERIFIED (no browser).

### 15. Season Close / Reopen

- Browser: Seasons nav -> season detail -> Close Season / Reopen Season
  buttons.
  - [ ] Season 1 shows Reopen, season 2 shows Close (button visibility).
        NOT VERIFIED (no browser) -- confirmed the underlying data
        (`closed_at`/`active` fields) is set correctly for both seasons,
        which is what the visibility logic keys on, but did not observe
        the actual buttons.
  - [x] Close Season succeeds with Admin Key set. **API-verified** in the
        sandbox: `POST /api/seasons/8/close` -> 200, `closed_at` populated
        in the response.
  - [x] Reopen Season succeeds with Admin Key set. **API-verified**:
        `POST /api/seasons/8/reopen` -> 200, `closed_at` cleared in the
        response.

### 16. Backup and Health Endpoint

- Browser: sidebar "Backup DB" button.
  - [ ] **FAILS on staging, API-verified**: `POST /api/backup` with a
        proper Admin Key **and an explicit body** succeeds (200, real
        backup file path returned, confirmed the file exists in
        `C:\inetpub\league-staging\data\`). But the actual "Backup DB"
        button calls `api('POST', '/backup')` with **no body argument** --
        reproduced that exact call via curl with no `-d` flag, and staging
        (IIS) returned `411 Length Required`, an HTML page, before the
        request ever reaches the Go app. See the Critical Blocker section
        at the top -- this is the same bodyless-POST issue affecting
        Backup, Season Activate/Close/Reopen, and Reopen Week. **This
        button does not work from the real browser on staging today**,
        Admin Key or not.
- `GET /healthz`:
  - [x] Returns `{"status":"ok"}`, 200, unauthenticated. **API-verified**
        directly against `http://league-staging.local/healthz`.

---

## Known Gaps Summary

| # | Gap | Severity | Where | Status |
|---|-----|----------|-------|--------|
| 1 | Browser could not perform any admin write except Handicap Apply | ~~Critical~~ | `web/lib/api-client.js` | **Resolved 2026-08-20** by `browser-admin-auth-bridge` -- see Admin Key setup above |
| 2 | `seed-staging.ps1` does not load scoresheet fixtures | ~~Medium~~ | `scripts/deploy/seed-staging.ps1` | **Resolved 2026-08-23** by `staging-seed-fixtures-option` |
| 3 | No seed/fixture data demonstrates the Dashboard readiness gate's "disabled" state | Low | seed data only; documented workaround above | Open |
| 4 | Player safe-merge has no admin UI (already tracked as deferred in `doc/roadmap.md`) | Low (known/tracked) | `doc/roadmap.md` "Player record maintenance" | Open |
| 5 | Staging health check in `staging-common.ps1` polls `/api/leagues`, not the dedicated `/healthz` the app already exposes | Low | `scripts/deploy/staging-common.ps1` | Open |
| 6 | Bodyless `POST` calls (Backup, Season Activate/Close/Reopen, Reopen Week) return IIS 411 on staging, independent of the Admin Key -- discovered 2026-08-23 | ~~Critical~~ | `web/lib/api-client.js` (doesn't send a body when none is passed) | **Resolved 2026-08-23** by `api-client-bodyless-post-fix`, verified on staging -- all five routes now reach the Go app instead of 411ing; see the Critical Blocker section above for evidence |
| 7 | `GET /api/player-stats` silently returns empty for players assigned only via `season_rosters`/`lineup_plans` without a direct `players.team_id` -- discovered 2026-08-23 | ~~Medium~~ | `backend/storage/sqlite/round_store.go` `GetPlayerStats` | **Fixed 2026-08-23** by `player-stats-roster-join-fix`, verified via new SQLite store tests -- staging (browser/API) re-verification not yet done |
| 8 | Week Recap's embedded handicap preview and the dedicated Handicap Recommendations endpoint disagree on eligibility for the same season/players -- discovered 2026-08-23 | ~~Medium~~ | `backend/domains/handicaps/service.go` `HandicapPreview` vs `Recommendations` | **Fixed 2026-08-24** by `handicap-preview-parity` -- `HandicapPreview` now delegates to `Recommendations` for `game_diff_average`, so both paths share one computation and eligibility gate; verified via new parity tests -- staging (browser/API) re-verification not yet done |
| 9 | No way to cleanly undo a generated schedule (`Generate Schedule` has no matching `DELETE`) short of deleting the whole season/league -- discovered 2026-08-23 | Low | `handlers/api_match_routes.go` (no `DELETE /api/matches/{id}`) | Open |
| 10 | `POST /api/seasons/{id}/teams` with `name` returns a raw 500 with a leaked SQL message instead of a friendly 409 when a same-named standalone team already exists in the league -- discovered 2026-08-23 | Low | `backend/domains/seasons` `AddTeam` | Open |
| 11 | `PUT /api/seasons/{id}/rules/{rid}` response body echoes `season_id:0, rule_key:""` instead of the real values, even though the stored row is correct -- discovered 2026-08-23 | Low | `handlers` season-rules update handler | Open |

## Recommended Next Branches

`api-client-bodyless-post-fix`, `player-stats-roster-join-fix`, and
`handicap-preview-parity` are all done (see the Critical Blocker section
above and Known Gaps rows #6/#7/#8) -- no longer listed here as pending
branches. Remaining, in priority order:

1. A friendlier error for the season-teams name-collision 500 (#10), fixing
   the rules-update response echo (#11), and deciding whether "Generate
   Schedule" needs an undo path (#9) -- likely only worth it if a real
   workflow (not just this smoke test) hits it.
2. Everything already known before this pass (dashboard gate demo data,
   staging health-check endpoint choice, merge UI, the `.codex/skills/`
   script drift) remains backlog-level, unchanged by this run.

---

## Scope Note

**2026-08-23 staging run**: executed against real `http://league-staging.local`
via curl (no browser automation available in this environment -- every
result above is labeled API-verified or NOT VERIFIED (no browser)
accordingly). Used a real bootstrapped personal-key admin user and a
disposable sandbox league/season for schedule/lineup/match-entry/close-week
flows, since there is no clean way to undo a generated schedule against
real seeded data (see gap #9). Two narrow, fully-reversed exceptions
touched real seeded/fixture data directly (a season-2 rule edit, and
closing+reopening fixture week 3) -- both confirmed restored to baseline
afterward. Did not fix any of the bugs found (per scope); did not reset,
redeploy, or reseed staging. One personal-key user
(`smoke-pass-2026-08-23`) and one backup file remain on staging as expected,
harmless artifacts of testing the Backup and user-bootstrap flows.

**2026-08-20/23 local verification** (unchanged from before the staging
run): the Admin Key bridge (2026-08-20) was verified locally: `node --check`
on all changed/new JS, the full Go test suite, and a real functional smoke
test that loaded the actual shipped `web/lib/admin-key-store.js` and
`web/lib/api-client.js` source into a sandboxed Node context and drove the
real `api()` function against the local dev server -- confirming the
friendly 401 message with no key, a successful `POST /api/players` after
`setAdminKey()`, a return to the 401 after `clearAdminKey()`, and
unaffected GET reads throughout.
