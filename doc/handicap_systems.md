# Pool Handicap Systems Research

Research date: 2026-06-02

**Document status:** Handicap research and formula comparison. Rule ownership,
season snapshots, status labels, and administrative workflow are governed by
`doc/architecture-decisions.md` and `doc/domains/rules/README.md`.

This document compares handicap systems that are relevant to a local pool league app. It separates systems with usable formulas from systems that publish race charts or inputs but keep the actual rating algorithm proprietary.

## Executive Summary

For this app, the best near-term path is a transparent house system based on the data already captured on the scoresheet:

1. Keep an 8-ball point-differential rating, because the app already records game winners, loser balls, and adjusted points.
2. Add configuration for the handicap multiplier, maximum handicap per player pairing, and maximum handicap per match.
3. Add a documented rolling-average formula so players can see how ratings move.
4. Keep optional compatibility fields for external systems such as FargoRate, APA skill level, TAP level, or NAPA/CueSpeed rating.

The major national systems are useful references, but several are intentionally proprietary. We should not pretend to clone APA, TAP, NAPA, IBA TruSpot, or FargoRate internals unless their operator-facing formulas are actually published.

## Current App Baseline

The app currently treats an 8-ball player's handicap as a point/differential style number:

```text
player_match_diff = games_won - games_lost
player_rating = average(player_match_diff over completed matches)
```

For an individual scoresheet pairing, the current UI/backend applies:

```text
rating_gap = abs(home_rating - away_rating)
spot_points = round(rating_gap * 2.55)
spot_goes_to = lower_rated_player
```

This is a simple, explainable model. Its weakness is that it only measures game win/loss differential, not opponent strength, ball count, innings, safeties, or quality of competition.

## System Comparison

| System | Published formula? | Main inputs | Output | Notes for this app |
| --- | --- | --- | --- | --- |
| APA Equalizer | No, proprietary | Scoresheets, wins/losses, tournament performance, advisory review | Skill level and race/points target | Good inspiration for race charts, not cloneable exactly |
| TAP | No, proprietary | Scoresheets and match performance | Handicap 2-7 | Similar to APA-style levels; not formula-transparent |
| NAPA CueSpeed | Partly described as Elo-style, exact adaptation not public | Match outcomes and performance | Numeric CueSpeed rating | Good inspiration for an Elo-style module |
| FargoRate | Probability model published; full rating engine is global/proprietary | Game wins/losses vs rated opponents | Global rating | Best external rating concept; can use published expectation math |
| BCA/LMS ball average | Yes, operator-level model | Ball/point average by game, round, or match | Team/player average and handicap difference | Very implementable for house leagues |
| BCA/LMS plus/minus | Yes, operator-level model | Wins/losses per match/game | Team/player average and handicap difference | Very implementable and simple |
| IBA TruSpot | No, proprietary | Points, innings, safeties, winner bonus | Rating/race number 30-150 | Useful idea: rating equals race number |
| Current app diff rating | Yes | Games won and lost | Average differential; spot points | Already implemented enough to refine |

## APA Equalizer

APA's Equalizer system is designed so lower-skill players need fewer games or points to win. In APA 8-ball, players give or receive games. In APA 9-ball, players give or receive points. APA states that Local League Management calculates skill levels using scoresheets, win/loss records, tournament performance, advisory review, and mathematical formulas, but the exact formula is not public.

Published behavior:

```text
8-ball:
skill_level -> race_to_games target from a chart

Example from APA:
SL6 vs SL4 => race 5 to 3

9-ball:
skill_level -> points_required_to_win

SL1 = 14
SL2 = 19
SL3 = 25
SL4 = 31
SL5 = 38
SL6 = 46
SL7 = 55
SL8 = 65
SL9 = 75
```

Implementation lesson: APA-style systems are best modeled as a configurable race chart, not as a formula clone. The app could support an "APA-style race chart" by letting the league define skill levels and target scores.

Sources: [APA Equalizer overview](https://poolplayers.com/equalizer/), [APA rules: how handicaps are determined](https://rules.poolplayers.com/the-equalizer-handicap-system/how-handicaps-are-determined/)

## TAP

TAP also uses a proprietary handicap algorithm. A TAP local rules/bylaws page states that handicaps range from 2 through 7, that new players can begin with a Race to 3 and count as a 4 for the 25 Rule, and that TAP's algorithm is proprietary.

Published behavior:

```text
handicap_range = 2..7
new_player_default_for_team_cap = 4
team_cap = 25 across five players
```

Implementation lesson: TAP-style support should be a team-cap and race-chart feature, not a copied algorithm.

Source: [BilliardLife TAP rules/bylaws](https://www.billiardlifetapleague.com/rules_bylaws.php)

## NAPA CueSpeed

NAPA's CueSpeed is described as an adaptation of the Elo system. The local NAPA source says players should start with a rating reflective of ability, that ratings move after every match, and that players are provisional for their first 20 matches.

Generic Elo-style formula:

```text
expected_score = 1 / (1 + 10 ^ ((opponent_rating - player_rating) / 400))
new_rating = old_rating + K * (actual_score - expected_score)
```

For a race or multi-game pool set:

```text
actual_score = player_games_won / total_games_played
expected_score = expected probability from rating gap
rating_delta = K * (actual_score - expected_score)
```

Implementation lesson: this app can implement a transparent Elo-inspired house rating without claiming it is NAPA CueSpeed.

Source: [NAPA of Central Missouri Handicap System](https://www.napa-missouri.com/handicap-system.html)

## FargoRate

FargoRate publishes the core expectation relationship:

```text
win_ratio = 2 ^ (rating_difference / 100)
```

If player A is `d` points stronger than player B:

```text
ratio_A_to_B = 2 ^ (d / 100)
prob_A_wins_game = ratio_A_to_B / (1 + ratio_A_to_B)
prob_B_wins_game = 1 / (1 + ratio_A_to_B)
```

Example:

```text
d = 100
ratio = 2 ^ (100 / 100) = 2
stronger player expected share = 2 / 3 = 66.7%
weaker player expected share = 1 / 3 = 33.3%
```

For a 9-game set:

```text
expected_stronger_games = 9 * 0.667 = 6.0
expected_weaker_games = 9 * 0.333 = 3.0
```

FargoRate's broader rating engine is global and data-driven, but this expectation math is useful for a house handicap calculator.

Implementation lesson: if the app later stores Fargo ratings, it can estimate expected game share and spot games/points without inventing its own meaning for the rating gap.

Sources: [FargoRate blog: Behind the Curtain](https://www.fargorate.com/fargorateblog/archive/behindthecurtain/), [BCA Pool League Operator Handbook](https://www.playcsipool.com/uploads/7/3/5/9/7359673/bcapl_lo_handbook_240806_web.pdf)

## BCA/LMS Ball Average

FargoRate LMS documents ball average handicapping as a system where each player has an average calculated from games played. That average can be calculated per game, round, or match. Player averages are totaled for each team, and the team difference determines the handicap.

Generic formula:

```text
player_ball_average = total_points_scored / games_played
team_average = sum(selected_player_ball_average)
team_gap = higher_team_average - lower_team_average
handicap_to_lower_team = round(team_gap * handicap_percentage)
```

With a cap:

```text
handicap_to_lower_team = min(max_allowed_handicap, round(team_gap * handicap_percentage))
```

This pairs naturally with common 8-ball point systems:

```text
10-point system:
winner = 10
loser = balls_pocketed, max 7

17-point system:
winner = 10 + opponent_balls_remaining
loser = balls_pocketed, max 7
winner + loser = 17
```

Implementation lesson: this is highly suitable for the app because the current scoresheet already tracks winner and loser balls.

Sources: [FargoRate LMS format/handicap settings](https://lms.fargorate.com/lms-help/docs/division/format/), [BCA Pool League Operator Handbook](https://www.playcsipool.com/uploads/7/3/5/9/7359673/bcapl_lo_handbook_240806_web.pdf)

## BCA/LMS Plus-Minus

FargoRate LMS describes plus/minus, also called win/loss, as a system where each game is worth one point. A player's average is based on wins or losses for matches played. Player averages are added by team, and the difference determines the handicap.

Two transparent variants:

```text
win_average = games_won / matches_played
```

or:

```text
plus_minus_average = (games_won - games_lost) / matches_played
```

Team application:

```text
team_rating = sum(selected_player_average)
gap = higher_team_rating - lower_team_rating
handicap_to_lower_team = round(gap * handicap_percentage)
```

Implementation lesson: this is the closest published model to the app's current `games_won - games_lost` approach.

Source: [FargoRate LMS format/handicap settings](https://lms.fargorate.com/lms-help/docs/division/format/)

## IBA TruSpot

IBA's TruSpot system is proprietary. Its rulebook says ratings range from 30 to 150, the rating becomes the player's race number, and ratings are recalculated after each set. It also lists scorekeeping inputs such as rating, safeties, points, innings, total points, and winner bonus.

Published behavior:

```text
rating_range = 30..150
race_number = current_rating
rating_inputs include points, innings, safeties, and winner bonus
```

Implementation lesson: IBA is interesting because the rating directly becomes the race target. That is easy for players to understand, but the actual rating update formula is not public.

Source: [IBA Rulebook](https://www.ibapool.com/Rulebook)

## House-League Options For This App

### Option A: Current Diff Rating, Refined

Use the model already implied by the app.

```text
match_diff = games_won - games_lost
rating = average(match_diff over eligible matches)
pairing_spot = round(abs(rating_a - rating_b) * multiplier)
spot_to = lower_rated_player
```

Recommended settings:

```text
multiplier = 2.55
max_pairing_spot = configurable, default 15
min_matches_for_established = 4
provisional_weight = lower confidence until established
```

Pros:

- Easy to explain.
- Already close to app behavior.
- Uses current scoresheet data.

Cons:

- Does not account for opponent strength.
- Can be distorted by schedule strength.
- Needs guardrails for new players.

### Option B: Ball Average Handicap

Use actual points from the 10-point scoring system.

```text
player_avg = total_points / games_played
team_avg = sum(lineup_player_avg)
team_spot = round((higher_team_avg - lower_team_avg) * handicap_percentage)
```

Pros:

- Rewards performance even in lost games.
- Uses data the scoresheet already captures.
- Familiar to BCA/VNEA-style operators.

Cons:

- Players may focus on maximizing balls instead of winning.
- Does not account for opponent strength unless combined with rating adjustment.

### Option C: Elo/Fargo-Inspired Rating

Use a transparent expectation formula.

```text
expected = 1 / (1 + 10 ^ ((opponent_rating - player_rating) / 400))
actual = games_won / total_games
new_rating = old_rating + K * (actual - expected)
```

Alternative Fargo-like expectation:

```text
ratio = 2 ^ ((player_rating - opponent_rating) / 100)
expected = ratio / (1 + ratio)
```

Pros:

- Accounts for opponent strength.
- More robust across uneven schedules.
- Good long-term model.

Cons:

- Harder to explain to casual players.
- Requires initial ratings and K-factor tuning.
- Needs more data before it feels stable.

### Option D: Race Chart

Store a configurable chart instead of calculating spots.

```text
race_to[player_level][opponent_level] = { player_target, opponent_target }
```

Pros:

- Easy to operate on league night.
- Supports APA/TAP-style play without cloning proprietary formulas.
- Captains understand "race to X" quickly.

Cons:

- Requires chart maintenance.
- Rating updates still need a separate system.

## Recommended Implementation Path

### Phase 1: Make The Current System Explicit

Add a Handicap Settings area:

```text
system = diff_rating
multiplier = 2.55
max_pairing_spot = 15
min_matches_established = 4
use_last_n_matches = 0  // 0 = all
```

Show the calculation on the scoresheet:

```text
Home rating: -2.71
Away rating: -0.83
Gap: 1.88
Spot: round(1.88 * 2.55) = 5
Spot to: Home
```

### Phase 2: Add Ball Average As An Alternative

Because the app already records winner and loser balls, add:

```text
system = ball_average
scoring = 10_point
handicap_percentage = 100
max_match_spot = configurable
```

### Phase 3: Add External Rating Fields

Let each player optionally store:

```text
fargo_rating
apa_8_level
apa_9_level
tap_level
napa_rating
external_rating_note
```

Do not use those fields to calculate automatically unless the league chooses that system.

### Phase 4: Add Elo/Fargo-Inspired House Rating

Only after the league has enough score history:

```text
rating_initial = 500
k_provisional = 32
k_established = 16
established_after_games = 50
expected_model = elo or fargo_ratio
```

## Questions To Decide Before Coding

1. Do we want handicaps to be based on individual pairings, whole team lineups, or both?
2. Should handicaps be applied as balls/points on the scoresheet, race-to targets, or standings points?
3. Should new players start at a fixed value, captain-entered value, or estimated value?
4. Should older matches age out, or should all history count forever?
5. Should players be able to see the exact formula and history that produced their handicap?

## Source Notes

- APA and TAP publish useful player-facing behavior but not their exact algorithms.
- FargoRate publishes a clear rating-difference-to-win-ratio relationship, but the full global rating engine is not something a small local app should attempt to reproduce exactly.
- BCA/FargoRate LMS documents ball average, plus/minus, and FargoRate handicap modes at the operator level.
- The most defensible house system is one whose formula we publish directly in the app.
