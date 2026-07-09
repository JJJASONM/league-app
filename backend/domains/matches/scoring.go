package matches

import (
	"league_app/models"
	"sort"
)

// TeamScore summarizes one team's performance in a single match.
type TeamScore struct {
	TeamID    int64
	GamesWon  int
	GamesLost int
}

// MatchOutcome determines win/loss/tie for both teams based on their match results.
// Returns (homePoints, awayPoints) where win=2, tie=1, loss=0.
func MatchOutcome(results []models.MatchResult, homeTeamID, awayTeamID int64) (homePoints, awayPoints int) {
	var homeWins, awayWins int
	for _, r := range results {
		if r.TeamID == homeTeamID {
			homeWins += r.GamesWon
		} else if r.TeamID == awayTeamID {
			awayWins += r.GamesWon
		}
	}

	switch {
	case homeWins > awayWins:
		return 2, 0
	case awayWins > homeWins:
		return 0, 2
	default:
		return 1, 1
	}
}

// ComputeStandings aggregates match results into a standings table.
// matches must be completed matches; results maps matchID -> []MatchResult.
func ComputeStandings(
	matches []models.Match,
	results map[int64][]models.MatchResult,
	teams []models.Team,
) []models.Standing {

	nameByID := make(map[int64]string, len(teams))
	for _, t := range teams {
		nameByID[t.ID] = t.Name
	}

	standing := make(map[int64]*models.Standing)
	for _, t := range teams {
		standing[t.ID] = &models.Standing{
			TeamID:   t.ID,
			TeamName: t.Name,
		}
	}

	for _, m := range matches {
		if !m.Completed {
			continue
		}
		res := results[m.ID]
		homePoints, awayPoints := MatchOutcome(res, m.HomeTeamID, m.AwayTeamID)

		hs := standing[m.HomeTeamID]
		as := standing[m.AwayTeamID]
		if hs == nil || as == nil {
			continue
		}

		hs.Played++
		as.Played++
		hs.Points += homePoints
		as.Points += awayPoints

		switch homePoints {
		case 2:
			hs.Wins++
			as.Losses++
		case 0:
			hs.Losses++
			as.Wins++
		default:
			hs.Ties++
			as.Ties++
		}

		for _, r := range res {
			if r.TeamID == m.HomeTeamID {
				hs.GamesWon += r.GamesWon
				hs.GamesLost += r.GamesLost
			} else {
				as.GamesWon += r.GamesWon
				as.GamesLost += r.GamesLost
			}
		}
	}

	out := make([]models.Standing, 0, len(standing))
	for _, s := range standing {
		if s.Played > 0 {
			s.WinPct = float64(s.Wins) / float64(s.Played)
		}
		out = append(out, *s)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Points != out[j].Points {
			return out[i].Points > out[j].Points
		}
		if out[i].Wins != out[j].Wins {
			return out[i].Wins > out[j].Wins
		}
		return out[i].GamesWon > out[j].GamesWon
	})

	return out
}
