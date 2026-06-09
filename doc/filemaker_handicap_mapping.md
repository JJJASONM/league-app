# FileMaker Handicap Field Mapping

Analysis date: 2026-06-03

**Document status:** Research and current-schema mapping. Some recommended
fields predate the approved domain-first season roster and rule snapshot design.
Use `doc/architecture-decisions.md` as the authority for future architecture,
while this document remains the authority for interpreting the FileMaker
exports and confirmed legacy formulas.

Source exports:

- `C:\Users\admin\OneDrive\Documents\tableExport.pdf`
- `C:\Users\admin\OneDrive\Desktop\fieldExport.pdf`
- `C:\Users\admin\OneDrive\Documents\relationshipExport.pdf`

Extracted text copies are in `doc/filemaker_exports/`.

## Short Answer

Yes, we can recreate the **scoresheet handicap spot algorithm** from the FileMaker fields.

The FileMaker formula is:

```text
spot = Abs(((player_a_handicap - player_b_handicap) * .85) * 3)
```

That simplifies to:

```text
spot = Abs(player_a_handicap - player_b_handicap) * 2.55
```

The current app already uses that same formula in two places:

```text
spot_points = round(abs(home_handicap - away_handicap) * 2.55)
```

So the app is aligned with the FileMaker scoresheet handicap calculation.

What we **cannot fully recreate from these exports alone** is the formula that updates a player's handicap after matches. The FileMaker exports show supporting fields such as `Match Total Difference`, `Handicap Adjustment`, `Kicker Average`, and `Player Estimated Handicap`, but the two most important update fields appear to be ordinary number fields, not calculation fields. That means the actual update process may have been manual, scripted, or handled in a layout/script that is not included in the table/field/relationship exports.

## FileMaker Tables Found

| FileMaker table | Purpose | Current app equivalent |
| --- | --- | --- |
| `NineBall Teams` | Team master table | `teams` |
| `NineBall_Players` | Player master table | `players` |
| `NineBall_Matches_Team` | Team-level match/standings stats | Derived from `matches`, `match_results`, `round_results`; partly `standings` output |
| `NineBall_Matches_Players` | Player match stats and handicap-related stats | `match_results`, plus possibly future `player_match_stats` view/table |
| `Scoresheet_Gameday` | One scoresheet/match with team/player slots and pairing handicap calculations | `matches`, `lineup_plans`, `round_results` |
| `Handicap Changes` | Manual/audit history of handicap changes | `handicap_history` |
| `Email_Layout` | Email templates | No current equivalent; optional future communication feature |

## Exact Handicap Spot Algorithm

The `Scoresheet_Gameday` table contains nine handicap calculation fields:

```text
R1G1 Hndcp = Abs(((H1_Hndcp - V1_Hndcp) * .85) * 3)
R1G2 Hndcp = Abs(((H2_Hndcp - V2_Hndcp) * .85) * 3)
R1G3 Hndcp = Abs(((H3_Hndcp - V3_Hndcp) * .85) * 3)

R2G1 Hndcp = Abs(((H1_Hndcp - V2_Hndcp) * .85) * 3)
R2G2 Hndcp = Abs(((H2_Hndcp - V3_Hndcp) * .85) * 3)
R2G3 Hndcp = Abs(((H3_Hndcp - V1_Hndcp) * .85) * 3)

R3G1 Hndcp = Abs(((H1_Hndcp - V3_Hndcp) * .85) * 3)
R3G2 Hndcp = Abs(((H2_Hndcp - V1_Hndcp) * .85) * 3)
R3G3 Hndcp = Abs(((H3_Hndcp - V2_Hndcp) * .85) * 3)
```

That gives the 3x3 rotation:

| Round | Pairing 1 | Pairing 2 | Pairing 3 |
| --- | --- | --- | --- |
| Round 1 | H1 vs V1 | H2 vs V2 | H3 vs V3 |
| Round 2 | H1 vs V2 | H2 vs V3 | H3 vs V1 |
| Round 3 | H1 vs V3 | H2 vs V1 | H3 vs V2 |

Current app mapping:

- `scoresheetHomeTeam[0..2]` = H1/H2/H3
- `scoresheetAwayTeam[0..2]` = V1/V2/V3
- `round_results.round_number` = round 1/2/3
- `round_results.home_player_id` and `round_results.away_player_id` store the actual pairing
- computed `handicap_pts` = rounded spot points

The current app formula uses rounding:

```text
round(abs(home_handicap - away_handicap) * 2.55)
```

FileMaker field type is `Calculation (Number)` and does not show rounding. FileMaker may display/format the number as an integer on the layout, or it may preserve decimals internally. We need one league-owner decision:

```text
Should handicap spots be rounded to whole balls/points, truncated, or displayed with decimals?
```

The current scoresheet uses whole point spots, which is operationally cleaner.

## Field Mapping

### Team Master

| FileMaker field | Current field | Status | Notes |
| --- | --- | --- | --- |
| `NineBall_Team` | `teams.name` | Maps directly | Team name |
| `NineBall_Team_Number` | no dedicated field | Missing but useful | Current app uses internal `teams.id`; poster assigns temporary team numbers. If imported legacy numbers matter, add `teams.team_number`. |
| `Final Rank` | no stored field | Derived | Current standings can compute final rank; store only if you need archived/manual final placements. |

Recommendation:

- Add `teams.team_number` if preserving printed schedule/team numbers matters.
- Do not store `Final Rank` yet; calculate it from standings unless historical final rank must be manually locked.

### Player Master

| FileMaker field | Current field | Status | Notes |
| --- | --- | --- | --- |
| `Nine_Ball_Player` | `players.first_name`, `players.last_name`, computed `name` | Maps with transformation | FileMaker stores full name as one field. |
| `Nine_Ball_Player_Number` | `players.player_number` | Maps directly | Current app stores as text; FileMaker used number. Text is better for leading zeros. |
| `In?` | no current field | Missing; optional | Could map to `players.active` or `players.status`. Useful for hiding inactive players. |
| `Phone Number1` | `players.phone` | Maps directly | Current app has one phone. |
| `Phone Number2` | no current field | Missing; optional | Could add `players.phone_alt` if needed. |
| `email` | `players.email` | Maps directly | |
| `Note` | no current field | Missing; useful | Add `players.note` if operator notes matter. |
| `email_body`, `email_subject`, `Send email?` | no current field | Not needed for handicap | Belongs to future communication workflow, not core handicap. |
| `Handicap` | `players.handicap` | Maps directly | Core input for scoresheet spot formula. |
| `League Fees` | no current field | Out of scope | Add only if payment tracking becomes a feature. |
| `League Fees Summary` | derived | Out of scope | Summary total of fees. |

Recommendation:

- Add `players.active` and `players.note` eventually.
- `phone_alt` is nice-to-have, not needed for handicap.
- No need to add email template fields to the player table.

### Scoresheet / Match Header

| FileMaker field | Current field | Status | Notes |
| --- | --- | --- | --- |
| `Date of Match` | `matches.match_date` | Maps directly | |
| `Match Number` | `matches.match_number` | Maps directly | Current schema has this. |
| `Home Team Number` | `matches.home_team_id`, possibly future `teams.team_number` | Partial | Current app stores team ID, not legacy team number. |
| `Visitor Team Number` | `matches.away_team_id`, possibly future `teams.team_number` | Partial | |
| `Home Team` | `home_team_name` joined from `teams.name` | Derived | No need to duplicate. |
| `Visitor Team` | `away_team_name` joined from `teams.name` | Derived | No need to duplicate. |
| `Table Assignment 1` | `matches.table_numbers` | Partial | Current app has one text field; FileMaker has two assignment fields. |
| `Table Assignment 2` | `matches.table_numbers` | Partial | Could store as `"1&2"` or add structured table assignment later. |
| `Match Played` | `matches.completed` plus stats | Partial | FileMaker uses a number, maybe count/flag. Current app has boolean completed. |

Recommendation:

- Current match fields are enough for handicap.
- Consider structured table assignment only if you need separate table numbers for simultaneous matches.

### Scoresheet Lineup Fields

| FileMaker field | Current field | Status | Notes |
| --- | --- | --- | --- |
| `H1_Number`, `H2_Number`, `H3_Number` | `lineup_plans.player_id`, `round_results.home_player_id` | Maps | Current app normalizes these into rows. |
| `V1_Number`, `V2_Number`, `V3_Number` | `lineup_plans.player_id`, `round_results.away_player_id` | Maps | |
| `H1_Name`, `H2_Name`, `H3_Name` | joined from `players` | Derived | No need to store copies. |
| `V1_Name`, `V2_Name`, `V3_Name` | joined from `players` | Derived | No need to store copies. |
| `H1_Hndcp`, `H2_Hndcp`, `H3_Hndcp` | `players.handicap`; computed snapshot missing | Partial | Current app reads current handicap. A historical snapshot would be safer. |
| `V1_Hndcp`, `V2_Hndcp`, `V3_Hndcp` | `players.handicap`; computed snapshot missing | Partial | `round_results` returns handicap values by joining current player rows, not stored at match time. |

Important gap:

The FileMaker scoresheet copies player handicaps into scoresheet fields at match time. That freezes the handicap used for that match. The current app can display old matches using the player's current handicap unless the handicap changed later.

Recommendation:

- Add handicap snapshot columns to `round_results`:

```text
home_handicap_used REAL NOT NULL DEFAULT 0
away_handicap_used REAL NOT NULL DEFAULT 0
handicap_pts_used INTEGER NOT NULL DEFAULT 0
handicap_to TEXT NOT NULL DEFAULT ''
```

Alternative:

- Keep computing from `players.handicap`, but then old scoresheets may shift if a player handicap changes. That is risky.

### Round Result / Scoring Fields

FileMaker exports do not show explicit per-game ball score fields in these table exports, only the handicap spot calculations. The current app is stronger here because it stores actual per-game point values:

```text
round_results.game1_home
round_results.game1_away
round_results.game2_home
round_results.game2_away
round_results.game3_home
round_results.game3_away
```

Current app can derive:

```text
games_won
games_lost
raw point totals
adjusted point totals
pairing winner
match result
```

Recommendation:

- Keep current normalized `round_results`; it is better than copying FileMaker's many H/V slot fields.
- Add handicap snapshots as above.

### Player Match Stats / Handicap Update Fields

| FileMaker field | Current field | Status | Notes |
| --- | --- | --- | --- |
| `Set Win` | `match_results.sets_won` | Maps | |
| `Set Loss` | `match_results.sets_lost` | Maps | |
| `Set Winning Percentage` | derived | Maps by calculation | `sets_won / (sets_won + sets_lost)` |
| `Games Won` | `match_results.games_won` | Maps | |
| `Games Lost` | `match_results.games_lost` | Maps | |
| `Total Games Played` | derived | Maps by calculation | `games_won + games_lost` |
| `Game Win Percentage` | derived | Maps by calculation | `games_won / total_games` |
| `Match Total Difference` | no exact current field | Missing/partly `match_results.diff` | FileMaker field is a number, not shown as calculation. |
| `Handicap Adjustment` | no exact current field | Missing | Likely manual or script-calculated. |
| `Summary` | derived summary | Not stored | Total of Match Total Difference. |
| `Kicker Average` | no exact current field | Can derive if `Match Total Difference` exists | Formula: `Match Total Difference / Total Games Played`. |
| `Kicker Average Summary` | no exact current field | Can derive | Average of Kicker Average. |
| `Player Estimated Handicap` | no exact current field | Missing | Likely proposed/next handicap. |
| `Player Handicap` | `players.handicap` snapshot | Maps/partial | Current app has current player handicap. |

The visible FileMaker formula:

```text
Kicker Average = Match Total Difference / Total Games Played
Kicker Average Summary = Average of Kicker Average
```

But the exports do not define:

```text
Match Total Difference = ?
Handicap Adjustment = ?
Player Estimated Handicap = ?
```

Therefore we cannot reconstruct the full handicap update algorithm with certainty from these exports alone.

Recommended interpretation:

```text
player_estimated_handicap = average(kicker_average over selected matches)
kicker_average = match_total_difference / total_games_played
```

But we still need to know how `Match Total Difference` was entered or calculated. It may be:

1. raw points scored minus opponent points,
2. adjusted points scored minus adjusted opponent points,
3. games won minus games lost,
4. scorekeeper-entered total difference from the paper scoresheet,
5. script-calculated from fields not present in these exports.

The current app currently uses option 3:

```text
match_results.diff = games_won - games_lost
player.handicap meaning = average(diff)
```

This is simpler than the FileMaker hints and may not match the original handicap update process if `Match Total Difference` was point-based.

## Can We Accomplish Missing Fields Another Way?

Yes. Most missing FileMaker fields are either lookup copies or summary calculations. In a relational app, we should derive those instead of storing them.

| Missing FileMaker-style field | Add field? | Better app approach |
| --- | --- | --- |
| Team/player name lookup copies | No | Join from `teams` / `players` |
| Total set wins/losses summaries | No | Aggregate from `match_results` |
| Total game wins/losses summaries | No | Aggregate from `match_results` |
| Winning percentages | No | Compute in SQL/API |
| Standings calculations | No | Existing `logic/scoring.go` computes standings |
| Team final rank | Maybe later | Compute, unless archived/manual final rank is required |
| Player `In?` | Yes, eventually | Add `players.active` |
| Player notes | Yes, useful | Add `players.note` |
| Second phone | Optional | Add `players.phone_alt` only if needed |
| Legacy team number | Yes, useful for imports/posters | Add `teams.team_number` |
| Handicap used on a historical scoresheet | Yes | Add snapshot columns to `round_results` |
| Match Total Difference | Maybe | Add if we choose FileMaker-style point-difference handicap updates |
| Handicap Adjustment | Maybe | Store only as audit/debug output, not required if formula is deterministic |
| Player Estimated Handicap | Maybe | Better as computed preview; store only when accepted into `handicap_history` |

## Recommended Schema Changes

### High Value

```sql
ALTER TABLE teams ADD COLUMN team_number TEXT NOT NULL DEFAULT '';
ALTER TABLE players ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
ALTER TABLE players ADD COLUMN note TEXT NOT NULL DEFAULT '';
ALTER TABLE round_results ADD COLUMN home_handicap_used REAL;
ALTER TABLE round_results ADD COLUMN away_handicap_used REAL;
ALTER TABLE round_results ADD COLUMN handicap_pts_used INTEGER;
ALTER TABLE round_results ADD COLUMN handicap_to TEXT;
```

### Optional, If We Recreate FileMaker Handicap Updates

```sql
CREATE TABLE player_match_handicap_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    match_id INTEGER NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    player_id INTEGER NOT NULL REFERENCES players(id),
    games_won INTEGER NOT NULL DEFAULT 0,
    games_lost INTEGER NOT NULL DEFAULT 0,
    total_games_played INTEGER NOT NULL DEFAULT 0,
    match_total_difference REAL NOT NULL DEFAULT 0,
    kicker_average REAL NOT NULL DEFAULT 0,
    estimated_handicap REAL NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(match_id, player_id)
);
```

This table is not strictly required if we compute these values on demand from `round_results`, but it can make handicap-review screens and audit trails easier.

## Proposed Handicap Algorithm Options

### Option 1: Exact Scoresheet Spot Formula Only

This is already done:

```text
spot = round(abs(player_a_handicap - player_b_handicap) * 2.55)
spot_to = lower_handicap_player
```

Use this for match entry and scoresheets.

### Option 2: Current App Handicap Update

Use match game differential:

```text
match_diff = games_won - games_lost
new_handicap = average(match_diff over completed matches)
```

This is simple, but probably not the same as FileMaker if `Match Total Difference` meant point differential.

### Option 3: FileMaker-Inspired Kicker Average

If `Match Total Difference` is point differential:

```text
match_total_difference = player_total_points - opponent_total_points
total_games_played = games_won + games_lost
kicker_average = match_total_difference / total_games_played
estimated_handicap = average(kicker_average over eligible matches)
```

Then the scoresheet spot formula remains:

```text
spot = round(abs(estimated_handicap_a - estimated_handicap_b) * 2.55)
```

This is the most likely way to connect the exported fields into a full handicap system, but it requires confirming what `Match Total Difference` represented in the original FileMaker workflow.

## Recommendation

Build the next handicap feature in two layers:

1. **Lock the known formula.**
   - Keep `spot = round(abs(diff) * 2.55)`.
   - Make `.85`, `3`, and/or `2.55` visible as season rule settings.
   - Snapshot handicap values used at match time.

2. **Add a Handicap Review screen before automatically changing player handicaps.**
   - Show current handicap.
   - Show games won/lost.
   - Show point differential, game differential, and kicker average.
   - Show estimated next handicap.
   - Let the operator accept the change into `players.handicap` and `handicap_history`.

This avoids overcommitting to a guessed FileMaker update algorithm while still using every formula we can prove from the exports.

## Open Questions

1. In FileMaker, how was `Match Total Difference` entered or calculated?
2. Was `Handicap Adjustment` manually entered, script-generated, or calculated elsewhere?
3. Was `Player Estimated Handicap` an operator preview or an actual stored result?
4. Did the league round handicap spots, truncate them, or allow decimals?
5. Should historical scoresheets preserve the handicap used that night even after a player's handicap changes?

My technical recommendation for question 5 is yes.
