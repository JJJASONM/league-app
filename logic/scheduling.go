package logic

import (
	"fmt"
	"sort"
	"time"
)

// ScheduleEntry is one match slot in the generated schedule.
// HomeTeamID/AwayTeamID may be 0 for unassigned (blanket-template) slots.
type ScheduleEntry struct {
	HomeTeamID int64
	AwayTeamID int64
	WeekNumber int
	MatchDate  string // YYYY-MM-DD; may be "" if no start date supplied
}

// ScheduleOptions holds parameters shared across all schedule generators.
type ScheduleOptions struct {
	StartDate time.Time
	SkipDates []time.Time     // calendar dates to skip when assigning week dates
	NumWeeks  int             // for "custom": total weeks; 0 = full cycle
	ByeByWeek map[int]int64   // week number → team ID that should receive the natural bye (approved bye requests)
}

// SingleRoundRobin generates a schedule where each team plays every other team once.
func SingleRoundRobin(teamIDs []int64, opts ScheduleOptions) ([]ScheduleEntry, error) {
	entries, err := roundRobinBase(teamIDs)
	if err != nil {
		return nil, err
	}
	return assignDates(entries, opts), nil
}

// DoubleRoundRobin generates a full home-and-away schedule (each pair plays twice).
// This is the classic double round-robin: first half + reversed second half.
func DoubleRoundRobin(teamIDs []int64, opts ScheduleOptions) ([]ScheduleEntry, error) {
	first, err := roundRobinBase(teamIDs)
	if err != nil {
		return nil, err
	}
	firstRounds := maxWeekNum(first)
	entries := make([]ScheduleEntry, len(first))
	copy(entries, first)
	for _, e := range first {
		entries = append(entries, ScheduleEntry{
			HomeTeamID: e.AwayTeamID,
			AwayTeamID: e.HomeTeamID,
			WeekNumber: e.WeekNumber + firstRounds,
		})
	}
	return assignDates(entries, opts), nil
}

// SplitSeason generates a double round-robin; the UI separates standings by half at the midpoint.
func SplitSeason(teamIDs []int64, opts ScheduleOptions) ([]ScheduleEntry, error) {
	return DoubleRoundRobin(teamIDs, opts)
}

// CustomSchedule generates exactly opts.NumWeeks weeks, cycling through the double-RR
// pairings as needed (repeating if NumWeeks exceeds one full cycle).
func CustomSchedule(teamIDs []int64, opts ScheduleOptions) ([]ScheduleEntry, error) {
	if opts.NumWeeks < 1 {
		return nil, fmt.Errorf("num_weeks must be at least 1")
	}
	base, err := DoubleRoundRobin(teamIDs, ScheduleOptions{})
	if err != nil {
		return nil, err
	}
	cycleLen := maxWeekNum(base)
	var entries []ScheduleEntry
	pass := 0
outer:
	for {
		for _, e := range base {
			wk := e.WeekNumber + pass*cycleLen
			if wk > opts.NumWeeks {
				break outer
			}
			entries = append(entries, ScheduleEntry{
				HomeTeamID: e.HomeTeamID,
				AwayTeamID: e.AwayTeamID,
				WeekNumber: wk,
			})
		}
		pass++
	}
	return assignDates(entries, opts), nil
}

// BlanketTemplate creates numWeeks × matchesPerWeek empty match slots (team IDs = 0).
// These are meant to be filled in manually via the schedule editor.
func BlanketTemplate(numWeeks, matchesPerWeek int, opts ScheduleOptions) ([]ScheduleEntry, error) {
	if numWeeks < 1 {
		return nil, fmt.Errorf("num_weeks must be at least 1")
	}
	if matchesPerWeek < 1 {
		return nil, fmt.Errorf("matches_per_week must be at least 1")
	}
	var entries []ScheduleEntry
	for w := 1; w <= numWeeks; w++ {
		for range matchesPerWeek {
			entries = append(entries, ScheduleEntry{WeekNumber: w})
		}
	}
	return assignDates(entries, opts), nil
}

// roundRobinBase generates one complete round-robin pass (unordered, no dates).
// Adds a bye placeholder (ID 0) if team count is odd.
func roundRobinBase(teamIDs []int64) ([]ScheduleEntry, error) {
	if len(teamIDs) < 2 {
		return nil, fmt.Errorf("need at least 2 teams to generate a schedule")
	}
	teams := make([]int64, len(teamIDs))
	copy(teams, teamIDs)
	if len(teams)%2 != 0 {
		teams = append(teams, 0) // bye slot
	}
	n := len(teams)
	rounds := n - 1
	half := n / 2

	var entries []ScheduleEntry
	for round := 0; round < rounds; round++ {
		week := round + 1
		for i := 0; i < half; i++ {
			home := teams[i]
			away := teams[n-1-i]
			if home == 0 || away == 0 {
				continue // skip bye matchups
			}
			entries = append(entries, ScheduleEntry{
				HomeTeamID: home,
				AwayTeamID: away,
				WeekNumber: week,
			})
		}
		// Rotate: keep teams[0] fixed, rotate the rest
		last := teams[n-1]
		copy(teams[2:], teams[1:n-1])
		teams[1] = last
	}
	return entries, nil
}

func maxWeekNum(entries []ScheduleEntry) int {
	max := 0
	for _, e := range entries {
		if e.WeekNumber > max {
			max = e.WeekNumber
		}
	}
	return max
}

// applyByeRequests reorders week assignments so approved bye requests are honoured.
// For odd-team schedules one team naturally sits out each week; byeByWeek maps a
// requested week number to the team that should receive that week's natural bye.
//
// The algorithm builds a full deterministic permutation of week numbers:
//   1. For each approved request, map the team's natural-bye source week to the
//      requested destination week.
//   2. Pair the remaining unclaimed source weeks with the remaining unclaimed
//      destination weeks in sorted order, preserving relative sequence.
//
// This handles independent and chained requests correctly regardless of the
// non-deterministic iteration order of Go maps.
func applyByeRequests(entries []ScheduleEntry, byeByWeek map[int]int64) []ScheduleEntry {
	if len(byeByWeek) == 0 {
		return entries
	}

	// Collect all real team IDs.
	allTeams := make(map[int64]bool)
	for _, e := range entries {
		if e.HomeTeamID != 0 {
			allTeams[e.HomeTeamID] = true
		}
		if e.AwayTeamID != 0 {
			allTeams[e.AwayTeamID] = true
		}
	}

	// Discover which team has the natural bye each week.
	weekPlaying := make(map[int]map[int64]bool)
	for _, e := range entries {
		if weekPlaying[e.WeekNumber] == nil {
			weekPlaying[e.WeekNumber] = make(map[int64]bool)
		}
		weekPlaying[e.WeekNumber][e.HomeTeamID] = true
		weekPlaying[e.WeekNumber][e.AwayTeamID] = true
	}
	naturalByeWeek := make(map[int64]int) // team → week where it has the natural bye
	for w, playing := range weekPlaying {
		for t := range allTeams {
			if !playing[t] {
				naturalByeWeek[t] = w
				break
			}
		}
	}

	// All weeks in sorted order for deterministic output.
	allWeeks := make([]int, 0, len(weekPlaying))
	for w := range weekPlaying {
		allWeeks = append(allWeeks, w)
	}
	sort.Ints(allWeeks)

	// Sort requests by week so processing order is deterministic.
	type byeReq struct {
		week int
		team int64
	}
	reqs := make([]byeReq, 0, len(byeByWeek))
	for w, t := range byeByWeek {
		reqs = append(reqs, byeReq{w, t})
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].week < reqs[j].week })

	// srcToNew[originalWeek] = newWeek — where each source week's content ends up.
	srcToNew := make(map[int]int)
	usedAsNew := make(map[int]bool)

	for _, req := range reqs {
		srcWeek, ok := naturalByeWeek[req.team]
		if !ok || usedAsNew[req.week] {
			continue
		}
		srcToNew[srcWeek] = req.week
		usedAsNew[req.week] = true
	}

	// Pair remaining unclaimed source weeks with unclaimed destination weeks,
	// preserving their original relative order.
	remainingSrc := make([]int, 0, len(allWeeks))
	for _, w := range allWeeks {
		if _, claimed := srcToNew[w]; !claimed {
			remainingSrc = append(remainingSrc, w)
		}
	}
	remainingNew := make([]int, 0, len(allWeeks))
	for _, w := range allWeeks {
		if !usedAsNew[w] {
			remainingNew = append(remainingNew, w)
		}
	}
	for i := range remainingNew {
		srcToNew[remainingSrc[i]] = remainingNew[i]
	}

	result := make([]ScheduleEntry, len(entries))
	for i, e := range entries {
		if newWeek, ok := srcToNew[e.WeekNumber]; ok {
			e.WeekNumber = newWeek
		}
		result[i] = e
	}
	return result
}

// assignDates maps sequential week numbers to actual calendar dates,
// stepping by 7 days per week and skipping any dates in opts.SkipDates.
func assignDates(entries []ScheduleEntry, opts ScheduleOptions) []ScheduleEntry {
	if opts.StartDate.IsZero() {
		// Apply bye reordering even without dates (week numbers still matter).
		return applyByeRequests(entries, opts.ByeByWeek)
	}
	// Honour approved bye requests by reordering week assignments before dates are fixed.
	entries = applyByeRequests(entries, opts.ByeByWeek)

	skipSet := make(map[string]bool, len(opts.SkipDates))
	for _, d := range opts.SkipDates {
		skipSet[d.Format("2006-01-02")] = true
	}

	totalWeeks := maxWeekNum(entries)
	weekDates := make(map[int]string, totalWeeks)
	week := 1
	cur := opts.StartDate
	for week <= totalWeeks {
		ds := cur.Format("2006-01-02")
		if !skipSet[ds] {
			weekDates[week] = ds
			week++
		}
		cur = cur.AddDate(0, 0, 7)
	}

	result := make([]ScheduleEntry, len(entries))
	for i, e := range entries {
		e.MatchDate = weekDates[e.WeekNumber]
		result[i] = e
	}
	return result
}

// RoundRobin is kept for backward compatibility. Prefer DoubleRoundRobin.
func RoundRobin(teamIDs []int64, startDate time.Time) ([]ScheduleEntry, error) {
	return DoubleRoundRobin(teamIDs, ScheduleOptions{StartDate: startDate})
}
