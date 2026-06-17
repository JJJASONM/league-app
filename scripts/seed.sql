-- =====================================================================
-- League App demo seed data -- all names, teams, and data are fictional.
-- Run via: go run . -data ./data -seed
-- Or directly: sqlite3 data/league.db < scripts/seed.sql
-- All inserts use INSERT OR IGNORE (safe to re-run on existing database).
-- =====================================================================

-- =====================================================================
-- Leagues
-- =====================================================================
INSERT OR IGNORE INTO leagues (id, name, game_format, day_of_week) VALUES
  (1, 'Demo Pool League',   '8ball', 'Monday'),
  (2, 'Demo 9-Ball League', '9ball', 'Tuesday');

-- =====================================================================
-- Seasons
-- teams_managed=1 enables explicit roster and captain management.
-- activated_at records first activation:
--   NULL               = draft (setup in progress)
--   NOT NULL + active=0 = historical (season closed)
--   NOT NULL + active=1 = active (current season)
-- =====================================================================
INSERT OR IGNORE INTO seasons
  (id, league_id, name, start_date, active, schedule_type, num_weeks,
   teams_managed, activated_at)
VALUES
  -- 8-ball: one historical, one active, one draft
  (1, 1, 'Fall 2025',   '2025-09-08', 0, 'double_rr', 0, 1, '2025-08-20 12:00:00'),
  (2, 1, 'Spring 2026', '2026-01-05', 1, 'double_rr', 0, 1, '2025-12-10 12:00:00'),
  (3, 1, 'Summer 2026', '2026-06-02', 0, 'double_rr', 0, 1, NULL),
  -- 9-ball: one active, one draft
  (4, 2, 'Spring 2026', '2026-01-06', 1, 'double_rr', 0, 1, '2025-12-10 12:00:00'),
  (5, 2, 'Summer 2026', '2026-06-03', 0, 'double_rr', 0, 1, NULL);

-- =====================================================================
-- Season Rules
-- Stored rules override system defaults.  Absent keys fall back to
-- system defaults (e.g. handicap_multiplier defaults to 2.55).
-- =====================================================================
INSERT OR IGNORE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES
  -- 8-ball active season
  (2, 'handicap_multiplier',     'Handicap multiplier',        '2.55'),
  (2, 'max_individual_handicap', 'Max individual handicap',    '4.5'),
  (2, 'lineup_players_per_team', 'Players per team per match', '3'),
  (2, 'games_per_pairing',       'Games per pairing',          '3'),
  -- 8-ball draft season (in-progress rules setup)
  (3, 'handicap_multiplier',     'Handicap multiplier',        '2.55'),
  (3, 'max_individual_handicap', 'Max individual handicap',    '4.5'),
  -- 9-ball active season
  (4, 'handicap_multiplier',     'Handicap multiplier',        '2.55'),
  (4, 'max_individual_handicap', 'Max individual handicap',    '7'),
  (4, 'lineup_players_per_team', 'Players per team per match', '3');

-- =====================================================================
-- Skipped Weeks
-- =====================================================================
INSERT OR IGNORE INTO skipped_weeks (season_id, skip_date, reason) VALUES
  -- 8-ball Spring 2026 (Mondays)
  (2, '2026-01-19', 'MLK Day'),
  (2, '2026-05-25', 'Memorial Day'),
  -- 9-ball Spring 2026 (Tuesdays)
  (4, '2026-04-07', 'Spring Break');

-- =====================================================================
-- Teams (permanent league teams -- exist independently of any season)
-- =====================================================================

-- 8-ball teams (League 1)
INSERT OR IGNORE INTO teams (id, league_id, name) VALUES
  (1, 1, 'Rack Attackers'),
  (2, 1, 'Bridge Over Troubled Cues'),
  (3, 1, 'Scratch and Win'),
  (4, 1, 'The Cueball Conspiracy'),
  (5, 1, 'Eight Is Enough'),
  (6, 1, 'Side Pocket Scientists');

-- 9-ball teams (League 2)
INSERT OR IGNORE INTO teams (id, league_id, name) VALUES
  (7,  2, 'Nine Lives'),
  (8,  2, 'Breaking Point'),
  (9,  2, 'On the Rail'),
  (10, 2, 'Dead Stroke Club'),
  (11, 2, 'Run and Hide'),
  (12, 2, 'Corner Pocket Kings'),
  (13, 2, 'Frozen Solid');

-- =====================================================================
-- Players (8-ball, League 1)
-- handicap = Diff rating: (total games won - total games lost) / matches played
--   positive = more wins than losses; negative = more losses than wins
-- player_number format: team# + slot# (e.g. "21" = team 2, slot 1)
-- =====================================================================

-- Team 1: Rack Attackers
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (1,  '11', 'Alex',  'Vance',    1,  0.05),
  (2,  '12', 'Dana',  'Frost',    1, -0.56),
  (3,  '13', 'Remy',  'Cole',     1,  1.83);

-- Team 2: Bridge Over Troubled Cues
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (4,  '21', 'Leo',   'Finch',    2,  0.23),
  (5,  '22', 'Opal',  'Kwan',     2, -2.51),
  (6,  '23', 'Brent', 'Solano',   2, -4.77),
  (7,  '25', 'Nina',  'Park',     2,  0.11);

-- Team 3: Scratch and Win
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (8,  '31', 'Cole',  'Morrow',   3, -2.15),
  (9,  '32', 'Ivy',   'Sloane',   3, -0.98),
  (10, '33', 'Wade',  'Larkin',   3, -2.71),
  (11, '34', 'Petra', 'Quinn',    3, -2.81),
  (12, '35', 'Finn',  'Yates',    3,  6.22);

-- Team 4: The Cueball Conspiracy
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (13, '41', 'Ava',   'Thorne',   4,  2.20),
  (14, '42', 'Dean',  'Wolfe',    4,  4.94),
  (15, '43', 'Sage',  'Holloway', 4, -1.60);

-- Team 5: Eight Is Enough
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (16, '51', 'Rex',   'Barlow',   5,  3.40),
  (17, '52', 'Lila',  'Crane',    5,  3.76),
  (18, '53', 'Zane',  'Mercer',   5,  2.29),
  (19, '54', 'Nora',  'Hendrix',  5,  1.53);

-- Team 6: Side Pocket Scientists
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (20, '61', 'Walt',  'Greer',    6, -0.83),
  (21, '62', 'Tess',  'Valdez',   6, -0.50),
  (22, '63', 'Mac',   'Sloane',   6, -2.15),
  (23, '64', 'Faye',  'Cullen',   6,  1.59);

-- =====================================================================
-- Players (9-ball, League 2)
-- handicap = race-to number (e.g. 5 = race to 5 balls)
-- admin_hold = 1 means handicap is locked at Administrative Discretion
-- =====================================================================

-- Team 7: Nine Lives
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap, admin_hold) VALUES
  (24, '11', 'Colt', 'Pryor',  7, 5, 0),
  (25, '12', 'Bree', 'Hadley', 7, 2, 1),  -- held at Admin Discretion
  (26, '14', 'Sid',  'Kato',   7, 0, 0);  -- race-to TBD, new player

-- Team 8: Breaking Point
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (27, '21', 'Gus',  'Tenley', 8, 2),
  (28, '22', 'Mira', 'Yoon',   8, 5);

-- Team 9: On the Rail
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (29, '31', 'Fern', 'Castillo', 9, 7),
  (30, '32', 'Otis', 'Frame',    9, 7);

-- Team 10: Dead Stroke Club
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (31, '41', 'Cora', 'Nash', 10, 3),
  (32, '42', 'Beau', 'Ward', 10, 2);

-- Team 11: Run and Hide
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (33, '51', 'Hank', 'Reyes',  11, 5),  -- promoted from 4 mid-season
  (34, '52', 'Gwen', 'Talbot', 11, 4);

-- Team 12: Corner Pocket Kings
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap, admin_hold) VALUES
  (35, '61', 'Eli',   'Strand', 12, 4, 0),
  (36, '62', 'Rho',   'Vance',  12, 6, 0),
  (37, '64', 'Pearl', 'Dix',    12, 0, 0),  -- race-to TBD, new player
  (38, '65', 'Axel',  'Stone',  12, 0, 0);  -- race-to TBD, new player

-- Team 13: Frozen Solid
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap, admin_hold) VALUES
  (39, '71', 'Dove', 'March',  13, 5, 0),
  (40, '72', 'Buck', 'Norris', 13, 4, 1);  -- held at Admin Discretion

-- =====================================================================
-- Season Teams
-- Explicit team participation per season.  season_name is the team's
-- display name for that season (defaults to permanent team name).
-- captain_id must reference a player on the season roster (checked at
-- activation; the rosters below match).
-- =====================================================================

-- Season 1: Fall 2025 (8-ball, historical) -- all 6 teams, fully set up
INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id) VALUES
  (1, 1, 'Rack Attackers',            1),   -- Alex Vance
  (1, 2, 'Bridge Over Troubled Cues', 4),   -- Leo Finch
  (1, 3, 'Scratch and Win',           8),   -- Cole Morrow
  (1, 4, 'The Cueball Conspiracy',   13),   -- Ava Thorne
  (1, 5, 'Eight Is Enough',          16),   -- Rex Barlow
  (1, 6, 'Side Pocket Scientists',   20);   -- Walt Greer

-- Season 2: Spring 2026 (8-ball, active) -- all 6 teams, fully set up
INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id) VALUES
  (2, 1, 'Rack Attackers',            1),   -- Alex Vance
  (2, 2, 'Bridge Over Troubled Cues', 4),   -- Leo Finch
  (2, 3, 'Scratch and Win',           8),   -- Cole Morrow
  (2, 4, 'The Cueball Conspiracy',   13),   -- Ava Thorne
  (2, 5, 'Eight Is Enough',          16),   -- Rex Barlow
  (2, 6, 'Side Pocket Scientists',   20);   -- Walt Greer

-- Season 3: Summer 2026 (8-ball, draft) -- 4 of 6 teams enrolled so far
-- Teams 5 and 6 have not been added yet (available via draft-team-actions).
-- Captains are partially assigned to show the draft captain UI.
INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id) VALUES
  (3, 1, 'Rack Attackers',            1),   -- Alex Vance (captain set)
  (3, 2, 'Bridge Over Troubled Cues', NULL), -- no captain yet
  (3, 3, 'Scratch and Win',           8),   -- Cole Morrow (captain set)
  (3, 4, 'The Cueball Conspiracy',   13);   -- Ava Thorne (captain set)

-- Season 4: Spring 2026 (9-ball, active) -- all 7 teams, fully set up
INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id) VALUES
  (4,  7, 'Nine Lives',          24),  -- Colt Pryor
  (4,  8, 'Breaking Point',      27),  -- Gus Tenley
  (4,  9, 'On the Rail',         29),  -- Fern Castillo
  (4, 10, 'Dead Stroke Club',    31),  -- Cora Nash
  (4, 11, 'Run and Hide',        33),  -- Hank Reyes
  (4, 12, 'Corner Pocket Kings', 35),  -- Eli Strand
  (4, 13, 'Frozen Solid',        39);  -- Dove March

-- Season 5: Summer 2026 (9-ball, draft) -- 2 of 7 teams enrolled
INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id) VALUES
  (5,  7, 'Nine Lives',    24),    -- Colt Pryor (captain set)
  (5,  8, 'Breaking Point', NULL); -- no captain yet

-- =====================================================================
-- Season Rosters
-- UNIQUE(season_id, player_id) enforces one team per player per season.
-- team_id must match a registered season_teams row (enforced by trigger).
-- =====================================================================

-- Season 1 (historical Fall 2025): full rosters
INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES
  (1, 1,  1), (1, 1,  2), (1, 1,  3),
  (1, 2,  4), (1, 2,  5), (1, 2,  6), (1, 2, 7),
  (1, 3,  8), (1, 3,  9), (1, 3, 10), (1, 3, 11), (1, 3, 12),
  (1, 4, 13), (1, 4, 14), (1, 4, 15),
  (1, 5, 16), (1, 5, 17), (1, 5, 18), (1, 5, 19),
  (1, 6, 20), (1, 6, 21), (1, 6, 22), (1, 6, 23);

-- Season 2 (active Spring 2026): full rosters
INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES
  (2, 1,  1), (2, 1,  2), (2, 1,  3),
  (2, 2,  4), (2, 2,  5), (2, 2,  6), (2, 2, 7),
  (2, 3,  8), (2, 3,  9), (2, 3, 10), (2, 3, 11), (2, 3, 12),
  (2, 4, 13), (2, 4, 14), (2, 4, 15),
  (2, 5, 16), (2, 5, 17), (2, 5, 18), (2, 5, 19),
  (2, 6, 20), (2, 6, 21), (2, 6, 22), (2, 6, 23);

-- Season 3 (draft Summer 2026): partial rosters to show available-player UI.
-- Team 1 (Rack Attackers): all 3 players rostered.
-- Team 2 (Bridge Over Troubled Cues): 2 of 4 added; Opal (#22) and Nina (#25) still available.
-- Team 3 (Scratch and Win): 4 of 5 added; Finn Yates (#35) still available.
-- Team 4 (The Cueball Conspiracy): 2 of 3 added; Sage Holloway (#43) still available.
-- Teams 5 and 6 not registered yet -- all their players show as available.
INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES
  (3, 1,  1), (3, 1,  2), (3, 1,  3),
  (3, 2,  4), (3, 2,  6),
  (3, 3,  8), (3, 3,  9), (3, 3, 10), (3, 3, 11),
  (3, 4, 13), (3, 4, 14);

-- Season 4 (active Spring 2026 9-ball): full rosters
INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES
  (4,  7, 24), (4,  7, 25), (4,  7, 26),
  (4,  8, 27), (4,  8, 28),
  (4,  9, 29), (4,  9, 30),
  (4, 10, 31), (4, 10, 32),
  (4, 11, 33), (4, 11, 34),
  (4, 12, 35), (4, 12, 36), (4, 12, 37), (4, 12, 38),
  (4, 13, 39), (4, 13, 40);

-- Season 5 (draft Summer 2026 9-ball): partial rosters.
-- Team 7 (Nine Lives): only Colt Pryor added; Bree and Sid still available.
-- Team 8 (Breaking Point): Gus and Mira added; no captain assigned yet.
-- Teams 9-13 not yet registered -- all their players are available.
INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES
  (5,  7, 24),
  (5,  8, 27), (5, 8, 28);

-- =====================================================================
-- Handicap History (9-ball, League 2)
-- Traces how each player's race-to number changed during Spring 2026.
-- Explicit IDs make this idempotent on re-seed.
-- =====================================================================

-- Week 4 (Jan 27): Mira Yoon 3->4; Beau Ward 3->2
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date) VALUES
  (1, 28, 3, 4, '2026-01-27'),
  (2, 32, 3, 2, '2026-01-27');

-- Week 5 (Feb 3): Eli Strand 3->4; Colt Pryor 4->5
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date) VALUES
  (3, 35, 3, 4, '2026-02-03'),
  (4, 24, 4, 5, '2026-02-03');

-- Week 6 (Feb 10): Otis Frame 5->6
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date) VALUES
  (5, 30, 5, 6, '2026-02-10');

-- Week 7 (Feb 17): Mira Yoon 4->5; Bree Hadley held at 3 (Admin Discretion)
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date, admin_hold, note) VALUES
  (6, 28, 4, 5, '2026-02-17', 0, NULL),
  (7, 25, 3, 3, '2026-02-17', 1, 'Admin Discretion -- held pending league review');

-- Week 8 (Feb 23): Gus Tenley 3->2; Bree Hadley reviewed and moved to 2
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date) VALUES
  (8,  27, 3, 2, '2026-02-23'),
  (9,  25, 3, 2, '2026-02-23');

-- Week 10 (Mar 3): Dove March 4->5; Buck Norris held at 4 (Admin Discretion)
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date, admin_hold, note) VALUES
  (10, 39, 4, 5, '2026-03-03', 0, NULL),
  (11, 40, 4, 4, '2026-03-03', 1, 'Admin Discretion -- held pending league review');

-- Week 11 (Mar 10): Otis Frame 6->7
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date) VALUES
  (12, 30, 6, 7, '2026-03-10');

-- Week 15 (Apr 14): Hank Reyes 4->5 (consistent winning streak)
INSERT OR IGNORE INTO handicap_history (id, player_id, old_handicap, new_handicap, effective_date) VALUES
  (13, 33, 4, 5, '2026-04-14');
