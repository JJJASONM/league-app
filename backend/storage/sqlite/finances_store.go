package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/models"
)

// FinanceStore is the SQLite implementation of finances.FinanceStore.
// Queries join players/teams directly for display names -- a SQL-layer
// convenience already used throughout this package (e.g. MatchStore,
// WeekStore), not a Go-level dependency on another domain package.
type FinanceStore struct {
	db *sql.DB
}

// NewFinanceStore returns a FinanceStore backed by the given database connection.
func NewFinanceStore(db *sql.DB) *FinanceStore {
	return &FinanceStore{db: db}
}

// InsertDuesPayment records one dues payment and returns the stored row
// with PlayerName/TeamName populated for immediate display.
func (s *FinanceStore) InsertDuesPayment(ctx context.Context, row models.DuesPayment) (models.DuesPayment, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO dues_payments (season_id, player_id, team_id, amount, paid_at, recorded_by_user_id, note)
		VALUES (?,?,?,?,?,?,?)
		RETURNING id, created_at`,
		row.SeasonID, row.PlayerID, row.TeamID, row.Amount, row.PaidAt, row.RecordedByUserID, row.Note,
	).Scan(&row.ID, &row.CreatedAt)
	if err != nil {
		return models.DuesPayment{}, fmt.Errorf("insert dues payment: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT first_name || ' ' || last_name FROM players WHERE id = ?`, row.PlayerID).
		Scan(&row.PlayerName); err != nil {
		return models.DuesPayment{}, fmt.Errorf("insert dues payment: player name lookup: %w", err)
	}
	if row.TeamID != nil {
		if err := s.db.QueryRowContext(ctx, `SELECT name FROM teams WHERE id = ?`, *row.TeamID).
			Scan(&row.TeamName); err != nil {
			return models.DuesPayment{}, fmt.Errorf("insert dues payment: team name lookup: %w", err)
		}
	}
	return row, nil
}

// ListDuesPayments returns every dues payment for the season, newest first.
// Returns a non-nil empty slice when none exist.
func (s *FinanceStore) ListDuesPayments(ctx context.Context, seasonID int64) ([]models.DuesPayment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dp.id, dp.season_id, dp.player_id, p.first_name || ' ' || p.last_name,
		       dp.team_id, COALESCE(t.name, ''),
		       dp.amount, dp.paid_at, dp.recorded_by_user_id, dp.note, dp.created_at
		FROM dues_payments dp
		JOIN players p ON p.id = dp.player_id
		LEFT JOIN teams t ON t.id = dp.team_id
		WHERE dp.season_id = ?
		ORDER BY dp.created_at DESC, dp.id DESC`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list dues payments: %w", err)
	}
	defer rows.Close()

	payments := []models.DuesPayment{}
	for rows.Next() {
		var p models.DuesPayment
		if err := rows.Scan(&p.ID, &p.SeasonID, &p.PlayerID, &p.PlayerName,
			&p.TeamID, &p.TeamName, &p.Amount, &p.PaidAt, &p.RecordedByUserID, &p.Note, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list dues payments: scan: %w", err)
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

// ListDuesPaymentsByPlayer returns one player's dues payments for the
// season, newest first. Returns a non-nil empty slice when none exist.
func (s *FinanceStore) ListDuesPaymentsByPlayer(ctx context.Context, seasonID, playerID int64) ([]models.DuesPayment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dp.id, dp.season_id, dp.player_id, p.first_name || ' ' || p.last_name,
		       dp.team_id, COALESCE(t.name, ''),
		       dp.amount, dp.paid_at, dp.recorded_by_user_id, dp.note, dp.created_at
		FROM dues_payments dp
		JOIN players p ON p.id = dp.player_id
		LEFT JOIN teams t ON t.id = dp.team_id
		WHERE dp.season_id = ? AND dp.player_id = ?
		ORDER BY dp.created_at DESC, dp.id DESC`, seasonID, playerID)
	if err != nil {
		return nil, fmt.Errorf("list dues payments by player: %w", err)
	}
	defer rows.Close()

	payments := []models.DuesPayment{}
	for rows.Next() {
		var p models.DuesPayment
		if err := rows.Scan(&p.ID, &p.SeasonID, &p.PlayerID, &p.PlayerName,
			&p.TeamID, &p.TeamName, &p.Amount, &p.PaidAt, &p.RecordedByUserID, &p.Note, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list dues payments by player: scan: %w", err)
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

// InsertPayout records one payout and returns the stored row with TeamName
// populated for immediate display.
func (s *FinanceStore) InsertPayout(ctx context.Context, row models.Payout) (models.Payout, error) {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO payouts (season_id, team_id, amount, recorded_by_user_id, note)
		VALUES (?,?,?,?,?)
		RETURNING id, created_at`,
		row.SeasonID, row.TeamID, row.Amount, row.RecordedByUserID, row.Note,
	).Scan(&row.ID, &row.CreatedAt)
	if err != nil {
		return models.Payout{}, fmt.Errorf("insert payout: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT name FROM teams WHERE id = ?`, row.TeamID).
		Scan(&row.TeamName); err != nil {
		return models.Payout{}, fmt.Errorf("insert payout: name lookup: %w", err)
	}
	return row, nil
}

// ListPayouts returns every payout for the season, newest first. Returns a
// non-nil empty slice when none exist.
func (s *FinanceStore) ListPayouts(ctx context.Context, seasonID int64) ([]models.Payout, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT po.id, po.season_id, po.team_id, t.name,
		       po.amount, po.recorded_by_user_id, po.note, po.created_at
		FROM payouts po
		JOIN teams t ON t.id = po.team_id
		WHERE po.season_id = ?
		ORDER BY po.created_at DESC, po.id DESC`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list payouts: %w", err)
	}
	defer rows.Close()

	payouts := []models.Payout{}
	for rows.Next() {
		var p models.Payout
		if err := rows.Scan(&p.ID, &p.SeasonID, &p.TeamID, &p.TeamName,
			&p.Amount, &p.RecordedByUserID, &p.Note, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list payouts: scan: %w", err)
		}
		payouts = append(payouts, p)
	}
	return payouts, rows.Err()
}
