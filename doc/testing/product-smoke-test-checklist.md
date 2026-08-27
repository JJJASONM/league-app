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
  - [ ] **Weekly Score Processing Phase 1C (2026-08-26), NOT VERIFIED (no
        browser):** on a completed, un-approved match, click **Approve**
        -- an "Approved" badge should appear next to Completed/Pending, the
        Save/Clear buttons should disappear, and an inline hint should
        explain that Unapprove is needed to edit again. Click **Process**
        -- badge changes to "Processed", and the hint should now mention
        Unprocess first. Click **Unprocess** then **Unapprove** -- badges
        and Save/Clear should return to normal, and editing scores should
        work again. All four underlying API calls and the resulting field
        values were confirmed via curl against local dev data (see Phase
        1C's roadmap/matches-README entries) -- only the actual button
        rendering, badge appearance, and click behavior in a real browser
        remain unverified.
  - [ ] **Weekly Score Processing Phase 1C correction (2026-08-26), NOT
        VERIFIED (no browser):** on a match whose week is closed (but
        whose season is not), Approve/Process/Unprocess/Unapprove and
        Save/Clear should all be hidden, a "Week Closed" badge should
        appear next to Completed/Pending, and a warning hint should tell
        the admin to reopen the week on the Schedule page first. After
        reopening the week (with the match still approved/processed from
        before), the normal unprocess/unapprove/edit correction path
        should reappear. Confirmed via `GET /api/matches/{id}` that
        `week_closed` is now present and boolean in the response, and via
        the new `TestMatchStore_GetMatch_WeekClosedFalseByDefault`/
        `WeekClosedTrueAfterSet`/`ListMatches_WeekClosedReflectsColumn`
        tests -- only the actual button suppression and hint rendering in
        a real browser remain unverified.

**Weekly Score Processing Phase 1A -- backend/API foundation verified on
staging 2026-08-25.** Ran against real `http://league-staging.local` after
commit `40e8ece` was merged, pushed, and deployed. Used the existing
"Fixture Scoresheet Season" (season 6, league 3), match 33 (week 2,
already scored) for the write-path checks and match 31 (week 1, unscored)
for the rejection check. A disposable bootstrap admin user
(`weekly-score-processing-1a-verify-2026-08-25`) was created via
`POST /api/users` with the static `LEAGUE_ADMIN_TOKEN`, same as prior
staging passes.

All 10 checks from the verification request passed:

  1. **Schema columns present**: `GET /api/matches/33` after approving it
     returned `approved_at`, `approved_by_user_id`, and `approval_note`
     populated with real values -- confirms all five Phase 1A columns
     exist on the deployed database (the omitted `processed_at`/
     `processed_by_user_id` were confirmed the same way one step later).
  2. **All four endpoints work**: `POST /api/matches/33/approve` (200),
     `POST /api/matches/33/process` (200), `POST /api/matches/33/unprocess`
     (200), `POST /api/matches/33/unapprove` (200) -- full cycle exercised
     in order.
  3. **Approve requires a scored match**: `POST /api/matches/31/approve`
     (week 1, `completed:false`) returned 422 `"match has no saved scores;
     enter scores before approving"`.
  4. **Score edits blocked after approval**: `POST /api/matches/33/results`
     while approved returned 409 `"match scores are approved; unapprove
     before editing"`.
  5. **Score edits blocked after processing**: `DELETE
     /api/matches/33/results` while processed returned 409 `"match scores
     are processed; unprocess before editing"` -- a distinct message from
     the approved case, confirming the two states are independently
     detected.
  6. **Unprocess preserves approval**: after `POST
     /api/matches/33/unprocess`, `GET /api/matches/33` showed
     `approved_at` still `2026-08-26T01:13:57Z` while `processed_at` was
     absent (cleared).
  7. **Unapprove clears approval after unprocess**: after `POST
     /api/matches/33/unapprove`, `GET /api/matches/33` showed neither
     field present -- match 33 was byte-for-byte back to the same shape as
     never-touched match 34.
  8. **Processed-but-open match contributes to handicap recommendations**:
     with `handicap_update_method=game_diff_average` set temporarily and
     zero weeks ever closed, `GET /api/seasons/6/handicap-recommendations`
     showed `weeks_closed:1` (from `ClosedWeekCount`'s new compatibility
     condition) and real `included_racks`/`lifetime_hc`/`window_hc` values
     for match 33's six players (`below_threshold`, but real data, not
     `no_data`) -- while match 34's six players, untouched, correctly
     stayed `no_data`. This is a clean per-match proof, not just a
     season-wide one.
  9. **Legacy closed-week match still counts**: closed week 2 for real
     (`POST /api/seasons/6/weeks/2/close`, no approve/process involved at
     all) and confirmed `GET /handicap-recommendations` then showed real
     `included_racks` for match 34's previously-`no_data` players too --
     the `OR week_closed = 1` compatibility clause works on real data, not
     just in unit tests.
  10. **Close Week behavior unchanged**: `GET
      /seasons/6/weeks/2/advance-preview` returned identical `can_close`/
      `validation_messages` before match 33 was touched, after it was
      approved+processed, and after a real week close -- Close Week does
      not check or care about approval/processing state in Phase 1A, exactly
      as scoped. `GET /seasons/6/weeks/1/advance-preview` (unscored week)
      still returned the same `WEEK_MATCH_NO_SCORES` errors as before this
      phase.

Restored to baseline immediately after: reopened week 2, deleted the
temporary `handicap_update_method` rule. Confirmed `GET
/api/seasons/6/rules`, `GET /api/seasons/6/weeks`, `GET /api/matches/33`,
and `GET /api/matches/34` all match the pre-verification baseline exactly,
and `GET /api/standings?season_id=6` shows 0 games played for every team.
The bootstrap verification user was left in place, consistent with every
prior staging pass (no user deletion endpoint exists).

All UI/rendering checks (Match Entry approve/process buttons, Schedule
week-card status badges) remain out of scope for this pass -- Phase 1A is
backend-only by design, and Phase 1C has not shipped yet, so there is
nothing to verify in the browser for this feature.

### 11. Close / Reopen Week

- Browser: Schedule nav -> Review & Close on a week with all matches scored.
  - [ ] **Weekly Score Processing Phase 1C (2026-08-26), NOT VERIFIED (no
        browser):** each match row in a week card should show an Approved/
        Processed badge next to Done/Pending when applicable. Opening
        Review & Close on a week with at least one approved-but-unprocessed
        match should show an info note ("N approved matches will be
        auto-processed..."). After a successful close, the success panel
        should show a new "Auto-processed" row with the count from
        `processed_count`. The underlying data (badge fields, the
        client-side count computation, and the response field) were
        confirmed via curl against local dev data; only rendering and the
        modal note in a real browser remain unverified.
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

  **Weekly Score Processing Phase 1B -- backend/API foundation verified on
  staging 2026-08-26.** Ran against real `http://league-staging.local`
  after commit `c516776` was merged, pushed, and deployed. Used the
  existing "Fixture Scoresheet Season" (season 6, league 3), week 4's two
  matches: match 37 (approved before close) and match 38 (left unapproved
  throughout) -- deliberately choosing one of each in the *same* week so
  the auto-process/skip distinction is a real per-match result, not just a
  season-wide one. A disposable bootstrap admin user
  (`weekly-score-processing-1b-verify-2026-08-26`) was created via
  `POST /api/users` with the static `LEAGUE_ADMIN_TOKEN`, same pattern as
  every prior staging pass.

  All 7 verification goals confirmed:

  1. **Close Week auto-processes the approved match**: approved match 37
     (`POST /api/matches/37/approve` -> 200), then
     `POST /api/seasons/6/weeks/4/close` -> 200. `GET /api/matches/37`
     afterward showed `processed_at` populated with a real timestamp.
  2. **Close Week does not process the unapproved match**: `GET
     /api/matches/38` after the same close showed `approved_at` and
     `processed_at` both absent -- match 38 was never touched.
  3. **`processed_count` matches the real count**: the close response
     included `"processed_count":1` at the top level, correctly counting
     only match 37 (the one approved match), not match 38 or the pair as a
     whole.
  4. **Auto-processed match contributes via `processed_at`, isolated**:
     rather than trust the aggregate recommendations response (which can't
     distinguish the two eligibility paths while the week is still
     closed), reopened week 4 (`POST .../weeks/4/reopen` -> 200, which
     clears `week_closed` but leaves `approved_at`/`processed_at` alone --
     confirmed via `GET /api/matches/37`) and re-fetched
     `GET /handicap-recommendations`: match 37's six players (Emery Frost,
     Finley Moss, Devon Reed, Blair Flint, Avery Slate, Casey Vale) still
     showed real `included_racks`/`lifetime_hc` values even with the week
     open again -- the only thing still making them eligible is
     `processed_at`.
  5. **Unapproved-but-week-closed match counts through the legacy path,
     also isolated**: in that same post-reopen response, match 38's six
     players (Gray Lumen, Indigo North, Harper Quill, Jules Pike, Kai
     Ridge, Lena Stone) all reverted to `"reason":"no_data"` and
     `included_racks:0` -- proving their *only* prior eligibility (while
     the week was closed) came from `week_closed=1`, and vanished the
     instant that flag cleared, since they were never individually
     processed. This before/after-reopen comparison is a cleaner, more
     rigorous proof than the equivalent Phase 1A staging check, since it
     isolates both paths from the same real dataset in one motion.
  6. **Reopen preserves state and the correction path still works**:
     confirmed `approved_at`/`processed_at` survived the reopen unchanged
     (see check 4). Then `POST /api/matches/37/unprocess` -> 200 followed
     by `POST /api/matches/37/unapprove` -> 200; `GET /api/matches/37`
     afterward showed match 37 byte-for-byte identical in shape to
     never-touched match 38 (no approval fields present).
  7. **No regression**: Close Week, Reopen Week, and Handicap
     Recommendations all behaved exactly as documented above, with no
     unexpected errors or state at any step. Week Recap was not
     independently re-hit this pass (week 4 was reopened before a recap
     view made sense to check) -- it shares the identical embedded
     handicap mechanism already exercised via the close response's
     `advance_result.handicap` block and via Phase 1A's dedicated Week
     Recap verification, so this is a scope note, not an open gap.

  Restored to baseline immediately after: deleted the temporary
  `handicap_update_method` rule (season 6 had none before this pass).
  Confirmed `GET /api/seasons/6/rules`, `GET /api/seasons/6/weeks`, `GET
  /api/matches/37`, and `GET /api/matches/38` all match the
  pre-verification baseline exactly (week 4 back to `open`, both matches
  with no approval fields), and `GET /api/standings?season_id=6` shows 0
  games played for every team. The bootstrap verification user was left in
  place, consistent with every prior staging pass.

  All UI/rendering checks remain out of scope -- Phase 1B is backend-only
  by design, and Phase 1C (frontend buttons/badges) has not shipped.

  **Weekly Score Processing Phase 1C -- API/data-level verification on
  staging 2026-08-26.** Ran on branch
  `staging-weekly-score-processing-phase-1c-verification` against real
  `http://league-staging.local` after commit `7fbd57a` was merged, pushed,
  and deployed. As with every prior pass, this environment has no browser
  automation available, so this is the same "closest practical substitute"
  used for Phase 1A/1B/1C-local: exercising the exact API routes and
  response fields the new UI code reads, not clicking through the actual
  rendered page. **Actual button rendering, badge appearance, and click
  behavior in a real browser remain NOT VERIFIED (no browser)** -- see the
  Deferred note at the end of this entry. Used the existing "Fixture
  Scoresheet Season" (season 6, league 3): match 33 (week 2, already
  scored, open week) for goals 1-4, and week 4's matches 37/38 for goals 5
  and 6. A disposable bootstrap admin user
  (`weekly-score-processing-1c-verify-2026-08-26`, id 7) was created via
  `POST /api/users` with the static `LEAGUE_ADMIN_TOKEN`, same pattern as
  every prior staging pass.

  All 7 verification goals from the PM's request confirmed at the API/data
  level:

  1. **Approve appears valid on a completed, unapproved, open-week
     match**: `GET /api/matches/33` before any action showed
     `completed:true`, `week_closed:false`, no `approved_at` -- exactly
     the state `match-entry-page-component.js`'s `approveBtn` condition
     (`!locked && m.completed && !isApproved`) requires to render the
     button.
  2. **After Approve**: `POST /api/matches/33/approve` -> 200
     `{"status":"approved"}`. `GET /api/matches/33` then showed
     `approved_at:"2026-08-26T16:44:56Z"`, `approved_by_user_id:7` --
     drives `isApproved=true`, which renders the "Approved" badge, hides
     Save/Clear (`canEditScores` becomes `false`), and selects the
     "Scores are approved and locked. Unapprove to edit scores again."
     hint. Confirmed the guard itself, not just the flag: `POST
     /api/matches/33/rounds` while approved returned 409 `"match scores
     are approved; unapprove before editing"` -- the exact condition the
     UI hint describes.
  3. **After Process**: `POST /api/matches/33/process` -> 200
     `{"status":"processed"}`. `GET /api/matches/33` then showed
     `processed_at` set alongside `approved_at` still present -- drives
     `isProcessed=true`, renders the "Processed" badge, shows the
     Unprocess button, and selects the "Unprocess, then unapprove, to
     edit scores again." hint. Confirmed the guard: the same `rounds`
     POST while processed returned a distinct 409 `"match scores are
     processed; unprocess before editing"`, proving the two states are
     independently detected exactly as the two different hint strings
     claim.
  4. **Correction path**: `POST /api/matches/33/unprocess` -> 200, then
     `POST /api/matches/33/unapprove` -> 200. `GET /api/matches/33`
     afterward showed neither `approved_at` nor `processed_at` present --
     back to the exact shape that renders Save/Clear and hides all four
     action buttons. Re-saved the original six rounds via `POST
     /api/matches/33/rounds` -> 200 `{"saved":6}`, confirming score
     editing genuinely works again post-correction, not just that the
     fields cleared. `GET /api/matches/33/rounds` afterward matched the
     pre-test scores exactly (same players, same game scores; only the
     round-result row IDs differ, which is inherent to how `SaveRounds`
     replaces rows).
  5. **Closed-week suppression**: closed week 4 (`POST
     /api/seasons/6/weeks/4/close` -> 200) to produce a real
     `week_closed:true` match (37) with the season still open. `GET
     /api/matches/37` confirmed `week_closed:true`. All four guarded
     actions returned the same 409 `"week is closed; reopen before
     editing scores"` -- `approve`, `process`, `unprocess`, and
     `unapprove` -- as did a `rounds` save attempt. This is exactly the
     condition `locked = seasonClosed || weekClosed` was added to catch,
     and the message matches the new `weekClosedHint` text ("Reopen the
     week on the Schedule page first..."). Reopened the week (`POST
     .../weeks/4/reopen` -> 200) and confirmed `week_closed` cleared back
     to `false` on both matches -- the state that makes the normal
     unprocess/unapprove/edit path reappear.
  6. **Schedule page data**: before closing week 4 a second time,
     approved match 37 only (`POST /api/matches/37/approve` -> 200) and
     confirmed `GET /api/matches?season_id=6` showed match 37 with
     `approved_at` set and `processed_at` absent while match 38 showed
     neither -- exactly the one-approved-one-not shape
     `#reviewCloseWeek`'s client-side count would render as "1 approved
     match will be auto-processed." Closed the week: response included
     `"processed_count":1`, and `GET /api/matches/37` afterward showed
     `processed_at` populated while match 38 remained fully untouched --
     the data source for the post-close "Auto-processed: 1" row and the
     per-row Approved/Processed badges Schedule renders.
  7. **No regression**: Close Week and Reopen Week were each exercised
     twice this pass (once with no approved matches, once with one) and
     both behaved identically to the Phase 1A/1B staging passes --
     correct `processed_count` (`0` then `1`), correct
     `advance_result`/`closed_week` shape, no unexpected errors. Admin Key
     bearer auth worked for every one of the 15+ mutating calls in this
     pass with no auth failures. The plain score re-save in check 4
     confirms normal Match Entry save/clear behavior is unaffected once a
     match is unlocked.

  Restored to baseline immediately after each step, not just at the end:
  unprocessed+unapproved match 33 (check 4) before closing week 4 at all;
  reopened week 4 immediately after the closed-week guard checks (check
  5) before re-approving anything; reopened week 4 again and
  unprocessed+unapproved match 37 after the `processed_count`-driven close
  (check 6). Final state confirmed via `GET /api/matches?season_id=6`
  (all 10 fixture matches show `week_closed:false` and no `approved_at`/
  `processed_at`, identical to the pre-verification baseline captured at
  the start of this pass) and `GET /api/seasons/6/weeks` (all 5 weeks
  `"open"`, `closed_count:0`, matching the pre-verification baseline
  exactly). `GET /api/standings?season_id=6` shows 0 games played for
  every team, confirming no week was left closed. The bootstrap
  verification user (id 7) was left in place, consistent with every prior
  staging pass (no user deletion endpoint exists).

  **Result: Phase 1C is API/data-verified on staging.** Everything the UI
  code reads (`week_closed`, `approved_at`, `processed_at`,
  `processed_count`) and every guard it depends on (the four 409
  messages, the closed-week rejection) are confirmed correct against real
  staging data and the deployed commit. This developer's Claude Code
  tool session has no browser automation tool available, so this pass
  could not itself click through the rendered page -- that is a
  limitation of this developer's own tool session, not a claim that
  browser automation is unavailable to the project or to PM's
  environment. A PM-side browser pass against staging is the outstanding
  step before the UI itself (as opposed to the data/guards behind it) is
  treated as fully verified -- the checklist items above (section 10 and
  this section) remain unchecked for that reason.

  **Correction (2026-08-26): Review & Close modal bug found during this
  pass and fixed.** PM's browser-side review of the Review & Close modal
  found that the "N approved matches will be auto-processed when this
  week closes" note disappeared whenever the week had no validation
  errors or warnings. Root cause in
  `web/domains/schedules/schedule-page-component.js`'s `#reviewCloseWeek`:
  the no-errors/no-warnings branch did `body = '<p>...All checks
  passed...</p>'` (assignment), overwriting the `body` string instead of
  appending to it -- silently discarding the auto-process note (and the
  prior-acknowledgments note, when present) that had already been added
  earlier in the same function. Fix: changed `body =` to `body +=` on
  that one line, so the "All checks passed" paragraph is appended after
  the existing notes rather than replacing them. Verified two ways:
  (1) a standalone Node harness reproducing the exact body-building logic
  line-for-line showed the pre-fix version dropping the auto-process
  note and the post-fix version keeping both; (2) re-ran the real
  staging scenario -- approved match 37 in week 4, confirmed `GET
  /api/seasons/6/weeks/4/validate` returned `{"messages":null}` (the
  exact no-errors/no-warnings condition that triggers the bug), then
  closed the week and got `processed_count:1` with match 37 processed
  and match 38 untouched, same as before. `node --check` on the changed
  file passes. Restored to baseline the same way as every other check in
  this pass (reopened week 4, unprocessed and unapproved match 37).
  Actual on-screen confirmation that the modal now shows both lines of
  text together is PM's browser pass to make, per the tool limitation
  noted above.

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

  **Staging verification: closed and verified 2026-08-24.** Ran against
  real `http://league-staging.local` after `f4492a3` (the merged fix) was
  deployed. Used the existing "Fixture Scoresheet Season" (season 6, league
  3) rather than a new disposable sandbox, since it already has real
  round-by-round score data across 5 weeks. A disposable bootstrap admin
  user (`handicap-preview-parity-verify-2026-08-24`) was created via
  `POST /api/users` with the static `LEAGUE_ADMIN_TOKEN` to get write access,
  per the existing Admin Key setup instructions above.

  Baseline captured before touching anything: season 6 had 4 season rules
  (no `handicap_update_method` row -> defaults to `manual_review`) and all 5
  weeks `open` (weeks 2-5 had `completed_count: 2`, `closed_count: 0`).

  Steps: set `handicap_update_method=game_diff_average`
  (`POST /api/seasons/6/rules`); closed week 2
  (`POST /api/seasons/6/weeks/2/close`, 200, no warnings). With only that one
  closed week's data (3-4 eligible racks per player, below the default
  15-rack window), `GET /api/seasons/6/handicap-recommendations` and the
  `handicap` block embedded in the week-2 close response **both** showed all
  12 players as `"below_threshold"`, `skipped: true`, with matching
  `included_racks` counts per player -- confirms check 2 (below-threshold
  consistency) and check 4 (field is `included_racks`, not `matches_played`,
  confirmed directly in a live API response).

  To also exercise an eligible/actionable case without generating four more
  weeks of history, temporarily lowered
  `handicap_min_games_for_recommendation` to `3` (still `POST
  /api/seasons/6/rules`) -- every player already had 3-4 included racks, so
  this crossed the (now lower) threshold for all 12 without changing any
  underlying match data. Then pulled the same season's recommendations from
  three independent surfaces and diffed them player-by-player:

  - `GET /api/seasons/6/handicap-recommendations` (dedicated endpoint)
  - `GET /api/seasons/6/weeks/1/advance-preview` (week 1 was still open --
    this is the live embedded preview, the Advance Preview / pre-close
    path)
  - `GET /api/seasons/6/weeks/2/recap` (Week Recap panel for the
    already-closed week)

  All three returned **byte-identical** `recommended_handicap`/`recommended_hc`,
  `reason`, and `included_racks` values for all 12 players -- including
  players whose recommendation exceeded `max_individual_handicap` and were
  capped (`reason: "capped"`), and players landing on an exact,
  uncapped value (e.g. Finley Moss: `current_handicap: 2`,
  `recommended_handicap: 4.14` in all three responses). This directly
  confirms checks 1 and 3 (Close Week / Advance Preview matches Handicap
  Review; eligible players show matching recommended values) with real
  staging data, not just backend unit tests. No regression observed in
  close-week preview, recap, or handicap review during the pass (check 5).

  Restored to baseline immediately after: reopened week 2
  (`POST /api/seasons/6/weeks/2/reopen`), deleted both temporary rules
  (`DELETE /api/seasons/6/rules/24` and `/25`). Confirmed
  `GET /api/seasons/6/rules` and `GET /api/seasons/6/weeks` afterward match
  the captured baseline exactly (4 rules, no `handicap_update_method`; all 5
  weeks `open`, week 2 back to `closed_count: 0`), and
  `GET /api/standings?season_id=6` shows 0 games played for every team,
  confirming the reopen fully unwound the close. The two week-3/week-4
  close attempts made mid-pass to gather more history were blocked by this
  environment's write-action classifier before they executed (confirmed via
  `GET /api/seasons/6/weeks` showing week 2 as the only closed week at that
  point) -- worked around by lowering the threshold rule instead, so no
  extra weeks were ever actually closed and no extra cleanup was needed.
  The bootstrap verification user was left in place, consistent with how
  prior smoke-pass bootstrap users (`smoke-pass-2026-08-23`,
  `bodyless-post-verify-2026-08-23`) were handled -- there is no user
  deletion endpoint, and this matches established precedent.

  All UI/rendering checks (the Handicap tab, the Week Recap panel, the
  Advance Preview modal) remain NOT VERIFIED (no browser available in this
  environment) -- this pass confirms the API/data layer only.

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

### 17. Users Admin Screen (Phase 1, 2026-08-26)

- Browser: sidebar "Admin Key" button/modal, and the new "Users" nav entry.
  - [x] Pasting a valid personal API key into the Admin Key modal
        resolves and shows "Signed in as `<username>` (`<role>`)";
        pasting an invalid/expired key shows "did not resolve."
        **Browser-verified by PM on staging (2026-08-26)** -- see the
        staging verification note below for full evidence. Also
        **API-verified**: `GET /api/users/me` returns the resolved user
        for a valid personal key, 403 for the static admin token (which
        has no user identity to resolve), and 401 with no
        `Authorization` header at all.
  - [x] The "Users" nav entry is hidden unless the resolved identity is
        `system_admin` or the legacy `admin` alias, and visible for
        those roles. **Browser-verified by PM on staging (2026-08-26)**:
        nav stayed hidden with an unrecognized key, became visible after
        a valid `system_admin` key resolved. Also **API-verified
        indirectly**: `GET /api/users` and `POST /api/users` correctly
        return 200/201 for `system_admin`/`admin` personal keys and 403
        for `league_admin` -- the same role check the nav-visibility
        logic reads from the same `/me` response.
  - [x] The Users screen lists existing users (username, role, active,
        created) and lets a `system_admin` create a new user with a
        role choice restricted to System Admin / League Admin, showing
        the one-time API key after creation. **Browser-verified by PM
        on staging (2026-08-26)** -- see the staging verification note
        below for full evidence. Also **API-verified** via a full
        local-server curl walkthrough: bootstrapped a `system_admin`
        via the static `LEAGUE_ADMIN_TOKEN`, then used that user's own
        personal key (not the static token) to create a second
        (`league_admin`) user -- `POST /api/users` returned 201 with a
        64-char one-time key; `GET /api/users` (same personal key)
        listed both users; `POST /api/users` with `role:"admin"`
        returned 400 (not a creatable role); the `league_admin` user's
        own key returned 403 from both `POST` and `GET /api/users` but
        200 from `GET /api/users/me` (it can still read its own
        identity, just not manage users).
  - [x] Paste an invalid or unrecognized key into the Admin Key modal
        and click Save. Expected: the modal stays open (does not
        close), the status line switches to "did not resolve" text, and
        the Users nav entry remains hidden. **Browser-verified by PM on
        staging (2026-08-26)** -- see the staging verification note
        below for full evidence. This replaces the earlier behavior
        where an invalid key still closed the modal with a green "Admin
        key set" toast, which hid the failure until the next admin
        action 401/403'd. Confirmed at the code level: `saveAdminKey()`
        now only hides the modal and shows the success toast when
        `resolveCurrentIdentity()` returns a non-null identity;
        otherwise it shows a danger toast and leaves the modal open,
        with the "did not resolve" status text already set by
        `updateIdentityUI()`.
- New focused Go tests (all passing): role validation on create (missing
  role -> 400, legacy `admin` role -> 400), `system_admin` personal key
  authorizing create/list (previously only the static token worked --
  this was the concrete backend gap this phase fixed), `league_admin`
  personal key rejected from create/list (403), `GET /api/users/me`
  covering no-token (401), static-token (403 -- it has no user identity),
  and valid-personal-key (200, correct username/role) cases, and
  (PM correction) `system_admin` personal-key access to `GET`/`POST
  /api/users` when `AdminToken` is completely unconfigured, plus a test
  proving an empty bearer token cannot accidentally match an
  unconfigured (empty-string) `AdminToken`.

**Users Admin Screen Phase 1 -- browser-verified by PM on staging
2026-08-26.** Staging was deployed from commit `d28cdff`. PM performed
this pass directly in a browser against real staging, which the
developer's tool session cannot do -- see the notes on the four checklist
items above for what specifically was confirmed. Summary of the pass:

  - A disposable `system_admin` user was created through `POST
    /api/users` using the static bootstrap token.
  - `GET /api/users/me` resolved that user's personal key as
    `system_admin`.
  - Invalid-key flow: pasting an unrecognized key into the Admin Key
    modal kept the modal open, showed the "did not resolve" status
    text, and left the Users nav hidden.
  - Valid `system_admin` key flow: the modal closed, the Admin Key
    button showed its "set" state, and the Users nav became visible.
  - Users screen: existing users listed, including the disposable
    `system_admin` created above; created a new `league_admin` user
    through the screen, which appeared in the list with role displayed
    as `league_admin`; the one-time API key alert appeared after
    creation.
  - No secrets or actual API keys are recorded here or elsewhere in this
    checklist entry.

Disposable users created on staging during this pass (the `system_admin`
bootstrap user and the `league_admin` user created through the browser)
remain in place afterward -- there is no delete-user endpoint, consistent
with every prior staging pass's bootstrap-user handling.

### 18. Player Overview Screen (Phase 1, 2026-08-27)

- Browser: "Player Overview" nav entry, and the new "View Overview"
  button on each Players list row.
  - [ ] **NOT VERIFIED (no browser)**: selecting a player in the
        Player Overview screen's dropdown should show their team/season
        header, schedule table, season stats, current handicap, and a
        "Dues and payouts are not tracked yet" money placeholder.
        **API-verified**: `GET /api/players/{id}/overview?season_id=`
        returns the full shape for a real seeded player against a local
        server build, and five focused Go tests cover explicit
        season_id, omitted season_id (defaults to the active season),
        a missing player (404), a not-rostered player falling back to
        their direct team, and a player with no resolvable team at all
        (team: null, empty schedule, zeroed stats) -- every case
        confirms `money.tracked=false`.
  - [ ] **NOT VERIFIED (no browser)**: clicking "View Overview" on a
        Players list row should navigate to the Player Overview screen
        with that player pre-selected in the dropdown. **Confirmed at
        the code level**: the button dispatches a
        `player-overview-nav-request` custom event that the shell
        handles via a new `openPlayerOverview(playerId)` bridge
        function, mirroring the existing `openMatchEntry`/
        `openHandicapForWeek` deep-link pattern (same `appContext`
        preselect/consume mechanism, new
        `overviewPreSelectPlayerId` field).
  - [ ] **NOT VERIFIED (no browser)**: a player with no resolvable team
        (no season roster entry and no direct team_id) should show "No
        team" in the header, an empty schedule table with a friendly
        "No scheduled matches this season" row, and zeroed stats --
        not an error page. **API-verified** via the dedicated Go test
        for this exact case.
- New focused Go tests (all passing, `handlers/api_player_overview_test.go`):
  explicit `season_id`, omitted `season_id` -> active season, missing
  player -> 404, not-rostered-but-has-direct-team -> fallback works,
  no-team-at-all -> `team: null` and empty schedule/stats, and every
  case asserting `money.tracked=false` with a non-empty explanatory
  message.
- Known, deliberately out-of-scope note: `overview.stats.win_pct` will
  always read `0` here too, since it inherits the pre-existing
  `GetPlayerStats` `WinPct`-never-computed gap (see Known Gaps Summary
  below) -- not a regression introduced by this phase, just inherited
  from the underlying query this endpoint reuses.

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
| 8 | Week Recap's embedded handicap preview and the dedicated Handicap Recommendations endpoint disagree on eligibility for the same season/players -- discovered 2026-08-23 | ~~Medium~~ | `backend/domains/handicaps/service.go` `HandicapPreview` vs `Recommendations` | **Closed/Verified 2026-08-24** by `handicap-preview-parity` -- `HandicapPreview` now delegates to `Recommendations` for `game_diff_average`; verified via new parity tests and directly on staging (season 6, real fixture data) -- see section 13's staging verification note for full evidence (below-threshold and eligible/capped cases both confirmed byte-identical across the recommendations endpoint, advance-preview, and week recap) |
| 9 | No way to cleanly undo a generated schedule (`Generate Schedule` has no matching `DELETE`) short of deleting the whole season/league -- discovered 2026-08-23 | Low | `handlers/api_match_routes.go` (no `DELETE /api/matches/{id}`) | Open |
| 10 | `POST /api/seasons/{id}/teams` with `name` returns a raw 500 with a leaked SQL message instead of a friendly 409 when a same-named standalone team already exists in the league -- discovered 2026-08-23 | Low | `backend/domains/seasons` `AddTeam` | Open |
| 11 | `PUT /api/seasons/{id}/rules/{rid}` response body echoes `season_id:0, rule_key:""` instead of the real values, even though the stored row is correct -- discovered 2026-08-23 | Low | `handlers` season-rules update handler | Open |
| 12 | `GetPlayerStats`'s season-scoped query never scans/computes `WinPct` -- every `GET /api/player-stats` and `GET /api/players/{id}/overview` response has `win_pct:0` regardless of real results; Standings' Win% column renders this directly -- discovered 2026-08-27 during Player Overview Phase 1 discovery | Medium | `backend/storage/sqlite/round_store.go` `GetPlayerStats` | Open -- explicitly deferred as a separate follow-up, not bundled into Player Overview Phase 1 |
| 13 | The league-scoped variant of `GetPlayerStats` still drops season-roster-only players (`INNER JOIN teams t ON t.id = p.team_id` on the legacy column) -- only the season-scoped branch was fixed by `player-stats-roster-join-fix` (row #7) -- discovered 2026-08-27 during Player Overview Phase 1 discovery | Medium | `backend/storage/sqlite/round_store.go` `GetPlayerStats` (league-scoped branch) | Open -- explicitly deferred as a separate follow-up, not bundled into Player Overview Phase 1 |

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
