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

	// team management stubs
	teamLeagueID    int64
	teamLeagueIDErr error
	seasonTeam      models.SeasonTeam
	seasonTeamErr   error
	addCopyErr      error
	addNewID        int64
	addNewErr       error
	onRoster        bool
	onRosterErr     error
	updateMetaErr   error
	removeTeamErr   error

	// bye request stubs
	participatingCount    int
	participatingCountErr error
	teamInSeason          bool
	teamInSeasonErr       error
	dupBye                bool
	dupByeErr             error
	insertedBye           models.ByeRequest
	insertByeErr          error
	gotBye                models.ByeRequest
	getByeErr             error
	byeConflict           bool
	byeConflictErr        error
	setBye                models.ByeRequest
	setByeErr             error

	// roster stubs
	rosterEntries         []models.SeasonRosterEntry
	rosterErr             error
	playerRosterTeamID    int64
	playerRosterTeamFound bool
	playerRosterTeamErr   error
	insertedRosterEntry   models.SeasonRosterEntry
	insertRosterErr       error
	deleteRosterErr       error
	availablePlayers      []models.Player
	availablePlayersErr   error

	// season team list stubs
	seasonTeamList    []models.SeasonTeam
	seasonTeamListErr error

	// skipped week stubs
	skippedWeeks      []models.SkippedWeek
	skippedWeeksErr   error
	createdSkipWeek   models.SkippedWeek
	createSkipErr     error
	deleteSkipErr     error

	// bye list/delete stubs
	byeList       []models.ByeRequest
	byeListErr    error
	deleteByeErr  error

	// season CRUD stubs
	seasonList         []models.Season
	seasonListErr      error
	season             models.Season
	seasonErr          error
	createdSeason      models.Season
	createSeasonErr    error
	createSeasonFn     func(seasons.CreateSeasonInput) (models.Season, error)
	updatedSeason      models.Season
	updateSeasonErr    error
	updateSeasonFn     func(seasons.UpdateSeasonInput) (models.Season, error)
	deleteSeasonCalled bool
	deleteSeasonErr    error
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

// ── team management stubs ──────────────────────────────────────────────────

func (s *stubSeasonStore) GetTeamLeagueID(_ context.Context, _ int64) (int64, error) {
	return s.teamLeagueID, s.teamLeagueIDErr
}
func (s *stubSeasonStore) GetSeasonTeam(_ context.Context, _, _ int64) (models.SeasonTeam, error) {
	return s.seasonTeam, s.seasonTeamErr
}
func (s *stubSeasonStore) AddSeasonTeamCopy(_ context.Context, _, _, _ int64, _ bool) error {
	return s.addCopyErr
}
func (s *stubSeasonStore) AddSeasonTeamNew(_ context.Context, _, _ int64, _ string) (int64, error) {
	return s.addNewID, s.addNewErr
}
func (s *stubSeasonStore) CheckPlayerOnSeasonRoster(_ context.Context, _, _, _ int64) (bool, error) {
	return s.onRoster, s.onRosterErr
}
func (s *stubSeasonStore) UpdateSeasonTeamMeta(_ context.Context, _, _ int64, _ string, _ *int64) error {
	return s.updateMetaErr
}
func (s *stubSeasonStore) RemoveSeasonTeam(_ context.Context, _, _ int64) error {
	return s.removeTeamErr
}

// ── bye request stubs ──────────────────────────────────────────────────────

func (s *stubSeasonStore) CountParticipatingTeams(_ context.Context, _, _ int64, _ bool) (int, error) {
	return s.participatingCount, s.participatingCountErr
}
func (s *stubSeasonStore) CheckTeamInSeason(_ context.Context, _, _ int64) (bool, error) {
	return s.teamInSeason, s.teamInSeasonErr
}
func (s *stubSeasonStore) HasDuplicateBye(_ context.Context, _, _ int64, _ int) (bool, error) {
	return s.dupBye, s.dupByeErr
}
func (s *stubSeasonStore) InsertByeRequest(_ context.Context, _, _ int64, _ int, _ string) (models.ByeRequest, error) {
	return s.insertedBye, s.insertByeErr
}
func (s *stubSeasonStore) GetByeRequest(_ context.Context, _, _ int64) (models.ByeRequest, error) {
	return s.gotBye, s.getByeErr
}
func (s *stubSeasonStore) HasByeConflict(_ context.Context, _ int64, _ int, _ int64) (bool, error) {
	return s.byeConflict, s.byeConflictErr
}
func (s *stubSeasonStore) SetByeApproval(_ context.Context, _, _ int64, _ bool) (models.ByeRequest, error) {
	return s.setBye, s.setByeErr
}

// ── roster stubs ──────────────────────────────────────────────────────────────

func (s *stubSeasonStore) ListRoster(_ context.Context, _, _ int64) ([]models.SeasonRosterEntry, error) {
	return s.rosterEntries, s.rosterErr
}
func (s *stubSeasonStore) GetPlayerRosterTeam(_ context.Context, _, _ int64) (int64, bool, error) {
	return s.playerRosterTeamID, s.playerRosterTeamFound, s.playerRosterTeamErr
}
func (s *stubSeasonStore) InsertOrGetRosterPlayer(_ context.Context, _, _, _ int64) (models.SeasonRosterEntry, error) {
	return s.insertedRosterEntry, s.insertRosterErr
}
func (s *stubSeasonStore) DeleteRosterPlayer(_ context.Context, _, _, _ int64) error {
	return s.deleteRosterErr
}
func (s *stubSeasonStore) ListAvailablePlayers(_ context.Context, _ int64) ([]models.Player, error) {
	return s.availablePlayers, s.availablePlayersErr
}

// ── season team list stubs ────────────────────────────────────────────────────

func (s *stubSeasonStore) ListSeasonTeams(_ context.Context, _ int64) ([]models.SeasonTeam, error) {
	return s.seasonTeamList, s.seasonTeamListErr
}

// ── skipped week stubs ────────────────────────────────────────────────────────

func (s *stubSeasonStore) ListSkippedWeeks(_ context.Context, _ int64) ([]models.SkippedWeek, error) {
	return s.skippedWeeks, s.skippedWeeksErr
}
func (s *stubSeasonStore) CreateSkippedWeek(_ context.Context, _ int64, _, _ string) (models.SkippedWeek, error) {
	return s.createdSkipWeek, s.createSkipErr
}
func (s *stubSeasonStore) DeleteSkippedWeek(_ context.Context, _, _ int64) error {
	return s.deleteSkipErr
}

// ── bye list/delete stubs ─────────────────────────────────────────────────────

func (s *stubSeasonStore) ListByeRequests(_ context.Context, _ int64) ([]models.ByeRequest, error) {
	return s.byeList, s.byeListErr
}
func (s *stubSeasonStore) DeleteByeRequest(_ context.Context, _, _ int64) error {
	return s.deleteByeErr
}

// ── season CRUD stubs ─────────────────────────────────────────────────────────

func (s *stubSeasonStore) ListSeasons(_ context.Context, _ *int64) ([]models.Season, error) {
	return s.seasonList, s.seasonListErr
}
func (s *stubSeasonStore) GetSeason(_ context.Context, _ int64) (models.Season, error) {
	return s.season, s.seasonErr
}
func (s *stubSeasonStore) CreateSeason(_ context.Context, input seasons.CreateSeasonInput) (models.Season, error) {
	if s.createSeasonFn != nil {
		return s.createSeasonFn(input)
	}
	return s.createdSeason, s.createSeasonErr
}
func (s *stubSeasonStore) UpdateSeason(_ context.Context, _ int64, input seasons.UpdateSeasonInput) (models.Season, error) {
	if s.updateSeasonFn != nil {
		return s.updateSeasonFn(input)
	}
	return s.updatedSeason, s.updateSeasonErr
}
func (s *stubSeasonStore) DeleteSeason(_ context.Context, _ int64) error {
	s.deleteSeasonCalled = true
	return s.deleteSeasonErr
}
func (s *stubSeasonStore) FindActiveSeasonByLeague(_ context.Context, _ int64) (int64, bool, error) {
	return 0, false, nil
}
func (s *stubSeasonStore) RosterEligible(_ context.Context, _ int64, _ int) (bool, string, error) {
	return true, "", nil
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

// ── AddTeam ───────────────────────────────────────────────────────────────────

func TestSeasonService_AddTeam_ActiveSeason_Returns422(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: false}
	_, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{Name: "X"})
	if !domainerr.IsCategory(err, domainerr.Unprocessable) {
		t.Errorf("want Unprocessable, got %v", err)
	}
}

func TestSeasonService_AddTeam_ManagedNoFromSeason_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult: true,
		meta:          seasons.SeasonMeta{TeamsManaged: true, LeagueID: 1},
	}
	_, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{FromTeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestSeasonService_AddTeam_WrongPriorSeason_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult: true,
		meta:          seasons.SeasonMeta{LeagueID: 1},
		findActive:    &models.Season{ID: 99}, // prior season id=99
	}
	_, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{
		FromTeamID:   5,
		FromSeasonID: 42, // wrong — prior is 99
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for wrong prior season, got %v", err)
	}
}

func TestSeasonService_AddTeam_TeamNotInLeague_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult:   true,
		meta:            seasons.SeasonMeta{LeagueID: 1},
		teamLeagueID:    2, // different league
		teamLeagueIDErr: nil,
	}
	_, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{FromTeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for wrong league, got %v", err)
	}
}

func TestSeasonService_AddTeam_AlreadyInSeason_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult: true,
		meta:          seasons.SeasonMeta{LeagueID: 1},
		teamLeagueID:  1,
		addCopyErr:    fmt.Errorf("dup: %w", seasons.ErrTeamAlreadyInSeason),
	}
	_, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{FromTeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for duplicate team, got %v", err)
	}
}

func TestSeasonService_AddTeam_NoNameNoTeamID_Returns400(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: true, meta: seasons.SeasonMeta{LeagueID: 1}}
	_, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for empty request, got %v", err)
	}
}

func TestSeasonService_AddTeam_NewTeam_CallsStoreAndReturns(t *testing.T) {
	want := models.SeasonTeam{ID: 1, TeamID: 10, SeasonName: "New"}
	store := &stubSeasonStore{
		isDraftResult: true,
		meta:          seasons.SeasonMeta{LeagueID: 1},
		addNewID:      10,
		seasonTeam:    want,
	}
	got, err := newSvc(store).AddTeam(context.Background(), 1, seasons.AddTeamRequest{Name: "New"})
	if err != nil {
		t.Fatalf("AddTeam: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("want SeasonTeam.ID=%d, got %d", want.ID, got.ID)
	}
	if !store.staleCalled {
		t.Error("want MarkStaleIfScheduled called")
	}
}

// ── RemoveTeam ────────────────────────────────────────────────────────────────

func TestSeasonService_RemoveTeam_ActiveSeason_Returns422(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: false}
	err := newSvc(store).RemoveTeam(context.Background(), 1, 5)
	if !domainerr.IsCategory(err, domainerr.Unprocessable) {
		t.Errorf("want Unprocessable, got %v", err)
	}
}

func TestSeasonService_RemoveTeam_NotFound_Returns404(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult: true,
		removeTeamErr: fmt.Errorf("x: %w", seasons.ErrTeamNotInSeason),
	}
	err := newSvc(store).RemoveTeam(context.Background(), 1, 5)
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestSeasonService_RemoveTeam_HappyPath_CallsStale(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: true}
	if err := newSvc(store).RemoveTeam(context.Background(), 1, 5); err != nil {
		t.Fatalf("RemoveTeam: %v", err)
	}
	if !store.staleCalled {
		t.Error("want MarkStaleIfScheduled called")
	}
}

// ── UpdateTeam ────────────────────────────────────────────────────────────────

func TestSeasonService_UpdateTeam_ActiveSeason_Returns422(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: false}
	_, err := newSvc(store).UpdateTeam(context.Background(), 1, 5, seasons.UpdateTeamRequest{SeasonName: "X"})
	if !domainerr.IsCategory(err, domainerr.Unprocessable) {
		t.Errorf("want Unprocessable, got %v", err)
	}
}

func TestSeasonService_UpdateTeam_EmptyName_Returns400(t *testing.T) {
	store := &stubSeasonStore{isDraftResult: true}
	_, err := newSvc(store).UpdateTeam(context.Background(), 1, 5, seasons.UpdateTeamRequest{SeasonName: "  "})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for empty name, got %v", err)
	}
}

func TestSeasonService_UpdateTeam_CaptainNotOnRoster_Returns400(t *testing.T) {
	capID := int64(99)
	store := &stubSeasonStore{isDraftResult: true, onRoster: false}
	_, err := newSvc(store).UpdateTeam(context.Background(), 1, 5,
		seasons.UpdateTeamRequest{SeasonName: "A", CaptainID: &capID})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for captain not on roster, got %v", err)
	}
}

func TestSeasonService_UpdateTeam_TeamNotFound_Returns404(t *testing.T) {
	store := &stubSeasonStore{
		isDraftResult: true,
		onRoster:      true,
		updateMetaErr: fmt.Errorf("x: %w", seasons.ErrTeamNotInSeason),
	}
	_, err := newSvc(store).UpdateTeam(context.Background(), 1, 5,
		seasons.UpdateTeamRequest{SeasonName: "A"})
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestSeasonService_UpdateTeam_HappyPath_ReturnsSeasonTeam(t *testing.T) {
	want := models.SeasonTeam{ID: 2, SeasonName: "Updated"}
	store := &stubSeasonStore{isDraftResult: true, seasonTeam: want}
	got, err := newSvc(store).UpdateTeam(context.Background(), 1, 5,
		seasons.UpdateTeamRequest{SeasonName: "Updated"})
	if err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	if got.SeasonName != want.SeasonName {
		t.Errorf("want SeasonName=%q, got %q", want.SeasonName, got.SeasonName)
	}
}

// ── CreateByeRequest ──────────────────────────────────────────────────────────

func TestSeasonService_CreateByeRequest_SeasonNotFound(t *testing.T) {
	store := &stubSeasonStore{metaErr: seasons.ErrNotFound}
	_, err := newSvc(store).CreateByeRequest(context.Background(), 99, seasons.CreateByeRequestInput{})
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestSeasonService_CreateByeRequest_EvenTeams_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		meta:               seasons.SeasonMeta{LeagueID: 1},
		participatingCount: 4, // even
	}
	_, err := newSvc(store).CreateByeRequest(context.Background(), 1, seasons.CreateByeRequestInput{TeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for even team count, got %v", err)
	}
}

func TestSeasonService_CreateByeRequest_TeamNotInLeague_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		meta:               seasons.SeasonMeta{LeagueID: 1},
		participatingCount: 3,
		teamLeagueID:       2, // different league
	}
	_, err := newSvc(store).CreateByeRequest(context.Background(), 1, seasons.CreateByeRequestInput{TeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for team not in league, got %v", err)
	}
}

func TestSeasonService_CreateByeRequest_ManagedTeamNotInSeason_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		meta:               seasons.SeasonMeta{LeagueID: 1, TeamsManaged: true},
		participatingCount: 3,
		teamLeagueID:       1,
		teamInSeason:       false,
	}
	_, err := newSvc(store).CreateByeRequest(context.Background(), 1, seasons.CreateByeRequestInput{TeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for team not in managed season, got %v", err)
	}
}

func TestSeasonService_CreateByeRequest_Duplicate_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		meta:               seasons.SeasonMeta{LeagueID: 1},
		participatingCount: 3,
		teamLeagueID:       1,
		dupBye:             true,
	}
	_, err := newSvc(store).CreateByeRequest(context.Background(), 1, seasons.CreateByeRequestInput{TeamID: 5})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for duplicate bye, got %v", err)
	}
}

func TestSeasonService_CreateByeRequest_HappyPath_ReturnsInserted(t *testing.T) {
	want := models.ByeRequest{ID: 7, TeamID: 5, WeekNumber: 3}
	store := &stubSeasonStore{
		meta:               seasons.SeasonMeta{LeagueID: 1},
		participatingCount: 3,
		teamLeagueID:       1,
		insertedBye:        want,
	}
	got, err := newSvc(store).CreateByeRequest(context.Background(), 1,
		seasons.CreateByeRequestInput{TeamID: 5, WeekNumber: 3})
	if err != nil {
		t.Fatalf("CreateByeRequest: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("want ID=%d, got %d", want.ID, got.ID)
	}
}

// ── UpdateByeRequest ──────────────────────────────────────────────────────────

func TestSeasonService_UpdateByeRequest_NotFound_Returns404(t *testing.T) {
	store := &stubSeasonStore{getByeErr: fmt.Errorf("x: %w", seasons.ErrByeNotFound)}
	_, err := newSvc(store).UpdateByeRequest(context.Background(), 1, 99, true)
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestSeasonService_UpdateByeRequest_ApproveWeekZero_Returns400(t *testing.T) {
	store := &stubSeasonStore{gotBye: models.ByeRequest{WeekNumber: 0}}
	_, err := newSvc(store).UpdateByeRequest(context.Background(), 1, 1, true)
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for week-0 approval, got %v", err)
	}
}

func TestSeasonService_UpdateByeRequest_Conflict_Returns400(t *testing.T) {
	store := &stubSeasonStore{
		gotBye:      models.ByeRequest{WeekNumber: 3},
		byeConflict: true,
	}
	_, err := newSvc(store).UpdateByeRequest(context.Background(), 1, 1, true)
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput for conflict, got %v", err)
	}
}

func TestSeasonService_UpdateByeRequest_HappyPath_ReturnsUpdated(t *testing.T) {
	want := models.ByeRequest{ID: 1, Approved: true, WeekNumber: 3}
	store := &stubSeasonStore{
		gotBye: models.ByeRequest{WeekNumber: 3},
		setBye: want,
	}
	got, err := newSvc(store).UpdateByeRequest(context.Background(), 1, 1, true)
	if err != nil {
		t.Fatalf("UpdateByeRequest: %v", err)
	}
	if !got.Approved {
		t.Error("want Approved=true")
	}
}

func TestSeasonService_UpdateByeRequest_Unapprove_SkipsConflictCheck(t *testing.T) {
	want := models.ByeRequest{ID: 1, Approved: false, WeekNumber: 3}
	store := &stubSeasonStore{
		gotBye:      models.ByeRequest{WeekNumber: 3, Approved: true},
		byeConflict: true, // would block approval but not unapproval
		setBye:      want,
	}
	got, err := newSvc(store).UpdateByeRequest(context.Background(), 1, 1, false)
	if err != nil {
		t.Fatalf("UpdateByeRequest unapprove: %v", err)
	}
	if got.Approved {
		t.Error("want Approved=false after unapproval")
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
