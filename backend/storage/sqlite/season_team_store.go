package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/seasons"
	"league_app/models"
)

// seasonTeamCols is the shared SELECT list for season team queries.
// Mirrors seasonTeamSelect in handlers/api.go; duplicated to keep each layer independent.
const seasonTeamCols = `
	SELECT st.id, st.season_id, st.team_id, t.name,
	       COALESCE(t.team_number,''),
	       CASE WHEN st.season_name != '' THEN st.season_name ELSE t.name END,
	       st.captain_id,
	       COALESCE(cp.first_name||' '||cp.last_name, ''),
	       (SELECT COUNT(*) FROM season_rosters sr
	        WHERE sr.season_id = st.season_id AND sr.team_id = st.team_id)
	FROM season_teams st
	JOIN teams t ON t.id = st.team_id
	LEFT JOIN players cp ON cp.id = st.captain_id`

func scanSeasonTeamRow(row interface{ Scan(...any) error }) (models.SeasonTeam, error) {
	var st models.SeasonTeam
	err := row.Scan(&st.ID, &st.SeasonID, &st.TeamID, &st.TeamName, &st.TeamNumber,
		&st.SeasonName, &st.CaptainID, &st.CaptainName, &st.RosterCount)
	return st, err
}

// ListSeasonTeams returns all teams registered in a season with full metadata,
// ordered by insertion.
func (s *SeasonStore) ListSeasonTeams(ctx context.Context, seasonID int64) ([]models.SeasonTeam, error) {
	rows, err := s.db.QueryContext(ctx,
		seasonTeamCols+` WHERE st.season_id=? ORDER BY st.id`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list season teams %d: %w", seasonID, err)
	}
	defer rows.Close()
	var out []models.SeasonTeam
	for rows.Next() {
		st, err := scanSeasonTeamRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan season team: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// GetTeamLeagueID returns the league_id for the given team.
// Returns ErrNotFound (wrapped) when the team does not exist.
func (s *SeasonStore) GetTeamLeagueID(ctx context.Context, teamID int64) (int64, error) {
	var lid int64
	err := s.db.QueryRowContext(ctx,
		`SELECT league_id FROM teams WHERE id=?`, teamID,
	).Scan(&lid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("team %d: %w", teamID, seasons.ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("get team league %d: %w", teamID, err)
	}
	return lid, nil
}

// GetSeasonTeam returns the full SeasonTeam for a season+team.
// Returns ErrTeamNotInSeason (wrapped) when not registered.
func (s *SeasonStore) GetSeasonTeam(ctx context.Context, seasonID, teamID int64) (models.SeasonTeam, error) {
	row := s.db.QueryRowContext(ctx,
		seasonTeamCols+` WHERE st.season_id=? AND st.team_id=?`, seasonID, teamID)
	st, err := scanSeasonTeamRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.SeasonTeam{}, fmt.Errorf("team %d season %d: %w", teamID, seasonID, seasons.ErrTeamNotInSeason)
	}
	if err != nil {
		return models.SeasonTeam{}, fmt.Errorf("get season team %d/%d: %w", seasonID, teamID, err)
	}
	return st, nil
}

// AddSeasonTeamCopy registers teamID in seasonID by copying metadata and roster
// from fromSeasonID. When fromSeasonID==0 and teamsManaged is false, active players
// from players.team_id are used as the initial roster.
// Returns ErrTeamAlreadyInSeason when already registered.
// Returns ErrTeamNotInPriorSeason when the prior season was managed but the team
// was not registered in it.
func (s *SeasonStore) AddSeasonTeamCopy(ctx context.Context, seasonID, teamID, fromSeasonID int64, teamsManaged bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("add season team copy: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var seasonName string
	var captainID *int64

	if fromSeasonID > 0 {
		var prevManaged int
		tx.QueryRowContext(ctx,
			`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, fromSeasonID,
		).Scan(&prevManaged)
		if prevManaged == 1 {
			var inPrev int
			tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM season_teams WHERE season_id=? AND team_id=?`,
				fromSeasonID, teamID,
			).Scan(&inPrev)
			if inPrev == 0 {
				return fmt.Errorf("team %d prior season %d: %w", teamID, fromSeasonID, seasons.ErrTeamNotInPriorSeason)
			}
		}
		tx.QueryRowContext(ctx,
			`SELECT CASE WHEN season_name != '' THEN season_name ELSE t.name END, captain_id
			 FROM season_teams st JOIN teams t ON t.id=st.team_id
			 WHERE st.season_id=? AND st.team_id=?`,
			fromSeasonID, teamID,
		).Scan(&seasonName, &captainID)
	}
	if seasonName == "" {
		tx.QueryRowContext(ctx, `SELECT name FROM teams WHERE id=?`, teamID).Scan(&seasonName)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id) VALUES (?,?,?,?)`,
		seasonID, teamID, seasonName, captainID)
	if err != nil {
		return fmt.Errorf("insert season_team %d/%d: %w", seasonID, teamID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("team %d season %d: %w", teamID, seasonID, seasons.ErrTeamAlreadyInSeason)
	}

	var copiedFromRoster bool
	if fromSeasonID > 0 {
		rr, _ := tx.QueryContext(ctx,
			`SELECT player_id FROM season_rosters WHERE season_id=? AND team_id=?`, fromSeasonID, teamID)
		if rr != nil {
			for rr.Next() {
				var pid int64
				rr.Scan(&pid)
				tx.ExecContext(ctx,
					`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
					seasonID, teamID, pid)
				copiedFromRoster = true
			}
			rr.Close()
		}
	}
	if !copiedFromRoster && !teamsManaged {
		pr, _ := tx.QueryContext(ctx,
			`SELECT id FROM players WHERE team_id=? AND COALESCE(active,1)=1`, teamID)
		if pr != nil {
			for pr.Next() {
				var pid int64
				pr.Scan(&pid)
				tx.ExecContext(ctx,
					`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
					seasonID, teamID, pid)
			}
			pr.Close()
		}
	}

	return tx.Commit()
}

// AddSeasonTeamNew creates a brand-new team in leagueID and registers it in seasonID.
// Returns the new team's ID.
func (s *SeasonStore) AddSeasonTeamNew(ctx context.Context, seasonID, leagueID int64, name string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("add season team new: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `INSERT INTO teams (league_id, name) VALUES (?,?)`, leagueID, name)
	if err != nil {
		return 0, fmt.Errorf("insert team %q: %w", name, err)
	}
	teamID, _ := res.LastInsertId()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,?)`,
		seasonID, teamID, name); err != nil {
		return 0, fmt.Errorf("insert season_team %d/%d: %w", seasonID, teamID, err)
	}

	return teamID, tx.Commit()
}

// CheckPlayerOnSeasonRoster reports whether playerID is on teamID's season roster.
func (s *SeasonStore) CheckPlayerOnSeasonRoster(ctx context.Context, seasonID, teamID, playerID int64) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`,
		seasonID, teamID, playerID,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("check player roster %d/%d/%d: %w", seasonID, teamID, playerID, err)
	}
	return n > 0, nil
}

// UpdateSeasonTeamMeta updates the season_name and captain_id for a registered team.
// Returns ErrTeamNotInSeason (wrapped) when the team is not registered.
func (s *SeasonStore) UpdateSeasonTeamMeta(ctx context.Context, seasonID, teamID int64, seasonName string, captainID *int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE season_teams SET season_name=?, captain_id=? WHERE season_id=? AND team_id=?`,
		seasonName, captainID, seasonID, teamID)
	if err != nil {
		return fmt.Errorf("update season team meta %d/%d: %w", seasonID, teamID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("team %d season %d: %w", teamID, seasonID, seasons.ErrTeamNotInSeason)
	}
	return nil
}

// RemoveSeasonTeam deletes a team from the season. When the team has no match
// history and no other season registrations, the team record and player
// assignments are also deleted.
// Returns ErrTeamNotInSeason (wrapped) when the team is not registered.
func (s *SeasonStore) RemoveSeasonTeam(ctx context.Context, seasonID, teamID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remove season team: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	tx.ExecContext(ctx, `DELETE FROM season_rosters WHERE season_id=? AND team_id=?`, seasonID, teamID)
	res, err := tx.ExecContext(ctx, `DELETE FROM season_teams WHERE season_id=? AND team_id=?`, seasonID, teamID)
	if err != nil {
		return fmt.Errorf("delete season_team %d/%d: %w", seasonID, teamID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("team %d season %d: %w", teamID, seasonID, seasons.ErrTeamNotInSeason)
	}

	var matchCount int
	tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM matches WHERE home_team_id=? OR away_team_id=?`, teamID, teamID,
	).Scan(&matchCount)
	if matchCount == 0 {
		var otherSeason int
		tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM season_teams WHERE team_id=?`, teamID,
		).Scan(&otherSeason)
		if otherSeason == 0 {
			tx.ExecContext(ctx, `UPDATE players SET team_id=NULL WHERE team_id=?`, teamID)
			tx.ExecContext(ctx, `DELETE FROM teams WHERE id=?`, teamID)
		}
	}

	return tx.Commit()
}
