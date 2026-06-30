# Pool League Manager - Quick Start

## Prerequisites

Install Go 1.22 or newer:

[https://go.dev/dl/](https://go.dev/dl/)

That is the only required dependency. The app uses a pure-Go SQLite driver, so
no C compiler is needed.

## Build

Windows:

```powershell
.\build.bat
```

Produces `league_app.exe`.

macOS / Linux:

```bash
chmod +x build.sh
./build.sh
```

Produces `./league_app`.

Cross-compile from Windows:

```powershell
.\build_all.bat
```

Produces binaries in `dist/`.

## Run

Direct run:

```powershell
go run . -seed
go run .
```

Binary run:

```powershell
.\league_app.exe
```

The browser opens automatically at [http://localhost:8080](http://localhost:8080).
The database is created in a `data/` folder next to the executable.

### Options

```powershell
league_app.exe --port 9090
league_app.exe --data C:\MyLeagueData
league_app.exe --reset-db
```

## Scoresheet Fixtures

Opt-in fictional fixture data is available for scoresheet, Close Week, and
handicap testing.

```powershell
go run . -seed-scoresheet-fixtures
go run . -seed-scoresheet-fixtures -fixture-week 2
go run . -seed-scoresheet-fixtures -fixture-weeks 3
go run . -seed-scoresheet-fixtures -fixture-weeks all
```

Behavior:

- no week flag -> week 1 only
- `-fixture-week N` -> week N only
- `-fixture-weeks N` -> weeks 1 through N
- `-fixture-weeks all` -> all available fixture weeks

## Current Workflow

1. `Seasons` -> create a season and manage setup
2. `Teams` -> register season teams, rosters, captains, and draft names
3. `Seasons -> Generate Schedule` -> create matches for the season
4. `Match Entry` -> enter scoresheets and save round data
5. `Schedule -> Review & Close` -> validate a week and make results official
6. `Standings` -> official team standings from closed weeks only
7. `Player Stats` -> official individual stats from closed weeks only
8. `Handicap` -> read-only season handicap review recommendations

This reflects the implemented workflow today. Future phases still include auth,
broader audit/history, and the eventual handicap apply UI.

## Sharing

Run `build_all.bat` once to get platform binaries. Give each user:

- the correct binary for their OS
- the matching `data/` folder if they need the same database

To back up: use the `Backup DB` action in the app. It writes a timestamped copy
to the data directory.

## Project Structure

```text
league_app/
|-- main.go
|-- go.mod
|-- db/
|-- handlers/
|-- backend/domains/
|-- backend/storage/sqlite/
|-- models/
|-- logic/
|-- web/
`-- doc/
```
