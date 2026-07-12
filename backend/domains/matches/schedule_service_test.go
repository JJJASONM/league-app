package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
)

// ── stub ──────────────────────────────────────────────────────────────────────

type stubScheduleStore struct {
	meta          matches.ScheduleSeasonMeta
	metaErr       error
	byes          map[int]int64
	byesErr       error
	histIDs       []int64
	histErr       error
	schedIDs      []int64
	schedErr      error
	closedWeeks   bool
	closedWeeksErr error
	saveErr       error
	savedReq      matches.SaveScheduleRequest
}

func (s *stubScheduleStore) GetScheduleSeasonMeta(_ context.Context, _ int64) (matches.ScheduleSeasonMeta, error) {
	return s.meta, s.metaErr
}
func (s *stubScheduleStore) LoadByeRequests(_ context.Context, _ int64) (map[int]int64, error) {
	if s.byes == nil {
		return map[int]int64{}, s.byesErr
	}
	return s.byes, s.byesErr
}
func (s *stubScheduleStore) LoadTeamIDsFromHistory(_ context.Context, _ int64) ([]int64, error) {
	return s.histIDs, s.histErr
}
func (s *stubScheduleStore) LoadTeamIDsForSchedule(_ context.Context, _, _ int64, _ bool) ([]int64, error) {
	return s.schedIDs, s.schedErr
}
func (s *stubScheduleStore) HasClosedWeeks(_ context.Context, _ int64) (bool, error) {
	return s.closedWeeks, s.closedWeeksErr
}
func (s *stubScheduleStore) SaveGeneratedSchedule(_ context.Context, req matches.SaveScheduleRequest) error {
	s.savedReq = req
	return s.saveErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newSvc(store *stubScheduleStore) *matches.ScheduleService {
	return matches.NewScheduleService(store)
}

func twoTeams() []int64 { return []int64{1, 2} }

// ── tests ─────────────────────────────────────────────────────────────────────

func TestGenerateSchedule_DefaultsToDoubleRR(t *testing.T) {
	store := &stubScheduleStore{
		meta:     matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		schedIDs: twoTeams(),
	}
	svc := newSvc(store)
	result, err := svc.GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID: 1,
		// ScheduleType intentionally blank — should default to double_rr
	})
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if result.MatchesCreated == 0 {
		t.Error("want non-zero matches_created")
	}
	if store.savedReq.ScheduleType != "double_rr" {
		t.Errorf("want schedule_type=double_rr, got %q", store.savedReq.ScheduleType)
	}
}

func TestGenerateSchedule_SeasonNotFound(t *testing.T) {
	store := &stubScheduleStore{metaErr: matches.ErrSeasonNotFound}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{SeasonID: 99})
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestGenerateSchedule_ManagedWithFromSeasonID(t *testing.T) {
	store := &stubScheduleStore{
		meta: matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: true},
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		FromSeasonID: 5,
		ScheduleType: "double_rr",
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestGenerateSchedule_ManagedWithNoTeams(t *testing.T) {
	store := &stubScheduleStore{
		meta:     matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: true},
		schedIDs: []int64{}, // empty
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "double_rr",
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for no teams, got %v", err)
	}
	var de *domainerr.Err
	if errors.As(err, &de) && de.Code != "SCHEDULE_NO_TEAMS" {
		t.Errorf("want code SCHEDULE_NO_TEAMS, got %q", de.Code)
	}
}

func TestGenerateSchedule_NotEnoughTeams(t *testing.T) {
	store := &stubScheduleStore{
		meta:     matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		schedIDs: []int64{1}, // only 1 team
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "double_rr",
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestGenerateSchedule_CustomRequiresNumWeeks(t *testing.T) {
	store := &stubScheduleStore{
		meta:     matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		schedIDs: twoTeams(),
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "custom",
		NumWeeks:     0, // missing
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
	var de *domainerr.Err
	if errors.As(err, &de) && de.Code != "SCHEDULE_NUM_WEEKS_REQUIRED" {
		t.Errorf("want code SCHEDULE_NUM_WEEKS_REQUIRED, got %q", de.Code)
	}
}

func TestGenerateSchedule_BlanketType(t *testing.T) {
	store := &stubScheduleStore{
		meta: matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: true},
	}
	result, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:       1,
		ScheduleType:   "blanket",
		NumWeeks:       3,
		MatchesPerWeek: 2,
	})
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if result.MatchesCreated != 6 {
		t.Errorf("want 6 blanket matches (3 weeks × 2), got %d", result.MatchesCreated)
	}
}

func TestGenerateSchedule_SaveFails(t *testing.T) {
	store := &stubScheduleStore{
		meta:     matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		schedIDs: twoTeams(),
		saveErr:  errors.New("db locked"),
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "double_rr",
	})
	if err == nil {
		t.Fatal("want error from save, got nil")
	}
}

func TestGenerateSchedule_SuccessDoubleRR_TwoTeams(t *testing.T) {
	store := &stubScheduleStore{
		meta:     matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		schedIDs: twoTeams(),
	}
	result, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "double_rr",
		StartDate:    "2026-09-01",
	})
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	// Two teams: single_rr=1 match, double_rr=2 matches.
	if result.MatchesCreated != 2 {
		t.Errorf("want 2 matches for double_rr 2-team, got %d", result.MatchesCreated)
	}
	if result.EndDate == "" {
		t.Error("want non-empty EndDate when start date provided")
	}
}

func TestGenerateSchedule_UsesFromSeasonIDForLegacy(t *testing.T) {
	store := &stubScheduleStore{
		meta:    matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		histIDs: twoTeams(),
	}
	result, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "single_rr",
		FromSeasonID: 7,
	})
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if result.MatchesCreated == 0 {
		t.Error("want non-zero matches for legacy from_season_id path")
	}
}

func TestGenerateSchedule_BypassesTeamCheckForBlanket(t *testing.T) {
	// Blanket never calls LoadTeamIDsForSchedule; store returns empty by default.
	store := &stubScheduleStore{
		meta: matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: true},
		// schedIDs is nil — would trigger SCHEDULE_NO_TEAMS for non-blanket
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:       1,
		ScheduleType:   "blanket",
		NumWeeks:       2,
		MatchesPerWeek: 1,
	})
	if err != nil {
		t.Errorf("blanket must not check team count, got %v", err)
	}
}

func TestGenerateSchedule_ClosedWeeksPresent_ReturnsConflict(t *testing.T) {
	store := &stubScheduleStore{
		meta:         matches.ScheduleSeasonMeta{LeagueID: 1, TeamsManaged: false},
		schedIDs:     twoTeams(),
		closedWeeks:  true,
	}
	_, err := newSvc(store).GenerateSchedule(context.Background(), matches.GenerateRequest{
		SeasonID:     1,
		ScheduleType: "double_rr",
	})
	if !domainerr.IsCategory(err, domainerr.Conflict) {
		t.Errorf("want Conflict error when closed weeks exist, got %v", err)
	}
	var de *domainerr.Err
	if errors.As(err, &de) && de.Code != "SCHEDULE_HAS_CLOSED_WEEKS" {
		t.Errorf("want code SCHEDULE_HAS_CLOSED_WEEKS, got %q", de.Code)
	}
}
