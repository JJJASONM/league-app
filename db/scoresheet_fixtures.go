package db

import (
	"database/sql"
	"fmt"
	"time"

	"league_app/logic"
)

const (
	scoresheetFixtureLeague = "Fixture Scoresheet League"
	scoresheetFixtureSeason = "Fixture Scoresheet Season"
)

// ScoresheetFixtureSummary describes what the fixture loader created or refreshed.
type ScoresheetFixtureSummary struct {
	LeagueName string
	SeasonName string
	Weeks      []int
	MatchCount int
}

type fixtureTeam struct {
	Name    string
	Players []fixturePlayer
}

type fixturePlayer struct {
	Number    string
	FirstName string
	LastName  string
	Handicap  float64
}

type fixtureMatch struct {
	Week        int
	Number      int
	HomeTeam    int
	AwayTeam    int
	Table       string
	RoundScores [9][6]int
}

// SeedScoresheetFixtures creates deterministic fictional match data for scoresheet testing.
// Re-running it refreshes only this fixture league/season and selected fixture weeks.
func SeedScoresheetFixtures(weeks []int) (ScoresheetFixtureSummary, error) {
	if len(weeks) == 0 {
		return ScoresheetFixtureSummary{}, fmt.Errorf("at least one fixture week is required")
	}

	tx, err := DB.Begin()
	if err != nil {
		return ScoresheetFixtureSummary{}, err
	}
	defer tx.Rollback()

	leagueID, err := upsertFixtureLeague(tx)
	if err != nil {
		return ScoresheetFixtureSummary{}, err
	}
	seasonID, err := upsertFixtureSeason(tx, leagueID)
	if err != nil {
		return ScoresheetFixtureSummary{}, err
	}

	teams, players, err := upsertFixtureTeamsAndPlayers(tx, leagueID, seasonID)
	if err != nil {
		return ScoresheetFixtureSummary{}, err
	}
	if err := upsertFixtureLineups(tx, seasonID, teams, players, weeks); err != nil {
		return ScoresheetFixtureSummary{}, err
	}

	matchCount := 0
	for _, week := range weeks {
		for _, fm := range fixtureMatchesForWeek(week) {
			matchID, err := upsertFixtureMatch(tx, seasonID, teams, fm)
			if err != nil {
				return ScoresheetFixtureSummary{}, err
			}
			if err := refreshFixtureScores(tx, matchID, teams, players, fm); err != nil {
				return ScoresheetFixtureSummary{}, err
			}
			matchCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return ScoresheetFixtureSummary{}, err
	}
	return ScoresheetFixtureSummary{
		LeagueName: scoresheetFixtureLeague,
		SeasonName: scoresheetFixtureSeason,
		Weeks:      weeks,
		MatchCount: matchCount,
	}, nil
}

func upsertFixtureLeague(tx *sql.Tx) (int64, error) {
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO leagues (name, game_format, day_of_week)
		VALUES (?, '8ball', 'Monday')`, scoresheetFixtureLeague); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRow(`SELECT id FROM leagues WHERE name=?`, scoresheetFixtureLeague).Scan(&id)
	return id, err
}

func upsertFixtureSeason(tx *sql.Tx, leagueID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM seasons WHERE league_id=? AND name=?`, leagueID, scoresheetFixtureSeason).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(`
			INSERT INTO seasons
			    (league_id, name, start_date, active, schedule_type, num_weeks, teams_managed, activated_at)
			VALUES (?, ?, '2026-08-03', 1, 'custom', 5, 1, CURRENT_TIMESTAMP)`,
			leagueID, scoresheetFixtureSeason)
		if err != nil {
			return 0, err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	} else {
		_, err = tx.Exec(`
			UPDATE seasons
			SET start_date='2026-08-03', active=1, schedule_type='custom',
			    num_weeks=5, teams_managed=1, activated_at=COALESCE(activated_at, CURRENT_TIMESTAMP)
			WHERE id=?`, id)
		if err != nil {
			return 0, err
		}
	}

	rules := []struct {
		key, label, value string
	}{
		{"handicap_multiplier", "Handicap multiplier", "2.55"},
		{"min_ball_handicap", "Minimum ball handicap", "2"},
		{"lineup_players_per_team", "Lineup players per team", "3"},
		{"games_per_pairing", "Games per pairing", "3"},
	}
	for _, r := range rules {
		if _, err := tx.Exec(`
			INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(season_id, rule_key) DO UPDATE
			SET rule_label=excluded.rule_label, rule_value=excluded.rule_value`,
			id, r.key, r.label, r.value); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func fixtureTeams() []fixtureTeam {
	return []fixtureTeam{
		{Name: "Fixture Breakers", Players: []fixturePlayer{
			{Number: "FS101", FirstName: "Avery", LastName: "Slate", Handicap: 0.00},
			{Number: "FS102", FirstName: "Blair", LastName: "Flint", Handicap: 0.39},
			{Number: "FS103", FirstName: "Casey", LastName: "Vale", Handicap: 1.00},
		}},
		{Name: "Fixture Bankers", Players: []fixturePlayer{
			{Number: "FS201", FirstName: "Devon", LastName: "Reed", Handicap: 0.00},
			{Number: "FS202", FirstName: "Emery", LastName: "Frost", Handicap: 1.00},
			{Number: "FS203", FirstName: "Finley", LastName: "Moss", Handicap: 2.00},
		}},
		{Name: "Fixture Cutters", Players: []fixturePlayer{
			{Number: "FS301", FirstName: "Gray", LastName: "Lumen", Handicap: -0.25},
			{Number: "FS302", FirstName: "Harper", LastName: "Quill", Handicap: 0.75},
			{Number: "FS303", FirstName: "Indigo", LastName: "North", Handicap: 1.50},
		}},
		{Name: "Fixture Safeties", Players: []fixturePlayer{
			{Number: "FS401", FirstName: "Jules", LastName: "Pike", Handicap: -0.25},
			{Number: "FS402", FirstName: "Kai", LastName: "Ridge", Handicap: 0.50},
			{Number: "FS403", FirstName: "Lena", LastName: "Stone", Handicap: 2.50},
		}},
	}
}

func upsertFixtureTeamsAndPlayers(tx *sql.Tx, leagueID, seasonID int64) ([]int64, [][]int64, error) {
	defs := fixtureTeams()
	teams := make([]int64, len(defs))
	players := make([][]int64, len(defs))

	for ti, team := range defs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO teams (league_id, name) VALUES (?, ?)`, leagueID, team.Name); err != nil {
			return nil, nil, err
		}
		if err := tx.QueryRow(`SELECT id FROM teams WHERE league_id=? AND name=?`, leagueID, team.Name).Scan(&teams[ti]); err != nil {
			return nil, nil, err
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name)
			VALUES (?, ?, ?)`, seasonID, teams[ti], team.Name); err != nil {
			return nil, nil, err
		}

		players[ti] = make([]int64, len(team.Players))
		for pi, p := range team.Players {
			id, err := upsertFixturePlayer(tx, teams[ti], p)
			if err != nil {
				return nil, nil, err
			}
			players[ti][pi] = id
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id)
				VALUES (?, ?, ?)`, seasonID, teams[ti], id); err != nil {
				return nil, nil, err
			}
		}
		if _, err := tx.Exec(`
			UPDATE season_teams SET captain_id=? WHERE season_id=? AND team_id=?`,
			players[ti][0], seasonID, teams[ti]); err != nil {
			return nil, nil, err
		}
	}
	return teams, players, nil
}

func upsertFixturePlayer(tx *sql.Tx, teamID int64, p fixturePlayer) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM players WHERE player_number=?`, p.Number).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(`
			INSERT INTO players (player_number, first_name, last_name, team_id, handicap, active)
			VALUES (?, ?, ?, ?, ?, 1)`,
			p.Number, p.FirstName, p.LastName, teamID, p.Handicap)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`
		UPDATE players
		SET first_name=?, last_name=?, team_id=?, handicap=?, active=1
		WHERE id=?`,
		p.FirstName, p.LastName, teamID, p.Handicap, id)
	return id, err
}

func upsertFixtureLineups(tx *sql.Tx, seasonID int64, teams []int64, players [][]int64, weeks []int) error {
	for _, week := range weeks {
		for ti, teamID := range teams {
			if _, err := tx.Exec(`DELETE FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=?`, seasonID, teamID, week); err != nil {
				return err
			}
			for _, playerID := range players[ti] {
				if _, err := tx.Exec(`
					INSERT INTO lineup_plans (season_id, team_id, week_number, player_id, is_sub)
					VALUES (?, ?, ?, ?, 0)`, seasonID, teamID, week, playerID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func fixtureMatchesForWeek(week int) []fixtureMatch {
	tableA, tableB := "1,2", "3,4"
	if week == 5 {
		tableA, tableB = "5,6", "7,8"
	}
	return []fixtureMatch{
		{Week: week, Number: week*2 - 1, HomeTeam: 0, AwayTeam: 1, Table: tableA, RoundScores: fixtureScores(week, 0)},
		{Week: week, Number: week * 2, HomeTeam: 2, AwayTeam: 3, Table: tableB, RoundScores: fixtureScores(week, 1)},
	}
}

func fixtureScores(week, matchIndex int) [9][6]int {
	var z [9][6]int
	if week == 1 {
		return z
	}
	if week == 2 {
		return [9][6]int{
			{10, 0, 10, 0, 0, 0}, {10, 5, 0, 0, 0, 0}, {5, 10, 10, 7, 0, 0},
			{0, 0, 0, 0, 0, 0}, {10, 6, 10, 4, 0, 0}, {0, 0, 0, 0, 0, 0},
			{10, 1, 10, 2, 0, 0}, {0, 0, 0, 0, 0, 0}, {4, 10, 0, 0, 0, 0},
		}
	}
	if week == 3 {
		return [9][6]int{
			{10, 4, 10, 2, 10, 7}, {3, 10, 10, 6, 2, 10}, {10, 5, 10, 1, 6, 10},
			{7, 10, 10, 7, 10, 3}, {10, 0, 10, 2, 10, 5}, {2, 10, 4, 10, 10, 7},
			{10, 6, 10, 5, 10, 1}, {1, 10, 10, 4, 10, 2}, {6, 10, 10, 7, 5, 10},
		}
	}
	if week == 4 {
		return [9][6]int{
			{10, 7, 7, 10, 10, 7}, {10, 0, 10, 0, 0, 0}, {6, 10, 10, 7, 7, 10},
			{10, 7, 7, 10, 10, 7}, {5, 10, 10, 6, 10, 4}, {10, 4, 3, 10, 10, 6},
			{7, 10, 10, 7, 10, 7}, {10, 2, 10, 1, 0, 0}, {4, 10, 10, 4, 10, 5},
		}
	}
	return [9][6]int{
		{10, 3, 10, 7, 0, 0}, {7, 10, 10, 7, 10, 6}, {10, 0, 10, 0, 0, 0},
		{2, 10, 5, 10, 10, 7}, {10, 6, 10, 3, 4, 10}, {0, 0, 0, 0, 0, 0},
		{10, 7, 7, 10, 10, 4}, {10, 1, 10, 2, 0, 0}, {6, 10, 10, 6, 10, 5},
	}
}

func upsertFixtureMatch(tx *sql.Tx, seasonID int64, teams []int64, fm fixtureMatch) (int64, error) {
	homeID := teams[fm.HomeTeam]
	awayID := teams[fm.AwayTeam]
	matchDate := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC).AddDate(0, 0, (fm.Week-1)*7).Format("2006-01-02")

	var id int64
	err := tx.QueryRow(`
		SELECT id FROM matches
		WHERE season_id=? AND week_number=? AND home_team_id=? AND away_team_id=? AND match_number=?`,
		seasonID, fm.Week, homeID, awayID, fm.Number).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(`
			INSERT INTO matches
			    (season_id, home_team_id, away_team_id, match_date, week_number, match_number, table_numbers, completed, week_closed)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0)`,
			seasonID, homeID, awayID, matchDate, fm.Week, fm.Number, fm.Table)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`
		UPDATE matches
		SET match_date=?, table_numbers=?, completed=0, week_closed=0
		WHERE id=?`, matchDate, fm.Table, id)
	return id, err
}

func refreshFixtureScores(tx *sql.Tx, matchID int64, teams []int64, players [][]int64, fm fixtureMatch) error {
	if _, err := tx.Exec(`DELETE FROM round_results WHERE match_id=?`, matchID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM match_results WHERE match_id=?`, matchID); err != nil {
		return err
	}

	homePlayers := players[fm.HomeTeam]
	awayPlayers := players[fm.AwayTeam]
	homeHC := fixtureTeams()[fm.HomeTeam].Players
	awayHC := fixtureTeams()[fm.AwayTeam].Players

	type tally struct{ gw, gl, diff int }
	tallies := map[int64]*tally{}
	ensure := func(pid int64) *tally {
		if tallies[pid] == nil {
			tallies[pid] = &tally{}
		}
		return tallies[pid]
	}

	hasScore := false
	for slot, scores := range fm.RoundScores {
		roundNumber := slot/3 + 1
		pair := slot % 3
		if scores == [6]int{} {
			continue
		}
		hp, ap := homePlayers[pair], awayPlayers[pair]
		hhc, ahc := homeHC[pair].Handicap, awayHC[pair].Handicap
		spot := logic.CalcSpotM(hhc, ahc, logic.Multiplier)
		if _, err := tx.Exec(`
			INSERT INTO round_results
			    (match_id, round_number, home_player_id, away_player_id,
			     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
			     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			matchID, roundNumber, hp, ap,
			scores[0], scores[1], scores[2], scores[3], scores[4], scores[5],
			hhc, ahc, spot.Pts, spot.To); err != nil {
			return err
		}
		for i := 0; i < 3; i++ {
			h, a := scores[i*2], scores[i*2+1]
			switch {
			case h == 10 && a != 10:
				hasScore = true
				ensure(hp).gw++
				ensure(ap).gl++
				ensure(hp).diff += h - a
				ensure(ap).diff += a - h
			case a == 10 && h != 10:
				hasScore = true
				ensure(ap).gw++
				ensure(hp).gl++
				ensure(hp).diff += h - a
				ensure(ap).diff += a - h
			}
		}
	}

	if hasScore {
		for pid, t := range tallies {
			teamID := teams[fm.HomeTeam]
			for _, awayPid := range awayPlayers {
				if pid == awayPid {
					teamID = teams[fm.AwayTeam]
				}
			}
			if _, err := tx.Exec(`
				INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
				VALUES (?, ?, ?, ?, ?, ?)`,
				matchID, pid, teamID, t.gw, t.gl, t.diff); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
		return err
	}
	_, err := tx.Exec(`UPDATE matches SET completed=0 WHERE id=?`, matchID)
	return err
}
