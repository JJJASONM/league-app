-- ============================================================
-- league_app seed data
-- Run against an initialized database:
--   sqlite3 data/league.db < scripts/seed.sql
-- ============================================================

-- ── Leagues ──────────────────────────────────────────────────
INSERT OR IGNORE INTO leagues (id, name, game_format, day_of_week) VALUES
  (1, 'Brass Ring Monday Night 8-Ball', '8ball', 'Monday'),
  (2, 'Brass Ring Tuesday Night 9-Ball', '9ball', 'Tuesday');

-- ── Seasons ───────────────────────────────────────────────────
-- schedule_type: 'double_rr' = full home-and-away round-robin (default)
-- end_date is left NULL — computed automatically after schedule generation
INSERT OR IGNORE INTO seasons (id, league_id, name, start_date, active, schedule_type, num_weeks) VALUES
  (1, 1, 'Spring 2026', '2026-01-05', 1, 'double_rr', 0),
  (2, 2, 'Spring 2026', '2026-01-06', 1, 'double_rr', 0);

-- ── Season Rules ──────────────────────────────────────────────
-- 8-ball league rules
INSERT OR IGNORE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES
  (1, 'max_scoresheet_handicap', 'Max handicap shown on scoresheet', '4.5'),
  (1, 'max_match_handicap',      'Max handicap for any match',       '15');

-- 9-ball league rules
INSERT OR IGNORE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES
  (2, 'max_scoresheet_handicap', 'Max handicap shown on scoresheet', '4.5'),
  (2, 'max_match_handicap',      'Max handicap for any match',       '15');

-- ── Skipped Weeks ─────────────────────────────────────────────
-- 8-ball (Monday): skip MLK Day and Memorial Day
INSERT OR IGNORE INTO skipped_weeks (season_id, skip_date, reason) VALUES
  (1, '2026-01-19', 'MLK Day'),
  (1, '2026-05-25', 'Memorial Day');

-- 9-ball (Tuesday): no major Tuesday holidays in Spring 2026 window

-- ── 8-Ball Teams ──────────────────────────────────────────────
-- First digit of player number = team number within league
INSERT OR IGNORE INTO teams (id, league_id, name) VALUES
  (1, 1, 'Cues But No Clues'),
  (2, 1, 'The Ferrule Children'),
  (3, 1, 'Sweep the Legacy'),
  (4, 1, 'Placentipede'),
  (5, 1, 'Troubleshooters'),
  (6, 1, '5SC');

-- ── 9-Ball Teams ──────────────────────────────────────────────
INSERT OR IGNORE INTO teams (id, league_id, name) VALUES
  (7,  2, 'Sharpshooters'),
  (8,  2, 'Fellowship of the Ring'),
  (9,  2, 'If There''s a Will, There''s a Jay'),
  (10, 2, 'The Night Maiers'),
  (11, 2, 'Ball Breakers 2'),
  (12, 2, 'Dog''s Bollocks'),
  (13, 2, 'Broken Hearts');

-- ── 8-Ball Players ────────────────────────────────────────────
-- handicap = Diff rating: (total games won − total games lost) ÷ matches played
--   e.g. 87 wins, 66 losses over 9 matches → (87−66)/9 = 2.33
--   Positive = wins more than they lose; negative = loses more than they win.
-- player_number format: team# + position# (e.g. "42" = team 4, slot 2)

-- Team 1: Cues But No Clues
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (1,  '11', 'Tracy',  'Lark',        1,  0.05),
  (2,  '12', 'Geoff',  'Eddinger',    1, -0.56),
  (3,  '13', 'Shawn',  'Ellenbecker', 1,  1.83);

-- Team 2: The Ferrule Children
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (4,  '21', 'Chris', 'Long',    2,  0.23),
  (5,  '22', 'Tom',   'Burke',   2, -2.51),
  (6,  '23', 'Joey',  'Morales', 2, -4.77),
  (7,  '25', 'Chris', 'Lewis',   2,  0.11);

-- Team 3: Sweep the Legacy
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (8,  '31', 'Jeremy', 'Singleton',   3, -2.15),
  (9,  '32', 'Yuri',   'Bialoskursky',3, -0.98),
  (10, '33', 'Mark',   'Kremer',      3, -2.71),
  (11, '34', 'Shawn',  'Frye',        3, -2.81),
  (12, '35', 'Mike',   'Kingslien',   3,  6.22);

-- Team 4: Placentipede
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (13, '41', 'Matt',  'Allenstein', 4,  2.20),
  (14, '42', 'Tarek', 'Hamdan',     4,  4.94),
  (15, '43', 'Seth',  'Armstrong',  4, -1.60);

-- Team 5: Troubleshooters
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (16, '51', 'Will',  'Sutherland', 5,  3.40),
  (17, '52', 'Jason', 'Matzke',     5,  3.76),
  (18, '53', 'Dave',  'Taras',      5,  2.29),
  (19, '54', 'James', 'Daly',       5,  1.53);

-- Team 6: 5SC
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (20, '61', 'Mitchel', 'Shepard',  6, -0.83),
  (21, '62', 'Keif',    'Callaway', 6, -0.50),
  (22, '63', 'Erick',   'Esse',     6, -2.15),
  (23, '64', 'Donnie',  'Anderson', 6,  1.59);

-- ── 9-Ball Players ────────────────────────────────────────────
-- handicap = race-to number as of April 14th, 2026
-- admin_hold = 1 means locked at Administrative Discretion

-- Team 7: Sharpshooters
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap, admin_hold) VALUES
  (24, '11', 'Mike',   'Coenen',  7, 5, 0),
  (25, '12', 'Greg',   'Lindner', 7, 2, 1),  -- held at Admin Discretion
  (26, '14', 'Cidney', 'Lange',   7, 0, 0);  -- race-to TBD

-- Team 8: Fellowship of the Ring
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (27, '21', 'Mike',  'Gengler',      8, 2),
  (28, '22', 'Shawn', 'Ellenbecker',  8, 5);

-- Team 9: If There's a Will, There's a Jay
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (29, '31', 'Will',  'Sutherland', 9, 7),
  (30, '32', 'Jason', 'Matzke',     9, 7);

-- Team 10: The Night Maiers
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (31, '41', 'Kurt', 'Maier', 10, 3),
  (32, '42', 'Evan', 'Maier', 10, 2);

-- Team 11: Ball Breakers 2
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap) VALUES
  (33, '51', 'Chris', 'Lewis', 11, 5),  -- moved from 4 to 5 on April 14th
  (34, '52', 'Chris', 'Long',  11, 4);

-- Team 12: Dog's Bollocks
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap, admin_hold) VALUES
  (35, '61', 'Mike',  'Kingslien', 12, 4, 0),
  (36, '62', 'Mike',  'Benoy',     12, 6, 0),
  (37, '64', 'Tracy', 'Lark',      12, 0, 0),  -- race-to TBD
  (38, '65', 'Todd',  'Ward',      12, 0, 0);  -- race-to TBD

-- Team 13: Broken Hearts
INSERT OR IGNORE INTO players (id, player_number, first_name, last_name, team_id, handicap, admin_hold) VALUES
  (39, '71', 'Tom',   'Kesselhon', 13, 5, 0),
  (40, '72', 'Jesse', 'Price',     13, 4, 1);  -- held at Admin Discretion

-- ── 9-Ball Handicap History ───────────────────────────────────
-- Tracking all changes from the email records

-- Jan 27: Shawn Ellenbecker 3→4, Evan Maier 3→2
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (28, 3, 4, '2026-01-27'),
  (32, 3, 2, '2026-01-27');

-- Feb 3: Mike Kingslien 4→5, Mike Coenen 5→4
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (35, 4, 5, '2026-02-03'),
  (24, 5, 4, '2026-02-03');

-- Feb 10: Jason Matzke moves up to 6
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (30, 5, 6, '2026-02-10');

-- Feb 17: Shawn Ellenbecker 4→5, Greg Lindner remains 3 (Admin Discretion)
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date, admin_hold, note) VALUES
  (28, 4, 5, '2026-02-17', 0, NULL),
  (25, 3, 3, '2026-02-17', 1, 'Administrative Discretion');

-- Feb 23: Mike Gengler 3→2, Greg Lindner 3→2
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (27, 3, 2, '2026-02-23'),
  (25, 3, 2, '2026-02-23');

-- Mar 3: Mike Coenen 4→5, Tom Kesselhon 4→5, Jesse Price remains 4 (Admin Discretion)
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date, admin_hold, note) VALUES
  (24, 4, 5, '2026-03-03', 0, NULL),
  (39, 4, 5, '2026-03-03', 0, NULL),
  (40, 4, 4, '2026-03-03', 1, 'Administrative Discretion');

-- Mar 10: Mike Kingslien 5→4
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (35, 5, 4, '2026-03-10');

-- Mar 31: Jason Matzke moves up to 7, Chris Long moves up to 4
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (30, 6, 7, '2026-03-31'),
  (34, 3, 4, '2026-03-31');

-- Apr 14: Chris Lewis 4→5
INSERT OR IGNORE INTO handicap_history (player_id, old_handicap, new_handicap, effective_date) VALUES
  (33, 4, 5, '2026-04-14');
