# League App — Claude Context

## Project Overview

League App is a Go + SQLite web application for managing pool (billiards) leagues.
It handles leagues, teams, players, seasons, schedules, scoresheets, lineups, standings,
and handicap calculations. The user is a solo developer building this for real-world
league operations. The primary game format is 8-ball.

## Architecture Status

The current codebase is a working monolithic implementation. The approved
target is documented in:

- `AGENTS.md` - engineering and documentation conventions
- `doc/architecture-decisions.md` - cross-domain workflows and open questions
- `doc/domains/*/README.md` - domain-specific decisions
- `doc/erd.mermaid` - physical schema implemented today

Do not treat target tables or workflows as already implemented. Migrate one
domain at a time, beginning with `rules`, while preserving behavior during the
first architecture pass.

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
    models.go                — all struct definitions, no logic
  logic/
    handicap.go              — authoritative handicap formula (CalcSpot, CalcSpotM)
    handicap_test.go         — 11 Go tests for handicap functions
  handlers/
    api.go                   — all HTTP handlers, route registration, seasonMultiplier()
  web/
    index.html               — HTML skeleton only (~642 lines), references styles.css + app.js
    styles.css               — all CSS (~382 lines), including scoresheet, poster, rules
    app.js                   — all JavaScript (~2251 lines), full SPA logic
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

All routes registered by `handlers.Register(mux, dataDir)`:

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
| GET | `/api/matches` | list (`?season_id=`) |
| POST | `/api/matches/generate` | generate schedule |
| GET | `/api/matches/{id}` | get match detail |
| PATCH | `/api/matches/{id}/assign` | assign home/away teams |
| POST/DELETE | `/api/matches/{id}/results` | submit / clear results |
| GET/POST | `/api/matches/{id}/rounds` | get / save scoresheet rounds |
| GET/POST | `/api/lineup-plans` | list / save team lineup |
| DELETE | `/api/lineup-plans/{id}` | delete plan |
| GET | `/api/standings` | standings (`?season_id=`) |
| GET | `/api/stats` | player stats (`?season_id=`) |

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
- `seasonMultiplier(seasonID int64) float64` in `api.go` reads `handicap_multiplier`
  from `season_rules`; defaults to `logic.Multiplier` (2.55) if absent or invalid

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

## Current System Rules (SYSTEM_RULES in app.js)

This section describes the implementation today. The approved target moves rule
definitions and validation to the backend, supports system -> league -> season
inheritance, and locks a season snapshot at activation. See
`doc/domains/rules/README.md`.

Three groups rendered in the Rules tab. Keys stored as `season_rules.rule_key`.
Defaults are shown by the UI from `SYSTEM_RULES`; no DB row is required.

**All rules are approved for backend enforcement.** Migration is on hold pending
the domain-first architecture rebuild. Do not add new frontend-only rule logic.

**Handicap Settings**
- `handicap_multiplier` — number, default 2.55 — **backend-enforced** (reads `seasonMultiplier()`)
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

System rule keys are tracked in `SYSTEM_RULE_KEYS` Set for O(1) lookup; any key not in
that set is rendered as a custom rule in a freeform section below the groups.

## Current Frontend Architecture (web/)

Single-page app — Bootstrap 5, no build step. Three files:

- **index.html** — HTML skeleton only. No inline `<script>` or `<style>` tags.
  Loads `<link rel="stylesheet" href="/styles.css">` and `<script src="/app.js" defer>`.
- **styles.css** — All CSS. Sections: layout, nav, scoresheet (`.ss-*`), poster (`.poster-*`),
  rules (`.rule-*`), print media queries.
- **app.js** — All JS. Key globals: `currentLeagueId`, `currentSeasonId`, `SYSTEM_RULES`,
  `SYSTEM_RULE_KEYS`. Key functions: `loadSeasonRules`, `renderRuleGroup`,
  `renderCustomRulesSection`, `saveSystemRule`, `openSchedulePoster`, `renderScoresheet`,
  `saveScoresheet`, `fmtDate`, `fmtDateRange`.

The target frontend uses native Web Components, ES modules, light DOM, and
shared CSS organized by domain. `index.html` becomes the app shell. Use small,
reviewable patches when splitting the current large files.

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
go test ./...                    # run all tests (11 handicap tests in logic/)
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

## Pending / Planned Work

- **Domain-first migration:** Begin with `rules`; keep frontend/backend domain
  names aligned and expose small public interfaces.
- **Season workflow:** Add explicit season teams and rosters, rule snapshots,
  schedule preview/pushback, match finalization, week close/reopen, and season
  closing with final standings snapshots.
- **Controlled codes and audit:** Design code sets as a whole, use optional
  `notes` for free text, and add one append-only system audit log.
- **Users and roles:** Keep users separate from players. Review the provisional
  optional one-to-one link and invitation workflow before implementation.
- **Open design questions:** Track `RULES-Q001`, `PLAYERS-Q001`, `USERS-Q001`,
  `CODES-Q001`, and `SCHEDULES-Q001` in `doc/architecture-decisions.md`.

- **Mobile scorekeeping:** The scoresheet entry screen may need a separate mobile-optimized
  view for live scoring at the table. Not yet started.
- **EF-style migrations:** Currently pure SQL additive migrations. No ORM planned.
- **Handicap auto-calculation:** Player `handicap` field is currently edited manually.
  Future work: compute from `round_results` after each match night.
- **9-ball support:** `admin_hold` flag exists; race-to handicap system differs from 8-ball
  diff rating. The `HandicapAdjustment` / `EffectiveWins` legacy functions in `handicap.go`
  are retained for this.
