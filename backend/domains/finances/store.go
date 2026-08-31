package finances

import (
	"context"

	"league_app/models"
)

// FinanceStore is the persistence interface for dues payments and payouts.
// It owns only its own two tables -- it has no knowledge of season rosters,
// season teams, or standings. Composing a full dues/payouts view with
// player/team names and standings context is the handler's job (Financial
// Phase 1 follows the same handler-level composition convention already
// used by Player Overview and Weekly Summary, rather than introducing a
// new cross-domain service dependency here).
type FinanceStore interface {
	// InsertDuesPayment records one dues payment and returns the stored row
	// (ID and CreatedAt populated by the database).
	InsertDuesPayment(ctx context.Context, row models.DuesPayment) (models.DuesPayment, error)

	// ListDuesPayments returns every dues payment for the season, newest
	// first. Returns a non-nil empty slice when none exist.
	ListDuesPayments(ctx context.Context, seasonID int64) ([]models.DuesPayment, error)

	// ListDuesPaymentsByPlayer returns one player's dues payments for the
	// season, newest first. Returns a non-nil empty slice when none exist.
	// Added for Player Overview Phase 2, which needs only one player's
	// history rather than the full season list ListDuesPayments returns.
	ListDuesPaymentsByPlayer(ctx context.Context, seasonID, playerID int64) ([]models.DuesPayment, error)

	// InsertPayout records one payout and returns the stored row (ID and
	// CreatedAt populated by the database).
	InsertPayout(ctx context.Context, row models.Payout) (models.Payout, error)

	// ListPayouts returns every payout for the season, newest first.
	// Returns a non-nil empty slice when none exist.
	ListPayouts(ctx context.Context, seasonID int64) ([]models.Payout, error)
}

// RecordDuesPaymentInput carries user-supplied fields for recording a dues payment.
type RecordDuesPaymentInput struct {
	SeasonID         int64
	PlayerID         int64
	TeamID           *int64
	Amount           float64
	PaidAt           string
	RecordedByUserID *int64
	Note             string
}

// RecordPayoutInput carries user-supplied fields for recording a payout.
type RecordPayoutInput struct {
	SeasonID         int64
	TeamID           int64
	Amount           float64
	RecordedByUserID *int64
	Note             string
}
