package seasons_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// ── ListSeasons ───────────────────────────────────────────────────────────────

func TestSeasonService_ListSeasons_Delegates(t *testing.T) {
	store := &stubSeasonStore{
		seasonList: []models.Season{{ID: 1, Name: "Alpha"}},
	}
	got, err := newSvc(store).ListSeasons(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("want [{ID:1}], got %v", got)
	}
}

func TestSeasonService_ListSeasons_EmptyNonNil(t *testing.T) {
	store := &stubSeasonStore{seasonList: nil}
	got, err := newSvc(store).ListSeasons(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	if got == nil {
		t.Error("want non-nil slice, got nil")
	}
}

func TestSeasonService_ListSeasons_PropagatesError(t *testing.T) {
	store := &stubSeasonStore{seasonListErr: errors.New("db down")}
	_, err := newSvc(store).ListSeasons(context.Background(), nil)
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── GetSeason ─────────────────────────────────────────────────────────────────

func TestSeasonService_GetSeason_Delegates(t *testing.T) {
	store := &stubSeasonStore{season: models.Season{ID: 7, Name: "Beta"}}
	got, err := newSvc(store).GetSeason(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetSeason: %v", err)
	}
	if got.ID != 7 {
		t.Errorf("want ID=7, got %d", got.ID)
	}
}

func TestSeasonService_GetSeason_NotFound(t *testing.T) {
	store := &stubSeasonStore{seasonErr: seasons.ErrNotFound}
	_, err := newSvc(store).GetSeason(context.Background(), 99)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound in chain, got %v", err)
	}
}

// ── CreateSeason ──────────────────────────────────────────────────────────────

func TestSeasonService_CreateSeason_ValidatesNameRequired(t *testing.T) {
	store := &stubSeasonStore{}
	_, err := newSvc(store).CreateSeason(context.Background(), seasons.CreateSeasonInput{
		LeagueID:     1,
		Name:         "  ",
		ScheduleType: "double_rr",
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for empty name, got %v", err)
	}
}

func TestSeasonService_CreateSeason_ValidatesLeagueIDRequired(t *testing.T) {
	store := &stubSeasonStore{}
	_, err := newSvc(store).CreateSeason(context.Background(), seasons.CreateSeasonInput{
		LeagueID:     0,
		Name:         "Valid Name",
		ScheduleType: "double_rr",
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for zero league_id, got %v", err)
	}
}

func TestSeasonService_CreateSeason_SetsDefaultScheduleType(t *testing.T) {
	var captured seasons.CreateSeasonInput
	store := &stubSeasonStore{
		createSeasonFn: func(in seasons.CreateSeasonInput) (models.Season, error) {
			captured = in
			return models.Season{ScheduleType: in.ScheduleType}, nil
		},
	}
	_, err := newSvc(store).CreateSeason(context.Background(), seasons.CreateSeasonInput{
		LeagueID: 1,
		Name:     "Test",
	})
	if err != nil {
		t.Fatalf("CreateSeason: %v", err)
	}
	if captured.ScheduleType != "double_rr" {
		t.Errorf("want ScheduleType=double_rr, got %q", captured.ScheduleType)
	}
}

func TestSeasonService_CreateSeason_Delegates(t *testing.T) {
	store := &stubSeasonStore{
		createdSeason: models.Season{ID: 42, Name: "Gamma", TeamsManaged: true},
	}
	got, err := newSvc(store).CreateSeason(context.Background(), seasons.CreateSeasonInput{
		LeagueID:     1,
		Name:         "Gamma",
		ScheduleType: "single_rr",
	})
	if err != nil {
		t.Fatalf("CreateSeason: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("want ID=42, got %d", got.ID)
	}
}

// ── UpdateSeason ──────────────────────────────────────────────────────────────

func TestSeasonService_UpdateSeason_SetsDefaultScheduleType(t *testing.T) {
	var captured seasons.UpdateSeasonInput
	store := &stubSeasonStore{
		updateSeasonFn: func(in seasons.UpdateSeasonInput) (models.Season, error) {
			captured = in
			return models.Season{ScheduleType: in.ScheduleType}, nil
		},
	}
	_, err := newSvc(store).UpdateSeason(context.Background(), 1, seasons.UpdateSeasonInput{Name: "X"})
	if err != nil {
		t.Fatalf("UpdateSeason: %v", err)
	}
	if captured.ScheduleType != "double_rr" {
		t.Errorf("want ScheduleType=double_rr, got %q", captured.ScheduleType)
	}
}

func TestSeasonService_UpdateSeason_Delegates(t *testing.T) {
	store := &stubSeasonStore{updatedSeason: models.Season{ID: 5, Name: "Updated", LeagueID: 2}}
	got, err := newSvc(store).UpdateSeason(context.Background(), 5, seasons.UpdateSeasonInput{
		Name:         "Updated",
		ScheduleType: "blanket",
	})
	if err != nil {
		t.Fatalf("UpdateSeason: %v", err)
	}
	if got.LeagueID != 2 {
		t.Errorf("want full row returned (LeagueID=2), got %d", got.LeagueID)
	}
}

func TestSeasonService_UpdateSeason_NotFound(t *testing.T) {
	store := &stubSeasonStore{updateSeasonErr: seasons.ErrNotFound}
	_, err := newSvc(store).UpdateSeason(context.Background(), 99, seasons.UpdateSeasonInput{
		Name:         "X",
		ScheduleType: "double_rr",
	})
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── DeleteSeason ──────────────────────────────────────────────────────────────

func TestSeasonService_DeleteSeason_Delegates(t *testing.T) {
	store := &stubSeasonStore{}
	if err := newSvc(store).DeleteSeason(context.Background(), 1); err != nil {
		t.Fatalf("DeleteSeason: %v", err)
	}
	if !store.deleteSeasonCalled {
		t.Error("want store.DeleteSeason called")
	}
}
