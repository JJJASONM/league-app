package matches

import (
	"testing"

	"league_app/models"
)

// --- MatchOutcome -----------------------------------------------------------

func TestMatchOutcome_HomeWins(t *testing.T) {
	results := []models.MatchResult{
		{TeamID: 1, GamesWon: 7},
		{TeamID: 2, GamesWon: 2},
	}
	hp, ap := MatchOutcome(results, 1, 2)
	if hp != 2 || ap != 0 {
		t.Errorf("home wins: want (2,0), got (%d,%d)", hp, ap)
	}
}

func TestMatchOutcome_AwayWins(t *testing.T) {
	results := []models.MatchResult{
		{TeamID: 1, GamesWon: 3},
		{TeamID: 2, GamesWon: 6},
	}
	hp, ap := MatchOutcome(results, 1, 2)
	if hp != 0 || ap != 2 {
		t.Errorf("away wins: want (0,2), got (%d,%d)", hp, ap)
	}
}

func TestMatchOutcome_Tie(t *testing.T) {
	results := []models.MatchResult{
		{TeamID: 1, GamesWon: 5},
		{TeamID: 2, GamesWon: 5},
	}
	hp, ap := MatchOutcome(results, 1, 2)
	if hp != 1 || ap != 1 {
		t.Errorf("tie: want (1,1), got (%d,%d)", hp, ap)
	}
}

func TestMatchOutcome_EmptyResults(t *testing.T) {
	hp, ap := MatchOutcome(nil, 1, 2)
	if hp != 1 || ap != 1 {
		t.Errorf("empty results: want tie (1,1), got (%d,%d)", hp, ap)
	}
}

// --- ComputeStandings -------------------------------------------------------

func makeTeams(ids ...int64) []models.Team {
	out := make([]models.Team, len(ids))
	for i, id := range ids {
		out[i] = models.Team{ID: id, Name: "Team " + string(rune('A'+i))}
	}
	return out
}

func TestComputeStandings_NoMatchesAllTeamsPresent(t *testing.T) {
	// All registered teams appear in standings with zeroed stats even if they
	// have not played yet. The output length equals the team count.
	teams := makeTeams(1, 2)
	out := ComputeStandings(nil, nil, teams)
	if len(out) != 2 {
		t.Errorf("want 2 standings rows (all teams), got %d", len(out))
	}
	for _, s := range out {
		if s.Played != 0 || s.Points != 0 {
			t.Errorf("team %d: want zeroed stats, got Played=%d Points=%d", s.TeamID, s.Played, s.Points)
		}
	}
}

func TestComputeStandings_IncompleteMatchNotCounted(t *testing.T) {
	// An incomplete match does not contribute to team standings; teams still
	// appear in the output but with Played=0.
	teams := makeTeams(1, 2)
	matches := []models.Match{
		{ID: 1, HomeTeamID: 1, AwayTeamID: 2, Completed: false},
	}
	out := ComputeStandings(matches, map[int64][]models.MatchResult{}, teams)
	if len(out) != 2 {
		t.Errorf("want 2 rows (all teams present), got %d", len(out))
	}
	for _, s := range out {
		if s.Played != 0 {
			t.Errorf("team %d: incomplete match must not increment Played, got %d", s.TeamID, s.Played)
		}
	}
}

func TestComputeStandings_SingleWin(t *testing.T) {
	teams := makeTeams(1, 2)
	matches := []models.Match{
		{ID: 1, HomeTeamID: 1, AwayTeamID: 2, Completed: true},
	}
	results := map[int64][]models.MatchResult{
		1: {
			{TeamID: 1, GamesWon: 6, GamesLost: 3},
			{TeamID: 2, GamesWon: 3, GamesLost: 6},
		},
	}
	out := ComputeStandings(matches, results, teams)
	if len(out) != 2 {
		t.Fatalf("want 2 standings rows, got %d", len(out))
	}
	// Winner is first
	if out[0].TeamID != 1 {
		t.Errorf("winner should be team 1, got team %d", out[0].TeamID)
	}
	if out[0].Wins != 1 || out[0].Points != 2 {
		t.Errorf("winner: want Wins=1 Points=2, got Wins=%d Points=%d", out[0].Wins, out[0].Points)
	}
	if out[1].Losses != 1 || out[1].Points != 0 {
		t.Errorf("loser: want Losses=1 Points=0, got Losses=%d Points=%d", out[1].Losses, out[1].Points)
	}
}

func TestComputeStandings_Tie(t *testing.T) {
	teams := makeTeams(1, 2)
	matches := []models.Match{
		{ID: 1, HomeTeamID: 1, AwayTeamID: 2, Completed: true},
	}
	results := map[int64][]models.MatchResult{
		1: {
			{TeamID: 1, GamesWon: 5, GamesLost: 5},
			{TeamID: 2, GamesWon: 5, GamesLost: 5},
		},
	}
	out := ComputeStandings(matches, results, teams)
	if len(out) != 2 {
		t.Fatalf("want 2 rows, got %d", len(out))
	}
	for _, s := range out {
		if s.Ties != 1 || s.Points != 1 {
			t.Errorf("team %d: want Ties=1 Points=1, got Ties=%d Points=%d", s.TeamID, s.Ties, s.Points)
		}
	}
}

func TestComputeStandings_SortByPoints(t *testing.T) {
	teams := makeTeams(1, 2, 3)
	// Team 1 beats team 2; team 3 beats team 1; team 2 beats team 3.
	matches := []models.Match{
		{ID: 1, HomeTeamID: 1, AwayTeamID: 2, Completed: true},
		{ID: 2, HomeTeamID: 3, AwayTeamID: 1, Completed: true},
		{ID: 3, HomeTeamID: 2, AwayTeamID: 3, Completed: true},
	}
	win := func(winner, loser int64) []models.MatchResult {
		return []models.MatchResult{
			{TeamID: winner, GamesWon: 6, GamesLost: 3},
			{TeamID: loser, GamesWon: 3, GamesLost: 6},
		}
	}
	results := map[int64][]models.MatchResult{
		1: win(1, 2),
		2: win(3, 1),
		3: win(2, 3),
	}
	out := ComputeStandings(matches, results, teams)
	if len(out) != 3 {
		t.Fatalf("want 3 rows, got %d", len(out))
	}
	// Each team has 1 win and 1 loss = 2 points; all tied on points.
	for _, s := range out {
		if s.Points != 2 {
			t.Errorf("team %d: want 2 points, got %d", s.TeamID, s.Points)
		}
	}
}

func TestComputeStandings_WinPctCalculated(t *testing.T) {
	teams := makeTeams(1, 2)
	matches := []models.Match{
		{ID: 1, HomeTeamID: 1, AwayTeamID: 2, Completed: true},
	}
	results := map[int64][]models.MatchResult{
		1: {
			{TeamID: 1, GamesWon: 6, GamesLost: 3},
			{TeamID: 2, GamesWon: 3, GamesLost: 6},
		},
	}
	out := ComputeStandings(matches, results, teams)
	for _, s := range out {
		if s.TeamID == 1 && s.WinPct != 1.0 {
			t.Errorf("winner WinPct: want 1.0, got %v", s.WinPct)
		}
		if s.TeamID == 2 && s.WinPct != 0.0 {
			t.Errorf("loser WinPct: want 0.0, got %v", s.WinPct)
		}
	}
}
