package sqlite

import (
	"context"
	"fmt"

	"league_app/models"
)

// ListSkippedWeeks returns all skipped weeks for a season, ordered by skip_date.
// Dates are normalized to YYYY-MM-DD at query time.
func (s *SeasonStore) ListSkippedWeeks(ctx context.Context, seasonID int64) ([]models.SkippedWeek, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, season_id, strftime('%Y-%m-%d', skip_date), COALESCE(reason,'')
		 FROM skipped_weeks WHERE season_id=? ORDER BY skip_date`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list skipped weeks %d: %w", seasonID, err)
	}
	defer rows.Close()
	var out []models.SkippedWeek
	for rows.Next() {
		var sw models.SkippedWeek
		if err := rows.Scan(&sw.ID, &sw.SeasonID, &sw.SkipDate, &sw.Reason); err != nil {
			return nil, fmt.Errorf("scan skipped week: %w", err)
		}
		out = append(out, sw)
	}
	return out, rows.Err()
}

// CreateSkippedWeek inserts a skipped week (INSERT OR IGNORE on unique
// (season_id, skip_date)) and returns the row whether newly inserted or pre-existing.
func (s *SeasonStore) CreateSkippedWeek(ctx context.Context, seasonID int64, skipDate, reason string) (models.SkippedWeek, error) {
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO skipped_weeks (season_id, skip_date, reason) VALUES (?,?,?)`,
		seasonID, skipDate, reason,
	); err != nil {
		return models.SkippedWeek{}, fmt.Errorf("insert skipped week %d/%s: %w", seasonID, skipDate, err)
	}
	var sw models.SkippedWeek
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, season_id, strftime('%Y-%m-%d', skip_date), COALESCE(reason,'')
		 FROM skipped_weeks WHERE season_id=? AND skip_date=?`, seasonID, skipDate,
	).Scan(&sw.ID, &sw.SeasonID, &sw.SkipDate, &sw.Reason); err != nil {
		return models.SkippedWeek{}, fmt.Errorf("fetch skipped week %d/%s: %w", seasonID, skipDate, err)
	}
	return sw, nil
}

// DeleteSkippedWeek removes a skipped week scoped to the season by id.
// No error is returned when the row does not exist.
func (s *SeasonStore) DeleteSkippedWeek(ctx context.Context, seasonID, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM skipped_weeks WHERE id=? AND season_id=?`, id, seasonID,
	); err != nil {
		return fmt.Errorf("delete skipped week %d/%d: %w", seasonID, id, err)
	}
	return nil
}
