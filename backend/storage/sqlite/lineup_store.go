package sqlite

import (
	"context"
	"database/sql"

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
