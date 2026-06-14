package logic

import (
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// threeTeamEntries returns the natural 3-team single round-robin:
//
//	week 1: team 2 vs team 3  (team 1 bye)
//	week 2: team 1 vs team 3  (team 2 bye)
//	week 3: team 1 vs team 2  (team 3 bye)
func threeTeamEntries() []ScheduleEntry {
	return []ScheduleEntry{
		{HomeTeamID: 2, AwayTeamID: 3, WeekNumber: 1},
		{HomeTeamID: 1, AwayTeamID: 3, WeekNumber: 2},
		{HomeTeamID: 1, AwayTeamID: 2, WeekNumber: 3},
	}
}

// byeInWeek returns the team that has the natural bye in the given week.
func byeInWeek(entries []ScheduleEntry, week int) int64 {
	allTeams := make(map[int64]bool)
	weekPlaying := make(map[int64]bool)
	for _, e := range entries {
		if e.HomeTeamID != 0 {
			allTeams[e.HomeTeamID] = true
		}
		if e.AwayTeamID != 0 {
			allTeams[e.AwayTeamID] = true
		}
		if e.WeekNumber == week {
			weekPlaying[e.HomeTeamID] = true
			weekPlaying[e.AwayTeamID] = true
		}
	}
	for t := range allTeams {
		if !weekPlaying[t] {
			return t
		}
	}
	return 0
}

// matchCountInWeek counts matches in a given week.
func matchCountInWeek(entries []ScheduleEntry, week int) int {
	n := 0
	for _, e := range entries {
		if e.WeekNumber == week {
			n++
		}
	}
	return n
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestApplyByeRequests_EmptyMap(t *testing.T) {
	entries := threeTeamEntries()
	result := applyByeRequests(entries, nil)
	// With no requests the slice is returned as-is (same content).
	for _, e := range result {
		orig := entries[0]
		for _, o := range entries {
			if o.WeekNumber == e.WeekNumber {
				orig = o
				break
			}
		}
		_ = orig
	}
	if len(result) != len(entries) {
		t.Fatalf("want %d entries, got %d", len(entries), len(result))
	}
	for i := range entries {
		if entries[i] != result[i] {
			t.Errorf("entry %d changed: %+v → %+v", i, entries[i], result[i])
		}
	}
}

func TestApplyByeRequests_AlreadyNatural(t *testing.T) {
	// Requesting team 1's bye for week 1 — team 1 already has that bye naturally.
	result := applyByeRequests(threeTeamEntries(), map[int]int64{1: 1})
	if bye := byeInWeek(result, 1); bye != 1 {
		t.Errorf("week 1 bye: want team 1, got team %d", bye)
	}
	// Other weeks unchanged.
	if bye := byeInWeek(result, 2); bye != 2 {
		t.Errorf("week 2 bye: want team 2, got team %d", bye)
	}
	if bye := byeInWeek(result, 3); bye != 3 {
		t.Errorf("week 3 bye: want team 3, got team %d", bye)
	}
}

func TestApplyByeRequests_SingleSwap(t *testing.T) {
	// Move team 3's natural bye (week 3) to week 1.
	result := applyByeRequests(threeTeamEntries(), map[int]int64{1: 3})
	if bye := byeInWeek(result, 1); bye != 3 {
		t.Errorf("week 1 bye: want team 3, got team %d", bye)
	}
	// Total match count must be unchanged.
	if len(result) != 3 {
		t.Errorf("total matches: want 3, got %d", len(result))
	}
	for _, w := range []int{1, 2, 3} {
		if n := matchCountInWeek(result, w); n != 1 {
			t.Errorf("week %d: want 1 match, got %d", w, n)
		}
	}
}

// TestApplyByeRequests_TwoRequests_Independent applies two approved requests
// that affect separate, non-overlapping week pairs.
//
// Natural:   week1=team1 bye, week2=team2 bye, week3=team3 bye
// Requests:  team3 bye → week1,  team1 bye → week3
// Expected:  week1=team3, week2=team2, week3=team1
func TestApplyByeRequests_TwoRequests_Independent(t *testing.T) {
	result := applyByeRequests(threeTeamEntries(), map[int]int64{1: 3, 3: 1})
	if bye := byeInWeek(result, 1); bye != 3 {
		t.Errorf("week 1 bye: want team 3, got team %d", bye)
	}
	if bye := byeInWeek(result, 3); bye != 1 {
		t.Errorf("week 3 bye: want team 1, got team %d", bye)
	}
	// Week 2 should be unchanged.
	if bye := byeInWeek(result, 2); bye != 2 {
		t.Errorf("week 2 bye: want team 2, got team %d", bye)
	}
	if len(result) != 3 {
		t.Errorf("total matches: want 3, got %d", len(result))
	}
}

// TestApplyByeRequests_TwoRequests_Chained covers the bug the previous
// implementation exhibited: when multiple requests force a 3-cycle (not a
// simple pairwise swap), pairwise processing would displace one request.
//
// Natural:   week1=team1, week2=team2, week3=team3
// Requests:  team3 bye → week1,  team2 bye → week3
//   srcToNew: natural-week3 → dest1, natural-week2 → dest3, natural-week1 → dest2
//   i.e. a 3-cycle: 1→2, 2→3, 3→1
// Expected:  week1=team3, week2=team1, week3=team2
func TestApplyByeRequests_TwoRequests_Chained(t *testing.T) {
	result := applyByeRequests(threeTeamEntries(), map[int]int64{1: 3, 3: 2})
	if bye := byeInWeek(result, 1); bye != 3 {
		t.Errorf("week 1 bye: want team 3, got team %d", bye)
	}
	if bye := byeInWeek(result, 3); bye != 2 {
		t.Errorf("week 3 bye: want team 2, got team %d", bye)
	}
	if len(result) != 3 {
		t.Errorf("total matches: want 3, got %d", len(result))
	}
	for _, w := range []int{1, 2, 3} {
		if n := matchCountInWeek(result, w); n != 1 {
			t.Errorf("week %d: want 1 match, got %d", w, n)
		}
	}
}

// TestApplyByeRequests_Deterministic verifies that calling the function
// multiple times with the same input produces identical output.
func TestApplyByeRequests_Deterministic(t *testing.T) {
	byeByWeek := map[int]int64{1: 3, 3: 2}
	first := applyByeRequests(threeTeamEntries(), byeByWeek)
	for i := 0; i < 10; i++ {
		run := applyByeRequests(threeTeamEntries(), byeByWeek)
		for w := 1; w <= 3; w++ {
			if byeInWeek(run, w) != byeInWeek(first, w) {
				t.Errorf("run %d: week %d bye differs from first run", i+1, w)
			}
		}
	}
}

// TestSingleRoundRobin_ThreeTeams_TwoApprovedByes is an end-to-end test through
// the public API confirming that two approved bye requests on different weeks
// are both reflected in the generated schedule.
//
// 5-team single RR natural byes: 1→A, 2→D, 3→B, 4→E, 5→C
// Requests: E bye → week 1 (E natural week 4), B bye → week 2 (B natural week 3)
func TestSingleRoundRobin_FiveTeams_TwoApprovedByes(t *testing.T) {
	teamIDs := []int64{1, 2, 3, 4, 5} // A=1, B=2, C=3, D=4, E=5

	// Approved: E(5) → week1, B(2) → week2
	opts := ScheduleOptions{
		ByeByWeek: map[int]int64{1: 5, 2: 2},
	}
	entries, err := SingleRoundRobin(teamIDs, opts)
	if err != nil {
		t.Fatalf("SingleRoundRobin: %v", err)
	}

	if bye := byeInWeek(entries, 1); bye != 5 {
		t.Errorf("week 1 bye: want team E(5), got team %d", bye)
	}
	if bye := byeInWeek(entries, 2); bye != 2 {
		t.Errorf("week 2 bye: want team B(2), got team %d", bye)
	}

	// Total matches = 5*4/2 = 10 (single RR, 5 teams).
	if len(entries) != 10 {
		t.Errorf("total matches: want 10, got %d", len(entries))
	}
	// Every week has exactly 2 matches (5 teams → 2 matches + 1 bye per week).
	for w := 1; w <= 5; w++ {
		if n := matchCountInWeek(entries, w); n != 2 {
			t.Errorf("week %d: want 2 matches, got %d", w, n)
		}
	}
}
