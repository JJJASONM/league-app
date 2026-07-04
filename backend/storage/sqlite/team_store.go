package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/teams"
	"league_app/models"
)

// TeamStore implements teams.TeamStore against a SQLite database.
type TeamStore struct {
	db *sql.DB
}

// NewTeamStore returns a TeamStore backed by db.
func NewTeamStore(db *sql.DB) *TeamStore {
	return &TeamStore{db: db}
}

// teamCols is the SELECT column list for full team rows.
// Requires the query to alias teams as t.
// Surrounded by single spaces so it can be concatenated with SELECT and FROM.
const teamCols = ` t.id, t.league_id, t.name, COALESCE(t.team_number,''), t.captain_id, t.created_at `
const teamFrom = ` FROM teams t `

func scanTeam(row interface{ Scan(...any) error }) (models.Team, error) {
	var t models.Team
	err := row.Scan(&t.ID, &t.LeagueID, &t.Name, &t.TeamNumber, &t.CaptainID, &t.CreatedAt)
	return t, err
}

func (s *TeamStore) ListTeams(ctx context.Context, leagueID *int64) ([]models.Team, error) {
	var rows *sql.Rows
	var err error
	if leagueID != nil {
		rows, err = s.db.QueryContext(ctx,
			`SELECT`+teamCols+teamFrom+`WHERE t.league_id = ? ORDER BY t.name`, *leagueID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT`+teamCols+teamFrom+`ORDER BY t.league_id, t.name`)
	}
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()
	out := []models.Team{}
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *TeamStore) GetTeam(ctx context.Context, id int64) (models.Team, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+teamCols+teamFrom+`WHERE t.id = ?`, id)
	t, err := scanTeam(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Team{}, fmt.Errorf("team %d: %w", id, teams.ErrNotFound)
	}
	if err != nil {
		return models.Team{}, fmt.Errorf("get team %d: %w", id, err)
	}

	// Embed players — direct SQL cross-table read (store-level, same pattern as
	// player_store.go joining teams). COALESCE guards the nullable player_number column.
	prows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(player_number,''), first_name, last_name,
		        first_name || ' ' || last_name, handicap
		 FROM players WHERE team_id = ? ORDER BY player_number`, id)
	if err != nil {
		return models.Team{}, fmt.Errorf("get team %d players: %w", id, err)
	}
	defer prows.Close()
	for prows.Next() {
		var p models.Player
		if err := prows.Scan(&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name, &p.Handicap); err != nil {
			return models.Team{}, fmt.Errorf("scan team player: %w", err)
		}
		p.TeamID = &t.ID
		p.LeagueID = t.LeagueID
		t.Players = append(t.Players, p)
	}
	return t, prows.Err()
}

// CreateTeam inserts a team row and returns the stored fields.
// TeamNumber is not inserted — it is managed by the season workflow.
func (s *TeamStore) CreateTeam(ctx context.Context, input teams.CreateTeamInput) (models.Team, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO teams (league_id, name) VALUES (?,?)`,
		input.LeagueID, input.Name)
	if err != nil {
		return models.Team{}, fmt.Errorf("create team: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Team{
		ID:       id,
		LeagueID: input.LeagueID,
		Name:     input.Name,
	}, nil
}

// UpdateTeam updates the mutable team fields (name and captain).
// TeamNumber is intentionally excluded from the UPDATE — it is not editable via the team API.
// No error is returned when the row does not exist (UPDATE affects 0 rows).
func (s *TeamStore) UpdateTeam(ctx context.Context, id int64, input teams.UpdateTeamInput) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE teams SET name=?, captain_id=? WHERE id=?`,
		input.Name, input.CaptainID, id)
	if err != nil {
		return fmt.Errorf("update team %d: %w", id, err)
	}
	return nil
}

// DeleteTeam nulls player team assignments, then removes the team row.
// The player nulling is a store-level side effect documented as acceptable for this
// phase — the same pattern used by season_team_store.go when removing teams from a season.
// No error is returned when the row does not exist.
func (s *TeamStore) DeleteTeam(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE players SET team_id=NULL WHERE team_id=?`, id); err != nil {
		return fmt.Errorf("null player team_id for team %d: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM teams WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete team %d: %w", id, err)
	}
	return nil
}
