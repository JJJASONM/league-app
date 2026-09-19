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

`GET /healthz` returns 200 with `{"status":"ok"}` when the server and its
database connection are up (503 if the database is unreachable). Useful for
confirming the app is running locally or as a lightweight staging/IIS health
check.

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

### Staging

`scripts/deploy/seed-staging.ps1` runs the base seed only by default. Pass
`-SeedFixtures` to also load the scoresheet fixtures above (all weeks)
immediately afterward, using the same staging executable and data
directory -- so match-entry, close-week, standings, handicap, and recap
workflows have ready data to smoke-test without generating a schedule by
hand:

```powershell
.\scripts\deploy\seed-staging.ps1 -ConfirmSeed SEED-STAGING -SeedFixtures
```

Staging operations are Project-Manager-owned; see `doc/testing/product-smoke-test-checklist.md`
for the full smoke-test checklist this supports.

## Current Workflow

1. `Seasons` -> create a season and manage setup
2. `Teams` -> register season teams, rosters, captains, and draft names
3. `Seasons -> Generate Schedule` -> create matches for the season
4. `Match Entry` -> enter scoresheets and save round data
5. `Schedule -> Review & Close` -> validate a week and make results official
6. `Standings` -> official team standings from closed weeks only
7. `Player Stats` -> official individual stats from closed weeks only
8. `Handicap` -> read-only season handicap review recommendations

This reflects the implemented workflow today. Users/Roles/Authentication
Phase 1 (see below) added real email+password login, sessions, and
scoped roles; broader audit/history and the eventual handicap apply UI
remain future phases.

## Signing In (Users/Roles Phase 1)

There is no self-registration yet -- a system_admin creates every
account.

1. **Bootstrap the first system_admin** (only needed once, on a fresh
   install): use the existing static-token bootstrap,
   `POST /api/users` with `Authorization: Bearer $LEAGUE_ADMIN_TOKEN`
   and `{"username":"...", "role":"system_admin"}`. This still returns a
   one-time API key (unchanged), and also grants that account a global
   `system_admin` role assignment automatically, so it can immediately
   use every account-administration action below.
2. From the Users screen (or directly via the API as that system_admin),
   **provision an email-identity account**: "Provision Email Login"
   with an email and, optionally, a player to link.
3. **Issue a one-time password setup token** for that account from its
   row menu -- shown once, copy it and share it with the account holder
   out of band (there is no email delivery in this phase).
4. The account holder opens the app; with no session and no Admin Key
   set, they see the login screen. They click "Have a setup token?",
   enter the token plus a new password (and confirm it), and submit --
   this calls `POST /api/auth/password-setup` for them. On success they
   are returned to the sign-in form and log in normally with email +
   that password.
5. **system_admin** may then assign `league_admin` for one or more
   leagues to any account via
   `POST /api/auth/admin/users/{id}/roles` (`{"role_code":"league_admin",
   "league_id":N}`), or `{"role_code":"system_admin"}` for another
   system_admin. A league_admin may also create new leagues on their
   own once they hold `league_admin` for at least one existing league
   -- doing so automatically grants them access to the league they just
   created.

**Testing with multiple accounts on one computer:** browser cookies are
per-profile, so open a separate browser profile (or a separate browser
entirely) per account to hold simultaneous, independent sessions --
Chrome/Edge "Add profile," Firefox "New Firefox Private Window" is NOT
sufficient since private windows can share state in some configurations;
use full separate profiles.

**Local HTTP testing note:** session and CSRF cookies default to
`Secure` (HTTPS-only), so logging in over plain `http://localhost` will
not work unless you explicitly set `INSECURE_LOCAL_COOKIES=1` in the
environment before starting the server. This is logged loudly at
startup as a warning and must **never** be set for a deployment
reachable by anyone other than the person at that same computer --
there is no way to safely auto-detect "this is local," so it is a
deliberate, visible opt-in every time.

**API-key compatibility:** every personal API key created before this
phase, and every one created via the legacy `POST /api/users` endpoint
going forward, continues to work exactly as before -- pass it as
`Authorization: Bearer <key>` on any route, no session or cookie
involved, and no CSRF header needed. The Admin Key sidebar button
remains available (inside the app shell, once signed in) as this
secondary, bootstrap/automation-oriented path; it is just no longer the
default screen a browser visitor sees. It is also reachable directly
from the login screen itself via "Use Admin Key instead," for a browser
that has neither a session nor a previously stored key.

**Session vs. Admin Key precedence:** an active password session always
takes precedence over a stored Admin Key, in the browser and on the
server alike -- while a session is active, requests never attach the
Admin Key even if one is still stored in that tab, so the identity
signed in is always the identity every action runs as. Logging in with
a password also clears any Admin Key stored in that browser tab. The
Admin Key is used only when no session is active in that tab.

## Sharing

Run `build_all.bat` once to get platform binaries. Give each user:

- the correct binary for their OS
- the matching `data/` folder if they need the same database

To back up: `POST /api/backup` (requires a system-admin personal API key)
writes a timestamped copy of the database to the data directory. Staging
deploys also create a timestamped backup automatically before each deploy.
API backups checkpoint WAL before copying. Staging deploy backups copy any
matching WAL sidecar files alongside the database, so both backup paths are
safe to restore from even if the app was not shut down cleanly.

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
