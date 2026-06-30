# Pool League Manager

Pool league management app for seasons, teams, rosters, schedules, score
entry, Close Week, standings, player stats, and handicap review.

It is a self-contained Go application with an embedded web UI and SQLite
storage. The server runs locally, opens the browser automatically, and stores
application data in `data/league.db` next to the executable by default.

The codebase is mid-migration toward a domain-first architecture:

- backend business logic is moving into `backend/domains/*`
- SQLite access is moving into `backend/storage/sqlite`
- frontend domain UI is moving into `web/domains/*`
- administrative workflows are becoming backend-authoritative

Useful starting points:

- `AGENTS.md` for engineering conventions
- `doc/architecture-decisions.md` for approved cross-domain decisions
- `doc/domains/*/README.md` for domain behavior
- `doc/erd.mermaid` for the implemented database

## Quick Start

Install Go 1.22 or newer, then either run directly:

```powershell
go run . -seed
go run .
```

Or build first:

```powershell
.\build.bat
.\league_app.exe
```

The app opens at [http://localhost:8080](http://localhost:8080) by default.

## Common Commands

```powershell
go test ./...
go run . --port 9090
go run . --seed
go run . --reset-db
go run . -seed-scoresheet-fixtures
go run . -seed-scoresheet-fixtures -fixture-weeks 3
```

## Current Product Surface

- `Seasons`: create, edit, activate, and manage season setup
- `Teams`: explicit season teams, rosters, captains, and draft editing flows
- `Schedule`: generation, viewing, week status, Close Week, reopen, and advance preview
- `Match Entry`: scoresheet entry with backend validation on save
- `Standings` and `Player Stats`: official totals gated by `week_closed=1`
- `Handicap`: read-only season handicap review screen

## Project Layout

```text
main.go                        # entry point and dependency wiring
db/                            # schema, additive migrations, backup, fixtures
handlers/                      # HTTP handlers and dependency contracts
backend/domains/               # domain services and pure business logic
backend/storage/sqlite/        # SQLite adapters for extracted domains
models/                        # shared API/read structs during migration
logic/                         # legacy logic still being migrated
web/                           # embedded SPA shell and domain UI
doc/                           # architecture and domain documentation
```

`QUICKSTART.md` covers build/run details and fixture commands. The `handicaps`
and `matches` domain docs are the best source of truth for the current Close
Week, handicap review, and apply-workflow status.
