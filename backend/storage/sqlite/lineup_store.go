package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// LineupStore implements matches.LineupStore using SQLite.
type LineupStore struct{ db *sql.DB }

// NewLineupStore returns a LineupStore backed by the given database.
func NewLineupStore(db *sql.DB) *LineupStore { return &LineupStore{db: db} }

// ListLineupPlans returns lineup plans for a season, optionally filtered by
// week and/or team. Results are ordered by team then insertion order.
func (s *LineupStore) ListLineupPlans(ctx context.Context, req matches.ListLineupPlansRequest) ([]models.LineupPlan, error) {
	q := `SELECT lp.id, lp.season_id, lp.team_id, t.name,
	             lp.player_id, p.first_name || ' ' || p.last_name, p.handicap,
	             lp.week_number, lp.is_sub, lp.sub_for_id
	      FROM lineup_plans lp
	      JOIN teams t ON t.id = lp.team_id
	      JOIN players p ON p.id = lp.player_id
	      WHERE lp.season_id = ?`
	args := []any{req.SeasonID}
	if req.WeekNumber != 0 {
		q += ` AND lp.week_number = ?`
		args = append(args, req.WeekNumber)
	}
	if req.TeamID != 0 {
		q += ` AND lp.team_id = ?`
		args = append(args, req.TeamID)
	}
	q += ` ORDER BY lp.team_id, lp.id`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plans []models.LineupPlan
	for rows.Next() {
		var lp models.LineupPlan
		var isSub int
		if err := rows.Scan(&lp.ID, &lp.SeasonID, &lp.TeamID, &lp.TeamName,
			&lp.PlayerID, &lp.PlayerName, &lp.Handicap,
			&lp.WeekNumber, &isSub, &lp.SubForID); err != nil {
			return nil, err
		}
		lp.IsSub = isSub == 1
		plans = append(plans, lp)
	}
	return plans, rows.Err()
}

// SaveTeamLineup atomically deletes all existing lineup slots for the
// given season/team/week and inserts the new player set.
// Zero player IDs are silently skipped (treated as empty slots).
func (s *LineupStore) SaveTeamLineup(ctx context.Context, req matches.SaveLineupRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=?`,
		req.SeasonID, req.TeamID, req.WeekNumber); err != nil {
		return err
	}
	for _, pid := range req.PlayerIDs {
		if pid == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO lineup_plans (season_id, team_id, week_number, player_id, is_sub) VALUES (?,?,?,?,0)`,
			req.SeasonID, req.TeamID, req.WeekNumber, pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteLineupPlan removes a lineup plan by ID.
// Deleting a non-existent plan is not an error.
func (s *LineupStore) DeleteLineupPlan(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM lineup_plans WHERE id=?`, id)
	return err
}

// GetLineupPlan returns one lineup plan row by id. Returns domainerr.NotFound
// when no row matches.
func (s *LineupStore) GetLineupPlan(ctx context.Context, id int64) (models.LineupPlan, error) {
	var lp models.LineupPlan
	var isSub int
	err := s.db.QueryRowContext(ctx, `
		SELECT lp.id, lp.season_id, lp.team_id, t.name,
		       lp.player_id, p.first_name || ' ' || p.last_name, p.handicap,
		       lp.week_number, lp.is_sub, lp.sub_for_id
		FROM lineup_plans lp
		JOIN teams t ON t.id = lp.team_id
		JOIN players p ON p.id = lp.player_id
		WHERE lp.id = ?`, id).
		Scan(&lp.ID, &lp.SeasonID, &lp.TeamID, &lp.TeamName,
			&lp.PlayerID, &lp.PlayerName, &lp.Handicap,
			&lp.WeekNumber, &isSub, &lp.SubForID)
	if err == sql.ErrNoRows {
		return models.LineupPlan{}, domainerr.New("LINEUP_PLAN_NOT_FOUND", domainerr.NotFound, "lineup plan not found")
	}
	if err != nil {
		return models.LineupPlan{}, fmt.Errorf("get lineup plan: %w", err)
	}
	lp.IsSub = isSub == 1
	return lp, nil
}

// FindMatchID returns the id of the match where teamID plays (home or away)
// in seasonID/weekNumber, or found=false when no such match exists yet.
func (s *LineupStore) FindMatchID(ctx context.Context, seasonID, teamID, weekNumber int64) (int64, bool, error) {
	var matchID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM matches
		WHERE season_id = ? AND week_number = ? AND (home_team_id = ? OR away_team_id = ?)
		LIMIT 1`, seasonID, weekNumber, teamID, teamID).Scan(&matchID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find match for team/week: %w", err)
	}
	return matchID, true, nil
}

// SetSubstitute updates the lineup slot to the substitute player, setting
// is_sub=1 and sub_for_id=req.OriginalPlayerID. Returns the updated row with
// names/handicap populated for immediate display.
func (s *LineupStore) SetSubstitute(ctx context.Context, req matches.SetSubstituteRequest) (models.LineupPlan, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE lineup_plans SET player_id = ?, is_sub = 1, sub_for_id = ? WHERE id = ?`,
		req.SubstitutePlayerID, req.OriginalPlayerID, req.LineupPlanID); err != nil {
		return models.LineupPlan{}, fmt.Errorf("set substitute: %w", err)
	}
	return s.GetLineupPlan(ctx, req.LineupPlanID)
}

// PlayerInMatchLineup returns true when playerID already has a lineup_plans
// row for seasonID/weekNumber under either homeTeamID or awayTeamID, other
// than excludePlanID (the slot currently being substituted).
func (s *LineupStore) PlayerInMatchLineup(ctx context.Context, seasonID, weekNumber, homeTeamID, awayTeamID, excludePlanID, playerID int64) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM lineup_plans
			WHERE season_id = ? AND week_number = ? AND team_id IN (?, ?)
			  AND player_id = ? AND id != ?
		)`, seasonID, weekNumber, homeTeamID, awayTeamID, playerID, excludePlanID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("player in match lineup: %w", err)
	}
	return exists == 1, nil
}

// ClearSubstitute reverts a substituted slot back to its original player
// (is_sub=0, sub_for_id=NULL). Returns domainerr.InvalidInput when the slot
// is not currently substituted.
func (s *LineupStore) ClearSubstitute(ctx context.Context, id int64) (models.LineupPlan, error) {
	plan, err := s.GetLineupPlan(ctx, id)
	if err != nil {
		return models.LineupPlan{}, err
	}
	if !plan.IsSub || plan.SubForID == nil {
		return models.LineupPlan{}, domainerr.New("SUB_NOT_ACTIVE", domainerr.InvalidInput, "this slot is not currently substituted")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE lineup_plans SET player_id = ?, is_sub = 0, sub_for_id = NULL WHERE id = ?`,
		*plan.SubForID, id); err != nil {
		return models.LineupPlan{}, fmt.Errorf("clear substitute: %w", err)
	}
	return s.GetLineupPlan(ctx, id)
}
