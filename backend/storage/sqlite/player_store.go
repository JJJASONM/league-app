package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/players"
	"league_app/models"
)

// PlayerStore implements players.PlayerStore against a SQLite database.
type PlayerStore struct {
	db *sql.DB
}

// NewPlayerStore returns a PlayerStore backed by db.
func NewPlayerStore(db *sql.DB) *PlayerStore {
	return &PlayerStore{db: db}
}

// playerCols is the SELECT column list for full player rows.
// Requires the query to alias players as p and LEFT JOIN teams as t.
// Surrounded by single spaces so it can be concatenated with SELECT and FROM.
const playerCols = ` p.id, COALESCE(p.player_number,''), p.first_name, p.last_name,` +
	` p.first_name || ' ' || p.last_name,` +
	` COALESCE(p.phone,''), COALESCE(p.email,''),` +
	` p.team_id, COALESCE(t.name,''), COALESCE(t.league_id,0),` +
	` p.handicap, p.admin_hold, COALESCE(p.active,1), COALESCE(p.note,''),` +
	` p.created_at `

const playerJoin = ` FROM players p LEFT JOIN teams t ON t.id = p.team_id `

func scanPlayer(row interface{ Scan(...any) error }) (models.Player, error) {
	var p models.Player
	var adminHold int
	var activeInt int
	err := row.Scan(
		&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name,
		&p.Phone, &p.Email, &p.TeamID, &p.TeamName, &p.LeagueID,
		&p.Handicap, &adminHold, &activeInt, &p.Note, &p.CreatedAt,
	)
	if err != nil {
		return models.Player{}, err
	}
	p.AdminHold = adminHold == 1
	p.Active = activeInt == 1
	return p, nil
}

func (s *PlayerStore) ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error) {
	var rows *sql.Rows
	var err error
	if leagueID != nil {
		rows, err = s.db.QueryContext(ctx,
			`SELECT`+playerCols+playerJoin+`WHERE t.league_id = ? ORDER BY p.last_name, p.first_name`,
			*leagueID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT`+playerCols+playerJoin+`ORDER BY p.last_name, p.first_name`)
	}
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer rows.Close()
	out := []models.Player{}
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PlayerStore) GetPlayer(ctx context.Context, id int64) (models.Player, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+playerCols+playerJoin+`WHERE p.id = ?`, id)
	p, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Player{}, fmt.Errorf("player %d: %w", id, players.ErrNotFound)
	}
	if err != nil {
		return models.Player{}, fmt.Errorf("get player %d: %w", id, err)
	}
	return p, nil
}

// CreatePlayer inserts a player row and returns the stored fields without
// re-fetching created_at, matching the previous handler's response shape.
func (s *PlayerStore) CreatePlayer(ctx context.Context, input players.CreatePlayerInput) (models.Player, error) {
	adminHold := 0
	if input.AdminHold {
		adminHold = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO players (player_number, first_name, last_name, phone, email, team_id, handicap, admin_hold)
		 VALUES (?,?,?,?,?,?,?,?)`,
		input.PlayerNumber, input.FirstName, input.LastName,
		input.Phone, input.Email, input.TeamID, input.Handicap, adminHold)
	if err != nil {
		return models.Player{}, fmt.Errorf("create player: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Player{
		ID:           id,
		PlayerNumber: input.PlayerNumber,
		FirstName:    input.FirstName,
		LastName:     input.LastName,
		Name:         input.FirstName + " " + input.LastName,
		Phone:        input.Phone,
		Email:        input.Email,
		TeamID:       input.TeamID,
		Handicap:     input.Handicap,
		AdminHold:    input.AdminHold,
	}, nil
}

// UpdatePlayer updates mutable player fields. PlayerNumber is intentionally
// excluded from the UPDATE — it is locked once set on creation.
// No error is returned when the row does not exist (UPDATE affects 0 rows).
func (s *PlayerStore) UpdatePlayer(ctx context.Context, id int64, input players.UpdatePlayerInput) error {
	adminHold := 0
	if input.AdminHold {
		adminHold = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE players SET first_name=?, last_name=?, phone=?, email=?,
		 team_id=?, handicap=?, admin_hold=? WHERE id=?`,
		input.FirstName, input.LastName, input.Phone, input.Email,
		input.TeamID, input.Handicap, adminHold, id)
	if err != nil {
		return fmt.Errorf("update player %d: %w", id, err)
	}
	return nil
}

// DeletePlayer removes a player by ID. Returns ErrHasHistory when
// handicap_history records exist for the player (business rule: deactivate
// players with history instead of deleting them).
// No error is returned when the row does not exist and no history is found.
func (s *PlayerStore) DeletePlayer(ctx context.Context, id int64) error {
	var historyCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM handicap_history WHERE player_id = ?`, id,
	).Scan(&historyCount); err != nil {
		return fmt.Errorf("check player history %d: %w", id, err)
	}
	if historyCount > 0 {
		return players.ErrHasHistory
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM players WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete player %d: %w", id, err)
	}
	return nil
}
