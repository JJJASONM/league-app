package finances

import (
	"context"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// FinanceService implements business logic for dues payments and payouts.
// Validation only -- it does not know about season rosters, season teams,
// or standings; see FinanceStore's doc comment for why that composition
// lives in the handler instead.
type FinanceService struct {
	store FinanceStore
}

// NewFinanceService returns a FinanceService backed by the given store.
func NewFinanceService(store FinanceStore) *FinanceService {
	return &FinanceService{store: store}
}

// RecordDuesPayment validates the input and inserts one dues payment row.
// Returns domainerr.InvalidInput when season_id/player_id are missing,
// amount is not positive, or paid_at is empty.
func (s *FinanceService) RecordDuesPayment(ctx context.Context, input RecordDuesPaymentInput) (models.DuesPayment, error) {
	if input.SeasonID == 0 {
		return models.DuesPayment{}, domainerr.New("DUES_SEASON_REQUIRED", domainerr.InvalidInput, "season_id is required")
	}
	if input.PlayerID == 0 {
		return models.DuesPayment{}, domainerr.New("DUES_PLAYER_REQUIRED", domainerr.InvalidInput, "player_id is required")
	}
	if input.Amount <= 0 {
		return models.DuesPayment{}, domainerr.New("DUES_AMOUNT_INVALID", domainerr.InvalidInput, "amount must be greater than zero")
	}
	if strings.TrimSpace(input.PaidAt) == "" {
		return models.DuesPayment{}, domainerr.New("DUES_PAID_AT_REQUIRED", domainerr.InvalidInput, "paid_at is required")
	}
	return s.store.InsertDuesPayment(ctx, models.DuesPayment{
		SeasonID:         input.SeasonID,
		PlayerID:         input.PlayerID,
		TeamID:           input.TeamID,
		Amount:           input.Amount,
		PaidAt:           input.PaidAt,
		RecordedByUserID: input.RecordedByUserID,
		Note:             input.Note,
	})
}

// ListDuesPayments returns every dues payment for the season.
func (s *FinanceService) ListDuesPayments(ctx context.Context, seasonID int64) ([]models.DuesPayment, error) {
	return s.store.ListDuesPayments(ctx, seasonID)
}

// ListDuesPaymentsByPlayer returns one player's dues payments for the season.
func (s *FinanceService) ListDuesPaymentsByPlayer(ctx context.Context, seasonID, playerID int64) ([]models.DuesPayment, error) {
	return s.store.ListDuesPaymentsByPlayer(ctx, seasonID, playerID)
}

// RecordPayout validates the input and inserts one payout row. Returns
// domainerr.InvalidInput when season_id/team_id are missing or amount is
// not positive. Amount is always admin-entered -- this method does not
// look at standings.
func (s *FinanceService) RecordPayout(ctx context.Context, input RecordPayoutInput) (models.Payout, error) {
	if input.SeasonID == 0 {
		return models.Payout{}, domainerr.New("PAYOUT_SEASON_REQUIRED", domainerr.InvalidInput, "season_id is required")
	}
	if input.TeamID == 0 {
		return models.Payout{}, domainerr.New("PAYOUT_TEAM_REQUIRED", domainerr.InvalidInput, "team_id is required")
	}
	if input.Amount <= 0 {
		return models.Payout{}, domainerr.New("PAYOUT_AMOUNT_INVALID", domainerr.InvalidInput, "amount must be greater than zero")
	}
	return s.store.InsertPayout(ctx, models.Payout{
		SeasonID:         input.SeasonID,
		TeamID:           input.TeamID,
		Amount:           input.Amount,
		RecordedByUserID: input.RecordedByUserID,
		Note:             input.Note,
	})
}

// ListPayouts returns every payout for the season.
func (s *FinanceService) ListPayouts(ctx context.Context, seasonID int64) ([]models.Payout, error) {
	return s.store.ListPayouts(ctx, seasonID)
}
