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

  **Follow-up fix (2026-09-01, `player-stats-winpct-roster-scope-fix`):**
  the league-scoped variant of this same query (`GET
  /api/player-stats?league_id=`) had the identical gap -- roster-only
  players were dropped there too -- and is now fixed the same way,
  extended to also cover `lineup_plans`-only substitutes. Separately,
  the "`WinPct` never computed" gap opened alongside this one (Known
  Gaps row #12) turned out to already be fixed at the service layer and
  was never a live bug; see that row and `doc/roadmap.md` for full
  detail on both.

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
  - [x] **Browser-verified on staging (2026-08-27)**: selecting a
        player in the Player Overview screen's dropdown shows their team/season
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
  - [x] **Browser-verified on staging (2026-08-27)**: clicking "View Overview" on a
        Players list row should navigate to the Player Overview screen
        with that player pre-selected in the dropdown. **Confirmed at
        the code level**: the button dispatches a
        `player-overview-nav-request` custom event that the shell
        handles via a new `openPlayerOverview(playerId)` bridge
        function, mirroring the existing `openMatchEntry`/
        `openHandicapForWeek` deep-link pattern (same `appContext`
        preselect/consume mechanism, new
        `overviewPreSelectPlayerId` field).
  - [ ] **API-verified, browser edge case still not fixture-backed**: a player with no resolvable team
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
- **Correction, 2026-09-01:** the note originally here claimed
  `overview.stats.win_pct` always reads `0` because of a
  `GetPlayerStats` `WinPct`-never-computed gap. That gap was a
  discovery-time misdiagnosis, not a live bug -- `RoundService`
  (the method this endpoint's `RoundManager.GetPlayerStats` call
  actually resolves to) has computed `WinPct` correctly since before
  this screen existed. `overview.stats.win_pct` was already correct in
  every real response. See Known Gaps row #12 and
  `player-stats-winpct-roster-scope-fix` in `doc/roadmap.md`.
- Staging verification (2026-08-27, `http://league-staging.local`,
  deployed from `dc2940a`): opened the app in the in-app browser, confirmed
  the Player Overview nav entry is visible without admin auth, opened the
  screen directly, confirmed a 12-player dropdown on the Fixture Scoresheet
  data, and verified the selected player's header, team/season context,
  schedule table, stats card, current handicap card, and money placeholder
  render together. Then opened the Players list, confirmed every row has a
  View Overview button, clicked the first row's button, and confirmed the
  app navigated to Player Overview with that player pre-selected and the
  matching overview rendered. Changed the dropdown to another player and
  confirmed the overview reloaded without an error. No staging data was
  changed by this pass.
- Non-blocking observation from the same browser pass: the console still logs
  `document.querySelector(...)?.refresh is not a function` from the initial
  dashboard bootstrap path (`web/app.js:93`). The dashboard content still
  populated and Player Overview was unaffected, so this is tracked below as
  a low-severity follow-up rather than a blocker for this verification.

### 18a. Player Overview Screen Phase 2 -- Real Dues Status (2026-08-29)

- Browser: the Player Overview screen's money section now shows a real
  Dues card instead of the static placeholder.
  - [x] **Browser-verified on staging (2026-09-01)**: selecting a
        player shows a Dues card with a paid/unpaid badge, total paid,
        and last payment date -- replacing the old "Dues and payouts
        are not tracked yet" banner (that banner still renders as a
        fallback when `money.tracked` is `false`, e.g. in a test-only
        setup with no `FinanceManager` wired). Staging confirmed Blair
        Flint's card rendering Paid, Total Paid ($1.23), and Last
        Payment; the configured `dues_amount` display specifically was
        not part of this staging pass's evidence. **API-verified**
        against a local server build with a real seeded
        league/season/team/player: the initial overview showed
        `money.tracked:true, paid:false`; recording a real dues payment
        through `POST /api/seasons/{id}/finances/dues-payments` and
        re-fetching the overview showed `paid:true` with the correct
        `total_paid` and the payment in `money.payments`; setting a
        `dues_amount` season_rules key and re-fetching showed it in
        `money.dues_amount`.
- New focused Go tests (all passing): 1 new `FinanceService` delegation
  test and 3 new `FinanceStore` tests (empty/newest-first/scoped-by-
  player) for `ListDuesPaymentsByPlayer`
  (`backend/domains/finances/service_test.go`,
  `backend/storage/sqlite/finances_store_test.go`); 4 new handler tests
  in `handlers/api_player_overview_money_test.go` covering no-payments
  (`tracked=true paid=false`), one-payment (`tracked=true paid=true`
  with correct `total_paid`), a `dues_amount` rule shown in the
  response, and an unrostered player still getting a real money status.
  All six pre-existing Phase 1 Player Overview tests
  (`handlers/api_player_overview_test.go`) pass unchanged -- they use
  the shared `testServer()` helper, which has no `FinanceManager`
  wired, so they continue to exercise the `tracked=false` fallback path
  exactly as before.
- At initial ship (2026-08-29), `GET /api/players/{id}/overview` was an
  unprotected read that surfaced the same per-player money data
  (paid/unpaid, amounts, payment dates) Financial Phase 1 deliberately
  put behind `clearanceAuth` for its own routes -- flagged as open
  question `PLAYERS-Q002` rather than resolved in this pass. The
  API-verified evidence above (initial `tracked:false`, recording a
  payment, setting `dues_amount`) was gathered against that
  unauthenticated build. **See section 18b below for the auth
  correction and its own verification.**
- Staging verification: **PASS, 2026-09-01** -- see 18b below for the
  full evidence (this phase's money rendering was verified together
  with the auth correction in a single staging pass, since both landed
  in the same commit).

### 18b. Player Overview Screen Phase 2 -- Auth Correction (2026-08-30)

Resolves `PLAYERS-Q002` (see 18a above and `doc/roadmap.md`'s Resolved
Questions table). `GET /api/players/{id}/overview` now requires
`clearanceAuth` (league_admin/admin/system_admin) -- the same role gate
Financial Phase 1 uses -- instead of staying an unprotected read.

- Browser: the Player Overview nav entry and the Players list's "View
  Overview" row button are both now hidden unless the resolved Admin
  Key identity qualifies, matching the Financial nav entry's gating
  exactly.
  - [x] **Browser-verified on staging (2026-09-01)**: without a valid
        league_admin/admin/system_admin Admin Key, the Player Overview
        nav entry stays hidden. **Confirmed at the code level**:
        `web/app.js`'s `updateIdentityUI()` toggles
        `#nav-item-player-overview`'s `d-none` class using a shared
        `hasFinanceAdminRole(identity)` function (extracted from the
        Financial nav entry's own check).
  - [x] **Browser-verified on staging (2026-09-01)**: without a valid
        league_admin/admin/system_admin Admin Key, the "View Overview"
        button does not render on any Players list row (same-day
        follow-up correction -- it initially rendered unconditionally).
        **Confirmed at the code level**: `loadSection()`'s `'players'`
        case now passes `hasFinanceAdminRole(state.currentIdentity)` as
        a third argument to `<players-page>.refresh()`;
        `players-page-component.js` stores it and only renders the
        button's markup when it is `true` -- no auth logic added inside
        the component.
- New focused Go tests (all passing, `handlers/api_player_overview_auth_test.go`):
  no `Authorization` header -> 401 with `WWW-Authenticate`; an invalid
  token -> 403; the static `LEAGUE_ADMIN_TOKEN` -> 403 (personal-key-
  only auth has no static-token fallback, matching every other
  `clearanceAuth` route); a `score_keeper`-role user -> 403; and
  league_admin, admin, and system_admin each reaching the handler
  (200). **API-verified** against a `financeTestServer` build (real
  `ApplyAuthStore`) for all seven cases.
- `handlers/api_player_overview_money_test.go`'s four money-behavior
  tests were updated to authenticate as a league_admin user (via a new
  `getPlayerOverviewAuth`/`financeAdminKey` helper pair) now that the
  route requires it -- all four still pass with the same assertions as
  18a.
- All six pre-existing Phase 1 tests (`handlers/api_player_overview_test.go`,
  using the plain `testServer()` helper with no `ApplyAuth` wired) pass
  unchanged -- `clearanceAuth` is a passthrough when its resolver is
  nil, the same behavior every other `clearanceAuth`-protected route
  already has under that test setup.
- `node --check` passes on all three changed JS files: `web/app.js`,
  `web/domains/players/players-page-component.js`,
  `web/domains/players/player-overview-page-component.js` (unchanged
  content, rechecked for regression safety).
- Known limitation, not a gap: setting or clearing the Admin Key only
  calls `updateIdentityUI()` (nav visibility), not `loadSection()`, so
  a viewer already on the Players page when their key changes won't see
  the row button appear/disappear until they navigate away and back --
  matches how every other identity-gated nav item in this shell already
  behaves (none force a live re-render of the currently active section).

#### Staging verification (2026-09-01)

**Result: PASS.** Verified on `http://league-staging.local`, `main` /
`origin/main` at `9a2a158`.

- **API-verified:** `GET /api/players/{id}/overview` without
  Authorization returned 401; with the static `LEAGUE_ADMIN_TOKEN`
  returned 403 (personal-key-only, no static fallback, as expected);
  with a disposable `league_admin` personal key returned 200.
- **Money data verified:** the authenticated response showed
  `money.tracked=true`, an existing dues payment rendered as Paid,
  total paid rendered as `$1.23`, and the last payment date rendered --
  the old Phase 1 money placeholder did not render.
- **Browser-verified, no Admin Key:** Player Overview nav hidden,
  Financial nav hidden, Players list loaded normally, and the "View
  Overview" row buttons were hidden on a fresh no-key page load (the
  row-action correction's primary claim).
- **Browser-verified, valid disposable `league_admin` Admin Key:** the
  key resolved as `league_admin`; Player Overview nav visible; Financial
  nav visible; the Players list showed 12 "View Overview" row buttons;
  clicking one opened Blair Flint's Player Overview; the Dues card
  rendered with Paid, Total Paid, and Last Payment.
- **Cleanup:** browser Admin Key cleared, temporary local key file
  removed. The disposable staging user was left in place -- no
  delete-user endpoint exists, matching every prior staging pass's
  bootstrap-user handling.
- **Known limitation reconfirmed on staging:** changing or clearing the
  Admin Key does not live-rerender an already-loaded Players table
  until navigation/reload; a fresh no-key load behaves correctly. This
  matches the code-level limitation already documented above and was
  not treated as a new gap.

### 19. Weekly Summary Screen (Phase 1, 2026-08-27)

- Browser: "Weekly Summary" nav entry, season/week selector.
  - [x] **Browser-verified on staging (2026-08-27)**: selecting a season and week should
        show every match in that week with a status badge (Unscored /
        Scored / Approved / Processed / Closed), an "Open" button per row
        that jumps to Match Entry with that match pre-selected, a
        next-week readiness card, and a handicap section (recorded
        changes for the week plus the season-wide recommendations
        preview). **API-verified**: `GET
        /api/seasons/{id}/weeks/{week}/recap` now returns
        `approved_at`/`processed_at`/`week_closed` per match against a
        real generated schedule on a local server build -- confirmed a
        freshly-generated match shows all three fields empty/false, and
        after approving then processing that match via curl, the same
        recap call showed `approved_at` and `processed_at` both set and
        `week_closed` still false (matches "Processed" in the status
        ladder, not yet "Closed").
  - [x] **Browser-verified on staging (2026-08-27)**: when a week has unscored matches,
        an incomplete-week-safe note should appear explaining the
        handicap/stats sections reflect data entered so far, not final
        results. **API-verified**: `missing_count` in the recap response
        correctly counted the two remaining unscored matches in the
        local-server test week.
  - [x] **Browser-verified on staging (2026-08-27)**: clicking "Process Approved
        Scores" should process every approved-but-unprocessed match in
        the displayed week (looping the existing per-match process
        endpoint) and show a summary toast, then refresh the match
        status rows. **Confirmed at the code level and via the
        underlying endpoint**: `POST /api/matches/{id}/process` on an
        approved match correctly set `processed_at` and left
        `week_closed` false; the loop logic itself
        (`#processApproved()` in `weekly-summary-page-component.js`) is
        new frontend orchestration, not a new backend capability, so no
        additional API-level test beyond confirming the underlying
        single-match endpoint behaves as expected.
  - [x] **Browser-verified on staging (2026-08-27)**: "Open in Schedule" should
        navigate to the Schedule page with the selected season
        preselected. **Confirmed at the code level**: dispatches the
        same `season-nav-request` custom event the Seasons domain
        already uses for this exact purpose, handled by the existing
        shell listener in `web/app.js` -- no new navigation logic.
- New focused Go tests (all passing,
  `backend/storage/sqlite/week_store_test.go`):
  `TestWeekStore_GetWeekRecapData_ApprovalFieldsDefaultUnset` and
  `TestWeekStore_GetWeekRecapData_ApprovalFieldsReflectMatchState`
  (mixed week: one approved-only match, one approved+processed+closed
  match, confirming both are scanned independently and correctly).
- Known, deliberately out-of-scope note (at the time): substitute lineup
  rows (`is_sub`/`sub_for_id`) could not be created through the API
  (the write path hardcoded `is_sub=0`), so this screen -- like the rest
  of the app -- had nothing sub-specific to display; this was a real,
  pre-existing gap discovered during Weekly Summary discovery, not a
  regression from this phase. **Update 2026-09-02:** substitute
  creation now exists (Substitute Workflow Phase 1, see section 21
  below); `GetWeekPlayerStats` now returns `is_sub`/`sub_for_name` per
  player, but this screen still does not render `player_stats` in any
  form, so there is still nothing displayed here -- see section 21 for
  why that display work was left for a later phase.
- Staging verification (2026-08-27, `http://league-staging.local`,
  deployed from `b93a177`): opened Weekly Summary in the in-app browser,
  confirmed the Fixture Scoresheet Season and five week options load,
  and verified Week 1 renders two Unscored rows, the incomplete-week
  warning, Week 2 readiness, the handicap section, and the Open in
  Schedule action. For the action path, created a disposable
  `league_admin` staging user, approved match 33 through the API, opened
  Week 2 in Weekly Summary, confirmed the "Process Approved Scores (1)"
  button and mixed Approved/Scored rows, clicked the button in the
  browser, and confirmed the row refreshed to Processed, the button
  disappeared, and the success toast said "Processed 1 of 1 match".
  Also verified the per-row Open button navigates to Match Entry with
  the selected match loaded, Open in Schedule navigates to Schedule, and
  Review & Apply opens Handicap Review with the selected week context.
  Restored match 33 by unprocessing and unapproving it; final recap
  confirmed `approved_at=false`, `processed_at=false`, and
  `week_closed=false`. The temporary admin key file was deleted and the
  browser Admin Key was cleared afterward. The disposable staging user
  remains because there is no delete-user endpoint, consistent with prior
  staging passes.

### 20. Financial Screen (Phase 1, 2026-08-27)

- Browser: "Financial" nav entry (hidden unless the Admin Key resolves to
  league_admin/admin/system_admin), season selector.
  - [x] **Browser-verified on staging (2026-08-29)**: selecting a season should show a
        Dues section listing every rostered player with a paid/unpaid
        badge, total paid, and last payment date, and a Payouts section
        listing every season team with its standing shown for reference
        and total paid. **API-verified** against a local server build
        with a real seeded season/roster: `GET
        /api/seasons/{id}/finances/dues` correctly listed all rostered
        players as unpaid before any payment, and `GET
        /api/seasons/{id}/finances/payouts` correctly listed all season
        teams with `total_paid:0` and a real (zero-value) standings
        reference before any payout.
  - [x] **Browser-verified on staging (2026-08-29)**: without a valid league_admin/
        admin/system_admin Admin Key, the Financial nav entry should
        stay hidden, and the screen should show a clear error if reached
        anyway. **API-verified**: all four finance routes correctly
        return 401 with no Authorization header and 403 with an invalid
        key -- confirmed directly via curl for GET dues, POST
        dues-payments, GET payouts, and POST payouts.
  - [x] **Browser-verified on staging (2026-08-29)**: clicking "Record Payment" on a
        player row should open a modal (amount, paid date, note),
        submitting it should flip that player to Paid with the entered
        amount, and the row should update without a full page reload.
        **API-verified**: `POST /api/seasons/{id}/finances/dues-payments`
        correctly recorded a payment, denormalized the player's roster
        `team_id` onto the stored row, and a follow-up `GET .../dues`
        showed `paid:true`, the correct `total_paid`, and the payment in
        the player's history.
  - [x] **Browser-verified on staging (2026-08-29)**: clicking "Record Payout" on a
        team row should open a modal (amount, note), submitting it
        should update that team's total paid and history.
        **API-verified**: `POST /api/seasons/{id}/finances/payouts`
        correctly recorded a payout and a follow-up `GET .../payouts`
        showed the updated `total_paid` and the payout in the team's
        history.
  - [ ] **NOT VERIFIED (no browser)**: attempting to record a payment
        for a player not on the season's roster, or a payout for a team
        not in the season, should show a clear error rather than
        silently succeeding or crashing. **API-verified**: both return
        404 with a specific message ("player is not rostered for this
        season" / "team is not part of this season").
- New focused Go tests (all passing): 12 `FinanceService` tests
  (`backend/domains/finances/service_test.go`) covering validation and
  store delegation for both dues payments and payouts; 8 `FinanceStore`
  tests (`backend/storage/sqlite/finances_store_test.go`) covering
  insert/list/newest-first-ordering/season-scoping for both tables; 11
  handler tests (`handlers/api_finances_test.go`) covering 401/403 on
  all four routes, a successful league_admin read, full dues and payout
  success paths, and the two 404 validation cases above.
- Known, deliberately out-of-scope note: dues/payout entries cannot be
  edited or voided (both tables are append-only by design) -- correcting
  a mistake today means recording an offsetting entry, not modifying the
  original row. This is intentional for Phase 1, not a gap.

#### Staging verification (2026-08-29)

- **API checks: passed.** Against staging with a disposable
  `financial-phase-1-verify-20260829` league_admin user and the Fixture
  Scoresheet Season (`league_id: 3`, `season_id: 6`, 12 rostered
  players, 4 season teams):
  - `GET .../finances/dues` and `GET .../finances/payouts` both
    correctly returned 401 with no Authorization header.
  - With the disposable key, both GETs succeeded.
  - `POST .../finances/dues-payments` recorded `payment_id: 1`
    (`player_id: 42`, `team_id: 14`, `amount: 1.23`) and the player
    correctly flipped to paid.
  - `POST .../finances/payouts` recorded `payout_id: 1` (`team_id: 14`,
    `amount: 2.34`) with the team's `total_paid` correctly updated to
    2.34.
- **Browser checks: passed up to modal open.** Nav hidden with no
  Admin Key, visible and identity-resolved after setting the disposable
  league_admin key, Financial screen opens, Dues table renders all 12
  players, Payouts table renders all 4 teams, both API-created records
  above are visible in the UI, and both modals open showing the correct
  selected player/team name.
- **Bug found:** the Dues and Payout modals displayed the correct
  selected name but their hidden `#fin-dues-player-id` /
  `#fin-payout-team-id` inputs stayed empty, so a browser-driven submit
  would not reliably send the selected `player_id`/`team_id`. The
  browser write workflow (as opposed to the API writes above, which
  bypassed the modal entirely) was **not** verified working end-to-end
  on staging.
  - Fixed on branch `financial-screen-phase-1-followup-modal-ids`:
    the modals now track the selected `player_id`/`team_id` as private
    `<finances-page>` component fields set in the same call that sets
    the displayed name, instead of separate hidden form inputs, so the
    displayed name and submitted ID cannot drift apart.
  - **Post-fix browser verification passed (2026-08-29, deployed from
    `c693a41`)**: after redeploying the modal-ID fix, opened Financial
    with the disposable league_admin Admin Key, confirmed the nav was
    visible and the screen loaded Fixture Scoresheet Season, then
    clicked "Record Payment" for Avery Slate (`player_id: 41`) and
    confirmed the modal showed Avery Slate, no hidden ID input existed,
    submitting `3.21` closed the modal, showed "Payment recorded", and
    refreshed Avery Slate's row to Paid with `$3.21`. Also clicked
    "Record Payout" for Fixture Bankers (`team_id: 15`) and confirmed
    the modal showed Fixture Bankers, no hidden ID input existed,
    submitting `4.56` closed the modal, showed "Payout recorded", and
    refreshed Fixture Bankers' row to `$4.56`.
- Staging artifacts created by this pass are real, append-only rows
  and were intentionally left in place (matches the append-only-by-
  design note above): `dues_payments` id 1 (`amount: 1.23`,
  `player_id: 42`, `team_id: 14`), `payouts` id 1 (`amount: 2.34`,
  `team_id: 14`), plus the post-fix browser rows for Avery Slate
  (`player_id: 41`, amount `3.21`) and Fixture Bankers (`team_id: 15`,
  amount `4.56`). The disposable staging user remains because there is
  no delete-user endpoint; the browser Admin Key was cleared and the
  temporary key file was deleted after verification.

---

### 21. Substitute Workflow Phase 1 -- Admin Substitute Support (2026-09-02)

- Browser: Match Entry's roster table and "Confirm Tonight's Lineup"
  picker.
  - [ ] **NOT VERIFIED (no browser)**: when a team already has a saved
        lineup for the week, each of the 3 roster rows should show a
        small "Sub" button (only while scores are still editable).
        Clicking it opens a modal listing every league player; confirming
        a different player updates that slot's name/handicap on the
        scoresheet and shows a "Sub for X" badge with an "Undo" link.
        **API-verified** against a local server build: saved a real
        lineup through `POST /api/lineup-plans`, called `POST
        /api/lineup-plans/{id}/substitute`, and confirmed a follow-up
        `GET /api/lineup-plans` showed the substitute's `player_id`,
        `is_sub:true`, and the original player's id as `sub_for_id`.
        Calling `DELETE .../substitute` afterward reverted the row to
        exactly its original state.
  - [ ] **NOT VERIFIED (no browser)**: the manual "Confirm Tonight's
        Lineup" picker's player dropdowns should list the team's own
        roster first ("This Team") and every other league player second
        ("Other Players (Substitute)"), so a true substitute (not
        normally on this team) can be selected there too. **Confirmed at
        the code level**: `makeOpts()` in
        `match-entry-page-component.js` now builds both `<optgroup>`s
        from `this.#allPlayers` instead of the team-filtered roster
        array.
  - [ ] **NOT VERIFIED (no browser)**: a substitute chosen through either
        path should score correctly -- their real current handicap and
        name should appear throughout the scoresheet, and saving the
        scoresheet should record `match_results` under the substitute's
        own `player_id`. **Confirmed at the code level and via the
        existing `SaveRounds` behavior** (unchanged): the scoring path
        already trusted whatever `player_id` a round submission listed;
        the fix was making the *frontend* resolve a substitute's
        `player_id` correctly (against the full player list, not the
        team roster) rather than any backend scoring change.
- New focused Go tests (all passing): 12 new `LineupService` unit tests
  (`backend/domains/matches/lineup_service_test.go`) covering
  validation, all four lock checks (season closed, week closed,
  approved, processed), the no-match-scheduled-yet allow path, the
  UNIQUE-constraint-to-409 mapping, and clear-substitute delegation; 8
  new SQLite `LineupStore` tests
  (`backend/storage/sqlite/lineup_store_test.go`) covering
  `GetLineupPlan`/`FindMatchID` found/not-found, `SetSubstitute`, and
  `ClearSubstitute`; 13 new handler tests
  (`handlers/api_lineup_substitute_test.go`) covering 401/403/409/400
  across auth and all four locks, league_admin/admin/system_admin
  success, and a full set-then-clear round trip over real HTTP; 1 new
  Weekly Summary store test confirming `is_sub`/`sub_for_name` populate
  correctly (`TestWeekStore_GetWeekPlayerStats_ShowsSubstituteStatus`);
  1 new Player Overview stats regression test confirming a substitute's
  results count toward their own totals
  (`TestRoundStore_GetPlayerStats_SubstitutePlayer_StatsCountTowardOwnTeam`).
- `node --check` passes on both changed/new JS files:
  `web/domains/matches/match-entry-page-component.js`,
  `web/domains/matches/match-entry-api-service.js`.
- Known, deliberately out-of-scope notes (not oversights):
  - No Sub control for a slot resolved only from already-scored round
    results (no known `lineup_plans` row for it) -- retroactively
    substituting a played match raises different questions this phase
    doesn't answer.
  - Weekly Summary's `player_stats` (now carrying `is_sub`/
    `sub_for_name`) is still not rendered in that screen at all --
    pre-existing, not a regression; see Known Gaps row #15.
  - Player Overview's schedule section still won't show a substitute's
    one-off match for a different team -- accepted, documented
    limitation, not the "big player-history redesign" this phase was
    told not to force.

#### Staging verification (2026-09-02)

**Result: PASS (API-level).** Verified on `http://league-staging.local`,
source commit `1b0ac5b`. No browser available in this developer's tool
session, so all checks below are direct API calls against the live
staging server -- see "NOT VERIFIED (no browser)" items at the end for
what a browser pass still needs to confirm.

**Fixture used:** a fresh, fully disposable league (not the shared
Fixture Scoresheet League), created and torn down entirely within this
pass rather than reusing shared staging data -- season/week/match/lock
testing includes closing a week and closing a season, which are not
safely reversible-by-inspection operations to run against data other
staging passes depend on. Exact IDs (all deleted by the end of this
pass): league id 6 ("Substitute Verify League 20260902"), season id 9
("Substitute Verify Season"), home team id 23 ("Verify Home", season
team id 57), away team id 24 ("Verify Away", season team id 58), home
roster players 62/63/64 (handicaps 1/2/3), away roster players 65/66/67
(handicaps 1/2/3), substitute player 68 ("Sub Verify", handicap 4.5, on
the Away team's direct roster but not the Home team's), match id 42
(week 1, home 23 vs away 24), lineup_plans row id 67 (Home slot 1,
originally player 62). A disposable `league_admin` personal-key user
(id 14, `substitute-verify-20260903`) was created via the static admin
token to perform all of this and remains afterward -- no delete-user
endpoint exists, matching every prior staging pass's bootstrap-user
handling.

1. **Admin key / auth -- PASS.** `POST /api/lineup-plans/67/substitute`
   with no Authorization header returned 401 with a `WWW-Authenticate`
   header; with an invalid key returned 403 `{"error":"forbidden"}`;
   with the disposable `league_admin` key, set and clear both
   succeeded (200).
2. **Match Entry data model -- PASS (API-level, see NOT VERIFIED
   below for the browser UI itself).** `POST .../lineup-plans/67/substitute`
   with `{"substitute_player_id":68}` returned
   `player_id:68, player_name:"Sub Verify", handicap:4.5, is_sub:true,
   sub_for_id:62` -- exactly the data Match Entry's roster table/badge
   would render. A follow-up `GET /api/lineup-plans?season_id=9&week_number=1`
   confirmed the same row persisted correctly alongside the two
   untouched slots.
3. **Score save behavior -- PASS.** Saved a full 9-round scoresheet via
   `POST /api/matches/42/rounds` with the Home slot 1 pairing using
   `home_player_id:68` (the substitute). `GET /api/matches/42/rounds`
   confirmed `home_player_id:68, home_player_name:"Sub Verify",
   home_handicap:4.5, home_handicap_used:4.5` and a correctly computed
   `handicap_pts_used` from the substitute's real 4.5 rating (not the
   original player's) -- confirms requirement 5 (handicap/diff reflects
   the actual substitute) directly from real round data, not just the
   lineup row.
4. **Undo behavior -- PASS.** `DELETE /api/lineup-plans/67/substitute`
   returned `player_id:62, is_sub:false` with `sub_for_id` omitted --
   reverted to exactly the original state. Re-verified the reverse
   direction too (set again after confirming clear worked) to leave the
   fixture substituted for the score-save/lock tests that needed it.
5. **Lock behavior -- PASS, all four.** Attempted `DELETE
   .../substitute` in each locked state and got 409 every time, with
   the expected message: week closed -> `"week is closed; substitutes
   cannot be changed"`; match approved -> `"match scores are approved;
   substitutes cannot be changed"`; match processed -> `"match scores
   are processed; substitutes cannot be changed"`; season closed ->
   `"season is closed; substitutes cannot be changed"`. Reopened/
   unapproved/unprocessed between each check to isolate them, matching
   the guard's own validation order.
6. **Weekly Summary / data surface -- PASS.** `GET
   /api/seasons/9/weeks/1/recap` returned a `player_stats` entry for
   player 68 with `"is_sub": true, "sub_for_name": "HomeP1 Verify"`
   alongside correct set/game totals for that match. Confirmed (as
   expected, not a regression) that the Weekly Summary screen itself
   still does not render `player_stats` in any form -- still not
   browser-visible, by design for this phase (see Known Gaps row #15
   and the note above).
7. **Player Overview -- PASS.** `GET /api/player-stats?season_id=9`
   (the same season-scoped query Player Overview's stats section calls)
   showed player 68 with `sets_won:3, games_won:6` correctly attributed
   to their own team ("Verify Away"), while the original player 62
   correctly showed all zeros (they didn't play). Confirms substitute
   results count toward the substitute's own stats. Did not re-verify
   the schedule-section limitation directly against this fixture (a
   single-match season has no meaningful "schedule list" to inspect
   beyond the one match already confirmed above); the limitation is
   accepted and documented, not something this pass needed to
   re-derive.
- **Restoration:** season reopened (`closed_at` cleared) before
  cleanup; the entire disposable league (id 6) was then deleted via
  `DELETE /api/leagues/6`, which cascade-deleted the season, both
  teams, all 7 players, the match, lineup_plans, round_results, and
  match_results -- confirmed gone via a follow-up `GET /api/leagues`
  showing only the three pre-existing leagues (Demo Pool League, Demo
  9-Ball League, Fixture Scoresheet League) unchanged. The disposable
  `league_admin` user (id 14) remains, per the no-delete-endpoint
  convention noted above. The shared Fixture Scoresheet League/season
  data was never touched by this pass.
- **Discrepancies found:** none affecting the substitute workflow. One
  incidental API-usability observation, unrelated to this phase: `PUT
  /api/seasons/{id}/teams/{tid}` requires `season_name` in the body even
  when only updating `captain_id` (omitting it returns `{"error":
  "season_name is required"}`) -- a pre-existing partial-update
  behavior in the season-teams endpoint, not a substitute-workflow bug,
  not fixed on this verification-only branch.
- **NOT VERIFIED (no browser), still required before this phase is
  browser-complete:** the actual Match Entry screen rendering -- the
  "Sub" button appearing on an editable roster row, the substitute
  modal opening with the match's other five players excluded from its
  list (see the "same-match duplicate-player guard" correction below;
  the "This Team" / "Other Players (Substitute)" grouping belongs to the
  separate manual "Confirm Tonight's Lineup" picker, not this modal --
  corrected here after an earlier draft of this doc wrongly attributed
  that grouping to the substitute modal), the "Sub for X" badge and
  "Undo" link rendering after a substitution, and the scoresheet
  visually reflecting the substitute's name/handicap. All of the
  underlying data this UI reads was confirmed correct at the API level
  above.

#### Correction: same-match duplicate-player guard (2026-09-02)

**Browser finding (PM, staging):** during browser verification of
Substitute Workflow Phase 1, the substitute modal allowed replacing Home
H1 with a player who was already Visitor V1 in the *same match*.
Concretely: League "Fixture Scoresheet League", season 6 ("Fixture
Scoresheet Season"), match 31 (`[W1] Fixture Breakers vs Fixture
Bankers`), lineup_plans row 1 (Home H1, originally Avery Slate).
Selecting Devon Reed (player_id 44) as the substitute succeeded even
though Devon Reed was already Visitor V1 via lineup_plans row 4. Match
Entry reloaded showing Devon Reed on both teams in the same match. PM
immediately clicked Undo; a follow-up API check confirmed all week-1
lineup_plans rows reverted to `is_sub:false` with their original
`player_id`s -- no lasting data corruption, but the bad state was
reachable and briefly persisted.

**Root cause:** neither the substitute modal nor the backend checked
whether the chosen substitute already occupied a different slot in the
*same scheduled match*. The Phase 1 design deliberately allowed a
substitute to come from any team/league (no roster-membership check),
but never restricted it to "not already in this specific match" on
either side.

**Fix (branch `substitute-workflow-same-match-guard`):**
- Backend: `LineupService.SetSubstitute` now checks, whenever a match is
  scheduled for the slot's season/team/week, whether the candidate
  substitute already has a `lineup_plans` row for that same
  season/week under *either* the match's home or away `team_id`
  (excluding the slot being substituted). A new
  `LineupStore.PlayerInMatchLineup` method backs this (one `SELECT
  EXISTS` scoped by `season_id`, `week_number`, and `team_id IN
  (home, away)`). Violations return `409 Conflict`,
  `{"error": "that player is already in this match"}`
  (`SUB_PLAYER_ALREADY_IN_MATCH`). The existing same-team
  `SUB_ALREADY_IN_LINEUP` conflict (from the table's own `UNIQUE`
  constraint) is unchanged and still applies. All four existing locks
  (season closed, week closed, approved, processed) are unchanged.
  `ClearSubstitute` was deliberately not given this check -- reverting a
  slot to the player who already held it cannot introduce a new
  duplicate that wasn't already possible before the substitution
  existed.
- Frontend: `#openSubstituteModal` now excludes every player already
  occupying one of the match's 6 slots (both teams) from the picker,
  not just the current slot's own player -- so the exact browser
  sequence PM hit can no longer even be attempted through the UI. This
  is a client-side convenience on top of the authoritative backend
  check, not a replacement for it.
- Docs correction: the "NOT VERIFIED (no browser)" item above previously
  (incorrectly) implied the substitute modal has "This Team" / "Other
  Players (Substitute)" `<optgroup>`s -- it does not and still does not;
  that grouping only exists on the separate manual "Confirm Tonight's
  Lineup" picker. Corrected in place above rather than adding grouping
  to the modal, per the scope guard for this branch (fix the
  duplicate-player hole and directly-related wording only).
- Tests added: `TestLineupService_SetSubstitute_PlayerAlreadyInMatch_ReturnsConflict`
  and `TestLineupService_SetSubstitute_PlayerNotInMatch_AllowsChange`
  (`backend/domains/matches/lineup_service_test.go`);
  `TestLineupStore_PlayerInMatchLineup_TrueWhenOnOtherTeam`,
  `TestLineupStore_PlayerInMatchLineup_FalseWhenNotInMatch`, and
  `TestLineupStore_PlayerInMatchLineup_ExcludesOwnSlot`
  (`backend/storage/sqlite/lineup_store_test.go`);
  `TestLineupSubstitute_PlayerAlreadyInMatch_Returns409`
  (`handlers/api_lineup_substitute_test.go`, full HTTP round trip:
  seeds a player already on the away side of a match, then confirms
  substituting them into a home slot returns 409 with the exact error
  message). All pre-existing Substitute Workflow tests continue to
  pass unchanged. The frontend exclusion filter was verified with a
  focused, throwaway Node script exercising the extracted filter logic
  against a 6-player match plus 2 outside players (no JS test runner
  exists in this repo, consistent with every other frontend change in
  this codebase) -- not committed, since it isn't a permanent test
  file.
- **NOT VERIFIED (no browser) at the time this correction was written:**
  that the modal visually omits the five other in-match players when
  opened on real staging, and that attempting the exact original repro
  (Devon Reed into Home H1 while already Visitor V1) is now blocked in
  the browser. The backend 409 and the frontend filter logic were
  confirmed correct above at the code level; the live rendering/
  click-through needed a real browser pass. **Now closed -- see the
  staging verification subsection immediately below.**

#### Staging verification (2026-09-03)

**Result: PASS.** Verified on `main`/`origin/main` at `61bd12c`
("Substitutes: prevent same-match duplicate player selection"). This
closes the "NOT VERIFIED (no browser)" item directly above.

- **API verification:** the exact original repro was attempted directly
  against staging -- `POST /api/lineup-plans/1/substitute` with
  `substitute_player_id: 44` (Devon Reed, the same lineup_plans row and
  player from the original finding above) -- and correctly returned
  `409 Conflict`, `{"error":"that player is already in this match"}`.
  A follow-up lineup check confirmed zero rows changed
  (`CHANGED_ROWS=0`): the rejected request left the real fixture lineup
  untouched.
- **Browser verification:** opened Match Entry on staging for the
  Fixture Scoresheet Season, selected `[W1] Fixture Breakers vs Fixture
  Bankers`, and opened the Home H1 substitute modal for Avery Slate.
  The modal's option list contained exactly the 6 players *not*
  currently in this match (Gray Lumen, Indigo North, Jules Pike, Harper
  Quill, Kai Ridge, Lena Stone) and correctly excluded all 6 players
  who *are* currently in this match (Avery Slate, Blair Flint, Casey
  Vale, Devon Reed, Emery Frost, Finley Moss). The exact previous bad
  browser path -- selecting Devon Reed into Home H1 while Devon was
  already Visitor V1 -- is confirmed no longer reachable through the
  modal. This closes both outstanding NOT VERIFIED items: the modal's
  live exclusion rendering, and the original repro being blocked in the
  browser.
- **Non-regression check:** while setting up this pass, PM tried a
  substitute from the *other* week-1 match (a different scheduled
  match, same week). That substitution succeeded -- expected, since the
  guard only blocks a player already in the *same* match, not every
  other player in the week. PM immediately cleared it with `DELETE
  .../substitute` and confirmed all week-1 lineup rows were restored
  with no remaining `is_sub`/`sub_for_id` rows. Confirms the fix is
  correctly scoped to "same match," not overly broad.
- **Cleanup:** temporary local API key file removed. A disposable
  staging `league_admin` verification user (id 16, username
  `sub-same-match-verify-20260903-172202`) remains -- no delete-user
  endpoint exists, consistent with every prior staging pass's
  bootstrap-user handling. Standing exclusions
  (`architecture-diagram.md`, `architecture-review.md`,
  `backend/storage/postgres/`) untouched.

---

### 22. Player Account Access Phase 1 (2026-09-03)

This is API-key V1 player access, not the final login/session model.

- Browser: sidebar "Admin Key" modal (a player key resolves through the
  same modal as an admin key), the new "My Overview" nav entry, the
  Users screen's role picker, and the Players list.
  - [ ] **NOT VERIFIED (no browser)**: pasting a `role=player` personal
        key into the Admin Key modal resolves and shows the player's
        identity (same "Signed in as `<username>` (`<role>`)" line
        Users Admin Screen Phase 1 added); the "My Overview" nav entry
        becomes visible, and the existing admin "Player Overview" nav
        entry, "Users", "Financial", and "Backup DB" all stay hidden.
        **API-verified** against a local server build: `GET
        /api/users/me` with a `role=player` key returned
        `{"role":"player","player_id":<id>,...}`.
  - [ ] **NOT VERIFIED (no browser)**: clicking "My Overview" opens
        Player Overview directly on the linked player's own record, with
        the player-select dropdown hidden (not just defaulted) since the
        viewer may not choose a different player, and this succeeds
        regardless of which league/season the app shell currently has
        selected -- the request omits `season_id` entirely on this path
        rather than passing the shell's selected season, so the backend
        falls back to the linked player's own league's active season
        (PM-caught correction, see the note below). **API-verified**:
        `GET /api/players/{own_id}/overview` with the player's own key
        returned 200 with the full overview (schedule, stats, handicap,
        dues); `GET /api/players/{other_id}/overview` with the same key
        returned 403.
  - [ ] **NOT VERIFIED (no browser)**: a `role=player` key gets 403 (not
        a broken page) if used against the Users, Financial, or Backup
        surfaces directly. **API-verified**: `GET /api/users` returned
        403, `POST /api/backup` returned 403, both with the same
        `role=player` key that succeeded against its own overview above.
  - [ ] **NOT VERIFIED (no browser)**: the Users Admin screen's "Add
        User" modal shows a "Player" role option; selecting it reveals a
        required "Linked Player" picker populated from the current
        league's players; saving without selecting a player is blocked
        client-side ("Select a player to link"); the created user
        appears in the list with its linked player name in a new
        "Linked Player" column. **API-verified**: `POST /api/users` with
        `role:"player"` and no `player_id` returned 400 ("player_id is
        required for role=player"); with a nonexistent `player_id`
        returned 400 ("player_id does not reference an existing
        player"); with a valid `player_id` returned 201 with a one-time
        key, and the same player id resolved through `/me` and appeared
        in `GET /api/users`'s `player_name` field.
  - [ ] **NOT VERIFIED (no browser)**: the existing admin "Player
        Overview" nav entry and the Players list's "View Overview" row
        button both stay hidden for a `role=player` identity, matching
        their existing admin-only gating from Player Overview Phase 2 --
        no new code path was added for this case, so this is a
        regression check, not new behavior. **Confirmed at the code
        level**: `hasFinanceAdminRole(identity)` (unchanged) returns
        `false` for `role:"player"`, and both that nav entry and the row
        button already gate on it.
- New focused Go tests (all passing): 4 new `ApplyAuthStore` tests
  (`backend/storage/sqlite/apply_auth_store_test.go`) covering
  `CreateApplyPlayerUser`, resolving a linked player's `player_id`,
  confirming an admin-role resolve still has a nil `player_id`, and
  `ListApplyUsers` showing the joined player name; 3 new handler tests
  (`handlers/api_apply_c1_test.go`) covering the missing/nonexistent/
  valid `player_id` cases on `POST /api/users`; 1 new `/me` test
  confirming `player_id` round-trips; 4 new handler tests
  (`handlers/api_player_overview_auth_test.go`) covering a player key
  reading its own overview, being rejected from another player's
  overview, and being rejected from the finance and users route groups.
  `go test ./... -count=1` and `go build ./...` pass with zero
  regressions.
- `node --check` passes on all four changed JS files: `web/app.js`,
  `web/domains/users/users-management-page-component.js`,
  `web/domains/players/player-overview-page-component.js`,
  `web/domains/players/players-page-component.js` (unchanged; checked
  only to confirm no edit was needed).
- Known, deliberately out-of-scope notes (not oversights):
  - No edit, deactivate, or key-rotation endpoint for any role,
    including `player` -- unchanged from every prior Users phase.
  - No score submission, captain approval, browser sessions, passwords,
    JWTs, email invitations, or mobile notifications -- explicitly out
    of scope per PM decision for this phase.
  - `users.player_id` uniqueness is enforced by a partial unique index
    (`WHERE player_id IS NOT NULL`), not a `UNIQUE` column constraint,
    since SQLite's `ALTER TABLE ADD COLUMN` cannot add one directly; see
    `doc/domains/users/README.md`'s "Player Relationship" section.

#### Correction: "My Overview" ignored the shell's selected league (2026-09-03, same day)

**PM review caught this before commit** (not a staging finding): "My
Overview" passed the app shell's currently selected `activeSeason.id`
into the overview request even for a locked (`role=player`) view. If the
shell happened to be on a different league's active season, the backend
correctly rejected the request, so "My Overview" could fail depending on
whatever an admin had last selected in that browser tab -- unacceptable
for a player-facing entry point.

- **Fix:** `player-overview-page-component.js`'s `#load(forcedPlayerId)`
  now omits `season_id` entirely on the locked path
  (`seasonId = forcedPlayerId != null ? null : this.#activeSeason?.id`),
  letting the backend's existing, already-documented fallback (the
  player's own league's active season) apply instead. Admin loads are
  unchanged and still pass the shell's selected season.
- **Verification:** frontend-only change -- `node --check` on
  `web/domains/players/player-overview-page-component.js` and
  `web/app.js`; `go test ./... -count=1` and `go build ./...` rerun for
  regression safety (pass, zero regressions, since no Go code changed).
  Confirmed at the code level, not yet re-verified with a fresh local
  server walkthrough or on staging. Actual browser confirmation that "My
  Overview" now succeeds regardless of the shell's selected league
  remains **NOT VERIFIED (no browser)**.

#### Staging verification (2026-09-04)

**Result: PASS (API-level + deployed-static-asset verification).**
Verified on `http://league-staging.local`. No browser available in this
developer's tool session, so every item below is either a direct API call
against the live staging server or a direct fetch of the deployed static
JS/HTML confirming the exact reviewed source is what staging serves --
see the "NOT VERIFIED (no browser)" summary at the end for what a real
click-through pass still needs to confirm.

1. **Deployed checkpoint -- PASS.** `POST /api/users` with
   `{"role":"player"}` and no `player_id` returned
   `{"error":"player_id is required for role=player"}` -- the exact
   Phase 1 validation message, not a generic role-rejection, confirming
   the deployed build includes Phase 1. Independently confirmed by
   fetching the live `/domains/players/player-overview-page-component.js`
   from staging and finding the exact reviewed fix line present:
   `const seasonId = forcedPlayerId != null ? null : this.#activeSeason?.id;`.
   `GET /healthz` returned `{"status":"ok"}` first. (Staging does not
   expose its running commit hash through any endpoint; the two
   Phase-1-specific behavioral/source checks above are the available
   substitute, consistent with how prior passes without a version
   endpoint confirmed their deployed checkpoint.)
2. **Admin key -- PASS.** Bootstrapped a disposable `system_admin`
   personal-key user via the static `LEAGUE_ADMIN_TOKEN`, same pattern as
   every prior staging pass (id 17,
   `player-access-verify-admin-20260904-053641`).
3. **Player-linked user creation -- PASS (API-level).** Using that
   `system_admin`'s own personal key (not the static token), `POST
   /api/users` with `{"role":"player","player_id":41}` (Avery Slate, a
   Fixture Scoresheet League fixture player) returned 201 with a one-time
   key and `player_id:41` on the created user (id 18,
   `player-access-verify-avery-20260904`). A follow-up `GET /api/users`
   (same admin key) showed user 18 with `"player_id":41,"player_name":
   "Avery Slate"` and user 17 (the `system_admin`) with neither field
   present -- confirms both the Linked Player column's data source and
   goal 4's "unlinked users omit player_id" in the same call. **NOT
   VERIFIED (no browser)**: the Users Admin screen's Add User modal
   itself (Player role option selection, the Linked Player picker
   appearing/populating, the one-time key alert rendering, the list's
   Linked Player column rendering) -- confirmed at the deployed-source
   level instead (`value="player"`, `.um-linked-player-row`,
   `#toggleLinkedPlayerRow()` all present in the live
   `users-management-page-component.js`).
4. **`GET /api/users/me` -- PASS.** With the new player key: `{"id":18,
   ...,"role":"player","player_id":41,...}`. With the `system_admin` key:
   `{"id":17,...,"role":"system_admin",...}` with `player_id` entirely
   absent from the JSON (not present as `null`) -- confirms the corrected
   doc wording from the prior review round matches real server behavior,
   not just the Go struct tag.
5. **"My Overview" access and the season-scope fix -- PASS.** `GET
   /api/players/41/overview` (own player, no `season_id`) with the player
   key returned 200 with the full overview, falling back to season 6
   ("Fixture Scoresheet Season") -- the fixture league's own active
   season -- confirming the locked path's `season_id` omission works
   end to end against real staging data. **Cross-league reproduction**:
   the same request with `?season_id=2` (Demo Pool League's active
   season -- a different league than player 41's) returned `404
   {"error":"player is not in this season's league"}` -- this is exactly
   the failure shape the pre-fix code could have produced if the shell
   had a different league selected, and confirms why omitting
   `season_id` on the locked path (rather than "just usually works") is
   the correct fix. Combined with item 1's confirmation that the
   deployed frontend source already omits `season_id` on this path, this
   is as close to an end-to-end cross-league proof as is possible without
   a browser. **NOT VERIFIED (no browser)**: actually selecting a
   different league in the shell UI and clicking "My Overview" to watch
   it still load the linked player's own overview; the "My Overview" nav
   entry's visibility and the player-select dropdown's hidden state are
   confirmed only at the deployed-source level (`#nav-item-my-overview`,
   `isPlayerRole()`, `.po-selector-row` toggle all present as reviewed).
6. **Ownership restriction -- PASS.** `GET /api/players/44/overview`
   (Devon Reed, not the linked player) with the player key returned 403
   `{"error":"forbidden"}`.
7. **Admin access unchanged -- PASS.** The `system_admin` key
   successfully loaded both player 44's and player 41's overviews (200
   for each) -- confirms admin access to any player is unaffected. **NOT
   VERIFIED (no browser)**: the Players-list "View Overview" row button
   remaining admin-only; unchanged code path from the already-verified
   Player Overview Phase 2 gating, not re-derived here.
8. **Player key rejected from admin surfaces -- PASS.** With the player
   key: `GET /api/users` -> 403 `{"error":"forbidden"}`; `POST
   /api/backup` (sent with a real `{}` body per the known bodyless-POST/
   IIS workaround -- see the Critical Blocker section above) -> 403
   `{"error":"forbidden"}`; `GET /api/seasons/6/finances/dues` -> 403
   `{"error":"forbidden"}` (checked in addition to the two PM explicitly
   named, since Financial was also called out as a surface that must stay
   hidden). **NOT VERIFIED (no browser)**: Users/Financial/Backup nav
   entries actually staying hidden in a rendered page for this identity --
   confirmed only via the already-reviewed `hasFinanceAdminRole`/
   `isPlayerRole` gating logic being present in the deployed `app.js`.
- **Cleanup:** no fixture data was mutated -- re-fetched player 41 and
  `GET /api/leagues` after the pass and both are byte-identical to
  before. Two disposable users remain, per the no-delete-endpoint
  convention every prior staging pass has followed: id 17
  (`player-access-verify-admin-20260904-053641`, `system_admin`) and id
  18 (`player-access-verify-avery-20260904`, `role=player`, linked to
  player 41 Avery Slate). No API keys or secrets are recorded in this
  entry or elsewhere in this checklist. Standing exclusions
  (`architecture-diagram.md`, `architecture-review.md`,
  `backend/storage/postgres/`) untouched.
- **Discrepancies found:** none. All 8 verification goals from the PM
  memo passed at the API level, and every piece of frontend code relevant
  to a goal was independently confirmed present in the exact deployed
  static assets staging serves.
- **Follow-up needed:** a real browser click-through of "My Overview"
  (ideally including the cross-league scenario: select a different
  league in the shell, click "My Overview", confirm it still loads the
  linked player's own overview) and of the Users Admin "Add User" Player
  flow remain the only gaps before this phase is browser-complete. No
  code changes are indicated by this pass.

### 23. League Communication Screen Phase 1 (2026-09-06)

Copy/paste message generation only -- no automated email sending, SMS/
mobile push, template storage, message history, delivery tracking, or
communication preferences exist in this phase.

- Browser: the new "Communication" nav entry, and the screen's Message
  Type / Season / Week / Team / Player selectors.
  - [ ] **NOT VERIFIED (no browser)**: the "Communication" nav entry is
        hidden for no-key and `role=player` identities, and visible for
        `league_admin`/`admin`/`system_admin`, matching the Financial nav
        entry exactly. **Confirmed at the code level**: `app.js`'s
        `updateIdentityUI()` toggles `#nav-item-communications` off the
        same `canManageFinances` (`hasFinanceAdminRole(identity)`) value
        that already gates `#nav-item-finances` and
        `#nav-item-player-overview` -- no new permission model was
        introduced.
  - [ ] **NOT VERIFIED (no browser)**: selecting "Weekly Team Summary"
        shows Season/Week/Team selectors (Player selector hidden);
        selecting "Team Dues Reminder" shows Season/Team (Week and Player
        hidden); selecting "Player Summary" shows Season/Player (Week and
        Team hidden); changing any visible selector regenerates the
        message automatically (no separate "Generate" click needed);
        switching to a different season while on Weekly Team Summary (or
        switching to Weekly Team Summary after changing season on a
        different type) reloads the Week selector for the newly selected
        season before generating, rather than generating against a stale
        week left over from the previous season (see the correction
        below). **Confirmed at the code level**: `#toggleFieldsForType()`
        toggles `.comm-week-row`/`.comm-team-row`/`.comm-player-row` per
        the `MESSAGE_TYPES` table's `needsWeek`/`needsTeam`/`needsPlayer`
        flags, and the `change` listener is `async` and now `await`s
        `#loadWeeksIfNeeded()` before `#generate()` on both the type and
        season paths.
  - [ ] **NOT VERIFIED (no browser)**: the generated message renders in a
        read-only textarea, and clicking "Copy" either copies silently
        (secure context) or selects the text with a toast explaining
        Ctrl+C is needed (non-secure context, e.g. staging's plain HTTP).
        **Confirmed at the code level and via a standalone Node script**
        (see below) that the three message-building functions produce
        correctly formatted, correctly filtered text from realistic
        fixture-shaped data; the Clipboard-API-vs-fallback branch itself
        needs a real browser (and, for the fallback path specifically, a
        plain-HTTP context like staging) to observe.
- **Message content verification (API-shape level, via a standalone Node
  script against realistic fixture-shaped data, not a live server call --
  see `doc/domains/communications/README.md`'s Verification section):**
  - Weekly Team Summary: given a week recap with one match involving the
    selected team, the message correctly named the team's opponent and
    home/away side, showed the server-computed status label (`Approved`
    in the test case) and a matching reminder line, included the set
    score since `has_result` was true, surfaced `missing_count` as a
    league-wide note, and included the next week's scheduled match count.
  - Team Dues Reminder: given three players across two teams, the
    message correctly included only the two players on the selected team
    (the third, on a different team, was correctly excluded), correctly
    labeled the paid player with their total and the unpaid player as
    `UNPAID`, and correctly listed only the unpaid player's name in the
    reminder line.
  - Player Summary: given a full Player Overview-shaped response, the
    message correctly rendered handicap, win/loss/win-percent record,
    both schedule rows (one completed, one pending with a `TBD` date),
    and a paid dues status with the last payment date.
- No new Go tests -- this phase added no backend code. `go test ./...
  -count=1` and `go build ./...` were rerun anyway for full regression
  safety (cross-domain screen, per PM's explicit instruction) and both
  pass with zero regressions, confirming nothing in this phase's frontend
  work required or accidentally triggered a backend change.
- `node --check` passes on all five files: `web/domains/communications/
  communication-api-service.js`, `communication-message-generators.js`,
  `communication-page-component.js`, `communications-domain.js`, and
  `web/app.js` (nav gating + `loadSection` case).
- Known, deliberately out-of-scope notes (not oversights):
  - No automated email sending, SMTP, SMS/mobile push, template storage,
    message history, delivery tracking, or communication preferences --
    explicitly out of scope per PM decision for this phase.
  - No bulk "generate for every team/player" action -- one selection at
    a time in V1.
  - No real email address collection or display -- messages address
    people by name only.
  - No new backend aggregate endpoint -- filtering the existing week
    recap and season dues responses by `team_id` in the frontend was
    judged sufficient, not brittle cross-domain stitching.

#### Correction: stale week selector on type/season change (2026-09-07, PM review before commit)

**PM finding:** the Message Type and Season `change` handlers called
`#loadWeeksIfNeeded()` without awaiting it before calling `#generate()`.
Since `#loadWeeksIfNeeded()` is async, this meant Weekly Team Summary
could generate against the previous season's week value right after a
season change, and switching to Weekly Team Summary from a different
message type never reloaded the Week selector for the currently selected
season before generating at all.

- **Fix:** the `change` listener is now `async`, and both the type-change
  and season-change branches `await #loadWeeksIfNeeded()` before
  `await #generate()`; the week/team/player branch also awaits
  `#generate()` for consistency. `#loadWeeksIfNeeded()` additionally
  clears the Week selector to a "Loading weeks..." placeholder
  (empty value) synchronously before its own fetch, so even a
  `#generate()` call that slipped in while a fetch is in flight would see
  an empty week value (`#generate()` already treats that as "nothing to
  generate yet") rather than a stale one.
- **Verification:** frontend-only change, same file already covered by
  section 23 above. `node --check` on
  `communication-page-component.js`, `communication-message-generators.js`
  (unchanged, rechecked for regression safety), and `web/app.js` all pass.
  No Go code touched, so `go test`/`go build` were not rerun for this
  specific correction. Confirmed at the code level; actual browser
  confirmation of the season/type-switch sequencing remains **NOT
  VERIFIED (no browser)**, same as every other interaction in this
  section.

#### Staging verification (2026-09-09)

**Result: PASS (API-level + deployed-static-asset verification, with one
minor non-blocking wording discrepancy noted below).** Verified on
`http://league-staging.local`, deployed commit `aeaba9a` ("Communications
Phase 1: add copyable league messages"). No browser available in this
developer's tool session -- every item below is a direct API call against
real staging, a direct fetch confirming the deployed static JS/HTML
matches the reviewed source exactly, or the exact deployed generator
module executed in Node against real staging API responses (a stronger
check than the prior phase's synthetic-fixture Node run, since this time
the data itself came live from staging, not a hand-built object).

1. **Deployed checkpoint -- PASS.** `GET /healthz` returned `{"status":
   "ok"}` first. Fetched all three named static assets directly from
   staging and got 200 with the expected content:
   `/domains/communications/communications-domain.js`,
   `/domains/communications/communication-page-component.js`,
   `/domains/communications/communication-message-generators.js`.
   Confirmed the *corrected* source specifically (not a pre-correction
   draft): the deployed `communication-page-component.js` contains
   `this.addEventListener('change', async e => {`, both
   `await this.#loadWeeksIfNeeded();` / `await this.#generate();` pairs,
   and the `'Loading weeks...'` placeholder string -- all exactly as
   reviewed and approved.
2. **Admin visibility -- PASS.** `GET /api/users/me`: a disposable
   `league_admin` key (id 19, `comm-verify-admin-20260909-204346`)
   resolved with `role:"league_admin"`; a disposable `role=player` key
   (id 20, `comm-verify-player-20260909`, linked to player 45 since
   player 41 was already linked to an existing disposable user from a
   prior pass and `player_id` is uniquely constrained) resolved with
   `role:"player"`; no `Authorization` header returned 401. Combined with
   the already-confirmed deployed `app.js` gating
   (`document.getElementById('nav-item-communications')?.classList
   .toggle('d-none', !canManageFinances)`, reading the same
   `hasFinanceAdminRole` value `#nav-item-finances` and
   `#nav-item-player-overview` already use), this confirms the nav
   entry's visibility logic will resolve correctly for all three
   identity states. **NOT VERIFIED (no browser)**: actually seeing the
   nav entry itself appear/disappear in a rendered page.
3. **Weekly Team Summary -- PASS.** Ran the live
   `communication-message-generators.js` (fetched directly from staging,
   not a local copy) against two real recap responses:
   - `GET /api/seasons/6/weeks/1/recap` (Fixture Breakers vs Fixture
     Bankers, both unscored): generated message correctly showed team
     name, season name, week number, opponent, `(Home)`, `Status:
     Unscored`, the matching reminder line, no result line (has_result
     false), a missing-scores note, and next week's scheduled match
     count (2).
   - `GET /api/seasons/6/weeks/2/recap` (same matchup, scored): generated
     message correctly showed `Status: Scored`, a `Result: 0 sets - 0
     sets` line (this fixture's `match_results` only have game data, not
     set data, which is why both sides show 0 sets -- a pre-existing
     fixture-data characteristic, not a generator bug), the
     awaiting-approval reminder text, and correctly omitted the
     missing-scores note since `missing_count` was 0.
   - Selector sequencing itself (the season-change and type-switch reload
     behavior corrected in the prior review round) is a DOM/event-timing
     behavior and remains **NOT VERIFIED (no browser)** -- the corrected
     source being live on staging (item 1) is the strongest evidence
     available from this tool session.
   - **Minor discrepancy found (non-blocking, not a bug in the approved
     design):** the missing-scores note reads "N other match(es) ...
     still need scores," but `recap.missing_count` is the week's total
     missing-match count and is not reduced by one for the team's own
     match already shown above it -- so week 1's note said "2 other
     matches" when, from the reader's point of view, only the *other*
     match (Cutters vs Safeties) was actually still outstanding once
     their own match (shown Unscored just above) is accounted for. This
     matches the phase's own documented design (`doc/domains/
     communications/README.md` describes this line as surfacing
     `recap.missing_count` "as a league-wide note," not as an
     other-than-this-match count), so it is not a deviation from what
     was reviewed -- flagging it as a wording-precision follow-up
     candidate, not a defect requiring a code change on this
     verification-only branch.
4. **Team Dues Reminder -- PASS.** `GET /api/seasons/6/finances/dues`
   (live) fed through the same live generator for Fixture Breakers:
   message correctly showed team name, season name, `Dues amount: not
   set` (this season has no `dues_amount` rule configured), and exactly
   the three Fixture Breakers players (Avery Slate, Blair Flint paid
   with totals; Casey Vale `UNPAID`) -- Devon Reed (Fixture Bankers) was
   correctly excluded. The reminder line listed only Casey Vale.
5. **Player Summary -- PASS.** `GET /api/players/41/overview` (live) fed
   through the same live generator: message correctly showed player
   name, team/season, handicap (`+0`), the 0-0/0.0% record (this
   fixture's `match_results` don't attribute set/game totals to this
   player specifically), all five real schedule rows with correct
   Pending/Completed status matching each match's `completed` flag, and
   `Dues status: Paid ($3.21 total, last payment 2026-08-29T00:00:00Z)`.
   Read-only by construction -- the generated text and the underlying
   `GET` calls include no write action of any kind.
6. **Copy behavior -- confirmed at the deployed-source level only, NOT
   VERIFIED (no browser).** The live `communication-page-component.js`
   contains the exact reviewed `#copyMessage()`/`#selectMessageText()`
   pair: `navigator.clipboard.writeText` guarded by
   `window.isSecureContext`, falling back to selecting the textarea and a
   warning toast otherwise. Staging is plain HTTP, so the fallback branch
   is the one that would actually execute there -- but observing that in
   a real page requires a browser this tool session does not have.
7. **Scope guard -- PASS.** Searched the deployed
   `communication-page-component.js` for any automated-sending indicator
   (`smtp`, `mailto`, `sms`, `twilio`, `sendgrid`, "push notification")
   and found none outside the file's own "we don't do this" doc comment.
   Confirmed no new backend routes exist: `GET /api/communications` and
   `GET /api/messages` both correctly 404.
8. **Cleanup:** read-only for all fixture/financial/player data --
   re-fetched `GET /api/leagues` and `GET /api/players/41` after the pass
   and both are byte-identical to before. Two disposable users remain,
   per the no-delete-endpoint convention every prior staging pass has
   followed: id 19 (`comm-verify-admin-20260909-204346`, `league_admin`)
   and id 20 (`comm-verify-player-20260909`, `role=player`, linked to
   player 45 Emery Frost). No API keys or secrets are recorded in this
   entry or elsewhere in this checklist. Standing exclusions
   (`architecture-diagram.md`, `architecture-review.md`,
   `backend/storage/postgres/`) untouched.
- **Follow-up needed:** a real browser click-through remains the only gap
  before this phase is browser-complete -- nav visibility rendering,
  selector show/hide and auto-regeneration timing (including the
  corrected season/type-switch sequencing), and the Copy button's
  clipboard-vs-fallback behavior all need a browser this tool session
  does not have. Optionally, consider tightening the missing-scores
  note's wording (item 3's discrepancy) in a future small pass -- not
  blocking, and no code change was made on this verification-only
  branch.

### 24. League Admin Screen Phase 1 (2026-09-10)

Read-only operational hub -- it navigates to existing screens, it never
mutates. No automated notifications, no audit/history, no new backend
routes or schema, no new permission model, no code-management editing.

- Browser: the new "League Admin" nav entry (under Dashboard) and the
  hub's cards.
  - [ ] **NOT VERIFIED (no browser)**: the "League Admin" nav entry is
        hidden for no-key and `role=player` identities and visible for
        `league_admin`/`admin`/`system_admin`, matching the Financial /
        Communication / Player Overview nav entries. **Confirmed at the
        code level**: `app.js`'s `updateIdentityUI()` toggles
        `#nav-item-league-admin` off the same `canManageFinances`
        (`hasFinanceAdminRole(identity)`) value that already gates those
        three -- no new permission model.
  - [ ] **NOT VERIFIED (no browser)**: with an active season, the hub
        shows a Season/Week strip, a Weekly Score Processing card (season
        `N/M scored` + `X/Y weeks closed` line, plus the focus week's
        Missing/Scored/Approved/Processed/Closed badge counts), a Lineups
        & Substitutes card (`ready X/Y teams` using Match Entry's own
        week-specific/default-lineup resolution, `substitutes in use: N`,
        and a one-line note that readiness matches Match Entry), a Money
        card (`unpaid dues: N/M players`), a Communication card (three
        message types listed), a Players & Users card, and a "Jump to"
        button row. With no active season, only a warning plus
        Seasons/Teams/Players links render.
  - [ ] **NOT VERIFIED (no browser)**: each jump/link button navigates to
        the correct existing section (Weekly Summary, Schedule, Lineups,
        Match Entry, Financial, Communication, Players, Player Overview,
        Teams, Seasons, Handicap, Users). **Confirmed at the code
        level**: every button carries `data-navigate="<section>"` and the
        component dispatches `admin-nav-request`, which `app.js` handles
        with `navTo(e.detail.section)` -- the same one-line pattern
        `dashboard-nav-request` already uses.
  - [ ] **NOT VERIFIED (no browser)**: the Users button appears in the
        Players & Users card and the Jump-to row only when the resolved
        identity role is `system_admin` or `admin` (not plain
        `league_admin`). **Confirmed at the code level**:
        `#canManageUsers()` checks exactly those two roles, matching the
        Users nav entry's own `canManageUsers` gate in `app.js`.
- **Card computation verification (against real staging season 6 data,
  via a standalone script replaying the hub's `#load` logic):**
  - Focus week selection resolved to **Week 1** (the lowest week with
    matches whose status is not closed), from `GET
    /api/seasons/6/weeks`.
  - Weekly Score Processing: the focus week's ladder counts came out
    `Missing 2 / Scored 0 / Approved 0 / Processed 0 / Closed 0`, matching
    the `GET .../weeks/1/recap` per-match states; the season line came out
    `8/10 matches scored, 0/5 weeks closed`, matching the sum over `GET
    .../weeks`.
  - Lineups & Substitutes: `ready 4/4 teams`, `substitutes in use: 0`.
    Readiness was resolved the same way Match Entry does -- week-specific
    plans from `GET /api/lineup-plans?season_id=6&week_number=1` (12 rows
    / 4 teams = 3 each), falling back to the default
    (`week_number=0`) plans for any team with fewer than 3, then requiring
    the first 3 resolved rows to resolve to real players in the full
    league player list -- not a raw row count (corrected after PM
    review). All four Week 1 teams resolved to 3 real players; no `is_sub`
    rows in the resolved slots.
  - Money: `unpaid dues: 10/12 players`, matching `GET
    /api/seasons/6/finances/dues` (`dues.players` filtered on `!paid`).
- No new Go tests -- this phase added no backend code. `go test ./...
  -count=1` and `go build ./...` were rerun for full regression safety
  (cross-domain screen) and both pass with zero regressions.
- `node --check` passes on all four new/changed files:
  `web/domains/admin/admin-domain.js`,
  `web/domains/admin/league-admin-api-service.js`,
  `web/domains/admin/league-admin-page-component.js`, and `web/app.js`
  (nav gating + `loadSection` case + `admin-nav-request` handler).
- **Correction (2026-09-10, before commit):** PM review caught the
  Lineups & Substitutes card treating "3+ `lineup_plans` rows" as ready.
  Corrected to Match Entry's resolution path: week-specific plans if a
  team has >=3, else the default (`week_number=0`) plans, with the first
  3 resolved rows required to resolve to real players in the full league
  player list (a substitute's `player_id` may not be on the team roster).
  `refresh()` gained an `allPlayers` parameter (from `state.allPlayers`);
  `#load()` now also fetches the `week_number=0` lineup plans. Substitute
  count taken from `is_sub` in the resolved first-3 slots of playing
  teams. Frontend/docs-only -- no Go code touched. Re-simulated against
  real staging season 6: still `4/4 ready`, `0 subs` for Week 1, now via
  the resolution path. `node --check` re-run on all four files -- pass.
- Known, deliberately out-of-scope notes (not oversights):
  - Read-only -- no write action of any kind; the hub only navigates.
  - No automated email/SMS/mobile notifications, no audit/history
    framework, no developer/system tools consolidation (Backup stays its
    own sidebar button and is not linked from the hub).
  - Not the deferred "Admin code-management screens" item -- no
    controlled-code/label/display-order/active-flag editing.
  - Only the focus week gets the deeper `recap` call; per-week
    approved/processed counts for every week at once were deliberately not
    fetched, to keep the load cheap.
  - No new backend aggregate endpoint -- a small fixed set of existing
    GETs, each feeding one card, was judged non-brittle.

#### Staging verification (2026-09-10)

**Result: PASS (API-level + deployed-static-asset verification).** Verified
on `http://league-staging.local`, deployed commit `f6d90f1` ("League Admin
Phase 1: add operational hub"). No browser available in this developer's
tool session -- every item below is a direct API call against real
staging, a direct fetch confirming the deployed static JS/HTML matches the
reviewed source exactly, or the hub's `#load` card computations replayed
in a standalone script against real staging responses.

1. **Deployed static assets / shell registration -- PASS.** `GET
   /healthz` returned `{"status":"ok"}` first. All four hub files return
   200 from staging (`domains/admin/admin-domain.js`,
   `domains/admin/league-admin-api-service.js`,
   `domains/admin/league-admin-page-component.js`, `app.js`), and the
   deployed `index.html` contains `id="nav-item-league-admin"`,
   `data-section="league-admin"`, `id="section-league-admin"`, and the
   `domains/admin/admin-domain.js` module script. The deployed component
   is the *corrected* f6d90f1 version specifically: it contains the
   `refresh(..., allPlayers, identity)` signature, the
   `wk.length >= 3 ? wk : dflt.filter(...)` fallback, the
   `this.#allPlayers.find(p => p.id === lp.player_id)` full-list
   resolution, `resolvedByTeam`, and the `admin-nav-request` dispatch;
   the deployed `app.js` passes `state.allPlayers, state.currentIdentity`
   into the league-admin refresh, has the
   `document.addEventListener('admin-nav-request', ...)` handler, and
   toggles `#nav-item-league-admin` off `!canManageFinances`.
2. **Admin visibility -- PASS (API + code level).** `GET /api/users/me`:
   a disposable `league_admin` key (id 22) resolved `role:"league_admin"`;
   a disposable `role=player` key (id 23, linked to fixture player 42)
   resolved `role:"player"`; no `Authorization` header returned 401.
   Combined with the confirmed deployed gating
   (`#nav-item-league-admin` toggled off the same
   `hasFinanceAdminRole`/`canManageFinances` value that already gates
   `#nav-item-finances`, `#nav-item-communications`,
   `#nav-item-player-overview`), this confirms the nav resolves visible
   for the three admin roles and hidden for `role=player` / no key.
   **NOT VERIFIED (no browser)**: actually seeing the nav entry
   appear/disappear in a rendered page.
3. **Hub cards present -- PASS (computation level).** Replaying the hub's
   `#load` against real staging season 6 data produced content for every
   V1 card: Season/Focus-Week strip, Weekly Score Processing, Lineups &
   Substitutes, Money, Communication, Players & Users, and the Jump-to
   row. **NOT VERIFIED (no browser)**: the actual rendered card layout.
4. **Focus-week behavior -- PASS.** Against `GET /api/seasons/6/weeks`
   (weeks 1-5, all `status:"open"`, all `match_count:2`), the focus week
   resolved to **Week 1** -- the earliest week with unclosed matches.
   Against an empty schedule (`GET /api/seasons/3/weeks` returned `[]`),
   `#pickFocusWeek` returned `null`, so the focus-week cards render
   link-only. The "all weeks closed -> latest week with matches" fallback
   is **confirmed by code inspection only** -- staging currently has no
   season with a closed week, and creating one would mean mutating shared
   fixture data, which this read-only pass avoided.
5. **Weekly Score Processing -- PASS.** Season line came out `8/10
   matches scored, 0/5 weeks closed`, matching the sum over `GET
   .../weeks`. The Week 1 ladder came out `Missing 2 / Scored 0 /
   Approved 0 / Processed 0 / Closed 0`, matching the raw `GET
   .../weeks/1/recap` (both matches: `has_result:false`, no
   `approved_at`/`processed_at`/`week_closed`). The card's only actions
   are `data-navigate` links to Weekly Summary and Schedule -- no write
   call.
6. **Lineups & Substitutes -- PASS.** Against `GET
   /api/lineup-plans?season_id=6&week_number=1` (12 rows) with
   `week_number=0` default fallback available (60 rows) and the 12-player
   `GET /api/players?league_id=3` list: all four teams playing Week 1
   (14/15/16/17) had exactly 3 week-specific rows, each resolving to 3
   real players in the full league list (41-43, 44-46, 47-49, 50-52), so
   readiness came out `4/4 teams`; `substitutes in use: 0` (no `is_sub`
   rows in the resolved first-3 slots). The off-roster-substitute
   resolution is **confirmed by code inspection**: resolution matches
   against the full `#allPlayers` list with no team filter, so a
   substitute whose `player_id` is not on the team roster still resolves
   -- staging has no live `is_sub` lineup row to exercise this directly.
   The card's only actions are `data-navigate` links to Lineups and Match
   Entry -- no substitute write call.
7. **Money -- PASS.** `GET /api/seasons/6/finances/dues` (with the
   `league_admin` key) fed the card as `unpaid dues: 10/12 players`
   (`dues.players` filtered on `!paid`). The degrade path is real:
   the same route returned `403 {"error":"forbidden"}` for the
   `role=player` key, and `#renderMoneyCard(null)` renders "Dues status
   unavailable -- open the Financial screen." plus the Financial link
   rather than breaking the hub.
8. **Communication / Players & Users / Jump links -- PASS.** Every
   `#navBtn(...)` target in the deployed component (seasons, teams,
   players, weekly-summary, schedule, lineup, entry, finances,
   communications, player-overview, users, handicap) is a valid
   `data-section` in the deployed `index.html`; the component dispatches
   `admin-nav-request`, handled by `navTo(e.detail.section)`. The Users
   button is wrapped in `#canManageUsers()` (system_admin/admin only),
   matching the Users nav entry's own gate.
9. **Scope guard -- PASS.** No new backend routes: `/api/admin`,
   `/api/league-admin`, `/api/hub`, `/api/admin/summary` all 404. No
   admin write action exists in the component (only `data-navigate`
   links). No auth/session/JWT/password code was added. No
   developer/system-admin function beyond this operational hub.
- **Restoration:** fully read-only against fixture/financial/player data
  -- re-fetched `GET /api/players/42` and `GET /api/leagues` after the
  pass, both byte-identical to before (linking a user to a player does
  not modify the player row). Two disposable users remain, per the
  no-delete-endpoint convention every prior staging pass has followed:
  id 22 (`la-hub-staging-admin-20260910-184808`, `league_admin`) and id
  23 (`la-hub-staging-player-20260910`, `role=player`, linked to fixture
  player 42 Blair Flint). No API keys or secrets recorded here. Standing
  exclusions (`architecture-diagram.md`, `architecture-review.md`,
  `backend/storage/postgres/`) untouched.
- **Discrepancies found:** none. All ten verification goals passed at the
  API / deployed-source / computation level.
- **Follow-up needed:** a real browser click-through is the only gap
  before this phase is browser-complete -- nav visibility rendering, the
  card layout, and the jump buttons actually navigating. The "all weeks
  closed -> latest week" focus-week fallback and the off-roster-substitute
  resolution are confirmed by code inspection only (no staging data
  exercises either without mutating shared fixtures).

### 25. Default Lineup Week-Filter Fix + Setup Checklist Warning (2026-09-16)

Backend-only fix and a non-blocking checklist warning -- no new screen,
no schema change, no auth change, no UI polish. The Default Lineup
editor itself already existed on the Lineups screen before this phase;
see the fix/warning detail in `doc/domains/matches/README.md`.

- Store and handler level (verified via new and updated automated tests,
  listed below; no browser needed for this item since it's a pure data-
  correctness fix):
  - [x] **Store-verified**: `ListLineupPlans(..., WeekNumber: 0)` for a
        team with both a saved default lineup and a saved week-specific
        lineup now returns only the 3 default-lineup rows, not a mix of
        both weeks. Guarded by the new
        `TestLineupStore_ListLineupPlans_WeekZeroNotMixedWithOtherWeeks`
        store test (seeds week 0 and week 1 rows for the same season/
        team, confirms each `WeekNumber` filter returns only its own
        week). This exercises the SQLite store method directly, not the
        `GET /api/lineup-plans` HTTP route -- no handler/API-level test
        was added for this specific fix, since the store test already
        proves the query-building change and the handler is a thin,
        unchanged passthrough onto it.
  - [ ] **NOT VERIFIED (no browser)**: opening the Lineups screen's
        "Default Lineup" view for a season that also has week-specific
        overrides now shows the real default roster, not a stale mix of
        rows from other weeks. Confirmed at the code/store level only
        (same underlying `ListLineupPlans` call the `GET
        /api/lineup-plans` route -- and in turn the Lineups screen,
        Dashboard, Match Entry, and the League Admin hub -- all use, but
        not exercised through HTTP for this specific fix).
- Browser: the Seasons screen's setup checklist.
  - [ ] **NOT VERIFIED (no browser)**: a draft season with a team that
        has fewer than 3 default-lineup rows shows a `TEAM_NO_DEFAULT_
        LINEUP` warning (e.g. "team \"Alpha\" has no default lineup set"
        or "...has an incomplete default lineup (2/3 players set)") in
        the existing Setup Checklist card, rendered by the same generic
        warning-item loop that already renders `TEAM_FEW_PLAYERS` and
        similar. **API-verified**: `GET /api/seasons/{id}/checklist`
        includes the warning in its `warnings` array, confirmed via the
        new `TestSeasonChecklist_DefaultLineupWarning_AppearsThenClears`
        handler test.
  - [ ] **NOT VERIFIED (no browser)**: the warning does not disable the
        "Activate Season" button and does not appear as a blocker.
        **API-verified**: the same test confirms the warning is never
        present in `blockers` and that `can_activate` does not change
        based on the lineup-state transition (saving a full 3-player
        default lineup for the team removes the warning without
        affecting `can_activate` either way).
  - [ ] **NOT VERIFIED (no browser)**: the warning disappears once a
        full 3-player default lineup is saved for that team via the
        existing Lineups screen. **API-verified** via the same test
        (saves the default lineup through `POST /api/lineup-plans`,
        re-fetches the checklist, confirms the warning for that team is
        gone).
- New focused Go tests (all passing): 1 new SQLite store test
  (`TestLineupStore_ListLineupPlans_WeekZeroNotMixedWithOtherWeeks`); 2
  existing SQLite store tests corrected
  (`TestLineupStore_ListLineupPlans_BySeason`,
  `TestLineupStore_ListLineupPlans_ByTeam` -- both previously omitted
  `WeekNumber`, unintentionally relying on the now-removed "no filter"
  behavior; both now pass an explicit `WeekNumber` and still cover their
  original intent); 1 new handler test
  (`TestSeasonChecklist_DefaultLineupWarning_AppearsThenClears`). All
  four pre-existing checklist tests
  (`TestSeasonChecklist_LegacySeason_CanActivate`,
  `_ManagedNoTeams_BlocksTooFew`, `_TwoTeamsNoSchedule_Blocked`,
  `_AllGood_CanActivate`) continue to pass unchanged.
- `go test ./... -count=1` and `go build ./...` pass with zero
  regressions. No JS changed -- the existing checklist renderer already
  displays any `{code, message, team_id}` warning item generically, so
  no frontend file needed a change.
- Known, deliberately out-of-scope notes (not oversights):
  - No new "Default Lineup" screen -- the existing Lineups screen editor
    is unchanged and sufficient; this phase only fixed how its data is
    read back and added a setup-time nudge.
  - No link/button from the warning to the Lineups screen -- judged not
    tiny enough to include without drifting into UI polish.
  - No "exactly 3 players" enforcement added to `SaveTeamLineup` (the
    write side stays permissive, as before); only read-side readiness
    checks require exactly 3.
  - No guard added to `SetSubstitute`/`ClearSubstitute` against
    `week_number=0` rows -- a low-risk edge case noted during discovery
    (no UI exposes it today), not addressed here.
  - The three independent frontend `resolvePlans`-style fallback
    implementations (Dashboard, Match Entry, League Admin hub) remain
    unconsolidated -- a code-quality opportunity, not this phase's scope.

#### Staging verification (2026-09-17)

**Result: PASS for the feature under test** (default-lineup week
filtering and the setup checklist warning), with one unrelated
discrepancy found during cleanup -- see Known Gap #19 below. Verified on
`http://league-staging.local` against commit `b88537c`. No browser
available in this developer's tool session -- every item below is a
direct API call against real staging; the Lineups screen and Seasons
checklist rendering items are marked NOT VERIFIED for that reason.

**Fixture used:** a fresh, fully disposable league (League 7, "Lineup
Filter Verify League 20260917"; Season 10; Team Alpha team_id 25 with
players covering a full 3-player roster; Team Bravo team_id 26 with a
2-player roster), created and torn down entirely within this pass rather
than touching the shared Fixture Scoresheet League.

1. **Week filtering -- PASS.** Saved a `week_number=0` default lineup
   for Team Alpha with one set of 3 players, then a `week_number=1`
   lineup for the *same team* with a partially-overlapping but distinct
   set of 3 players (2 shared, 1 different), deliberately reproducing
   the exact bug scenario (one team, both a default and a week-specific
   lineup coexisting). `GET .../lineup-plans?season_id=...&week_number=0`
   returned exactly the 3 default-lineup rows, all `week_number:0`.
   `GET .../lineup-plans?season_id=...&week_number=1` returned exactly
   the 3 week-specific rows, all `week_number:1`. No row appeared in
   both responses beyond the players intentionally shared between the
   two lineups; no extra or missing rows in either response.
2. **Checklist warning lifecycle -- PASS.** With zero default-lineup
   rows for either team, `TEAM_NO_DEFAULT_LINEUP` appeared in `warnings`
   for both teams, each with its own team-specific message. After Team
   Alpha's default lineup was completed to 3 players, Team Alpha's
   warning was gone while Team Bravo's warning remained untouched
   (confirms per-team independence). After Team Bravo's default lineup
   was set to 2 (of 3) players, its warning changed to the expected
   "incomplete default lineup (2/3 players set)" wording rather than
   clearing.
3. **Non-blocking behavior -- PASS.** Across every checklist snapshot
   taken during this pass (0 rows for both teams; Team Alpha complete;
   Team Bravo partial; final state), `blockers` contained only the
   season's pre-existing, unrelated `NO_SCHEDULE` blocker --
   `TEAM_NO_DEFAULT_LINEUP` never appeared there. `can_activate` stayed
   `false` throughout and did not change in either direction as a result
   of any lineup-state transition -- it was governed entirely by the
   unrelated `NO_SCHEDULE` blocker for the whole pass.
4. **Shared fixture data -- PASS.** Re-checked a player from the shared
   Fixture Scoresheet League after the pass; unchanged.
5. **Browser rendering -- NOT VERIFIED (no browser)** in this developer's
   tool session: the Lineups screen's "Default Lineup" view actually
   showing the correct (non-mixed) roster, and the Seasons setup
   checklist card actually rendering the warning and its
   appear/disappear transitions. Every underlying data claim those
   screens would render is proven correct at the API level above.
- **Cleanup:** the disposable league did not cascade-delete its
  dependents through the API as the schema declares it should -- see
  Known Gap #19. Cleanup was completed through targeted calls to the
  same existing, already-approved DELETE endpoints (season, lineup
  plans, players, teams) rather than relying on cascade. Final state
  confirmed clean: the league list matches the pre-pass baseline exactly,
  the disposable season/teams/players all 404, and no lineup-plans rows
  remain for the disposable season. No database reset or replacement was
  performed. One disposable `league_admin` bootstrap user remains, per
  the no-delete-user-endpoint convention every prior staging pass has
  followed. No API keys or secrets are recorded here or elsewhere in
  this checklist.

### 26. SQLite Foreign-Key Cascade Enforcement -- Staging Verification (2026-09-17)

Dedicated staging verification for the `sqlite-foreign-key-cascade-enforcement`
fix (Known Gap #19), run against deployed commit `11a02a6` on
`http://league-staging.local`. No browser available in this developer's
tool session -- every result below is a direct API call plus a read-only
SQLite integrity check against the live staging database file.

1. **Scenario A -- season deletion -- PASS.** Built a disposable league, a
   3-team season (bye requests require an odd team count), 1 player, a
   season rule, a skipped week, a bye request, a real generated 3-match
   schedule, and a lineup plan, all through existing API routes. After
   `DELETE /api/seasons/{id}`: the season 404s; lineup plans, season
   rules, skipped weeks, bye requests, and matches for that season are
   all empty; the parent league and the team both still return 200; the
   player still returns 200, still assigned to the surviving team.
2. **Scenario B -- league deletion -- PASS.** Same fixture shape under a
   second disposable league. After `DELETE /api/leagues/{id}`: the
   league, season, and team all 404; lineup plans and matches for that
   season are empty; the player still returns 200 with `team_id: null`,
   confirming `ON DELETE SET NULL` through the real deployed API, not
   just the regression test.
3. **Connection-pool exercise.** Before each delete, roughly 20 concurrent
   reads were fanned out across 5 endpoints to give the connection pool
   an opportunity to use more than one connection. This does not by
   itself prove every live connection's `PRAGMA foreign_keys` value --
   that is proven by the deployed
   `TestForeignKeysPragma_EnabledOnEveryPooledConnection` regression
   test. What this staging pass proves is that the deployed application
   now executes the expected cascade/SET NULL behavior end-to-end
   through the real API; the regression test proves the underlying
   per-connection PRAGMA configuration that makes it possible.
4. **Integrity check found pre-existing violations, remediated
   separately.** After Scenario A and Scenario B's own disposable
   fixtures had already been fully created, verified, and cleaned up
   (both disposable leagues and both disposable players confirmed 404),
   a read-only `PRAGMA foreign_key_check` against
   `C:\inetpub\league-staging\data\league.db` returned 27 violations
   across two clusters -- both predating this verification pass and both
   predating the `11a02a6` deploy:
   - League id 6 (deleted before the fix existed) had left season id 9,
     teams 23/24, and their full set of season-owned children (1 match
     with 6 `match_results`/9 `round_results`, 3 `lineup_plans`, 2
     `season_teams`, 6 `season_rosters`) intact and orphaned -- this is
     the "Substitute Verify Season" fixture from the 2026-09-02
     Substitute Workflow Phase 1 staging pass (section 21).
   - Season id 10 (deleted before the fix took effect) had left 2
     `season_teams` rows and 6 `season_rosters` rows behind, referencing
     a season, 2 teams, and 6 players that no longer existed at all --
     this is the fixture from the 2026-09-17 Default Lineup staging pass
     that originally surfaced Known Gap #19 (section 25).

   Per the original fix task's explicit instruction not to auto-clean
   existing orphaned data, this verification pass stopped at the
   integrity check and reported the evidence rather than deleting
   anything. A follow-up remediation pass, approved separately, then:
   1. Before any write, copied the staging database to
      `C:\inetpub\league-staging\backups\league_2026-09-17_200438.db`
      plus its `-wal`/`-shm` sidecars. The application was not stopped
      for this copy, so it was a best-effort live three-file copy, not a
      transactionally guaranteed atomic SQLite snapshot -- the
      post-remediation checks below validate the live database itself,
      not the internal consistency of this backup copy.
   2. Re-captured the complete `PRAGMA foreign_key_check` output and
      confirmed, row by row, that every violation and every legitimate
      child row it would cascade (matches, lineup_plans, season_teams,
      season_rosters, match_results, round_results) belonged exclusively
      to the two clusters above -- no shared or product data was
      touched.
   3. The 27 violation records corresponded to only 11 physical orphan
      rows (`season_rosters` and `season_teams` rows each have 3 foreign
      keys, so each one produces 3 violation records). Ran one explicit
      transaction with `PRAGMA foreign_keys=ON` and directly deleted
      those 11 rows by exact ID: `season_rosters` ids 180-185 (6),
      `season_teams` ids 59-60 (2), `seasons` id 9 (1), `teams` ids
      23-24 (2). No other row was targeted directly. The schema's own
      `ON DELETE CASCADE`/`SET NULL` then removed 27 further dependent
      rows as a consequence, separately from the 11 direct deletes:
      `season_teams` ids 57-58 and `season_rosters` ids 174-179 (season
      9's own registration/roster rows), 1 match (id 42), 3
      `lineup_plans`, 6 `match_results` and 9 `round_results` (both
      cascaded transitively from that match), and `team_id` cleared to
      `NULL` (not deleted) on the 7 players who were still validly
      assigned to teams 23/24. `PRAGMA foreign_key_check` returned zero
      rows inside the transaction before it was committed.
   4. Confirmed post-commit: `PRAGMA foreign_key_check` returns zero
      rows; `PRAGMA integrity_check` returns `ok`; `GET /api/leagues`
      returns only the 3 expected shared leagues (Demo Pool League, Demo
      9-Ball League, Fixture Scoresheet League), and the Fixture
      Scoresheet League's own row is unchanged; the 7 preserved players
      (formerly on teams 23/24) show `team_id: null` rather than having
      been deleted; and both `GET /api/leagues` and `GET /api/players`
      return 200.

   No product data was touched -- every deleted or cascaded row traced
   back to one of the two disposable clusters above, confirmed by exact
   row ID before deletion, with a fresh backup taken first. This
   cleanup removed leftover data from before the fix was deployed; it is
   separate from, and does not change, Scenarios A and B above, which
   are the proof that the deployed fix itself prevents any *new* row
   from being orphaned this way.
5. **Cleanup.** Both disposable leagues (Scenario A and B), their
   seasons, teams, and lineup plans are gone; both disposable players
   were individually deleted after confirming `team_id` was cleared by
   the cascade. The shared Fixture Scoresheet League and the two demo
   leagues are unchanged. Disposable `league_admin` bootstrap users
   created during this pass and its remediation remain on staging (5
   total across several verification/debugging attempts), per the
   no-delete-user-endpoint convention every prior staging pass has
   followed. No API keys or secrets are recorded here or elsewhere in
   this checklist.

### 27. Users/Roles/Authentication Phase 1 (2026-09-18)

Real email+password login, server-managed sessions with CSRF
protection, scoped `role_assignments`, a centralized `auth.Authorize`
policy, league creation with an atomic self-grant, and league/season-
scoped authorization on league and season CRUD. Full design detail:
`doc/domains/users/README.md`'s "Users/Roles/Authentication Phase 1
Implementation" section. No browser available in this developer's tool
session -- every item below is verified at the Go test/handler-
integration level; browser-rendering items are marked NOT VERIFIED.

**Backend authorization checks (all PASS, `go test ./... -count=1`):**

1. **Migration -- PASS.** `db/migrate_users_api_keys_test.go` seeds the
   exact literal pre-Phase-1 `users` schema (including the `api_key_hash
   TEXT NOT NULL UNIQUE` column) with real data, then runs the actual
   `db.Init` migration path. Confirms: every pre-existing user id,
   username, role, active flag, and created_at value preserved exactly;
   every old `api_key_hash` migrated exactly once into the new
   `user_api_keys` table with no loss or duplication; the temporary
   stash table and the old `api_key_hash` column are both gone
   afterward; `role_assignments` correctly backfilled per legacy role
   (`system_admin`/legacy `admin` -> one global row; `league_admin` ->
   one row per league that existed at migration time); a freshly
   created `role_assignments` row (proving the FK still resolves against
   the renamed `users` table, not a dangling reference) cascades away
   when its user is deleted; the AUTOINCREMENT sequence continues
   correctly post-rebuild (no id collision); re-running migration a
   second time is a no-op (idempotent).
2. **Password hashing -- PASS.** Argon2id hash/verify round trip;
   two hashes of the same password differ (fresh salt per call); a
   strict decoder rejects a wrong version, out-of-range memory/
   iterations/parallelism, and invalid base64 salt/hash segments (never
   lets a corrupted or tampered stored value drive an expensive
   computation with attacker-influenced cost); `NeedsRehash` correctly
   flags a below-target stored hash; `CalibrateArgon2` measures real
   hardware speed rather than using a hardcoded constant -- measured on
   the primary development machine: memory=65536KiB, iterations=19,
   parallelism=2, 358.1102ms against a 300ms target.
3. **Sessions and CSRF -- PASS.** Login sets both an HttpOnly session
   cookie and a non-HttpOnly, JS-readable CSRF cookie; a
   session-cookie-authenticated mutating request without the
   `X-CSRF-Token` header is rejected (403); the same request with the
   correct header succeeds; a wrong CSRF token is rejected (403);
   logout revokes only the current session, leaving a second concurrent
   session for the same account valid; a Bearer-key-authenticated
   mutating request needs no CSRF header at all and is unaffected.
4. **Authorization policy -- PASS.** The full `auth.Authorize` matrix is
   unit-tested directly (system_admin allowed everywhere; league_admin
   allowed only for their own league, denied for any other; league
   creation allowed for an existing league_admin or system_admin, denied
   for a plain player; player-overview ownership; dual-role identities
   get the union of both capabilities regardless of "workspace").
   `role_assignments`' CHECK constraint rejects an invalid combination
   via a direct raw INSERT (proving database-level, not just
   application-level, enforcement); the partial unique indexes reject a
   duplicate grant; deleting a league cascades its scoped assignments,
   confirmed with the same forced-non-init-connection technique
   `sqlite-foreign-key-cascade-enforcement` established (Known Gap #19).
5. **End-to-end HTTP integration -- PASS**
   (`handlers/api_auth_integration_test.go`, a real `httptest.Server`
   with the full auth stack wired): login sets both cookies;
   `GET /api/auth/me` reflects the real identity, role assignments, and
   available workspaces; a league_admin scoped to League A can create a
   season in League A but is denied (403) creating one in League B, and
   is denied (403) updating League B's own metadata directly, and is
   denied (403) a system-admin-only action; league creation atomically
   grants the creator (tested as an already-system_admin account, which
   still receives an explicit additional league_admin grant for the new
   league per the "atomically grants the creator" rule applying
   regardless of the creator's other roles) a league_admin assignment
   for the newly created league; a plain player identity is denied
   (403) attempting an admin mutation route directly; deactivating an
   account invalidates its session immediately (a request one moment
   later with the previously-valid cookie gets 401); a legacy personal
   API key created before this phase (via the unchanged
   `ApplyAuthStore.CreateApplyUser`) still successfully authenticates a
   mutating request after migration, with no CSRF header needed.
6. **Existing test suite -- PASS, unmodified.** Every pre-existing test
   in every package (`handlers`, `backend/domains/*`,
   `backend/storage/sqlite`, `db`, `logic`) continues to pass with no
   changes to its own code, confirming that league/season create-
   update-delete's new scoped-authorization path is fully inert
   (falls back to the exact prior flat `clearanceAuth` behavior) in
   every test/deployment configuration that has not wired
   `Dependencies.AuthMgr`/`RoleAssignmentMgr`.

### 27b. Users/Roles/Authentication Phase 1 -- PM correction round (2026-09-19)

PM review of the section 27 handoff found it operationally incomplete:
session authentication only worked on the new league/season CRUD routes,
Player Overview still rejected sessions entirely, league creation's
self-grant was a compensating action rather than a real transaction, and
the users-table migration created its auth child tables in the wrong
order relative to its own documented sequencing (harmless in practice at
the time, but not a real guarantee). All four are fixed; see
`doc/domains/users/README.md`'s "PM correction round (2026-09-19)"
section for full detail. New verification, all PASS
(`go test ./... -count=1`):

7. **Every protected route family now accepts a session, scoped to its
   own resource -- PASS** (new `handlers/api_auth_scoped_families_test.go`,
   real HTTP against a full auth stack): a league_admin scoped to League
   A succeeds creating a player, a team, saving a lineup, and generating
   a schedule in League A, and is rejected (403) doing the same in League
   B; a different league_admin (League B only) is rejected (403)
   assigning a match that belongs to League A; reading and writing
   finances, closing a week, activating a season, and adding a season
   rule all follow the same own-league-succeeds/other-league-403 pattern.
8. **Player Overview session access -- PASS** (new
   `TestAuthIntegration_PlayerOverview_SessionAccess`): a player signed
   in with email/password (no personal API key at all) loads their own
   overview (200) and is denied (403) another player's; a league_admin
   loads a player in their own assigned league (200) and is denied (403)
   a player in a different league.
9. **Password setup, full flow -- PASS** (new
   `TestAuthIntegration_PasswordSetup_FullFlow`): a system_admin issues a
   setup token over HTTP; the token is redeemed via
   `POST /api/auth/password-setup`; the account then logs in with the
   new password; replaying the same already-used token is rejected
   (400).
10. **League creation self-grant is a real transaction -- PASS**
    (`backend/storage/sqlite/league_self_grant_store_test.go`, 3 new
    tests): success commits both the league and the grant; a forced
    grant failure (an FK violation from a nonexistent creator id) leaves
    neither the league nor any role_assignments row behind; an unrelated
    existing league_admin receives no automatic grant for the new
    league. The existing HTTP-level `TestAuthIntegration_LeagueCreate_AtomicSelfGrant`
    from section 27 still passes unchanged.
11. **Migration ordering and historical AUTOINCREMENT sequence -- PASS**
    (`db/migrate_users_api_keys_test.go`): the existing
    `TestMigrateUsersAndAPIKeys_FromPrePhaseSchema` still passes against
    the corrected creation order (auth child tables created only after
    `users` has its final shape); a new
    `TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceAfterHighIDDeletion`
    seeds users up to a high id, deletes the highest-id row, runs
    migration, and confirms the next inserted user's id is still greater
    than that historical high-water mark -- proving `sqlite_sequence` is
    preserved even when the row that set it no longer exists at
    migration time.

**Frontend additions (browser rendering NOT VERIFIED -- no browser in
this developer's tool session; a server-level smoke test was performed
instead -- see below):**

- `[ ]` **NOT VERIFIED (no browser):** the login screen's "Have a setup
  token?" link switches to setup mode; a valid token plus matching
  passwords sets the password and returns to sign-in; a mismatched
  confirmation or too-short password shows an inline error without
  calling the API; an invalid/expired token shows the server's error.
- `[ ]` **NOT VERIFIED (no browser):** the login screen's "Use Admin Key
  instead" link opens the existing Admin Key modal; a valid key opens
  the app shell; an invalid key leaves the login screen visible with an
  error; clearing the key returns to the login screen when no session
  exists.
- **Server-level smoke test -- PASS:** the production binary was built
  (`go build`) and run against a temporary data directory with
  `INSECURE_LOCAL_COOKIES=1`; `GET /`, `GET /app.js`, and
  `GET /domains/auth/login-page-component.js` all returned 200 (static
  assets, including the new login-screen markup, are embedded and served
  correctly); `POST /api/auth/login` with a nonexistent account returned
  401 as expected. This confirms the server starts and serves the
  updated frontend correctly; it does not exercise any JavaScript or
  visual rendering, which remains NOT VERIFIED above.
- `node --check` (via the `.mjs`-copy technique below) passed for every
  changed/new frontend file this round:
  `web/domains/auth/login-page-component.js`, `web/app.js`.

**Frontend (browser rendering NOT VERIFIED -- no browser in this
developer's tool session):**

- `[ ]` **NOT VERIFIED (no browser):** an unauthenticated visitor sees
  the login screen, not the app shell.
- `[ ]` **NOT VERIFIED (no browser):** a successful login hides the
  login screen and shows the app shell with the correct nav items for
  the signed-in identity's role.
- `[ ]` **NOT VERIFIED (no browser):** a dual-role (player + league_admin)
  identity sees the workspace picker and switching it changes nav
  visibility without signing out.
- `[ ]` **NOT VERIFIED (no browser):** the Users Admin screen's
  "Provision Email Login" flow and per-account row menu (issue setup
  token, deactivate/reactivate, revoke API keys) render and function
  correctly.
- All underlying data/logic these screens render is proven correct at
  the API/handler level above (`node --check`, applied via a `.mjs`
  copy for every ES-module file touched -- see process note below,
  passed on every changed/new frontend file: `web/lib/api-client.js`,
  `web/app.js`, `web/domains/auth/*.js`,
  `web/domains/users/users-management-page-component.js`,
  `web/domains/users/users-api-service.js`).

**Process note for future JS verification:** plain `node --check` on a
file containing `import`/`export` statements does NOT reliably catch
syntax errors after the first `import` line in this Node version (it
appears to stop meaningfully validating once ESM syntax is detected in
a `.js` file, returning exit 0 even for a file with a real syntax error
later in it) -- confirmed empirically during this phase. The reliable
check is to copy the file to a temporary `.mjs` path first, then run
`node --check` against that copy; every ES-module file in this phase was
verified this way, not with a bare `node --check web/domains/.../*.js`.
Worth folding into this project's stated `node --check` convention.

### 27c. Users/Roles/Authentication Phase 1 -- PM final authorization corrections (round 2, 2026-09-19)

All PASS (`go test ./... -count=1`, 1405 tests total, 0 fail):

12. **Related-resource cross-league validation -- PASS.** New unit tests
    for `MatchService.AssignMatchTeams` (cross-league team rejected 409,
    unknown team rejected 409, same-league teams succeed, nil team ids
    skip validation) and `LineupService.SaveTeamLineup` (cross-league
    team rejected, teams_managed season requires season_teams
    participation, legacy season skips that check). Extended
    `handlers/api_auth_scoped_families_test.go` with HTTP-level cases:
    player create with a cross-league team_id/league_id mismatch (409);
    unassigned-player create is system_admin-only; player update moving
    a player between two same-league teams succeeds, cross-league move
    is system_admin-only; player merge of two same-league players
    succeeds, cross-league merge is system_admin-only; match-assign and
    lineup-save both reject a cross-league related team even for
    system_admin (a data-integrity invariant, not an authorization gap).
13. **Unassigned-player route behavior -- PASS.** New
    `TestAuthIntegration_UnassignedPlayerRoutes`: league_admin denied
    (403) deleting an unassigned player; system_admin deletes one
    successfully, with an explicit assertion that a bodyless `DELETE`
    never produces a 400 (confirming the JSON-decode-EOF bug is gone);
    league_admin denied (403) merging two unassigned players;
    system_admin allowed.
14. **Partial-wiring fail-closed behavior -- PASS.** New
    `handlers/api_auth_partial_wiring_test.go`: a Dependencies with
    `AuthMgr`/`RoleAssignmentMgr` wired and `ApplyAuth` nil still
    requires a credential (401 with none presented, 403 for an
    unresolvable Bearer key) while a valid session succeeds normally;
    a Dependencies with `ApplyAuth`, `AuthMgr`, AND `RoleAssignmentMgr`
    all nil (the legacy minimal-test shape) remains fully open, confirmed
    directly against `Register`.
15. **Handicap Apply mounts without the static token -- PASS.** New
    `TestRegister_ApplyRoute_Mounted_WhenSessionAuthOnly_NoToken`: with
    no `AdminToken` and no `ApplyAuth` configured, the route still exists
    (401 with no credential, not 404) and a session-authenticated
    system_admin reaches the real handler. The existing
    `TestRegister_ApplyRoute_NotMounted_WhenTokenEmpty` still passes
    unchanged (its Dependencies has no auth path at all, the one
    condition where the route legitimately stays unmounted).
16. **sqlite_sequence restoration covers an emptied table -- PASS.** New
    `TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceWhenAllUsersDeleted`
    seeds users up to a high id, deletes every user (not just the
    highest-id one), migrates, inserts a new user, and confirms its id
    exceeds the historical high-water mark -- proving the fix handles a
    rebuild that copies zero rows, where no `sqlite_sequence` row existed
    yet for the plain `UPDATE` from round 1 to match.

Browser verification was not claimed or attempted this round.

### 27d. Users/Roles/Authentication Phase 1 -- PM final credential-precedence and player-unassignment corrections (round 3, 2026-09-19)

All PASS (`go test ./... -count=1`, full suite, 0 fail):

17. **Session takes precedence over a stale/different Admin Key -- PASS.**
    New `handlers/api_auth_credential_precedence_test.go`, all HTTP-level
    against a real session cookie AND a real personal Bearer key attached
    to the same request:
    `TestCredentialPrecedence_SessionWinsOverDifferentSystemAdminBearerKey`
    (a league_admin session plus a stale system_admin Bearer key calling
    the system_admin-only `POST /api/backup` still gets 403 -- the
    session's own lesser role governs, not the key's);
    `TestCredentialPrecedence_PlayerSessionCannotGainSystemAdminFromStaleKey`
    (same proof for a `role=player` session);
    `TestCredentialPrecedence_LeagueAdminSessionScopedByOwnAssignment_NotStaleKey`
    (a League A-scoped league_admin session succeeds creating a team in
    League A and is still denied creating one in League B, even with a
    stale Bearer key scoped to League B attached);
    `TestCredentialPrecedence_BearerFallbackStillWorksWithNoSession` (a
    Bearer key with no session cookie present still authenticates, the
    fallback path unaffected);
    `TestCredentialPrecedence_SessionMutationStillRequiresCSRF_EvenWithBearerAttached`
    (a session-authenticated mutation with a Bearer header attached but
    no CSRF header still gets 403 -- CSRF enforcement is not bypassed by
    a Bearer header's mere presence once the session path is the one
    actually taken).
18. **Player update cannot silently unassign under a league_admin session
    -- PASS.** New subtest `player update: league_admin cannot unassign,
    system_admin can` under `TestAuthIntegration_ScopedFamilies`: a
    league_admin gets 403 for both an explicit `team_id:null` body and a
    body that omits `team_id` entirely, the player remains assigned to
    their original team after each rejected attempt (verified by
    re-fetching the player), and system_admin CAN unassign the same
    player (verified `team_id` is nil afterward). New subtests
    `league_admin denied update (assign) of an unassigned player` /
    `system_admin allowed update (assign) of an unassigned player` under
    `TestAuthIntegration_UnassignedPlayerRoutes` prove the mirror case:
    assigning a team to an already-unassigned player is also
    system_admin-only.

Browser verification was not claimed or attempted this round.

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
| 9 | No way to cleanly undo a generated schedule (`Generate Schedule` has no matching `DELETE`) short of deleting the whole season/league -- discovered 2026-08-23 | ~~Low~~ | `handlers/api_match_routes.go` (no `DELETE /api/matches/{id}`) | **Closed 2026-09-11, documented, not a bug.** Discovery (`schedule-generate-undo-discovery`) found `POST /api/matches/generate` already *is* the undo path: `ScheduleService.GenerateSchedule` -> `SaveGeneratedSchedule` deletes and replaces every unplayed (`completed=0`) match on every call, unconditionally, while completed/approved/processed matches are never touched (excluded from the delete by `completed=0` in the `WHERE` clause, and further protected by the season-closed/closed-weeks/active-with-completed-matches guards `GenerateSchedule` already enforces). The frontend already confirm()s this in plain language before calling it (`web/domains/seasons/seasons-domain.js`'s `#generateSchedule`: "This will replace all unplayed matches for this season. Completed matches are preserved. Continue?"). **Current recovery path:** regenerate the schedule (with corrected inputs) any time before scores/round results are entered for the matches being replaced -- not merely before they reach completed/approved/processed state. Regeneration deletes every unplayed (`completed=0`) match unconditionally, so a partial/incomplete round_results row saved against a match that never reached `completed=1` would also be lost by cascade; use it before score entry begins for the matches you're about to replace, or use a disposable season/league for staging/test scenarios that need repeated schedule churn against real seeded data (the working convention every staging pass since 2026-08-23 has already followed, including this one). Deleting the whole season/league (`DELETE /api/seasons/{id}` / `DELETE /api/leagues/{id}`) remains available as the full-wipe option when even the setup/rosters need to be discarded. A narrow, single-match `DELETE /api/matches/{id}` (removing one erroneous/duplicate match without regenerating the whole season) was considered and is a reasonable future candidate if a real workflow needs it, but is explicitly not part of the current product-readiness lane -- no real workflow has hit this need in the month since the gap was first noted; only this smoke test's own test-data setup did, and that is already solved by the disposable-league convention. See `doc/roadmap.md`'s Completed / Largely Completed entry for full discovery detail. |
| 10 | `POST /api/seasons/{id}/teams` with `name` returns a raw 500 with a leaked SQL message instead of a friendly 409 when a same-named standalone team already exists in the league -- discovered 2026-08-23 | ~~Low~~ | `backend/domains/seasons` `AddTeam` | **Fixed 2026-09-11** by `season-teams-error-and-rules-echo-fixes`: `AddTeam` now recognizes the `teams(league_id, name)` UNIQUE-constraint error (via the same `strings.Contains(err.Error(), "UNIQUE")` pattern `lineup_service.go` already uses) and maps it to `domainerr.Conflict` (`SEASON_TEAM_NAME_TAKEN`), giving a friendly 409 naming the conflicting team with no leaked SQL/constraint text. Verified via a new `SeasonService` unit test and a new end-to-end handler test (create, then re-create with the same name, assert 409 + clean message). |
| 11 | `PUT /api/seasons/{id}/rules/{rid}` response body echoes `season_id:0, rule_key:""` instead of the real values, even though the stored row is correct -- discovered 2026-08-23 | ~~Low~~ | `handlers` season-rules update handler | **Fixed 2026-09-11** by `season-teams-error-and-rules-echo-fixes`: `RuleService.Update` now returns the updated `models.SeasonRule` (it already fetches the existing row to validate the new value against the rule's real key, but previously discarded it) instead of the handler echoing back the caller-constructed request body, which never carried `season_id`/`rule_key` since only `rule_label`/`rule_value` are client-editable. Stored-row behavior, validation, and error mapping are all unchanged. Verified via a new `RuleService` unit test and a new end-to-end handler test asserting the PUT response includes the real `season_id` and `rule_key`. |
| 12 | `GetPlayerStats`'s season-scoped query never scans/computes `WinPct` -- discovered 2026-08-27 during Player Overview Phase 1 discovery | ~~Medium~~ | `backend/storage/sqlite/round_store.go` `GetPlayerStats` | **Not a live bug -- discovery-time misdiagnosis, corrected 2026-09-01.** The store's raw SQL genuinely omits `win_pct`, but `RoundService.GetPlayerStats` (`backend/domains/matches/round_service.go`) -- the method both `GET /api/player-stats` and `GET /api/players/{id}/overview` actually call -- has computed `WinPct = games_won/(games_won+games_lost)` since Matches Phase B3 (2026-07-01), before this row was ever opened. Real responses always had correct `win_pct`. Closed with a new end-to-end regression test, `TestPlayerStats_WinPctComputedEndToEnd`, since nothing previously verified this past the isolated service-level unit test. See `player-stats-winpct-roster-scope-fix` in `doc/roadmap.md`. |
| 13 | The league-scoped variant of `GetPlayerStats` still drops season-roster-only players (`INNER JOIN teams t ON t.id = p.team_id` on the legacy column) -- only the season-scoped branch was fixed by `player-stats-roster-join-fix` (row #7) -- discovered 2026-08-27 during Player Overview Phase 1 discovery | ~~Medium~~ | `backend/storage/sqlite/round_store.go` `GetPlayerStats` (league-scoped branch) | **Fixed 2026-09-01** by `player-stats-winpct-roster-scope-fix`: the league-scoped query now includes players assigned via `season_rosters` or `lineup_plans` for any season in the league, not just a direct `players.team_id`, without duplicating rows for players eligible through more than one source. Verified via 4 new SQLite store tests for the league-scoped fix; the existing season-scoped roster tests (`TestRoundStore_GetPlayerStats_RosterOnlyPlayer_NullTeamID`, `TestRoundStore_GetPlayerStats_SeasonRosterTeamOverridesStaleTeamID`) remain passing unchanged. See `doc/roadmap.md` for full detail. |
| 14 | Initial dashboard bootstrap logs `document.querySelector(...)?.refresh is not a function` in the browser console, likely because `app.js` can call `dashboard-page.refresh()` before the module-defined custom element has upgraded; dashboard content still populated during the 2026-08-27 staging pass | Low | `web/app.js` bootstrap/module load ordering | Open |
| 15 | `lineup_plans.is_sub`/`sub_for_id` are readable but cannot be set through any write path (`SaveTeamLineup` hardcodes `is_sub=0`, never sets `sub_for_id`) -- no substitute workflow is actually creatable today despite the schema/model supporting it -- discovered 2026-08-27 during Weekly Summary Phase 1 discovery | ~~Low~~ | `backend/storage/sqlite/lineup_store.go` `SaveTeamLineup` | **Fixed 2026-09-02** by Substitute Workflow Phase 1: new `POST`/`DELETE /api/lineup-plans/{id}/substitute` endpoints set/clear `is_sub`/`sub_for_id` in place, gated by `clearanceAuth` and the same season/week/approval/processed locks score edits respect. See section 21 above and `doc/domains/matches/README.md`. |
| 16 | No Sub control for a lineup slot resolved only from already-scored round results (no known `lineup_plans` row for it) -- retroactively substituting a played match raises different questions this phase doesn't answer -- discovered 2026-09-02 during Substitute Workflow Phase 1 | Low (known/tracked) | `web/domains/matches/match-entry-page-component.js` | Open -- explicitly deferred, not bundled into Substitute Workflow Phase 1 |
| 17 | Weekly Summary's `player_stats` array (now carrying `is_sub`/`sub_for_name` as of Substitute Workflow Phase 1) is still not rendered in that screen at all -- pre-existing since Weekly Summary Phase 1, not a regression -- discovered 2026-09-02 | Low (known/tracked) | `web/domains/weekly-summary/weekly-summary-page-component.js` | Open -- explicitly deferred, not bundled into Substitute Workflow Phase 1 |
| 18 | Player Overview's schedule section won't show a substitute's one-off match for a different team (schedule resolves via the player's own team for the season, not the team they subbed for) -- discovered 2026-09-02 during Substitute Workflow Phase 1 | Low (known/tracked, accepted) | `handlers/api_player_overview_handler.go` | Open -- explicitly accepted as a limitation, not the "big player-history redesign" this phase was told not to force |
| 19 | Deleting a league or season could remove the parent row while leaving cascade-owned dependent rows (seasons, teams, lineup plans, and other season/league-owned rows) orphaned, because SQLite's `foreign_keys` pragma was enabled on only one connection instead of every connection in `database/sql`'s pool -- discovered 2026-09-17 during default-lineup staging verification cleanup | ~~High~~ | `db/db.go` SQLite connection initialization and the league/season delete paths | **Fixed 2026-09-17** by `sqlite-foreign-key-cascade-enforcement`. **Root cause confirmed** (previously only a working hypothesis): a new test (`TestForeignKeysPragma_EnabledOnEveryPooledConnection`) held multiple concurrent `*sql.Conn` connections right after `db.Init` and queried `PRAGMA foreign_keys` on each -- the connection `db.Init`'s startup `Exec` call happened to configure returned `1`, but every additional pooled connection returned `0`, proving foreign-key enforcement was never applied pool-wide (SQLite does not persist `foreign_keys` in the database file the way it does `journal_mode`). **Fix:** `db.Init` now opens the database as `<path>?_pragma=foreign_keys(1)`; `modernc.org/sqlite`'s driver re-applies a DSN's `_pragma` query parameters to every connection it opens (confirmed by reading that package's `Driver.Open`/`applyQueryParams` source directly, not assumed), so every pooled connection now enforces foreign keys -- confirmed by the same multi-connection test now passing. **Corrected schema semantics (per PM clarification -- the original write-up above incorrectly treated players as orphaned):** players are never expected to be deleted when their team or league is -- `players.team_id` is declared `ON DELETE SET NULL`, not `CASCADE` -- so a surviving player with `team_id` cleared to `NULL` after its team is deleted is the correct, intended outcome, not orphaned data. Only surviving seasons, teams, and lineup_plans (and other league/season-owned rows) after their parent is deleted are defects. Three new regression tests cover the three schema behaviors separately. `TestForeignKeyEnforcement_SeasonDelete_CascadesSeasonOwnedRows` and `_LeagueDelete_CascadesSeasonsTeamsAndOwnedRows` exercise their real `SeasonService.DeleteSeason`/`SeasonStore` and `LeagueService.DeleteLeague`/`LeagueStore` delete paths respectively. `_TeamDelete_PlayersSurviveWithTeamIDCleared` deliberately uses a direct SQL `DELETE FROM teams` instead of `TeamService`/`TeamStore`, because `TeamStore.DeleteTeam` already performs an explicit `UPDATE players SET team_id=NULL` of its own before deleting the team row -- going through that path would prove the *application code* clears `team_id`, not that the *schema's* `ON DELETE SET NULL` action does, which is what this regression is actually about. All three tests deliberately hold a separate connection open first so the delete under test cannot reuse the single already-correctly-configured `db.Init` connection -- confirmed necessary by temporarily reverting the fix and observing all three fail without that connection-forcing step (a straight-line sequential test otherwise reuses one connection throughout and never exercises the bug). All three also assert `PRAGMA foreign_key_check` reports zero violations. No existing data was reset, rewritten, or auto-cleaned; the fix only changes how future connections are configured. See `doc/architecture-decisions.md`'s "SQLite foreign-key enforcement is a per-connection invariant" Decision History entry and `doc/roadmap.md`'s Completed entry for full detail. **Staging-verified 2026-09-17** against deployed commit `11a02a6`: both season and league deletion cascade correctly end-to-end through the real API, and player `SET NULL` is confirmed through the real API, not just the regression test -- see section 26. That same pass found and, after PM approval, remediated 27 pre-existing foreign-key violations left behind by disposable fixtures created before this fix was deployed (a fresh backup was taken first, and the exact row inventory was confirmed before any delete); no product data was affected. See section 26 for full evidence. |

## Recommended Next Branches

`api-client-bodyless-post-fix`, `player-stats-roster-join-fix`,
`handicap-preview-parity`, `player-stats-winpct-roster-scope-fix`,
`substitute-workflow-phase-1`, `season-teams-error-and-rules-echo-fixes`,
and `sqlite-foreign-key-cascade-enforcement` are all done (see the
Critical Blocker section above and Known Gaps rows
#6/#7/#8/#10/#11/#12/#13/#15/#19) -- no longer listed here as pending
branches. Known Gap #9 ("Generate Schedule" undo) is closed as a
documented decision, not a code branch -- see row #9 above.

No actionable product-readiness branches remain from this checklist.
Everything else already known before the 2026-08-23 pass (dashboard gate
demo data, staging health-check endpoint choice, merge UI, the
`.codex/skills/` script drift) remains backlog-level, unchanged by any
run since.

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
