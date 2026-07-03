package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/seasons"
	"league_app/models"
)

// CountParticipatingTeams returns the effective team count for bye-request validation.
// For managed seasons (or legacy seasons with season_teams rows), uses season_teams count.
// Falls back to league team count for legacy seasons with no season_teams rows.
func (s *SeasonStore) CountParticipatingTeams(ctx context.Context, seasonID, leagueID int64, teamsManaged bool) (int, error) {
	var stCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM season_teams WHERE season_id=?`, seasonID,
	).Scan(&stCount); err != nil {
		return 0, fmt.Errorf("count season_teams %d: %w", seasonID, err)
	}
	if teamsManaged || stCount > 0 {
		return stCount, nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM teams WHERE league_id=?`, leagueID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count league teams %d: %w", leagueID, err)
	}
	return n, nil
}

// CheckTeamInSeason reports whether teamID is registered in seasonID via season_teams.
func (s *SeasonStore) CheckTeamInSeason(ctx context.Context, seasonID, teamID int64) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM season_teams WHERE season_id=? AND team_id=?`, seasonID, teamID,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("check team in season %d/%d: %w", seasonID, teamID, err)
	}
	return n > 0, nil
}

// HasDuplicateBye reports whether a bye request already exists for the season/team/week combo.
func (s *SeasonStore) HasDuplicateBye(ctx context.Context, seasonID, teamID int64, weekNumber int) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bye_requests WHERE season_id=? AND team_id=? AND week_number=?`,
		seasonID, teamID, weekNumber,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("check dup bye %d/%d/%d: %w", seasonID, teamID, weekNumber, err)
	}
	return n > 0, nil
}

// InsertByeRequest inserts a new bye request and returns the created record with team name.
func (s *SeasonStore) InsertByeRequest(ctx context.Context, seasonID, teamID int64, weekNumber int, reason string) (models.ByeRequest, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO bye_requests (season_id, team_id, week_number, reason) VALUES (?,?,?,?)`,
		seasonID, teamID, weekNumber, reason)
	if err != nil {
		return models.ByeRequest{}, fmt.Errorf("insert bye request %d/%d: %w", seasonID, teamID, err)
	}
	id, _ := res.LastInsertId()
	return s.GetByeRequest(ctx, seasonID, id)
}

// GetByeRequest returns a bye request scoped to the season.
// Returns ErrByeNotFound (wrapped) when not found in the season.
func (s *SeasonStore) GetByeRequest(ctx context.Context, seasonID, byeID int64) (models.ByeRequest, error) {
	var b models.ByeRequest
	var approved int
	err := s.db.QueryRowContext(ctx,
		`SELECT br.id, br.season_id, br.team_id, COALESCE(t.name,''),
		        br.week_number, br.reason, br.approved
		 FROM bye_requests br LEFT JOIN teams t ON t.id=br.team_id
		 WHERE br.id=? AND br.season_id=?`, byeID, seasonID,
	).Scan(&b.ID, &b.SeasonID, &b.TeamID, &b.TeamName, &b.WeekNumber, &b.Reason, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ByeRequest{}, fmt.Errorf("bye %d: %w", byeID, seasons.ErrByeNotFound)
	}
	if err != nil {
		return models.ByeRequest{}, fmt.Errorf("get bye request %d: %w", byeID, err)
	}
	b.Approved = approved == 1
	return b, nil
}

// HasByeConflict reports whether another bye request (not excludeByeID) is already
// approved for the same season+week.
func (s *SeasonStore) HasByeConflict(ctx context.Context, seasonID int64, weekNumber int, excludeByeID int64) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bye_requests WHERE season_id=? AND week_number=? AND approved=1 AND id!=?`,
		seasonID, weekNumber, excludeByeID,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("check bye conflict %d/week%d: %w", seasonID, weekNumber, err)
	}
	return n > 0, nil
}

// SetByeApproval updates the approved flag on a bye request and returns the updated record.
// Returns ErrByeNotFound (wrapped) when the request is not found in the season.
func (s *SeasonStore) SetByeApproval(ctx context.Context, seasonID, byeID int64, approved bool) (models.ByeRequest, error) {
	app := 0
	if approved {
		app = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE bye_requests SET approved=? WHERE id=? AND season_id=?`, app, byeID, seasonID)
	if err != nil {
		return models.ByeRequest{}, fmt.Errorf("set bye approval %d: %w", byeID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ByeRequest{}, fmt.Errorf("bye %d: %w", byeID, seasons.ErrByeNotFound)
	}
	return s.GetByeRequest(ctx, seasonID, byeID)
}
