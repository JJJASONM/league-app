package seasons_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// ── ListRoster ────────────────────────────────────────────────────────────────

func TestSeasonService_ListRoster_ReturnsEntries(t *testing.T) {
	want := []models.SeasonRosterEntry{{ID: 1, PlayerID: 10}}
	store := &stubSeasonStore{rosterEntries: want}
	got, err := newSvc(store).ListRoster(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("ListRoster: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("want entry ID=1, got %v", got)
	}
}

func TestSeasonService_ListRoster_ReturnsEmptySliceWhenNone(t *testing.T) {
	store := &stubSeasonStore{rosterEntries: nil}
	got, err := newSvc(store).ListRoster(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("ListRoster: %v", err)
	}
	if got == nil {
		t.Error("want empty slice, got nil")
	}
}

func TestSeasonService_ListRoster_StoreErrorPropagates(t *testing.T) {
	store := &stubSeasonStore{rosterErr: errors.New("db down")}
	_, err := newSvc(store).ListRoster(context.Background(), 1, 2)
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── AddRosterPlayer ───────────────────────────────────────────────────────────

func TestSeasonService_AddRosterPlayer_ActiveSeasonReturns422(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: false}
	_, err := newSvc(store).AddRosterPlayer(context.Background(), 1, 2, 3)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Unprocessable {
		t.Errorf("want Unprocessable, got %v", err)
	}
}

func TestSeasonService_AddRosterPlayer_TeamNotInSeasonReturns400(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: true, teamInSeason: false}
	_, err := newSvc(store).AddRosterPlayer(context.Background(), 1, 2, 3)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.InvalidInput {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestSeasonService_AddRosterPlayer_PlayerOnOtherTeamReturns400(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult:         true,
		teamInSeason:          true,
		playerRosterTeamFound: true,
		playerRosterTeamID:    99, // different team
	}
	_, err := newSvc(store).AddRosterPlayer(context.Background(), 1, 2, 3)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.InvalidInput {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestSeasonService_AddRosterPlayer_IdempotentWhenOnSameTeam(t *testing.T) {
	want := models.SeasonRosterEntry{ID: 5, PlayerID: 3}
	store := &stubSeasonStore{
		isDraftResult:         true,
		teamInSeason:          true,
		playerRosterTeamFound: true,
		playerRosterTeamID:    2, // same team
		insertedRosterEntry:   want,
	}
	got, err := newSvc(store).AddRosterPlayer(context.Background(), 1, 2, 3)
	if err != nil {
		t.Fatalf("AddRosterPlayer: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("want entry ID=%d, got %d", want.ID, got.ID)
	}
}

func TestSeasonService_AddRosterPlayer_InsertsWhenNotRostered(t *testing.T) {
	want := models.SeasonRosterEntry{ID: 7, PlayerID: 3}
	store := &stubSeasonStore{
		isDraftResult:         true,
		teamInSeason:          true,
		playerRosterTeamFound: false,
		insertedRosterEntry:   want,
	}
	got, err := newSvc(store).AddRosterPlayer(context.Background(), 1, 2, 3)
	if err != nil {
		t.Fatalf("AddRosterPlayer: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("want entry ID=%d, got %d", want.ID, got.ID)
	}
}

// ── RemoveRosterPlayer ────────────────────────────────────────────────────────

func TestSeasonService_RemoveRosterPlayer_ActiveSeasonReturns422(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: false}
	err := newSvc(store).RemoveRosterPlayer(context.Background(), 1, 2, 3)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Unprocessable {
		t.Errorf("want Unprocessable, got %v", err)
	}
}

func TestSeasonService_RemoveRosterPlayer_NotFoundReturns404(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult:   true,
		deleteRosterErr: fmt.Errorf("wrap: %w", seasons.ErrRosterEntryNotFound),
	}
	err := newSvc(store).RemoveRosterPlayer(context.Background(), 1, 2, 3)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestSeasonService_RemoveRosterPlayer_SucceedsOnDraft(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: true}
	if err := newSvc(store).RemoveRosterPlayer(context.Background(), 1, 2, 3); err != nil {
		t.Fatalf("RemoveRosterPlayer: %v", err)
	}
}

// ── ListAvailablePlayers ──────────────────────────────────────────────────────

func TestSeasonService_ListAvailablePlayers_ReturnsPlayers(t *testing.T) {
	want := []models.Player{{ID: 5, FirstName: "Alice"}}
	store := &stubSeasonStore{availablePlayers: want}
	got, err := newSvc(store).ListAvailablePlayers(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListAvailablePlayers: %v", err)
	}
	if len(got) != 1 || got[0].ID != 5 {
		t.Errorf("want player ID=5, got %v", got)
	}
}

func TestSeasonService_ListAvailablePlayers_ReturnsEmptySliceWhenNone(t *testing.T) {
	store := &stubSeasonStore{availablePlayers: nil}
	got, err := newSvc(store).ListAvailablePlayers(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListAvailablePlayers: %v", err)
	}
	if got == nil {
		t.Error("want empty slice, got nil")
	}
}

func TestSeasonService_ListAvailablePlayers_SeasonNotFoundPropagates(t *testing.T) {
	store := &stubSeasonStore{availablePlayersErr: seasons.ErrNotFound}
	_, err := newSvc(store).ListAvailablePlayers(context.Background(), 99)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
