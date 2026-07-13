package matches

import (
	"context"
	"errors"
	"fmt"
	"time"

	"league_app/backend/domainerr"
)

// GenerateRequest is the input for ScheduleService.GenerateSchedule.
type GenerateRequest struct {
	SeasonID       int64    `json:"season_id"`
	StartDate      string   `json:"start_date"`      // YYYY-MM-DD
	ScheduleType   string   `json:"schedule_type"`   // "single_rr"|"double_rr"|"split"|"custom"|"blanket"
	NumWeeks       int      `json:"num_weeks"`       // for "custom" and "blanket"
	MatchesPerWeek int      `json:"matches_per_week"` // for "blanket" only
	SkipDates      []string `json:"skip_dates"`
	FromSeasonID   int64    `json:"from_season_id"` // legacy: use teams from this season's match history
}

// GenerateResult is the response for a successful schedule generation.
type GenerateResult struct {
	MatchesCreated int    `json:"matches_created"`
	EndDate        string `json:"end_date"`
}

// ScheduleService orchestrates schedule generation: resolving teams, calling
// the scheduling generators, and persisting the result.
// It implements the ScheduleManager interface declared in handlers/deps.go.
type ScheduleService struct {
	store ScheduleStore
}

// NewScheduleService returns a ScheduleService backed by the given store.
func NewScheduleService(store ScheduleStore) *ScheduleService {
	return &ScheduleService{store: store}
}

// GenerateSchedule generates a league schedule for the given season and persists it.
// It replaces any existing unplayed matches and updates the season's schedule metadata.
func (s *ScheduleService) GenerateSchedule(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	if req.ScheduleType == "" {
		req.ScheduleType = ScheduleTypeDoubleRR
	}

	meta, err := s.store.GetScheduleSeasonMeta(ctx, req.SeasonID)
	if err != nil {
		if errors.Is(err, ErrSeasonNotFound) {
			return GenerateResult{}, domainerr.New("SCHEDULE_SEASON_NOT_FOUND", domainerr.NotFound, "season not found")
		}
		return GenerateResult{}, fmt.Errorf("generate schedule: get season: %w", err)
	}

	// Managed seasons always generate from season_teams; from_season_id is legacy-only.
	if meta.TeamsManaged && req.FromSeasonID > 0 {
		return GenerateResult{}, domainerr.New("SCHEDULE_MANAGED_FROM_SEASON",
			domainerr.InvalidInput,
			"managed seasons generate from season_teams; from_season_id is not supported")
	}

	// Closed weeks exist: regeneration would desynchronize week numbers with
	// committed official results. Require pushback or manual correction instead.
	if closed, err := s.store.HasClosedWeeks(ctx, req.SeasonID); err != nil {
		return GenerateResult{}, fmt.Errorf("generate schedule: check closed weeks: %w", err)
	} else if closed {
		return GenerateResult{}, domainerr.New("SCHEDULE_HAS_CLOSED_WEEKS", domainerr.Conflict,
			"cannot regenerate schedule: one or more weeks in this season are already closed")
	}

	// Active seasons with completed matches: regeneration would overwrite official
	// play records whose match dates and pairings have already been communicated.
	if meta.Active {
		if completed, err := s.store.HasCompletedMatches(ctx, req.SeasonID); err != nil {
			return GenerateResult{}, fmt.Errorf("generate schedule: check completed: %w", err)
		} else if completed {
			return GenerateResult{}, domainerr.New("SCHEDULE_ACTIVE_HAS_COMPLETED", domainerr.Conflict,
				"cannot regenerate schedule: the season is active and has completed matches")
		}
	}

	// Parse dates.
	startDate, _ := time.Parse("2006-01-02", req.StartDate)
	skipDates := make([]time.Time, 0, len(req.SkipDates))
	for _, ds := range req.SkipDates {
		ds = normDate(ds)
		if t, parseErr := time.Parse("2006-01-02", ds); parseErr == nil {
			skipDates = append(skipDates, t)
		}
	}

	// Load approved bye requests with specific week numbers.
	byeByWeek, err := s.store.LoadByeRequests(ctx, req.SeasonID)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("generate schedule: load byes: %w", err)
	}

	opts := ScheduleOptions{
		StartDate: startDate,
		SkipDates: skipDates,
		NumWeeks:  req.NumWeeks,
		ByeByWeek: byeByWeek,
	}

	var entries []ScheduleEntry
	var genErr error

	if req.ScheduleType == ScheduleTypeBlanket {
		mpw := req.MatchesPerWeek
		if mpw < 1 {
			mpw = 1
		}
		entries, genErr = BlanketTemplate(req.NumWeeks, mpw, opts)
	} else {
		teamIDs, collectErr := s.collectTeamIDs(ctx, req, meta)
		if collectErr != nil {
			return GenerateResult{}, collectErr
		}

		switch req.ScheduleType {
		case ScheduleTypeSingleRR:
			entries, genErr = SingleRoundRobin(teamIDs, opts)
		case ScheduleTypeSplit:
			entries, genErr = SplitSeason(teamIDs, opts)
		case ScheduleTypeCustom:
			if req.NumWeeks < 1 {
				return GenerateResult{}, domainerr.New("SCHEDULE_NUM_WEEKS_REQUIRED",
					domainerr.InvalidInput, "num_weeks is required for custom schedule")
			}
			entries, genErr = CustomSchedule(teamIDs, opts)
		default: // ScheduleTypeDoubleRR
			entries, genErr = DoubleRoundRobin(teamIDs, opts)
		}
	}

	if genErr != nil {
		return GenerateResult{}, domainerr.New("SCHEDULE_GEN_ERROR", domainerr.InvalidInput, genErr.Error())
	}

	// Compute end date and convert entries to domain type.
	var endDate string
	matchEntries := make([]MatchEntry, len(entries))
	for i, e := range entries {
		matchEntries[i] = MatchEntry{
			HomeTeamID: e.HomeTeamID,
			AwayTeamID: e.AwayTeamID,
			WeekNumber: e.WeekNumber,
			MatchDate:  e.MatchDate,
		}
		if e.MatchDate > endDate {
			endDate = e.MatchDate
		}
	}

	saveReq := SaveScheduleRequest{
		SeasonID:     req.SeasonID,
		ScheduleType: req.ScheduleType,
		NumWeeks:     req.NumWeeks,
		EndDate:      endDate,
		Entries:      matchEntries,
	}
	if err := s.store.SaveGeneratedSchedule(ctx, saveReq); err != nil {
		return GenerateResult{}, fmt.Errorf("generate schedule: save: %w", err)
	}

	return GenerateResult{MatchesCreated: len(entries), EndDate: endDate}, nil
}

// collectTeamIDs resolves the set of team IDs to schedule.
func (s *ScheduleService) collectTeamIDs(ctx context.Context, req GenerateRequest, meta ScheduleSeasonMeta) ([]int64, error) {
	var teamIDs []int64
	var err error

	if req.FromSeasonID > 0 {
		teamIDs, err = s.store.LoadTeamIDsFromHistory(ctx, req.FromSeasonID)
		if err != nil {
			return nil, fmt.Errorf("generate schedule: load teams from history: %w", err)
		}
	} else {
		teamIDs, err = s.store.LoadTeamIDsForSchedule(ctx, req.SeasonID, meta.LeagueID, meta.TeamsManaged)
		if err != nil {
			return nil, fmt.Errorf("generate schedule: load season teams: %w", err)
		}
		if meta.TeamsManaged && len(teamIDs) == 0 {
			return nil, domainerr.New("SCHEDULE_NO_TEAMS", domainerr.InvalidInput,
				"no teams registered in this season; add teams before generating a schedule")
		}
	}

	if len(teamIDs) < 2 {
		return nil, domainerr.New("SCHEDULE_TOO_FEW_TEAMS", domainerr.InvalidInput,
			"need at least 2 teams in this league to generate a schedule")
	}
	return teamIDs, nil
}

// normDate trims a date string to YYYY-MM-DD, discarding any time component.
// The SQLite driver may return DATE columns as full ISO timestamps.
func normDate(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:10]
}
