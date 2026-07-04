package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// stubMatchStore implements MatchStore for unit testing.
type stubMatchStore struct {
	listResult []models.Match
	listErr    error
	getResult  models.MatchDetail
	getErr     error
	assignErr  error

	// captured args
	lastListReq    matches.ListMatchesRequest
	lastGetID      int64
	lastAssignID   int64
	lastHomeTeamID *int64
	lastAwayTeamID *int64
}

func (s *stubMatchStore) ListMatches(_ context.Context, req matches.ListMatchesRequest) ([]models.Match, error) {
	s.lastListReq = req
	return s.listResult, s.listErr
}

func (s *stubMatchStore) GetMatch(_ context.Context, id int64) (models.MatchDetail, error) {
	s.lastGetID = id
	return s.getResult, s.getErr
}

func (s *stubMatchStore) AssignMatchTeams(_ context.Context, id int64, home, away *int64) error {
	s.lastAssignID = id
	s.lastHomeTeamID = home
	s.lastAwayTeamID = away
	return s.assignErr
}

func TestListMatches_ReturnsEmptySliceWhenNone(t *testing.T) {
	svc := matches.NewMatchService(&stubMatchStore{listResult: nil})
	ms, err := svc.ListMatches(context.Background(), matches.ListMatchesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ms == nil {
		t.Error("want non-nil empty slice, got nil")
	}
	if len(ms) != 0 {
		t.Errorf("want 0 matches, got %d", len(ms))
	}
}

func TestListMatches_PassesSeasonIDToStore(t *testing.T) {
	stub := &stubMatchStore{listResult: []models.Match{{ID: 1}}}
	svc := matches.NewMatchService(stub)
	req := matches.ListMatchesRequest{SeasonID: 42}
	_, _ = svc.ListMatches(context.Background(), req)
	if stub.lastListReq.SeasonID != 42 {
		t.Errorf("want season_id=42 passed to store, got %d", stub.lastListReq.SeasonID)
	}
}

func TestListMatches_PassesLeagueIDToStore(t *testing.T) {
	stub := &stubMatchStore{listResult: []models.Match{}}
	svc := matches.NewMatchService(stub)
	req := matches.ListMatchesRequest{LeagueID: 7}
	_, _ = svc.ListMatches(context.Background(), req)
	if stub.lastListReq.LeagueID != 7 {
		t.Errorf("want league_id=7 passed to store, got %d", stub.lastListReq.LeagueID)
	}
}

func TestListMatches_StoreErrorBecomesInternal(t *testing.T) {
	svc := matches.NewMatchService(&stubMatchStore{listErr: errors.New("db down")})
	_, err := svc.ListMatches(context.Background(), matches.ListMatchesRequest{})
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}

func TestGetMatch_Found(t *testing.T) {
	want := models.MatchDetail{
		Match:   models.Match{ID: 5, SeasonID: 1},
		Results: []models.MatchResult{},
	}
	svc := matches.NewMatchService(&stubMatchStore{getResult: want})
	got, err := svc.GetMatch(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Match.ID != 5 {
		t.Errorf("want match id=5, got %d", got.Match.ID)
	}
}

func TestGetMatch_NotFoundBecomesDomainNotFound(t *testing.T) {
	svc := matches.NewMatchService(&stubMatchStore{getErr: matches.ErrMatchNotFound})
	_, err := svc.GetMatch(context.Background(), 99)
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.NotFound {
		t.Errorf("want NotFound category, got %v", de.Category)
	}
}

func TestGetMatch_StoreErrorBecomesInternal(t *testing.T) {
	svc := matches.NewMatchService(&stubMatchStore{getErr: errors.New("disk error")})
	_, err := svc.GetMatch(context.Background(), 1)
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}

func TestAssignMatchTeams_PassesArgsToStore(t *testing.T) {
	stub := &stubMatchStore{}
	svc := matches.NewMatchService(stub)
	home := int64(10)
	away := int64(20)
	if err := svc.AssignMatchTeams(context.Background(), 7, &home, &away); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastAssignID != 7 {
		t.Errorf("want match id=7, got %d", stub.lastAssignID)
	}
	if stub.lastHomeTeamID == nil || *stub.lastHomeTeamID != 10 {
		t.Errorf("want home_team_id=10, got %v", stub.lastHomeTeamID)
	}
	if stub.lastAwayTeamID == nil || *stub.lastAwayTeamID != 20 {
		t.Errorf("want away_team_id=20, got %v", stub.lastAwayTeamID)
	}
}

func TestAssignMatchTeams_NilTeamIDsPassedThrough(t *testing.T) {
	stub := &stubMatchStore{}
	svc := matches.NewMatchService(stub)
	if err := svc.AssignMatchTeams(context.Background(), 3, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastHomeTeamID != nil {
		t.Errorf("want nil home_team_id, got %v", stub.lastHomeTeamID)
	}
	if stub.lastAwayTeamID != nil {
		t.Errorf("want nil away_team_id, got %v", stub.lastAwayTeamID)
	}
}

func TestAssignMatchTeams_StoreErrorBecomesInternal(t *testing.T) {
	svc := matches.NewMatchService(&stubMatchStore{assignErr: errors.New("constraint")})
	err := svc.AssignMatchTeams(context.Background(), 1, nil, nil)
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}
