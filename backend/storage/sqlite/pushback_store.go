package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domains/matches"
)

// ApplyPushback executes the precomputed shift plan atomically.
// Completed matches are never touched. No validation is performed here;
// the service validates before calling.
func (s *PushbackStore) ApplyPushback(ctx context.Context, input matches.PushbackApplyInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pushback apply: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	modifier := fmt.Sprintf("+%d days", input.DayShift)
	for _, id := range input.ShiftedIDs {
		_, err = tx.ExecContext(ctx, `
			UPDATE matches
			SET    week_number = week_number + ?,
			       match_date  = CASE WHEN match_date IS NOT NULL
			                         THEN date(match_date, ?)
			                         ELSE NULL END
			WHERE  id = ?
			  AND  season_id = ?
			  AND  COALESCE(completed, 0) = 0`,
			input.WeeksToAdd, modifier, id, input.SeasonID)
		if err != nil {
			return fmt.Errorf("pushback apply: shift match %d: %w", id, err)
		}
	}

	if input.NewEndDate != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE seasons SET end_date = ?, schedule_stale = 0 WHERE id = ?`,
			*input.NewEndDate, input.SeasonID)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE seasons SET schedule_stale = 0 WHERE id = ?`,
			input.SeasonID)
	}
	if err != nil {
		return fmt.Errorf("pushback apply: update season %d: %w", input.SeasonID, err)
	}

	return tx.Commit()
}

// PushbackStore implements matches.PushbackStore against a SQLite database.
type PushbackStore struct {
	db *sql.DB
}

// NewPushbackStore returns a PushbackStore backed by the given connection.
func NewPushbackStore(db *sql.DB) *PushbackStore {
	return &PushbackStore{db: db}
}

// SeasonExists reports whether a season row exists for the given ID.
func (s *PushbackStore) SeasonExists(ctx context.Context, seasonID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM seasons WHERE id=?`, seasonID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("pushback: season exists %d: %w", seasonID, err)
	}
	return n > 0, nil
}

// HasClosedWeeksAtOrAfter reports whether any league_weeks row for the season
// has status "closed" with week_number >= cutoffWeek.
func (s *PushbackStore) HasClosedWeeksAtOrAfter(ctx context.Context, seasonID int64, cutoffWeek int) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM league_weeks
		 WHERE season_id=? AND status=? AND week_number>=?`,
		seasonID, matches.WeekStatusClosed, cutoffWeek,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("pushback: has closed weeks at or after %d/%d: %w", seasonID, cutoffWeek, err)
	}
	return n > 0, nil
}

// GetPushbackMatches returns all matches for the season with the columns needed
// for pushback preview, ordered by week_number, id.
func (s *PushbackStore) GetPushbackMatches(ctx context.Context, seasonID int64) ([]matches.PushbackMatchRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,
		       week_number,
		       strftime('%Y-%m-%d', match_date),
		       COALESCE(completed, 0),
		       COALESCE(home_team_id, 0),
		       COALESCE(away_team_id, 0)
		FROM   matches
		WHERE  season_id=?
		ORDER  BY week_number, id`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("pushback: get matches %d: %w", seasonID, err)
	}
	defer rows.Close()

	var out []matches.PushbackMatchRow
	for rows.Next() {
		var r matches.PushbackMatchRow
		var rawDate sql.NullString
		var completed int
		if err := rows.Scan(&r.ID, &r.WeekNumber, &rawDate, &completed, &r.HomeTeamID, &r.AwayTeamID); err != nil {
			return nil, fmt.Errorf("pushback: scan match: %w", err)
		}
		r.Completed = completed == 1
		if rawDate.Valid && rawDate.String != "" {
			d := rawDate.String
			r.MatchDate = &d
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
