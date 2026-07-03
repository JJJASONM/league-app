package seasons_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/seasons"
	"league_app/models"
)

// stubSeasonStore is a configurable test double for seasons.SeasonStore.
// Set fields to control return values; inspect booleans to verify calls.
type stubSeasonStore struct {
	meta       seasons.SeasonMeta
	metaErr    error
	teams      []seasons.TeamSummary
	teamsErr   error
	matchCount int
	matchErr   error
	staleErr   error
	staleCalled bool
	activateErr    error
	activateCalled bool
	isDraftResult  bool
	isDraftErr     error
	findActive    *models.Season
	findActiveErr  error
	findPrior    *models.Season
	findPriorErr  error
	seasonTeams    []seasons.SeasonTeamEntry
	seasonTeamsErr error
	matchTeams    []seasons.SeasonTeamEntry
	matchTeamsErr  error
}

func (s *stubSeasonStore) IsDraft(_ context.Context, _ int64) (bool, error) {
	return s.isDraftResult, s.isDraftErr
}
func (s *stubSeasonStore) GetMeta(_ context.Context, _ int64) (seasons.SeasonMeta, error) {
	return s.meta, s.metaErr
}
func (s *stubSeasonStore) GetTeamSummaries(_ context.Context, _ int64) ([]seasons.TeamSummary, error) {
	return s.teams, s.teamsErr
}
func (s *stubSeasonStore) GetMatchCount(_ context.Context, _ int64) (int, error) {
	return s.matchCount, s.matchErr
}
func (s *stubSeasonStore) Activate(_ context.Context, _, _ int64) error {
	s.activateCalled = true
	return s.activateErr
}
func (s *stubSeasonStore) MarkStaleIfScheduled(_ context.Context, _ int64) error {
	s.staleCalled = true
	return s.staleErr
}
func (s *stubSeasonStore) FindActiveWithNoEndDate(_ context.Context, _, _ int64) (*models.Season, error) {
	return s.findActive, s.findActiveErr
}
func (s *stubSeasonStore) FindClosestPriorByEndDate(_ context.Context, _, _ int64, _ *string) (*models.Season, error) {
	return s.findPrior, s.findPriorErr
}
func (s *stubSeasonStore) GetSeasonTeams(_ context.Context, _ int64) ([]seasons.SeasonTeamEntry, error) {
	if s.seasonTeams == nil {
		return []seasons.SeasonTeamEntry{}, s.seasonTeamsErr
	}
	return s.seasonTeams, s.seasonTeamsErr
}
func (s *stubSeasonStore) GetMatchTeams(_ context.Context, _ int64) ([]seasons.SeasonTeamEntry, error) {
	if s.matchTeams == nil {
		return []seasons.SeasonTeamEntry{}, s.matchTeamsErr
	}
	return s.matchTeams, s.matchTeamsErr
}

func newSvc(store *stubSeasonStore) *seasons.SeasonService {
	return seasons.NewSeasonService(store)
}

// ── IsDraft ───────────────────────────────────────────────────────────────────

func TestSeasonService_IsDraft_Delegates(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: true}
	got, err := newSvc(store).IsDraft(context.Background(), 1)
	if err != nil {
		t.Fatalf("IsDraft: %v", err)
	}
	if !got {
		t.Error("want true, got false")
	}
}

func TestSeasonService_IsDraft_PropagatesError(t *testing.T) {
	store := &stubSeasonStore{isDraftErr: errors.New("db down")}
	_, err := newSvc(store).IsDraft(context.Background(), 1)
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── MarkStaleIfScheduled ──────────────────────────────────────────────────────

func TestSeasonService_MarkStaleIfScheduled_Delegates(t *testing.T) {
	store := &stubSeasonStore{}
	if err := newSvc(store).MarkStaleIfScheduled(context.Background(), 1); err != nil {
		t.Fatalf("MarkStaleIfScheduled: %v", err)
	}
	if !store.staleCalled {
		t.Error("want store.MarkStaleIfScheduled to be called")
	}
}

// ── Checklist ─────────────────────────────────────────────────────────────────

func TestSeasonService_Checklist_NotFound(t *testing.T) {
	store := &stubSeasonStore{metaErr: seasons.ErrNotFound}
	_, err := newSvc(store).Checklist(context.Background(), 99)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound in chain, got %v", err)
	}
}

func TestSeasonService_Checklist_LegacySeason_CanActivate(t *testing.T) {
	store := &stubSeasonStore{meta: seasons.SeasonMeta{TeamsManaged: false}}
	c, err := newSvc(store).Checklist(context.Background(), 1)
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	if !c.CanActivate {
		t.Error("want CanActivate=true for legacy season")
	}
	if len(c.Blockers) != 0 {
		t.Errorf("want 0 blockers, got %v", c.Blockers)
	}
}

func TestSeasonService_Checklist_ManagedNoTeams_BlocksTooFew(t *testing.T) {
	store := &stubSeasonStore{
		meta:       seasons.SeasonMeta{TeamsManaged: true},
		teams:      []seasons.TeamSummary{},
		matchCount: 1,
	}
	end := "2026-12-01"
	store.meta.EndDate = &end
	c, err := newSvc(store).Checklist(context.Background(), 1)
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	assertHasCode(t, c.Blockers, "TEAMS_TOO_FEW")
	if c.CanActivate {
		t.Error("want CanActivate=false")
	}
}

func TestSeasonService_Checklist_CaptainNotOnRoster_Blocked(t *testing.T) {
	capID := int64(5)
	store := &stubSeasonStore{
		meta: seasons.SeasonMeta{TeamsManaged: true},
		teams: []seasons.TeamSummary{
			{TeamID: 1, Name: "A", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: false},
			{TeamID: 2, Name: "B", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
		},
		matchCount: 1,
	}
	end := "2026-12-01"
	store.meta.EndDate = &end
	c, _ := newSvc(store).Checklist(context.Background(), 1)
	assertHasCode(t, c.Blockers, "CAPTAIN_NOT_ON_ROSTER")
}

func TestSeasonService_Checklist_StaleSchedule_Blocked(t *testing.T) {
	capID := int64(1)
	store := &stubSeasonStore{
		meta: seasons.SeasonMeta{TeamsManaged: true, ScheduleStale: true},
		teams: []seasons.TeamSummary{
			{TeamID: 1, Name: "A", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
			{TeamID: 2, Name: "B", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
		},
		matchCount: 1,
	}
	end := "2026-12-01"
	store.meta.EndDate = &end
	c, _ := newSvc(store).Checklist(context.Background(), 1)
	assertHasCode(t, c.Blockers, "SCHEDULE_STALE")
}

func TestSeasonService_Checklist_NoSchedule_Blocked(t *testing.T) {
	capID := int64(1)
	store := &stubSeasonStore{
		meta: seasons.SeasonMeta{TeamsManaged: true},
		teams: []seasons.TeamSummary{
			{TeamID: 1, Name: "A", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
			{TeamID: 2, Name: "B", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
		},
		matchCount: 0,
	}
	c, _ := newSvc(store).Checklist(context.Background(), 1)
	assertHasCode(t, c.Blockers, "NO_SCHEDULE")
}

func TestSeasonService_Checklist_AllGood_CanActivate(t *testing.T) {
	capID := int64(1)
	end := "2026-12-01"
	store := &stubSeasonStore{
		meta: seasons.SeasonMeta{TeamsManaged: true, EndDate: &end},
		teams: []seasons.TeamSummary{
			{TeamID: 1, Name: "A", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
			{TeamID: 2, Name: "B", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
		},
		matchCount: 1,
	}
	c, err := newSvc(store).Checklist(context.Background(), 1)
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	if !c.CanActivate {
		t.Errorf("want CanActivate=true, got blockers=%v", c.Blockers)
	}
}

// ── Activate ──────────────────────────────────────────────────────────────────

func TestSeasonService_Activate_NotFound(t *testing.T) {
	store := &stubSeasonStore{metaErr: seasons.ErrNotFound}
	err := newSvc(store).Activate(context.Background(), 99)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound in chain, got %v", err)
	}
}

func TestSeasonService_Activate_LegacySeason_CallsStoreActivate(t *testing.T) {
	store := &stubSeasonStore{meta: seasons.SeasonMeta{TeamsManaged: false}}
	if err := newSvc(store).Activate(context.Background(), 1); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !store.activateCalled {
		t.Error("want store.Activate to be called for legacy season")
	}
}

func TestSeasonService_Activate_ChecklistBlockers_ReturnsBlockErr(t *testing.T) {
	store := &stubSeasonStore{
		meta:  seasons.SeasonMeta{TeamsManaged: true},
		teams: []seasons.TeamSummary{}, // TEAMS_TOO_FEW
	}
	err := newSvc(store).Activate(context.Background(), 1)
	var blockErr *seasons.ChecklistBlockErr
	if !errors.As(err, &blockErr) {
		t.Fatalf("want *ChecklistBlockErr, got %T: %v", err, err)
	}
	if len(blockErr.Blockers) == 0 {
		t.Error("want non-empty blockers")
	}
}

func TestSeasonService_Activate_AllGood_CallsStoreActivate(t *testing.T) {
	capID := int64(1)
	end := "2026-12-01"
	store := &stubSeasonStore{
		meta: seasons.SeasonMeta{TeamsManaged: true, EndDate: &end},
		teams: []seasons.TeamSummary{
			{TeamID: 1, Name: "A", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
			{TeamID: 2, Name: "B", CaptainID: &capID, RosterCount: 3, CaptainOnRoster: true},
		},
		matchCount: 1,
	}
	if err := newSvc(store).Activate(context.Background(), 1); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !store.activateCalled {
		t.Error("want store.Activate to be called when checklist passes")
	}
}

// ── PreviousSeason ────────────────────────────────────────────────────────────

func TestSeasonService_PreviousSeason_NotFound(t *testing.T) {
	store := &stubSeasonStore{metaErr: seasons.ErrNotFound}
	_, err := newSvc(store).PreviousSeason(context.Background(), 99)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound in chain, got %v", err)
	}
}

func TestSeasonService_PreviousSeason_NoneExists_ReturnsNilSeason(t *testing.T) {
	store := &stubSeasonStore{
		meta:       seasons.SeasonMeta{LeagueID: 1},
		findActive: nil,
		findPrior:  nil,
	}
	result, err := newSvc(store).PreviousSeason(context.Background(), 2)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season != nil {
		t.Errorf("want nil season, got %+v", result.Season)
	}
	if result.Teams == nil {
		t.Error("want non-nil teams slice")
	}
}

func TestSeasonService_PreviousSeason_PrefersActiveWithNoEndDate(t *testing.T) {
	activeSeason := &models.Season{ID: 10}
	store := &stubSeasonStore{
		meta:       seasons.SeasonMeta{LeagueID: 1},
		findActive: activeSeason,
		seasonTeams: []seasons.SeasonTeamEntry{
			{TeamID: 1, TeamName: "Alpha", SeasonName: "Alpha"},
		},
	}
	result, err := newSvc(store).PreviousSeason(context.Background(), 2)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season == nil || result.Season.ID != 10 {
		t.Errorf("want season id=10, got %v", result.Season)
	}
}

func TestSeasonService_PreviousSeason_FallsBackToClosestPrior(t *testing.T) {
	priorSeason := &models.Season{ID: 5}
	store := &stubSeasonStore{
		meta:      seasons.SeasonMeta{LeagueID: 1},
		findPrior: priorSeason,
		seasonTeams: []seasons.SeasonTeamEntry{
			{TeamID: 1, TeamName: "Beta", SeasonName: "Beta"},
		},
	}
	result, err := newSvc(store).PreviousSeason(context.Background(), 2)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season == nil || result.Season.ID != 5 {
		t.Errorf("want season id=5, got %v", result.Season)
	}
}

func TestSeasonService_PreviousSeason_FallsBackToMatchTeams(t *testing.T) {
	prev := &models.Season{ID: 3}
	store := &stubSeasonStore{
		meta:        seasons.SeasonMeta{LeagueID: 1},
		findActive:  prev,
		seasonTeams: nil, // empty → falls back
		matchTeams: []seasons.SeasonTeamEntry{
			{TeamID: 7, TeamName: "Gamma", SeasonName: "Gamma"},
		},
	}
	result, err := newSvc(store).PreviousSeason(context.Background(), 2)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if len(result.Teams) != 1 || result.Teams[0].TeamID != 7 {
		t.Errorf("want match-teams fallback, got %v", result.Teams)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertHasCode(t *testing.T, items []models.ChecklistItem, code string) {
	t.Helper()
	for _, it := range items {
		if it.Code == code {
			return
		}
	}
	t.Errorf("expected checklist code %q, got %v", code, items)
}
