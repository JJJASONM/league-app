package matches_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// ─── stub store ──────────────────────────────────────────────────────────────

type stubRoundStore struct {
	weekClosed    bool
	weekClosedErr error

	matchCtx    matches.MatchContext
	matchCtxErr error

	playerHCs    map[int64]float64
	playerHCErr  error

	priorSnaps    []matches.PriorSnapshotRow
	priorSnapsErr error

	deleteRoundResultsErr error
	insertRoundResultErr  error
	deleteMatchResultsErr error
	insertMatchResultErr  error
	markCompletedErr      error
	markIncompleteErr     error

	roundResults    []models.RoundResult
	roundResultsErr error

	standingsData    matches.StandingsData
	standingsDataErr error

	playerStats    []models.PlayerStat
	playerStatsErr error

	submitMatchResultsErr error
	clearMatchResultsErr  error

	// capture what was passed to InsertRoundResult / InsertMatchResult
	insertedRoundRows  []matches.RoundResultRow
	insertedMatchRows  []matches.MatchResultRow
}

func (s *stubRoundStore) IsWeekClosed(_ context.Context, _ int64) (bool, error) {
	return s.weekClosed, s.weekClosedErr
}
func (s *stubRoundStore) RunTx(_ context.Context, fn func(matches.RoundStore) error) error {
	return fn(s)
}
func (s *stubRoundStore) LoadMatchContext(_ context.Context, _ int64) (matches.MatchContext, error) {
	return s.matchCtx, s.matchCtxErr
}
func (s *stubRoundStore) LoadPlayerHandicap(_ context.Context, pid int64) (float64, error) {
	if s.playerHCErr != nil {
		return 0, s.playerHCErr
	}
	return s.playerHCs[pid], nil
}
func (s *stubRoundStore) LoadPriorSnapshots(_ context.Context, _ int64) ([]matches.PriorSnapshotRow, error) {
	return s.priorSnaps, s.priorSnapsErr
}
func (s *stubRoundStore) DeleteRoundResults(_ context.Context, _ int64) error {
	return s.deleteRoundResultsErr
}
func (s *stubRoundStore) InsertRoundResult(_ context.Context, row matches.RoundResultRow) error {
	if s.insertRoundResultErr != nil {
		return s.insertRoundResultErr
	}
	s.insertedRoundRows = append(s.insertedRoundRows, row)
	return nil
}
func (s *stubRoundStore) DeleteMatchResults(_ context.Context, _ int64) error {
	return s.deleteMatchResultsErr
}
func (s *stubRoundStore) InsertMatchResult(_ context.Context, row matches.MatchResultRow) error {
	if s.insertMatchResultErr != nil {
		return s.insertMatchResultErr
	}
	s.insertedMatchRows = append(s.insertedMatchRows, row)
	return nil
}
func (s *stubRoundStore) MarkMatchCompleted(_ context.Context, _ int64) error {
	return s.markCompletedErr
}
func (s *stubRoundStore) MarkMatchIncomplete(_ context.Context, _ int64) error {
	return s.markIncompleteErr
}
func (s *stubRoundStore) GetRoundResults(_ context.Context, _ int64) ([]models.RoundResult, error) {
	return s.roundResults, s.roundResultsErr
}
func (s *stubRoundStore) GetStandingsData(_ context.Context, _ int64) (matches.StandingsData, error) {
	return s.standingsData, s.standingsDataErr
}
func (s *stubRoundStore) GetPlayerStats(_ context.Context, _ matches.PlayerStatsRequest) ([]models.PlayerStat, error) {
	return s.playerStats, s.playerStatsErr
}
func (s *stubRoundStore) SubmitMatchResults(_ context.Context, _ int64, _ []models.MatchResult) error {
	return s.submitMatchResultsErr
}
func (s *stubRoundStore) ClearMatchResults(_ context.Context, _ int64) error {
	return s.clearMatchResultsErr
}

// ─── helper ──────────────────────────────────────────────────────────────────

func newTestRoundSvc(store matches.RoundStore) *matches.RoundService {
	return matches.NewRoundService(store, &stubRuleStore{})
}

// ─── SaveRounds ──────────────────────────────────────────────────────────────

func TestSaveRounds_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{weekClosed: true}
	svc := newTestRoundSvc(store)
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{MatchID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Conflict {
		t.Errorf("want domainerr.Conflict, got %v", err)
	}
}

func TestSaveRounds_ValidationError_ReturnsRoundValidationError(t *testing.T) {
	store := &stubRoundStore{
		weekClosed:  false,
matchCtx:    matches.MatchContext{SeasonID: 1, HomeTeamID: 10, AwayTeamID: 20},
		playerHCs:   map[int64]float64{1: 1.0, 2: 2.0},
	}
	svc := newTestRoundSvc(store)
	// Submit a round with both players scoring 10 in the same game (validation error).
	input := matches.SaveRoundsInput{
		MatchID: 1,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2,
				Game1Home: 10, Game1Away: 10}, // both winners — invalid
		},
	}
	err := svc.SaveRounds(context.Background(), input)
	var vErr *matches.RoundValidationError
	if !errors.As(err, &vErr) {
		t.Errorf("want *RoundValidationError, got %T: %v", err, err)
	}
}

func TestSaveRounds_Happy_InsertsRoundAndMatchResults(t *testing.T) {
	store := &stubRoundStore{
		weekClosed:  false,
matchCtx:    matches.MatchContext{SeasonID: 1, HomeTeamID: 10, AwayTeamID: 20},
		playerHCs:   map[int64]float64{1: 0.0, 2: 0.0},
	}
	svc := newTestRoundSvc(store)
	input := matches.SaveRoundsInput{
		MatchID: 7,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2,
				Game1Home: 10, Game1Away: 3,
				Game2Home: 10, Game2Away: 5,
				Game3Home: 10, Game3Away: 2},
		},
	}
	if err := svc.SaveRounds(context.Background(), input); err != nil {
		t.Fatalf("SaveRounds: %v", err)
	}
	if len(store.insertedRoundRows) != 1 {
		t.Errorf("want 1 round row inserted, got %d", len(store.insertedRoundRows))
	}
	if len(store.insertedMatchRows) != 2 {
		t.Errorf("want 2 match result rows (one per player), got %d", len(store.insertedMatchRows))
	}
}

func TestSaveRounds_SnapshotPreservation_SameHomePlayer(t *testing.T) {
	// Prior row has homePlayerID=1 with home_handicap_used=3.0.
	// Resubmitting with same player should preserve 3.0, not use currentHC=1.0.
	store := &stubRoundStore{
		weekClosed:  false,
matchCtx:    matches.MatchContext{SeasonID: 1, HomeTeamID: 10, AwayTeamID: 20},
		playerHCs:   map[int64]float64{1: 1.0, 2: 2.0},
		priorSnaps: []matches.PriorSnapshotRow{
			{
				RoundNumber:      1,
				HomePlayerID:     1,
				AwayPlayerID:     2,
				HomeHandicapUsed: sql.NullFloat64{Float64: 3.0, Valid: true},
				AwayHandicapUsed: sql.NullFloat64{Float64: 4.0, Valid: true},
			},
		},
	}
	svc := newTestRoundSvc(store)
	input := matches.SaveRoundsInput{
		MatchID: 7,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2,
				Game1Home: 10, Game1Away: 3,
				Game2Home: 10, Game2Away: 5,
				Game3Home: 10, Game3Away: 2},
		},
	}
	if err := svc.SaveRounds(context.Background(), input); err != nil {
		t.Fatalf("SaveRounds: %v", err)
	}
	if len(store.insertedRoundRows) != 1 {
		t.Fatalf("want 1 round row, got %d", len(store.insertedRoundRows))
	}
	row := store.insertedRoundRows[0]
	if row.HomeHCUsed != 3.0 {
		t.Errorf("want preserved home HC 3.0, got %v", row.HomeHCUsed)
	}
	if row.AwayHCUsed != 4.0 {
		t.Errorf("want preserved away HC 4.0, got %v", row.AwayHCUsed)
	}
}

func TestSaveRounds_Substitution_FreshSnapshot(t *testing.T) {
	// Prior row has homePlayerID=1. Submitting homePlayerID=99 (sub) should use
	// currentHC for player 99, not any prior snapshot.
	store := &stubRoundStore{
		weekClosed:  false,
matchCtx:    matches.MatchContext{SeasonID: 1, HomeTeamID: 10, AwayTeamID: 20},
		playerHCs:   map[int64]float64{99: 5.0, 2: 2.0},
		priorSnaps: []matches.PriorSnapshotRow{
			{
				RoundNumber:      1,
				HomePlayerID:     1, // different player
				AwayPlayerID:     2,
				HomeHandicapUsed: sql.NullFloat64{Float64: 3.0, Valid: true},
				AwayHandicapUsed: sql.NullFloat64{Float64: 2.0, Valid: true},
			},
		},
	}
	svc := newTestRoundSvc(store)
	input := matches.SaveRoundsInput{
		MatchID: 7,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 99, AwayPlayerID: 2,
				Game1Home: 10, Game1Away: 3,
				Game2Home: 10, Game2Away: 4,
				Game3Home: 10, Game3Away: 2},
		},
	}
	if err := svc.SaveRounds(context.Background(), input); err != nil {
		t.Fatalf("SaveRounds: %v", err)
	}
	if len(store.insertedRoundRows) != 1 {
		t.Fatalf("want 1 round row, got %d", len(store.insertedRoundRows))
	}
	row := store.insertedRoundRows[0]
	if row.HomeHCUsed != 5.0 {
		t.Errorf("want fresh home HC 5.0 for sub, got %v", row.HomeHCUsed)
	}
	// Away player same as prior → preserve
	if row.AwayHCUsed != 2.0 {
		t.Errorf("want preserved away HC 2.0, got %v", row.AwayHCUsed)
	}
}

func TestSaveRounds_AmbiguousSnapshot_ReturnsUnprocessable(t *testing.T) {
	// Away player 2 appears in two prior rows for the same round — ambiguous.
	store := &stubRoundStore{
		weekClosed:  false,
matchCtx:    matches.MatchContext{SeasonID: 1, HomeTeamID: 10, AwayTeamID: 20},
		playerHCs:   map[int64]float64{99: 5.0, 2: 2.0},
		priorSnaps: []matches.PriorSnapshotRow{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2,
				AwayHandicapUsed: sql.NullFloat64{Float64: 2.0, Valid: true}},
			{RoundNumber: 1, HomePlayerID: 3, AwayPlayerID: 2,
				AwayHandicapUsed: sql.NullFloat64{Float64: 2.0, Valid: true}},
		},
	}
	svc := newTestRoundSvc(store)
	input := matches.SaveRoundsInput{
		MatchID: 7,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 99, AwayPlayerID: 2,
				Game1Home: 10, Game1Away: 3,
				Game2Home: 10, Game2Away: 4,
				Game3Home: 10, Game3Away: 2},
		},
	}
	err := svc.SaveRounds(context.Background(), input)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Unprocessable {
		t.Errorf("want domainerr.Unprocessable for ambiguous snapshot, got %v", err)
	}
}

// ─── GetRounds ───────────────────────────────────────────────────────────────

func TestGetRounds_EmptyMatch_ReturnsEmptySlice(t *testing.T) {
	store := &stubRoundStore{roundResults: nil}
	svc := newTestRoundSvc(store)
	rounds, err := svc.GetRounds(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRounds: %v", err)
	}
	if len(rounds) != 0 {
		t.Errorf("want empty slice, got %d rows", len(rounds))
	}
}

func TestGetRounds_WithSnapshot_UsesSnapshot(t *testing.T) {
	// Row has handicap_pts_used=3 and handicap_to_used="home" — snapshot must be used.
	pts := 3
	to := "home"
	store := &stubRoundStore{
		matchCtx:    matches.MatchContext{SeasonID: 1},
roundResults: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2,
				HomeHandicap: 1.0, AwayHandicap: 2.0,
				Game1Home: 10, Game1Away: 3,
				Game2Home: 10, Game2Away: 4,
				Game3Home: 10, Game3Away: 2,
				HandicapPtsUsed: &pts,
				HandicapToUsed:  &to,
			},
		},
	}
	svc := newTestRoundSvc(store)
	rounds, err := svc.GetRounds(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRounds: %v", err)
	}
	if len(rounds) != 1 {
		t.Fatalf("want 1 round, got %d", len(rounds))
	}
	if rounds[0].HandicapPts != 3 || rounds[0].HandicapTo != "home" {
		t.Errorf("want HandicapPts=3 HandicapTo=home from snapshot, got pts=%d to=%q",
			rounds[0].HandicapPts, rounds[0].HandicapTo)
	}
}

func TestGetRounds_NoSnapshot_ComputesFromHC(t *testing.T) {
	// Row has no snapshot — should compute from HomeHandicap/AwayHandicap.
	// HC diff = 0.0-2.0 = -2.0, abs=2.0, *2.55=5.1 → 5 balls to home.
	store := &stubRoundStore{
		matchCtx:    matches.MatchContext{SeasonID: 1},
roundResults: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2,
				HomeHandicap: 0.0, AwayHandicap: 2.0,
				Game1Home: 10, Game1Away: 3,
				Game2Home: 10, Game2Away: 4,
				Game3Home: 10, Game3Away: 2,
			},
		},
	}
	svc := newTestRoundSvc(store)
	rounds, err := svc.GetRounds(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRounds: %v", err)
	}
	if len(rounds) != 1 {
		t.Fatalf("want 1 round, got %d", len(rounds))
	}
	if rounds[0].HandicapTo != "home" {
		t.Errorf("want handicap_to=home (home is lower-rated), got %q", rounds[0].HandicapTo)
	}
}

// ─── GetStandings ────────────────────────────────────────────────────────────

func TestGetStandings_EmptySeason_ReturnsEmptySlice(t *testing.T) {
	store := &stubRoundStore{standingsData: matches.StandingsData{
		ResultMap: make(map[int64][]models.MatchResult),
	}}
	svc := newTestRoundSvc(store)
	standings, err := svc.GetStandings(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetStandings: %v", err)
	}
	if len(standings) != 0 {
		t.Errorf("want empty slice, got %d standings", len(standings))
	}
}

func TestGetStandings_StoreError_Propagates(t *testing.T) {
	store := &stubRoundStore{standingsDataErr: errors.New("db offline")}
	svc := newTestRoundSvc(store)
	_, err := svc.GetStandings(context.Background(), 1)
	if err == nil {
		t.Fatal("want error from store, got nil")
	}
}

// ─── GetPlayerStats ──────────────────────────────────────────────────────────

func TestGetPlayerStats_NoScope_ReturnsEmpty(t *testing.T) {
	store := &stubRoundStore{}
	svc := newTestRoundSvc(store)
	stats, err := svc.GetPlayerStats(context.Background(), matches.PlayerStatsRequest{})
	if err != nil {
		t.Fatalf("GetPlayerStats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("want empty slice with no scope, got %d", len(stats))
	}
}

func TestGetPlayerStats_WinPctComputed(t *testing.T) {
	store := &stubRoundStore{
		playerStats: []models.PlayerStat{
			{PlayerID: 1, GamesWon: 6, GamesLost: 2},
		},
	}
	svc := newTestRoundSvc(store)
	stats, err := svc.GetPlayerStats(context.Background(), matches.PlayerStatsRequest{SeasonID: 1})
	if err != nil {
		t.Fatalf("GetPlayerStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("want 1 stat, got %d", len(stats))
	}
	want := 0.75
	if stats[0].WinPct != want {
		t.Errorf("want WinPct=%.2f, got %.2f", want, stats[0].WinPct)
	}
}

// ─── SubmitResults ───────────────────────────────────────────────────────────

func TestSubmitResults_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{weekClosed: true}
	svc := newTestRoundSvc(store)
	err := svc.SubmitResults(context.Background(), 1, nil)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Conflict {
		t.Errorf("want Conflict, got %v", err)
	}
}

func TestSubmitResults_Open_DelegatesToStore(t *testing.T) {
	store := &stubRoundStore{weekClosed: false}
	svc := newTestRoundSvc(store)
	if err := svc.SubmitResults(context.Background(), 1, nil); err != nil {
		t.Fatalf("SubmitResults: %v", err)
	}
}

// ─── ClearResults ────────────────────────────────────────────────────────────

func TestClearResults_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{weekClosed: true}
	svc := newTestRoundSvc(store)
	err := svc.ClearResults(context.Background(), 1)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Conflict {
		t.Errorf("want Conflict, got %v", err)
	}
}

func TestClearResults_Open_DelegatesToStore(t *testing.T) {
	store := &stubRoundStore{weekClosed: false}
	svc := newTestRoundSvc(store)
	if err := svc.ClearResults(context.Background(), 1); err != nil {
		t.Fatalf("ClearResults: %v", err)
	}
}
