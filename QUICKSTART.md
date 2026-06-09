# Pool League Manager — Quick Start

## Prerequisites

Install Go 1.22+: https://go.dev/dl/

That's the only dependency. The app uses a pure-Go SQLite driver (no C compiler needed).

## Build

**Windows:**
```
build.bat
```
Produces `league_app.exe`.

**macOS / Linux:**
```
chmod +x build.sh && ./build.sh
```
Produces `./league_app`.

**Cross-compile for all platforms at once (from Windows):**
```
build_all.bat
```
Produces binaries in `dist/` for Windows, macOS Intel, and macOS Apple Silicon.

## Run

Double-click `league_app.exe` (Windows) or `./league_app` (macOS).  
The browser opens automatically to **http://localhost:8080**.

The database (`league.db`) is created next to the executable in a `data/` folder.

### Options

```
league_app.exe --port 9090            # use a different port
league_app.exe --data C:\MyLeagueData # custom data directory
```

## Current Workflow

1. **Seasons** → Create a season, set it as Active
2. **Teams** → Add your teams
3. **Players** → Add players, assign to teams, set skill levels (1–9)
4. **Seasons → Generate Schedule** → Pick the season & first match date → Generate
5. **Match Entry** → Select season & match, enter games won/lost per player, Save
6. **Standings** → Live team standings
7. **Player Stats** → Individual win rates

This workflow describes the application implemented today. The approved target
adds rule snapshots, explicit season teams and rosters, schedule preview and
pushback, match finalization, week close/reopen, season close, audit history,
and future users and roles. See `doc/architecture-decisions.md`.

## Sharing with others

Run `build_all.bat` once to get platform binaries. Give each person:
- `league_app_windows.exe` **or** `league_app_macos_*` (the right one for their OS)
- Nothing else needed — the database travels with the app in `data/league.db`

To share data: copy the `data/` folder alongside the binary.  
To back up: click **Backup DB** in the sidebar — saves a timestamped copy to `data/`.

## Project Structure

```
league_app/
├── main.go            # Entry point, server, browser launch
├── go.mod
├── db/db.go           # SQLite init, migrations, backup
├── models/models.go   # Data structs
├── logic/
│   ├── handicap.go    # Handicap formula (swappable)
│   ├── scoring.go     # Win/loss/tie + standings calc
│   └── scheduling.go  # Round-robin generator
├── handlers/api.go    # All REST endpoints
├── web/index.html     # Embedded SPA (Bootstrap 5)
└── doc/               # Schema and target workflow documentation
```
