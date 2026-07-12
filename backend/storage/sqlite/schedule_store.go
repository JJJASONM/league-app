package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/matches"
)

// ScheduleStore implements matches.ScheduleStore against a SQLite database.
type ScheduleStore struct {
	db *sql.DB
}

// NewScheduleStore returns a ScheduleStore backed by the given connection.
func NewScheduleStore(db *sql.DB) *ScheduleStore {
	return &ScheduleStore{db: db}
}

// GetScheduleSeasonMeta returns the season's league_id and teams_managed flag.
// Returns matches.ErrSeasonNotFound when the season does not exist.
func (s *ScheduleStore) GetScheduleSeasonMeta(ctx context.Context, seasonID int64) (matches.ScheduleSeasonMeta, error) {
	var meta matches.ScheduleSeasonMeta
	var managed int
	err := s.db.QueryRowContext(ctx,
		`SELECT league_id, COALESCE(teams_managed,0) FROM seasons WHERE id=?`, seasonID,
	).Scan(&meta.LeagueID, &managed)
	if errors.Is(err, sql.ErrNoRows) {
		return matches.ScheduleSeasonMeta{}, fmt.Errorf("season %d: %w", seasonID, matches.ErrSeasonNotFound)
	}
	if err != nil {
		return matches.ScheduleSeasonMeta{}, fmt.Errorf("get schedule season meta %d: %w", seasonID, err)
	}
	meta.TeamsManaged = managed == 1
	return meta, nil
}

// LoadByeRequests returns approved bye requests with a specific week number for the season.
func (s *ScheduleStore) LoadByeRequests(ctx context.Context, seasonID int64) (map[int]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT team_id, week_number FROM bye_requests WHERE season_id=? AND approved=1 AND week_number > 0`,
		seasonID)
	if err != nil {
		return nil, fmt.Errorf("load bye requests %d: %w", seasonID, err)
	}
	defer rows.Close()
	byeByWeek := make(map[int]int64)
	for rows.Next() {
		var tid int64
		var wn int
		if err := rows.Scan(&tid, &wn); err != nil {
			return nil, fmt.Errorf("scan bye request: %w", err)
		}
		byeByWeek[wn] = tid
	}
	return byeByWeek, rows.Err()
}

// LoadTeamIDsFromHistory returns the distinct team IDs that appeared in matches
// for the given season (legacy from_season_id path).
func (s *ScheduleStore) LoadTeamIDsFromHistory(ctx context.Context, fromSeasonID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT home_team_id FROM matches WHERE season_id=? AND home_team_id IS NOT NULL
		UNION
		SELECT DISTINCT away_team_id FROM matches WHERE season_id=? AND away_team_id IS NOT NULL`,
		fromSeasonID, fromSeasonID)
	if err != nil {
		return nil, fmt.Errorf("load teams from history %d: %w", fromSeasonID, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan team id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// LoadTeamIDsForSchedule returns the team IDs to use for schedule generation.
// When teamsManaged is true, returns only season_teams for seasonID.
// When teamsManaged is false, returns season_teams if any exist, otherwise all league teams.
func (s *ScheduleStore) LoadTeamIDsForSchedule(ctx context.Context, seasonID, leagueID int64, teamsManaged bool) ([]int64, error) {
	var stCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM season_teams WHERE season_id=?`, seasonID,
	).Scan(&stCount); err != nil {
		return nil, fmt.Errorf("count season teams %d: %w", seasonID, err)
	}

	var rows *sql.Rows
	var err error
	if teamsManaged || stCount > 0 {
		rows, err = s.db.QueryContext(ctx,
			`SELECT team_id FROM season_teams WHERE season_id=? ORDER BY id`, seasonID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT t.id FROM teams t
			JOIN seasons s ON s.league_id = t.league_id
			WHERE s.id=? ORDER BY t.id`, seasonID)
	}
	if err != nil {
		return nil, fmt.Errorf("load team ids for schedule %d: %w", seasonID, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan team id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// HasClosedWeeks reports whether any league_weeks row for the season has
// status "closed".
func (s *ScheduleStore) HasClosedWeeks(ctx context.Context, seasonID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM league_weeks WHERE season_id=? AND status='closed'`, seasonID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("has closed weeks %d: %w", seasonID, err)
	}
	return n > 0, nil
}

// SaveGeneratedSchedule atomically deletes unplayed matches, inserts new match
// rows, and updates the season's schedule metadata.
func (s *ScheduleStore) SaveGeneratedSchedule(ctx context.Context, req matches.SaveScheduleRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save schedule: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete only unplayed matches so completed results are preserved.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM matches WHERE season_id=? AND completed=0`, req.SeasonID,
	); err != nil {
		return fmt.Errorf("save schedule: delete unplayed: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number) VALUES (?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("save schedule: prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range req.Entries {
		var hid, aid any
		if e.HomeTeamID != 0 {
			hid = e.HomeTeamID
		}
		if e.AwayTeamID != 0 {
			aid = e.AwayTeamID
		}
		var matchDate any
		if e.MatchDate != "" {
			matchDate = e.MatchDate
		}
		if _, err := stmt.ExecContext(ctx, req.SeasonID, hid, aid, matchDate, e.WeekNumber); err != nil {
			return fmt.Errorf("save schedule: insert match: %w", err)
		}
	}

	var endDate any
	if req.EndDate != "" {
		endDate = req.EndDate
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE seasons SET schedule_type=?, num_weeks=?, end_date=?, schedule_stale=0 WHERE id=?`,
		req.ScheduleType, req.NumWeeks, endDate, req.SeasonID,
	); err != nil {
		return fmt.Errorf("save schedule: update season: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save schedule: commit: %w", err)
	}
	return nil
}
