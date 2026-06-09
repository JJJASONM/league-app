# Pool League Manager

A self-contained Go application for managing a pool league. It runs a local web
server, opens the browser automatically, stores league data in SQLite, and
serves an embedded Bootstrap web UI.

The application is being migrated toward a domain-first architecture with
native Web Components, shared CSS, backend-owned business logic, explicit
season rosters, controlled codes, and audited administrative workflows.

See `AGENTS.md` for engineering conventions,
`doc/architecture-decisions.md` for the approved target design, and
`doc/erd.mermaid` for the database implemented today.

## Quick Start

Install Go 1.22 or newer, then build and run:

```powershell
.\build.bat
.\league_app.exe
```

The app opens at `http://localhost:8080` by default. The SQLite database is
created in `data/league.db` next to the executable.

## Common Commands

```powershell
go test ./...
go run . --port 9090
go run . --seed
go run . --reset-db
```

## Project Layout

```text
main.go            # Entry point, server, browser launch
db/db.go           # SQLite init, migrations, backup
models/models.go   # Data structs
logic/             # Handicap, scoring, scheduling
handlers/api.go    # REST API endpoints
web/index.html     # Embedded single-page app
scripts/seed.sql   # Starter data
doc/               # Current schema, target architecture, and domain decisions
```

See `QUICKSTART.md` for packaging and user workflow details.
