package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"league_app/backend/domains/matches"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// seasonFullCols is the SELECT column list for full season rows (includes
// teams_managed and activated_at; uses strftime so the driver returns TEXT
// for DATE columns rather than converting them to time.Time).
// Surrounded by single spaces so it can be concatenated with SELECT and FROM.
const seasonFullCols = ` id, league_id, name,` +
	` strftime('%Y-%m-%d', start_date), strftime('%Y-%m-%d', end_date),` +
	` active, schedule_type, COALESCE(num_weeks,0),` +
	` COALESCE(schedule_stale,0), COALESCE(teams_managed,0),` +
	` activated_at, created_at `

func scanFullSeason(row interface{ Scan(...any) error }) (models.Season, error) {
	var s models.Season
	var startDate, endDate, activatedAt sql.NullString
	var active, stale, managed int
	var createdAt time.Time
	err := row.Scan(
		&s.ID, &s.LeagueID, &s.Name,
		&startDate, &endDate,
		&active, &s.ScheduleType, &s.NumWeeks,
		&stale, &managed,
		&activatedAt, &createdAt,
	)
	if err != nil {
		return models.Season{}, err
	}
	s.Active = active == 1
	s.ScheduleStale = stale == 1
	s.TeamsManaged = managed == 1
	s.CreatedAt = createdAt
	if startDate.Valid && startDate.String != "" {
		s.StartDate = &startDate.String
	}
	if endDate.Valid && endDate.String != "" {
		s.EndDate = &endDate.String
	}
	if activatedAt.Valid && activatedAt.String != "" {
		s.ActivatedAt = &activatedAt.String
	}
	if s.ScheduleType == "" {
		s.ScheduleType = matches.ScheduleTypeDoubleRR
	}
	return s, nil
}

// ListSeasons returns all seasons, ordered by id DESC when a leagueID filter is
// applied, or by league_id, id DESC when listing all leagues.
func (s *SeasonStore) ListSeasons(ctx context.Context, leagueID *int64) ([]models.Season, error) {
	var rows *sql.Rows
	var err error
	q := `SELECT` + seasonFullCols + ` FROM seasons`
	if leagueID != nil {
		rows, err = s.db.QueryContext(ctx, q+` WHERE league_id=? ORDER BY id DESC`, *leagueID)
	} else {
		rows, err = s.db.QueryContext(ctx, q+` ORDER BY league_id, id DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("list seasons: %w", err)
	}
	defer rows.Close()
	out := []models.Season{}
	for rows.Next() {
		season, err := scanFullSeason(rows)
		if err != nil {
			return nil, fmt.Errorf("scan season: %w", err)
		}
		out = append(out, season)
	}
	return out, rows.Err()
}

// GetSeason returns the full season row by ID.
// Returns ErrNotFound (wrapped) when no row exists.
func (s *SeasonStore) GetSeason(ctx context.Context, seasonID int64) (models.Season, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+seasonFullCols+` FROM seasons WHERE id=?`, seasonID)
	season, err := scanFullSeason(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Season{}, fmt.Errorf("season %d: %w", seasonID, seasons.ErrNotFound)
	}
	if err != nil {
		return models.Season{}, fmt.Errorf("get season %d: %w", seasonID, err)
	}
	return season, nil
}

// CreateSeason inserts a new season record and returns the stored row.
// teams_managed is always set to 1 for all new seasons.
func (s *SeasonStore) CreateSeason(ctx context.Context, input seasons.CreateSeasonInput) (models.Season, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks, teams_managed)
		 VALUES (?,?,?,?,?,1)`,
		input.LeagueID, input.Name, input.StartDate, input.ScheduleType, input.NumWeeks)
	if err != nil {
		return models.Season{}, fmt.Errorf("create season: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetSeason(ctx, id)
}

// UpdateSeason updates the mutable fields for a season and returns the full stored row.
// Returns ErrNotFound (wrapped) when no row exists.
func (s *SeasonStore) UpdateSeason(ctx context.Context, seasonID int64, input seasons.UpdateSeasonInput) (models.Season, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE seasons SET name=?, start_date=?, schedule_type=?, num_weeks=? WHERE id=?`,
		input.Name, input.StartDate, input.ScheduleType, input.NumWeeks, seasonID)
	if err != nil {
		return models.Season{}, fmt.Errorf("update season %d: %w", seasonID, err)
	}
	return s.GetSeason(ctx, seasonID)
}

// DeleteSeason removes the season record by ID.
// No error is returned when the row does not exist.
func (s *SeasonStore) DeleteSeason(ctx context.Context, seasonID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM seasons WHERE id=?`, seasonID); err != nil {
		return fmt.Errorf("delete season %d: %w", seasonID, err)
	}
	return nil
}
