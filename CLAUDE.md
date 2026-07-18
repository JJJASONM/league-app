# League App — Claude Context

## Project Overview

League App is a Go + SQLite web application for managing pool (billiards) leagues.
It handles leagues, teams, players, seasons, schedules, scoresheets, lineups, standings,
and handicap calculations. The user is a solo developer building this for real-world
league operations. The primary game format is 8-ball.

## Architecture Status

Major backend and frontend extraction is complete. `handlers/api.go` delegates
all routes to domain services with no direct DB access. `web/app.js` is now
shell-level coordination: navigation, shared league/season context, and
cross-domain event wiring. Domain screens, API services, and components live
under `web/domains/`. The approved target is documented in:

- `AGENTS.md` - engineering and documentation conventions
- `doc/architecture-decisions.md` - cross-domain workflows and open questions
- `doc/domains/*/README.md` - domain-specific decisions
- `doc/erd.mermaid` - physical schema implemented today

Do not treat target tables or workflows as already implemented. Migrate one
domain at a time and preserve behavior during each architecture pass.

## Tech Stack

- **Language:** Go 1.22+, module `league_app`
- **HTTP:** `net/http` with Go 1.22 method-pattern routing (`GET /api/...`)
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGo required)
- **Frontend:** Bootstrap 5 SPA — no build step, no bundler, no transpilation
- **Embedding:** `//go:embed web` in `main.go` — the entire `web/` directory is embedded
  in the binary; adding a file to `web/` makes it auto-served with no registration needed
- **Testing:** standard `go test ./...`

## Project Structure

```
league_app/
  main.go                    — entry point, embed directive, server startup
  CLAUDE.md                  — this file
  db/
    db.go                    — Init(), migrate(), Backup(), Seed(); holds var DB *sql.DB
  models/
    models.go                — shared struct definitions, no logic
  logic/
    handicap.go              — authoritative handicap formula (CalcSpot, CalcSpotM)
    handicap_test.go         — handicap formula tests
  backend/
    domains/
      handicaps/             — handicap formula, review, apply service + store interfaces
      matches/               — scoresheet, week close/reopen, round/match/lineup/schedule services
      rules/                 — rule definitions, ValidateValue, RuleStore interface
      seasons/               — season CRUD, rosters, bye requests, skipped weeks
      leagues/               — league CRUD service
      players/               — player CRUD service
      teams/                 — team CRUD service
    storage/
      sqlite/                — SQLite adapter implementations for all domain store interfaces
  handlers/
    api.go                   — thin HTTP delegation layer; route registration
    deps.go                  — Dependencies struct; domain manager interfaces
  web/
    index.html               — app shell; loads lib/ utilities, app.js, and domain modules
    styles.css               — all CSS, including scoresheet, poster, rules, print media
    app.js                   — shell coordination: navigation, context sync, event wiring
    domains/
      dashboard/             — dashboard page component
      handicaps/             — handicap review/apply component, API service, code constants
      leagues/               — leagues modal, API service, game-format codes
      lineups/               — lineup planning page and components
      matches/               — match entry and scoresheet components
      players/               — players page, API service
      rules/                 — rules editor component and system rule definitions
      schedules/             — schedule page, week management
      seasons/               — seasons domain components
      standings/             — standings page
      teams/                 — teams page and components
    lib/
      api-client.js          — api() global: shared fetch wrapper
      app-context.js         — appContext: shell shared state (league/season/player/preselect)
      html-escape.js         — escapeHTML() global for XSS-safe HTML rendering
      ui-feedback.js         — toast(), openModal(), closeModal() globals
    components/
      date-display.js        — fmtDate(), fmtDateRange(), displayDate() — ES module exports
```

## Database Schema

The database file is `league.db` in the configured data directory (default `./data`).
All tables use `INTEGER PRIMARY KEY AUTOINCREMENT`. Foreign keys are ON with cascading deletes.
WAL journal mode and `synchronous=NORMAL` are set at startup.

### Core Tables

- **leagues** — id, name, game_format (`8ball`|`9ball`|`10ball`|`straight`), day_of_week, created_at
- **teams** — id, league_id, name, captain_id, team_number, created_at
- **players** — id, player_number (two-digit code, locked once set), first_name, last_name,
  phone, email, team_id, handicap (REAL), admin_hold (0/1), active (0/1), note, created_at
- **seasons** — id, league_id, name, start_date, end_date (computed), active (0/1),
  schedule_type (`single_rr`|`double_rr`|`split`|`custom`|`blanket`), num_weeks, created_at
- **season_rules** — id, season_id, rule_key, rule_label, rule_value (TEXT); UNIQUE(season_id, rule_key)
- **skipped_weeks** — id, season_id, skip_date (DATE), reason; UNIQUE(season_id, skip_date)
- **bye_requests** — id, season_id, team_id, week_number (0=TBD), reason, approved (0/1)
- **matches** — id, season_id, home_team_id, away_team_id, match_date, week_number,
  match_number, table_numbers, completed (0/1), created_at
- **match_results** — id, match_id, player_id, team_id, sets_won, sets_lost, games_won,
  games_lost, diff (REAL), created_at
- **handicap_history** — id, player_id, old_handicap, new_handicap, effective_date,
  admin_hold, note, created_at
- **round_results** — id, match_id, round_number (1–3), home_player_id, away_player_id,
  game1_home…game3_away (INTEGER 0–10), **plus snapshot columns** (see below), created_at;
  UNIQUE(match_id, round_number, home_player_id)
- **lineup_plans** — id, team_id, player_id, week_number, season_id, is_sub (0/1),
  sub_for_id; UNIQUE(team_id, week_number, season_id, player_id)

### Handicap Snapshot Columns (round_results)

Added via additive migration to preserve historical accuracy:
```
home_handicap_used  REAL     — home player's handicap at the time the match was played
away_handicap_used  REAL     — away player's handicap at the time
handicap_pts_used   INTEGER  — computed spot (balls) at save time
handicap_to         TEXT     — "home" | "away" | "" at save time
```
**Naming asymmetry:** The DB column is `handicap_to` but the Go struct field is
`HandicapToUsed *string` (JSON: `handicap_to_used`). It is selected as `rr.handicap_to`
and scanned into `&rr.HandicapToUsed`. `HandicapTo` (no "Used") is the computed field.

When reading, snapshot takes priority over current player handicap. Older rows without
snapshots fall back to recomputing from current handicap.

### Additive Migration Pattern

SQLite has no `ALTER TABLE … ADD COLUMN IF NOT EXISTS`. New columns are added in
`db.go`'s `additiveMigrations` slice. Errors are intentionally ignored (column already
exists on fresh DBs that ran the full schema):
```go
for _, stmt := range additiveMigrations {
    DB.Exec(stmt) // ignore error — column already exists on fresh DBs
}
```
**Always add new columns here, never to the `CREATE TABLE` block directly.**

## API Routes (handlers/api.go)

All routes registered by `handlers.Register(mux, dataDir, deps)`:

| Method | Path | Purpose |
|--------|------|---------|
| GET/POST | `/api/leagues` | list / create |
| GET/PUT/DELETE | `/api/leagues/{id}` | get / update / delete |
| GET/POST | `/api/players` | list (`?league_id=`) / create |
| GET/PUT/DELETE | `/api/players/{id}` | get / update / delete |
| GET/POST | `/api/teams` | list (`?league_id=`) / create |
| GET/PUT/DELETE | `/api/teams/{id}` | get / update / delete |
| GET/POST | `/api/seasons` | list (`?league_id=`) / create |
| GET/PUT/DELETE | `/api/seasons/{id}` | get / update / delete |
| POST | `/api/seasons/{id}/activate` | set active season |
| GET/POST | `/api/seasons/{id}/rules` | list / upsert (INSERT OR REPLACE) |
| PUT/DELETE | `/api/seasons/{id}/rules/{rid}` | update / delete rule |
| GET/POST | `/api/seasons/{id}/skipped-weeks` | list / add |
| DELETE | `/api/seasons/{id}/skipped-weeks/{sid}` | remove |
| GET/POST | `/api/seasons/{id}/bye-requests` | list / add |
| PUT/DELETE | `/api/seasons/{id}/bye-requests/{bid}` | update / delete |
| GET/POST | `/api/seasons/{id}/teams` | list / add season teams |
| PUT/DELETE | `/api/seasons/{id}/teams/{tid}` | update / remove season team |
| GET/POST | `/api/seasons/{id}/teams/{tid}/roster` | list / add roster player |
| DELETE | `/api/seasons/{id}/teams/{tid}/roster/{pid}` | remove roster player |
| GET | `/api/seasons/{id}/players/available` | available players for roster |
| GET | `/api/seasons/{id}/previous` | previous season teams |
| GET | `/api/seasons/{id}/checklist` | setup checklist |
| GET | `/api/matches` | list (`?season_id=`) |
| POST | `/api/matches/generate` | generate schedule |
| POST | `/api/seasons/{id}/schedule/pushback-preview` | pushback preview (read-only) |
| POST | `/api/seasons/{id}/schedule/pushback-apply` | apply pushback atomically |
| GET | `/api/matches/{id}` | get match detail |
| PATCH | `/api/matches/{id}/assign` | assign home/away teams |
| POST/DELETE | `/api/matches/{id}/results` | submit / clear results |
| GET/POST | `/api/matches/{id}/rounds` | get / save scoresheet rounds |
| GET/POST | `/api/lineup-plans` | list / save team lineup |
| DELETE | `/api/lineup-plans/{id}` | delete plan |
| GET | `/api/standings` | standings (`?season_id=`) |
| GET | `/api/player-stats` | player stats (`?season_id=`) |
| GET | `/api/rules/definitions` | system rule definitions (developer-owned) |
| GET | `/api/seasons/{id}/weeks` | list weeks with status and ack counts |
| GET | `/api/seasons/{id}/weeks/{week}/validate` | dry-run week validation |
| POST | `/api/seasons/{id}/weeks/{week}/close` | validate + commit week close |
| POST | `/api/seasons/{id}/weeks/{week}/reopen` | reopen a closed week |
| GET | `/api/seasons/{id}/weeks/{week}/advance-preview` | pre-close advance preview |
| GET | `/api/seasons/{id}/weeks/{week}/acknowledgments` | list prior close acks |
| GET | `/api/seasons/{id}/handicap-recommendations` | season handicap review (read-only) |
| POST | `/api/seasons/{id}/handicap-apply` | apply handicap recommendations (bearer auth) |
| POST | `/api/users` | create user with one-time API key (static admin token gated) |
| GET | `/api/users` | list users without hash (static admin token gated) |

## Handicap Formula (logic/handicap.go)

Derived from the original FileMaker Scoresheet_Gameday calculation:
```
R1G1 Hndcp = Abs(((H1_Hndcp - V1_Hndcp) * .85) * 3)
           = Abs(diff) * 2.55
```

```go
const Multiplier = 2.55  // 0.85 × 3

// CalcSpot — uses default multiplier
func CalcSpot(homeHC, awayHC float64) SpotResult

// CalcSpotM — per-season configurable multiplier
func CalcSpotM(homeHC, awayHC, multiplier float64) SpotResult

type SpotResult struct {
    Pts int    // balls spotted to lower-rated player; 0 when equal
    To  string // "home" | "away" | ""
}
```

- Result is `math.Round(abs(diff) * multiplier)` — nearest whole ball
- "home" receives spot when homeHC < awayHC (home is lower-rated)
- `matches.ResolveRoundConfig` in `backend/domains/matches/round_config.go` reads
  `handicap_multiplier` from `season_rules` via `rules.RuleStore`; defaults to
  `logic.Multiplier` (2.55) if absent or invalid

## 8-Ball Scoring

- 3 games always played per pairing (all 3, regardless of who wins 2)
- **Winner** of each game: 10 pts (7 object balls × 1 pt + 8-ball × 3 pt)
- **Loser** of each game: balls pocketed (0–7 pts)
- Handicap spot added to lower-rated player's raw total for adjusted score
- Pairing winner = player with higher adjusted total across all 3 games

## Pairing Rotation (Schedule Generation)

For a 3-team format, round `r` (0-based), pairing slot `p` (0-based):
```
home = home[p]
away = away[(p + r) % 3]
```
This ensures each team faces each other team exactly once in a single round-robin.

## Current System Rules

System rules are defined and rendered by the rules domain component
(`web/domains/rules/rules-domain.js`). Keys are stored as `season_rules.rule_key`.
When no DB row exists for a key the frontend shows the default value. The
approved target moves rule definitions and validation fully to the backend,
supporting system -> league -> season inheritance and a snapshot at activation.
See `doc/domains/rules/README.md`.

**All rules are approved for backend enforcement.** Migration is on hold pending
the domain-first architecture rebuild. Do not add new frontend-only rule logic.

**Handicap Settings**
- `handicap_multiplier` — number, default 2.55 — **backend-enforced** (read by `matches.ResolveRoundConfig` via `rules.RuleStore`)
- `max_individual_handicap` — number, default 4.5 — backend pending
- `handicap_rounding` — select: `nearest`|`floor`|`ceiling`, default `nearest` — backend pending
- `max_pairing_spot` — integer, default 15 — backend pending
- `max_match_spot` — integer, default 15 — backend pending
- `handicap_update_method` — select: `manual_review`|`game_diff_average`|`kicker_average_preview`, default `manual_review` — backend pending

**Lineup Settings**
- `lineup_players_per_team` — integer, default 3 — backend pending
- `games_per_pairing` — integer, default 3 — backend pending
- `allow_substitutes` — boolean, default true — backend pending

**Scheduling Settings**
- `allow_bye_requests` — boolean, default true — backend pending
- `require_bye_approval` — boolean, default true — backend pending

Known system rule keys are tracked inside the rules domain component; any key not
recognized as a system rule is rendered as a custom rule in a freeform section.

## Current Frontend Architecture (web/)

Single-page app — Bootstrap 5, no build step. The app shell (`index.html`) mounts
domain components registered via ES modules. `app.js` is shell-level coordination
only; domain screens live under `web/domains/`.

- **index.html** — App shell. No inline `<script>` or `<style>` tags. Loads
  `<link rel="stylesheet" href="/styles.css">` and `<script src="/app.js" defer>`.
  Domain component `<script type="module">` tags added for each extracted domain.
- **styles.css** — All CSS. Sections: layout, nav, scoresheet (`.ss-*`), poster (`.poster-*`),
  rules (`.rule-*`), print media queries.
- **app.js** — Shell coordination only. Owns navigation (`navTo`, `activateSection`,
  `loadSection`), cross-domain event wiring (season, players, leagues, schedule,
  dashboard events), and shell bridge functions (`openMatchEntry`). No domain
  workflow logic remains here. State lives in `appContext` from `web/lib/app-context.js`.
- **web/lib/** — Shared browser utilities loaded as classic `defer` scripts before
  any domain module runs: `api-client.js` (`api()` fetch wrapper), `app-context.js`
  (`appContext` with league/season/player state and entry preselect), `ui-feedback.js`
  (`toast`, `openModal`, `closeModal`), `html-escape.js` (`escapeHTML()`).
- **web/domains/** — Extracted domain Web Components and named API services.
  Handicaps, schedules, matches, players, leagues, seasons, and standings are
  domain-owned here. Each domain folder has a `*-domain.js` entry point,
  `*-page-component.js` or similar, and `*-api-service.js`.

The target frontend uses native Web Components, ES modules, light DOM, and
shared CSS organized by domain. `index.html` becomes the app shell. Use small,
reviewable patches when splitting the current large files.

### Target Frontend Domain Pattern

The frontend must mirror backend domain boundaries without requiring an
immediate framework migration. The target flow is:

```text
application shell
  -> named domain entry point
      -> domain workflow component
          -> domain API service
              -> shared HTTP client
      -> purpose-specific reusable components
```

The application shell owns navigation, selected league/season context,
notifications, and domain mounting. It does not own domain rendering, domain
API details, or domain-local state. Do not add substantial new workflows to
`app.js` or `index.html`.

Use descriptive filenames rather than generic entry points:

```text
web/domains/handicaps/
  handicaps-domain.js
  handicap-review-component.js
  handicap-api-service.js

web/components/input/
  boolean-choice-input.js
  controlled-code-select.js

web/components/feedback/
  validation-message-list.js
  loading-indicator.js
```

Every filename must identify its domain or reusable purpose and its
responsibility. Do not use generic names such as `index.js`, `service.js`,
`state.js`, `view.js`, `component.js`, `utils.js`, or `helpers.js`.

Domain components own their rendering, interactions, lifecycle, and local
state. They communicate with the shell through explicit properties, methods,
and custom events. Domain API calls belong in a named domain API service rather
than being scattered through rendering code.

Shared UI components must be domain-neutral and must not depend on hidden
global caches, global event buses, domain editors, or domain APIs. Add shared
infrastructure such as paging, caching, routing, or state only after at least
two real consumers demonstrate the same need and semantics.

Native Web Components remain the approved migration path. Handicaps is the
preferred frontend extraction pilot because its backend boundary and read-only
API are stable. Vue remains a future evaluation checkpoint if the pilot shows
that custom lifecycle, event coordination, or shared state is too complex.

Use `node --check web/app.js` (not `new Function(js)`) to verify syntax.
`new Function` wraps code in a function body and can misdiagnose top-level
declarations.

## Print Support

**Scoresheet** (`#ss-print-area`): portrait letter, 0.45 in margins.
```css
@media print { body * { visibility: hidden; }
  #ss-print-area { position: fixed; visibility: visible; ... }
  @page { size: letter portrait; margin: 0.45in; } }
```

**Schedule Poster** (`#schedule-poster-view`): landscape letter, pool-table green felt
aesthetic with pool ball decorations, pairings with superscript match numbers,
team rosters at bottom.
```css
@page { size: letter landscape; margin: 0.3in; }
```

## Known Issues & Workarounds

| Issue | Fix |
|-------|-----|
| `Edit` tool truncates large files | Use Python `with open()` read/modify/write |
| `new Function(js)` false syntax errors | Use `node --check web/app.js` instead |
| `/.git/index.lock` held by Windows | All git commits must be run from Windows terminal |
| SQLite `ALTER TABLE … ADD COLUMN` no IF NOT EXISTS | Use additiveMigrations slice, ignore errors |

## Build & Run

```bash
# From Windows terminal in C:\Users\admin\source\league_app
go test ./...                    # run all tests (logic/, backend/domains/*, backend/storage/sqlite/*, handlers/)
go build ./...                   # verify compilation
go run . -data ./data            # start server (default port :8080)
go run . -data ./data -seed      # seed starter leagues/teams/players then exit
```

### Slash Commands (.claude/commands/)

Three project-scoped skills are available in Claude Code:

| Command | What it does |
|---------|-------------|
| `/build` | `go test ./...` then `go build ./...` |
| `/seed-build` | Seeds `./data/league.db` with starter data, then tests + builds |
| `/restart` | Kills :8080, tests, builds, starts server |
| `/ship "message"` | Commits modified files, builds, starts server |

The binary serves the embedded `web/` directory at `/` and the API at `/api/`.
SQLite database is created automatically in the `-data` directory on first run.

**Git:** All commits must be made from the Windows terminal due to `/.git/index.lock`
ownership. Typical commit after a task:
```
cd C:\Users\admin\source\league_app
git add <files>
git commit -m "<message>"
```

Branch policy:
- Keep `main` stable and representative of accepted milestones.
- Start new implementation work on a feature branch, not directly on `main`.
- The Project Manager creates and names the branch for the active work item.
- The Project Manager owns the Git and environment control steps around an
  approved handoff:
  - create and switch feature branches
  - push feature branches and `main`
  - merge approved work into `main`
  - deploy to staging and clean up merged branches
- After a branch is accepted, merged, and pushed to `origin/main`, the Project
  Manager should delete the merged branch locally and on `origin` unless there
  is a specific short-term reason to keep it.
- Use the feature or phase name directly, for example:
  - `handicap-apply-b3`
  - `close-week-phase-3c`
  - `teams-phase-8`
- Keep incomplete or in-review work on the feature branch until it is accepted.
- The developer role is to implement, test, document, and provide commit-ready
  handoff notes. Do not assume push, merge, deploy, or branch-cleanup steps are
  developer-owned unless the Project Manager explicitly delegates them.
- The Project Manager also owns local learning-journal maintenance in
  `PROJECT_LOG.log`. After major accepted phases, the PM may add or backfill
  entries that explain what changed, why it mattered, how the relevant code now
  works, what stayed intentionally unchanged, and what should be remembered for
  future work.
- `PROJECT_LOG.log` may include short code examples for complex patterns. It is
  local-only teaching material and is not a commit target.
- Default workflow responsibilities:
  1. Project Manager creates or switches the named feature branch.
  2. Developer implements the phase, runs verification, and prepares the review
     handoff.
  3. Project Manager reviews the handoff and approves corrections or commit.
  4. Developer creates the approved commit and reports the hash, staged files,
     and post-commit status.
  5. Project Manager pushes the feature branch.
  6. Project Manager merges the accepted branch into `main`.
  7. Project Manager pushes `main`.
  8. Project Manager runs `DEPLOY-STAGING` when deployment is needed.
  9. Project Manager verifies staging health and closes out the branch.
  10. Project Manager deletes the merged branch locally and on `origin` unless
      there is a short-term documented reason to keep it.
- Strict PM mode is the default. PM directions such as "next", "go ahead", or
  "keep moving" mean continue only through PM-owned steps unless the PM
  explicitly asks the same agent to perform Developer work too.
- Do not treat a clear next coding slice as permission to start implementation.
  Implementation, code edits, and developer-side verification still require an
  explicit request to act as Developer.
- Developer memos should clearly distinguish implementation completion from
  PM-owned repository or environment steps.
- Handoff commands:
  - PM may use `.claude/commands/handoff.md` via `/handoff "branch-name: next task description"`
    to write `.claude/pending-task.md` as the next-session primer.
  - Developer may use `.claude/commands/pickup.md` via `/pickup` to load that
    pending-task file at session start when the developer session is also
    Claude Code.
  - If the developer session is in Cursor instead, the PM should still create
    the handoff file and paste its contents into Cursor manually.
  - `.claude/pending-task.md` is workflow state, not a commit target.

## Pending / Planned Work

Roadmap direction check:
- Favor restructuring, domain extraction, and current admin workflow stability
  before pulling broader auth, audit, or platform work forward.
- Treat the current Handicap Apply auth model as a bridge, not the final users
  or roles design.
- Keep shared audit/history as a later cross-app capability, not an immediate
  implementation target.
- Treat browser/mobile scorekeeping as a small future workflow prototype first,
  not a current client-expansion program.
- If a proposed phase does not clearly fit the roadmap's Now or Next sections,
  defer it unless the Project Manager explicitly reprioritizes it.

- **Domain extraction:** Backend and frontend extraction substantially complete. All
  handlers delegate to domain services. `web/app.js` is now shell coordination only.
  Remaining: rule snapshot at activation, any new domains not yet extracted.
- **Season workflow:** Season teams/rosters, schedule pushback (Phases M/N/O), and
  week close/reopen are implemented. Remaining: rule snapshot at activation, match
  finalization, and season closing with final standings snapshots.
- **Controlled codes and audit:** Design code sets as a whole, use optional
  `notes` for free text, and add one append-only system audit log.
- **Users and roles:** Keep users separate from players. Review the provisional
  optional one-to-one link and invitation workflow before implementation.
- **Open design questions:** Track `RULES-Q001` and `USERS-Q001` in
  `doc/architecture-decisions.md`. (`PLAYERS-Q001`, `CODES-Q001`, and
  `SCHEDULES-Q001` are resolved — see `doc/roadmap.md` Resolved Questions.)

- **Mobile scorekeeping:** The scoresheet entry screen may need a separate mobile-optimized
  view for live scoring at the table. Not yet started.
- **EF-style migrations:** Currently pure SQL additive migrations. No ORM planned.
- **Handicap auto-calculation:** Player `handicap` field is currently edited manually.
  Future work: compute from `round_results` after each match night.
- **9-ball support:** `admin_hold` flag exists; race-to handicap system differs from 8-ball
  diff rating. The `HandicapAdjustment` / `EffectiveWins` legacy functions in `handicap.go`
  are retained for this.
