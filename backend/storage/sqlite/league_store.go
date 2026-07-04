package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/leagues"
	"league_app/models"
)

// LeagueStore implements leagues.LeagueStore against a SQLite database.
type LeagueStore struct {
	db *sql.DB
}

// NewLeagueStore returns a LeagueStore backed by db.
func NewLeagueStore(db *sql.DB) *LeagueStore {
	return &LeagueStore{db: db}
}

// leagueCols is the SELECT column list for league rows.
// Surrounded by single spaces so it can be concatenated with SELECT and FROM.
const leagueCols = ` id, name, game_format, COALESCE(day_of_week,''), created_at `

func scanLeague(row interface{ Scan(...any) error }) (models.League, error) {
	var l models.League
	err := row.Scan(&l.ID, &l.Name, &l.GameFormat, &l.DayOfWeek, &l.CreatedAt)
	return l, err
}

func (s *LeagueStore) ListLeagues(ctx context.Context) ([]models.League, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT`+leagueCols+`FROM leagues ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list leagues: %w", err)
	}
	defer rows.Close()
	out := []models.League{}
	for rows.Next() {
		l, err := scanLeague(rows)
		if err != nil {
			return nil, fmt.Errorf("scan league: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *LeagueStore) GetLeague(ctx context.Context, id int64) (models.League, error) {
	row := s.db.QueryRowContext(ctx, `SELECT`+leagueCols+`FROM leagues WHERE id=?`, id)
	l, err := scanLeague(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.League{}, fmt.Errorf("league %d: %w", id, leagues.ErrNotFound)
	}
	if err != nil {
		return models.League{}, fmt.Errorf("get league %d: %w", id, err)
	}
	return l, nil
}

// CreateLeague inserts a league row and returns the stored fields without
// re-fetching created_at, matching the previous handler's response shape.
func (s *LeagueStore) CreateLeague(ctx context.Context, input leagues.CreateLeagueInput) (models.League, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO leagues (name, game_format, day_of_week) VALUES (?,?,?)`,
		input.Name, input.GameFormat, input.DayOfWeek)
	if err != nil {
		return models.League{}, fmt.Errorf("create league: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.League{
		ID:         id,
		Name:       input.Name,
		GameFormat: input.GameFormat,
		DayOfWeek:  input.DayOfWeek,
	}, nil
}

func (s *LeagueStore) UpdateLeague(ctx context.Context, id int64, input leagues.UpdateLeagueInput) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE leagues SET name=?, game_format=?, day_of_week=? WHERE id=?`,
		input.Name, input.GameFormat, input.DayOfWeek, id)
	if err != nil {
		return fmt.Errorf("update league %d: %w", id, err)
	}
	return nil
}

func (s *LeagueStore) DeleteLeague(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM leagues WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete league %d: %w", id, err)
	}
	return nil
}
