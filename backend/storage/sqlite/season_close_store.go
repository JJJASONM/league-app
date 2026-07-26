package sqlite

import (
	"context"
	"fmt"
)

// GetSeasonMissingCount returns the number of matches for the season with
// completed=0 (no results entered). Returns 0 when no matches exist.
func (s *SeasonStore) GetSeasonMissingCount(ctx context.Context, seasonID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM matches WHERE season_id=? AND completed=0`, seasonID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get missing count season %d: %w", seasonID, err)
	}
	return count, nil
}

// CloseSeasonRecord stores the final standings snapshot, sets closed_at, and
// (when setInactive is true) sets active=0 in a single UPDATE.
func (s *SeasonStore) CloseSeasonRecord(ctx context.Context, seasonID int64, snapshotJSON string, setInactive bool) error {
	var err error
	if setInactive {
		_, err = s.db.ExecContext(ctx,
			`UPDATE seasons
			    SET closed_at=CURRENT_TIMESTAMP,
			        final_standings_snapshot=?,
			        active=0
			  WHERE id=?`,
			snapshotJSON, seasonID)
	} else {
		_, err = s.db.ExecContext(ctx,
			`UPDATE seasons
			    SET closed_at=CURRENT_TIMESTAMP,
			        final_standings_snapshot=?
			  WHERE id=?`,
			snapshotJSON, seasonID)
	}
	if err != nil {
		return fmt.Errorf("close season record %d: %w", seasonID, err)
	}
	return nil
}
