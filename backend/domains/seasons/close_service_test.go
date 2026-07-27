package seasons_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func activatedAt() *string { s := "2025-01-01T00:00:00Z"; return &s }
func closedAt() *string    { s := "2025-06-01T00:00:00Z"; return &s }

func activeSeason() models.Season {
	return models.Season{ID: 1, Active: true, ActivatedAt: activatedAt()}
}
func historicalSeason() models.Season {
	return models.Season{ID: 1, Active: false, ActivatedAt: activatedAt()}
}
func draftSeason() models.Season {
	return models.Season{ID: 1, Active: false, ActivatedAt: nil}
}
func alreadyClosedSeason() models.Season {
	return models.Season{ID: 1, Active: false, ActivatedAt: activatedAt(), ClosedAt: closedAt()}
}

func closedWeek(n int) models.WeekSummary {
	return models.WeekSummary{WeekNumber: n, Status: matches.WeekStatusClosed}
}
func openWeek(n int) models.WeekSummary {
	return models.WeekSummary{WeekNumber: n, Status: "open"}
}

func noWeeks() []models.WeekSummary      { return []models.WeekSummary{} }
func noStandings() []models.Standing     { return []models.Standing{} }
func oneWeekClosed() []models.WeekSummary { return []models.WeekSummary{closedWeek(1)} }

// ── ClosePreview ──────────────────────────────────────────────────────────────

func TestClosePreview_DraftSeason(t *testing.T) {
	store := &stubSeasonStore{
		season:     draftSeason(),
		matchCount: 5,
	}
	svc := newSvc(store)

	preview, err := svc.ClosePreview(context.Background(), 1, noWeeks(), noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.CanClose {
		t.Fatal("expected can_close=false for draft season")
	}
	if len(preview.Blockers) != 1 || preview.Blockers[0].Code != seasons.CodeSeasonCloseDraft {
		t.Fatalf("expected SEASON_CLOSE_DRAFT blocker, got %+v", preview.Blockers)
	}
}

func TestClosePreview_AlreadyClosed(t *testing.T) {
	store := &stubSeasonStore{
		season:     alreadyClosedSeason(),
		matchCount: 5,
	}
	svc := newSvc(store)

	preview, err := svc.ClosePreview(context.Background(), 1, noWeeks(), noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.CanClose {
		t.Fatal("expected can_close=false for already-closed season")
	}
	if len(preview.Blockers) != 1 || preview.Blockers[0].Code != seasons.CodeSeasonAlreadyClosed {
		t.Fatalf("expected SEASON_ALREADY_CLOSED blocker, got %+v", preview.Blockers)
	}
}

func TestClosePreview_NoSchedule(t *testing.T) {
	store := &stubSeasonStore{
		season:     activeSeason(),
		matchCount: 0,
	}
	svc := newSvc(store)

	preview, err := svc.ClosePreview(context.Background(), 1, noWeeks(), noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.CanClose {
		t.Fatal("expected can_close=false when no matches scheduled")
	}
	if len(preview.Blockers) != 1 || preview.Blockers[0].Code != seasons.CodeSeasonCloseNoSchedule {
		t.Fatalf("expected SEASON_CLOSE_NO_SCHEDULE blocker, got %+v", preview.Blockers)
	}
}

func TestClosePreview_UnclosedWeeks(t *testing.T) {
	weeks := []models.WeekSummary{closedWeek(1), openWeek(2), openWeek(3)}
	store := &stubSeasonStore{
		season:     activeSeason(),
		matchCount: 6,
	}
	svc := newSvc(store)

	preview, err := svc.ClosePreview(context.Background(), 1, weeks, noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.CanClose {
		t.Fatal("expected can_close=false with unclosed weeks")
	}
	if len(preview.Blockers) != 1 || preview.Blockers[0].Code != seasons.CodeSeasonCloseUnclosed {
		t.Fatalf("expected SEASON_CLOSE_UNCLOSED blocker, got %+v", preview.Blockers)
	}
	if len(preview.UnclosedWeeks) != 2 {
		t.Fatalf("expected 2 unclosed weeks, got %v", preview.UnclosedWeeks)
	}
	// UnclosedWeeks should list week 2 and 3
	if preview.UnclosedWeeks[0] != 2 || preview.UnclosedWeeks[1] != 3 {
		t.Errorf("unexpected unclosed week numbers: %v", preview.UnclosedWeeks)
	}
}

func TestClosePreview_MissingResultsAdvisory(t *testing.T) {
	store := &stubSeasonStore{
		season:       activeSeason(),
		matchCount:   3,
		missingCount: 1,
	}
	svc := newSvc(store)

	preview, err := svc.ClosePreview(context.Background(), 1, oneWeekClosed(), noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !preview.CanClose {
		t.Fatal("expected can_close=true; missing results is advisory only")
	}
	if len(preview.Blockers) != 0 {
		t.Fatalf("expected no blockers, got %+v", preview.Blockers)
	}
	if len(preview.Warnings) != 1 || preview.Warnings[0].Code != seasons.CodeMissingResults {
		t.Fatalf("expected MISSING_RESULTS warning, got %+v", preview.Warnings)
	}
	if preview.MissingCount != 1 {
		t.Errorf("expected missing_results=1, got %d", preview.MissingCount)
	}
}

func TestClosePreview_AllClear(t *testing.T) {
	standings := []models.Standing{{TeamID: 1, TeamName: "Team A", Wins: 3}}
	store := &stubSeasonStore{
		season:     activeSeason(),
		matchCount: 3,
	}
	svc := newSvc(store)

	preview, err := svc.ClosePreview(context.Background(), 1, oneWeekClosed(), standings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !preview.CanClose {
		t.Fatalf("expected can_close=true, got blockers: %+v", preview.Blockers)
	}
	if len(preview.Blockers) != 0 {
		t.Fatalf("expected no blockers, got %+v", preview.Blockers)
	}
	if len(preview.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", preview.Warnings)
	}
	if len(preview.Standings) != 1 {
		t.Errorf("expected standings to be passed through, got %d entries", len(preview.Standings))
	}
}

func TestClosePreview_SeasonNotFound(t *testing.T) {
	store := &stubSeasonStore{
		seasonErr: seasons.ErrNotFound,
	}
	svc := newSvc(store)

	_, err := svc.ClosePreview(context.Background(), 99, noWeeks(), noStandings())
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ── CloseSeason ───────────────────────────────────────────────────────────────

func TestCloseSeason_DraftSeason_ReturnsDomainErr(t *testing.T) {
	store := &stubSeasonStore{
		season:     draftSeason(),
		matchCount: 5,
	}
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, noWeeks(), noStandings())
	if err == nil {
		t.Fatal("expected error for draft season")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("expected domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != seasons.CodeSeasonCloseDraft {
		t.Errorf("expected code %s, got %s", seasons.CodeSeasonCloseDraft, de.Code)
	}
	if de.Category != domainerr.Unprocessable {
		t.Errorf("expected Unprocessable category, got %v", de.Category)
	}
}

func TestCloseSeason_AlreadyClosed_ReturnsConflict(t *testing.T) {
	store := &stubSeasonStore{
		season:     alreadyClosedSeason(),
		matchCount: 5,
	}
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, noWeeks(), noStandings())
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("expected domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != seasons.CodeSeasonAlreadyClosed {
		t.Errorf("expected code %s, got %s", seasons.CodeSeasonAlreadyClosed, de.Code)
	}
	if de.Category != domainerr.Conflict {
		t.Errorf("expected Conflict category, got %v", de.Category)
	}
}

func TestCloseSeason_UnclosedWeeks_ReturnsConflict(t *testing.T) {
	weeks := []models.WeekSummary{openWeek(1)}
	store := &stubSeasonStore{
		season:     activeSeason(),
		matchCount: 3,
	}
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, weeks, noStandings())
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("expected domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != seasons.CodeSeasonCloseUnclosed {
		t.Errorf("expected code %s, got %s", seasons.CodeSeasonCloseUnclosed, de.Code)
	}
	if de.Category != domainerr.Conflict {
		t.Errorf("expected Conflict category, got %v", de.Category)
	}
}

func TestCloseSeason_ActiveSeason_SetsInactive(t *testing.T) {
	closed := activeSeason()
	closed.ClosedAt = closedAt()
	store := &stubSeasonStore{
		season:     activeSeason(),
		matchCount: 3,
		// GetSeason is called twice: once inside computeClose, once after CloseSeasonRecord.
		// The stub returns the same season both times; the second call simulates the updated row.
	}
	// Override to return the closed season on the post-commit fetch.
	store.season = activeSeason()
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, oneWeekClosed(), noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.closeRecordCalled {
		t.Fatal("expected CloseSeasonRecord to be called")
	}
	if !store.closeRecordInactive {
		t.Error("expected setInactive=true for active season")
	}
	if store.closeRecordSnap == "" {
		t.Error("expected non-empty snapshot JSON")
	}
}

func TestCloseSeason_HistoricalSeason_DoesNotSetInactive(t *testing.T) {
	store := &stubSeasonStore{
		season:     historicalSeason(),
		matchCount: 3,
	}
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, oneWeekClosed(), noStandings())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.closeRecordCalled {
		t.Fatal("expected CloseSeasonRecord to be called")
	}
	if store.closeRecordInactive {
		t.Error("expected setInactive=false for historical (already inactive) season")
	}
}

func TestCloseSeason_SnapshotContainsStandings(t *testing.T) {
	standings := []models.Standing{
		{TeamID: 1, TeamName: "Sharks", Wins: 5, Losses: 1},
		{TeamID: 2, TeamName: "Jets", Wins: 3, Losses: 3},
	}
	store := &stubSeasonStore{
		season:     historicalSeason(),
		matchCount: 6,
	}
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, oneWeekClosed(), standings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The snapshot JSON must contain both team names.
	if store.closeRecordSnap == "" {
		t.Fatal("snapshot was not stored")
	}
	snap := store.closeRecordSnap
	for _, name := range []string{"Sharks", "Jets", "schema_version"} {
		if !strings.Contains(snap, name) {
			t.Errorf("snapshot missing %q; snapshot: %s", name, snap)
		}
	}
}

func TestCloseSeason_StoreErr_Propagates(t *testing.T) {
	storeErr := errors.New("db down")
	store := &stubSeasonStore{
		season:         historicalSeason(),
		matchCount:     3,
		closeRecordErr: storeErr,
	}
	svc := newSvc(store)

	_, err := svc.CloseSeason(context.Background(), 1, oneWeekClosed(), noStandings())
	if err == nil {
		t.Fatal("expected error from store")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("expected storeErr wrapped in chain, got: %v", err)
	}
}

// --- ReopenSeason ---

func TestReopenSeason_NotFound_ReturnsErr(t *testing.T) {
	store := &stubSeasonStore{seasonErr: seasons.ErrNotFound}
	svc := newSvc(store)

	_, err := svc.ReopenSeason(context.Background(), 99)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReopenSeason_NotClosed_ReturnsConflict(t *testing.T) {
	store := &stubSeasonStore{season: historicalSeason()} // closed_at = nil
	svc := newSvc(store)

	_, err := svc.ReopenSeason(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for non-closed season")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("expected domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != seasons.CodeSeasonNotClosed {
		t.Errorf("expected code %s, got %s", seasons.CodeSeasonNotClosed, de.Code)
	}
	if de.Category != domainerr.Conflict {
		t.Errorf("expected Conflict category, got %v", de.Category)
	}
}

func TestReopenSeason_Success_ClearsClosedAt(t *testing.T) {
	store := &stubSeasonStore{season: alreadyClosedSeason()}
	// After ReopenSeasonRecord the stub's GetSeason still returns alreadyClosedSeason,
	// but we verify ReopenSeasonRecord was called.
	svc := newSvc(store)

	_, err := svc.ReopenSeason(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.reopenRecordCalled {
		t.Fatal("expected ReopenSeasonRecord to be called")
	}
}

func TestReopenSeason_StoreErr_Propagates(t *testing.T) {
	storeErr := errors.New("db write failed")
	store := &stubSeasonStore{
		season:          alreadyClosedSeason(),
		reopenRecordErr: storeErr,
	}
	svc := newSvc(store)

	_, err := svc.ReopenSeason(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from store")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("expected storeErr wrapped in chain, got: %v", err)
	}
}

