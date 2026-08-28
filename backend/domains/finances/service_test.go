package finances_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/finances"
	"league_app/models"
)

type stubFinanceStore struct {
	insertDuesFn   func(ctx context.Context, row models.DuesPayment) (models.DuesPayment, error)
	listDuesFn     func(ctx context.Context, seasonID int64) ([]models.DuesPayment, error)
	insertPayoutFn func(ctx context.Context, row models.Payout) (models.Payout, error)
	listPayoutsFn  func(ctx context.Context, seasonID int64) ([]models.Payout, error)
}

func (s *stubFinanceStore) InsertDuesPayment(ctx context.Context, row models.DuesPayment) (models.DuesPayment, error) {
	if s.insertDuesFn != nil {
		return s.insertDuesFn(ctx, row)
	}
	return row, nil
}
func (s *stubFinanceStore) ListDuesPayments(ctx context.Context, seasonID int64) ([]models.DuesPayment, error) {
	if s.listDuesFn != nil {
		return s.listDuesFn(ctx, seasonID)
	}
	return []models.DuesPayment{}, nil
}
func (s *stubFinanceStore) InsertPayout(ctx context.Context, row models.Payout) (models.Payout, error) {
	if s.insertPayoutFn != nil {
		return s.insertPayoutFn(ctx, row)
	}
	return row, nil
}
func (s *stubFinanceStore) ListPayouts(ctx context.Context, seasonID int64) ([]models.Payout, error) {
	if s.listPayoutsFn != nil {
		return s.listPayoutsFn(ctx, seasonID)
	}
	return []models.Payout{}, nil
}

func newFinanceSvc(store finances.FinanceStore) *finances.FinanceService {
	return finances.NewFinanceService(store)
}

// -- RecordDuesPayment ----------------------------------------------------

func TestFinanceService_RecordDuesPayment_ZeroSeasonID_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordDuesPayment(context.Background(), finances.RecordDuesPaymentInput{
		PlayerID: 1, Amount: 25, PaidAt: "2026-01-01",
	})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "DUES_SEASON_REQUIRED" || de.Category != domainerr.InvalidInput {
		t.Fatalf("want DUES_SEASON_REQUIRED InvalidInput, got %v", err)
	}
}

func TestFinanceService_RecordDuesPayment_ZeroPlayerID_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordDuesPayment(context.Background(), finances.RecordDuesPaymentInput{
		SeasonID: 1, Amount: 25, PaidAt: "2026-01-01",
	})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "DUES_PLAYER_REQUIRED" || de.Category != domainerr.InvalidInput {
		t.Fatalf("want DUES_PLAYER_REQUIRED InvalidInput, got %v", err)
	}
}

func TestFinanceService_RecordDuesPayment_ZeroAmount_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordDuesPayment(context.Background(), finances.RecordDuesPaymentInput{
		SeasonID: 1, PlayerID: 2, Amount: 0, PaidAt: "2026-01-01",
	})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "DUES_AMOUNT_INVALID" || de.Category != domainerr.InvalidInput {
		t.Fatalf("want DUES_AMOUNT_INVALID InvalidInput, got %v", err)
	}
}

func TestFinanceService_RecordDuesPayment_NegativeAmount_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordDuesPayment(context.Background(), finances.RecordDuesPaymentInput{
		SeasonID: 1, PlayerID: 2, Amount: -5, PaidAt: "2026-01-01",
	})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "DUES_AMOUNT_INVALID" {
		t.Fatalf("want DUES_AMOUNT_INVALID, got %v", err)
	}
}

func TestFinanceService_RecordDuesPayment_EmptyPaidAt_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordDuesPayment(context.Background(), finances.RecordDuesPaymentInput{
		SeasonID: 1, PlayerID: 2, Amount: 25, PaidAt: "  ",
	})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "DUES_PAID_AT_REQUIRED" {
		t.Fatalf("want DUES_PAID_AT_REQUIRED, got %v", err)
	}
}

func TestFinanceService_RecordDuesPayment_ValidInput_DelegatesToStore(t *testing.T) {
	var received models.DuesPayment
	teamID := int64(7)
	svc := newFinanceSvc(&stubFinanceStore{
		insertDuesFn: func(_ context.Context, row models.DuesPayment) (models.DuesPayment, error) {
			received = row
			row.ID = 11
			return row, nil
		},
	})
	got, err := svc.RecordDuesPayment(context.Background(), finances.RecordDuesPaymentInput{
		SeasonID: 1, PlayerID: 2, TeamID: &teamID, Amount: 25, PaidAt: "2026-01-01", Note: "cash",
	})
	if err != nil {
		t.Fatalf("RecordDuesPayment: %v", err)
	}
	if got.ID != 11 {
		t.Errorf("want ID=11, got %d", got.ID)
	}
	if received.SeasonID != 1 || received.PlayerID != 2 || received.Amount != 25 || received.PaidAt != "2026-01-01" || received.Note != "cash" {
		t.Errorf("want fields passed through to store, got %+v", received)
	}
	if received.TeamID == nil || *received.TeamID != 7 {
		t.Errorf("want TeamID=7 passed to store, got %v", received.TeamID)
	}
}

// -- ListDuesPayments -------------------------------------------------------

func TestFinanceService_ListDuesPayments_DelegatesToStore(t *testing.T) {
	want := []models.DuesPayment{{ID: 1}, {ID: 2}}
	svc := newFinanceSvc(&stubFinanceStore{
		listDuesFn: func(_ context.Context, seasonID int64) ([]models.DuesPayment, error) {
			if seasonID != 5 {
				t.Errorf("want seasonID=5, got %d", seasonID)
			}
			return want, nil
		},
	})
	got, err := svc.ListDuesPayments(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListDuesPayments: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 payments, got %d", len(got))
	}
}

// -- RecordPayout ------------------------------------------------------------

func TestFinanceService_RecordPayout_ZeroSeasonID_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordPayout(context.Background(), finances.RecordPayoutInput{TeamID: 1, Amount: 100})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "PAYOUT_SEASON_REQUIRED" {
		t.Fatalf("want PAYOUT_SEASON_REQUIRED, got %v", err)
	}
}

func TestFinanceService_RecordPayout_ZeroTeamID_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordPayout(context.Background(), finances.RecordPayoutInput{SeasonID: 1, Amount: 100})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "PAYOUT_TEAM_REQUIRED" {
		t.Fatalf("want PAYOUT_TEAM_REQUIRED, got %v", err)
	}
}

func TestFinanceService_RecordPayout_ZeroAmount_ReturnsError(t *testing.T) {
	svc := newFinanceSvc(&stubFinanceStore{})
	_, err := svc.RecordPayout(context.Background(), finances.RecordPayoutInput{SeasonID: 1, TeamID: 2, Amount: 0})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "PAYOUT_AMOUNT_INVALID" {
		t.Fatalf("want PAYOUT_AMOUNT_INVALID, got %v", err)
	}
}

func TestFinanceService_RecordPayout_ValidInput_DelegatesToStore(t *testing.T) {
	var received models.Payout
	svc := newFinanceSvc(&stubFinanceStore{
		insertPayoutFn: func(_ context.Context, row models.Payout) (models.Payout, error) {
			received = row
			row.ID = 22
			return row, nil
		},
	})
	got, err := svc.RecordPayout(context.Background(), finances.RecordPayoutInput{
		SeasonID: 1, TeamID: 3, Amount: 200, Note: "1st place",
	})
	if err != nil {
		t.Fatalf("RecordPayout: %v", err)
	}
	if got.ID != 22 {
		t.Errorf("want ID=22, got %d", got.ID)
	}
	if received.SeasonID != 1 || received.TeamID != 3 || received.Amount != 200 || received.Note != "1st place" {
		t.Errorf("want fields passed through to store, got %+v", received)
	}
}

// -- ListPayouts ---------------------------------------------------------------

func TestFinanceService_ListPayouts_DelegatesToStore(t *testing.T) {
	want := []models.Payout{{ID: 1}}
	svc := newFinanceSvc(&stubFinanceStore{
		listPayoutsFn: func(_ context.Context, seasonID int64) ([]models.Payout, error) {
			if seasonID != 9 {
				t.Errorf("want seasonID=9, got %d", seasonID)
			}
			return want, nil
		},
	})
	got, err := svc.ListPayouts(context.Background(), 9)
	if err != nil {
		t.Fatalf("ListPayouts: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 payout, got %d", len(got))
	}
}
