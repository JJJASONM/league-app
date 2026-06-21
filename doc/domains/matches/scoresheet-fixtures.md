# Scoresheet Fixture Loader

## Purpose

The scoresheet fixture loader creates fictional, repeatable 8-ball match data for
local or staging testing. It is explicitly opt-in and does not run during normal
application startup.

## Commands

```powershell
go run . -data ./data -seed-scoresheet-fixtures
go run . -data ./data -seed-scoresheet-fixtures -fixture-weeks 3
go run . -data ./data -seed-scoresheet-fixtures -fixture-weeks all
go run . -data ./data -seed-scoresheet-fixtures -fixture-week 2
```

## Week Selection

- No week flag: creates week 1 only.
- `-fixture-weeks N`: creates weeks 1 through N.
- `-fixture-weeks all`: creates all fixture weeks, currently weeks 1 through 5.
- `-fixture-week N`: creates only week N.
- `-fixture-weeks` and `-fixture-week` cannot be used together.
- Week numbers must be positive integers.

Invalid values fail with a clear startup error. Invalid input is never silently
ignored.

## Fixture Data

The loader creates or refreshes one fictional league:

```text
Fixture Scoresheet League
Fixture Scoresheet Season
```

It creates four fictional teams with three fictional players each. Player numbers
use the `FS###` prefix so repeated runs can find and update the same records.

The fixture season uses:

- `handicap_multiplier = 2.55`
- `min_ball_handicap = 2`
- `lineup_players_per_team = 3`
- `games_per_pairing = 3`

## Fixture Weeks

| Week | Purpose |
|------|---------|
| 1 | Blank matches with rosters and lineup plans, ready for manual scoresheet entry |
| 2 | Partial and early-stop examples |
| 3 | Completed examples with full score rows |
| 4 | Adjusted-score tie with games-won tie-break examples |
| 5 | Mixed table assignments and additional partial/completed edge cases |

Each fixture week has two matches. Table assignments cover `1,2`, `3,4`, `5,6`,
and `7,8` across the fixture set.

## Safety And Idempotency

The loader only runs when `-seed-scoresheet-fixtures` is present.

It uses stable fictional names and player numbers. Re-running the command:

- reuses the same fixture league and season;
- reuses the same fixture teams and players;
- refreshes lineup plans for the selected weeks;
- refreshes only the selected fixture matches, round rows, and match results;
- does not delete unrelated league, team, player, season, match, or score data.

The loader resets selected fixture matches to `week_closed = 0` so each run is
ready for scoresheet and Close Week testing.

## Notes

The fixture is development/demo data. It is not intended for production league
history or import workflows.
