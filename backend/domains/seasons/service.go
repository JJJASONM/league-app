package seasons

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// SeasonService orchestrates season lifecycle: activation, checklist evaluation,
// previous-season lookup, draft checks, and schedule-staleness marking.
type SeasonService struct {
	store SeasonStore
}

// NewSeasonService returns a SeasonService backed by the given store.
func NewSeasonService(store SeasonStore) *SeasonService {
	return &SeasonService{store: store}
}

// IsDraft returns true when the season has never been activated.
func (s *SeasonService) IsDraft(ctx context.Context, seasonID int64) (bool, error) {
	return s.store.IsDraft(ctx, seasonID)
}

// MarkStaleIfScheduled sets schedule_stale=1 when unplayed matches exist.
func (s *SeasonService) MarkStaleIfScheduled(ctx context.Context, seasonID int64) error {
	return s.store.MarkStaleIfScheduled(ctx, seasonID)
}

// Checklist returns the activation readiness checklist for the season.
// Legacy seasons (teams_managed=0) always return CanActivate=true.
// Returns ErrNotFound (wrapped) when the season does not exist.
func (s *SeasonService) Checklist(ctx context.Context, seasonID int64) (models.SetupChecklist, error) {
	meta, err := s.getMeta(ctx, seasonID)
	if err != nil {
		return models.SetupChecklist{}, fmt.Errorf("checklist season %d: %w", seasonID, err)
	}
	return s.computeChecklist(ctx, seasonID, meta)
}

// Activate runs the checklist and, when it passes, atomically activates the season.
// Returns *ChecklistBlockErr (HTTP 422) when blockers exist.
// Returns ErrNotFound (wrapped) when the season does not exist.
func (s *SeasonService) Activate(ctx context.Context, seasonID int64) error {
	meta, err := s.getMeta(ctx, seasonID)
	if err != nil {
		return fmt.Errorf("activate season %d: %w", seasonID, err)
	}
	checklist, err := s.computeChecklist(ctx, seasonID, meta)
	if err != nil {
		return fmt.Errorf("activate season %d: %w", seasonID, err)
	}
	if !checklist.CanActivate {
		return &ChecklistBlockErr{Blockers: checklist.Blockers}
	}
	// TODO: snapshot rules at activation — lock season_rules against further changes (deferred)
	return s.store.Activate(ctx, seasonID, meta.LeagueID)
}

// PreviousSeason returns the immediately previous season for seasonID within the
// same league, along with its registered teams. Season is nil when no previous
// season exists. Teams is always non-nil.
func (s *SeasonService) PreviousSeason(ctx context.Context, seasonID int64) (PreviousSeasonResult, error) {
	empty := PreviousSeasonResult{Teams: []SeasonTeamEntry{}}

	meta, err := s.getMeta(ctx, seasonID)
	if err != nil {
		return empty, fmt.Errorf("previous season %d: %w", seasonID, err)
	}

	// Priority 1: active season in the same league with no end_date.
	prev, err := s.store.FindActiveWithNoEndDate(ctx, meta.LeagueID, seasonID)
	if err != nil {
		return empty, fmt.Errorf("previous season %d: %w", seasonID, err)
	}

	// Priority 2: closest prior end_date relative to this season's start_date.
	if prev == nil {
		prev, err = s.store.FindClosestPriorByEndDate(ctx, meta.LeagueID, seasonID, meta.StartDate)
		if err != nil {
			return empty, fmt.Errorf("previous season %d: %w", seasonID, err)
		}
	}

	if prev == nil {
		return empty, nil
	}

	// Prefer season_teams; fall back to match participants.
	teams, err := s.store.GetSeasonTeams(ctx, prev.ID)
	if err != nil {
		return empty, fmt.Errorf("previous season %d: season teams: %w", seasonID, err)
	}
	if len(teams) == 0 {
		teams, err = s.store.GetMatchTeams(ctx, prev.ID)
		if err != nil {
			return empty, fmt.Errorf("previous season %d: match teams: %w", seasonID, err)
		}
	}
	return PreviousSeasonResult{Season: prev, Teams: teams}, nil
}

// AddTeam adds a team to a draft season. When FromTeamID is set, the team is
// copied from a prior season (roster included). When Name is set, a brand-new
// team is created. Returns domainerr.Unprocessable when the season is active,
// domainerr.InvalidInput for validation failures.
func (s *SeasonService) AddTeam(ctx context.Context, seasonID int64, req AddTeamRequest) (models.SeasonTeam, error) {
	draft, err := s.store.IsDraft(ctx, seasonID)
	if err != nil {
		return models.SeasonTeam{}, err
	}
	if !draft {
		return models.SeasonTeam{}, domainerr.New("SEASON_NOT_DRAFT", domainerr.Unprocessable,
			"cannot modify teams in an active season")
	}

	meta, err := s.getMeta(ctx, seasonID)
	if err != nil {
		return models.SeasonTeam{}, err
	}

	if meta.TeamsManaged && req.FromTeamID > 0 && req.FromSeasonID == 0 {
		return models.SeasonTeam{}, domainerr.New("SEASON_MANAGED_FROM_REQUIRED", domainerr.InvalidInput,
			"managed seasons require from_season_id with from_team_id; use name to create a new team")
	}

	if req.FromTeamID > 0 && req.FromSeasonID > 0 {
		prev, err := s.PreviousSeason(ctx, seasonID)
		if err != nil {
			return models.SeasonTeam{}, err
		}
		if prev.Season == nil || prev.Season.ID != req.FromSeasonID {
			return models.SeasonTeam{}, domainerr.New("SEASON_WRONG_PRIOR", domainerr.InvalidInput,
				"from_season_id must be the immediately previous season")
		}
	}

	var teamID int64
	if req.FromTeamID > 0 {
		teamLeagueID, err := s.store.GetTeamLeagueID(ctx, req.FromTeamID)
		if err != nil || teamLeagueID != meta.LeagueID {
			return models.SeasonTeam{}, domainerr.New("SEASON_TEAM_NOT_IN_LEAGUE", domainerr.InvalidInput,
				"team not found in this league")
		}
		teamID = req.FromTeamID
		if err := s.store.AddSeasonTeamCopy(ctx, seasonID, teamID, req.FromSeasonID, meta.TeamsManaged); err != nil {
			if errors.Is(err, ErrTeamAlreadyInSeason) {
				return models.SeasonTeam{}, domainerr.New("SEASON_TEAM_DUPLICATE", domainerr.InvalidInput, err.Error())
			}
			if errors.Is(err, ErrTeamNotInPriorSeason) {
				return models.SeasonTeam{}, domainerr.New("SEASON_TEAM_NOT_IN_PRIOR", domainerr.InvalidInput, err.Error())
			}
			return models.SeasonTeam{}, err
		}
	} else if strings.TrimSpace(req.Name) != "" {
		newID, err := s.store.AddSeasonTeamNew(ctx, seasonID, meta.LeagueID, req.Name)
		if err != nil {
			return models.SeasonTeam{}, err
		}
		teamID = newID
	} else {
		return models.SeasonTeam{}, domainerr.New("SEASON_TEAM_BAD_REQUEST", domainerr.InvalidInput,
			"provide from_team_id (copy) or name (new team)")
	}

	_ = s.store.MarkStaleIfScheduled(ctx, seasonID)
	return s.store.GetSeasonTeam(ctx, seasonID, teamID)
}

// RemoveTeam removes a team from a draft season, cleaning up roster and team
// record when the team has no match history and no other season registrations.
// Returns domainerr.Unprocessable when the season is active.
// Returns domainerr.NotFound when the team is not in the season.
func (s *SeasonService) RemoveTeam(ctx context.Context, seasonID, teamID int64) error {
	draft, err := s.store.IsDraft(ctx, seasonID)
	if err != nil {
		return err
	}
	if !draft {
		return domainerr.New("SEASON_NOT_DRAFT", domainerr.Unprocessable,
			"cannot modify teams in an active season")
	}
	if err := s.store.RemoveSeasonTeam(ctx, seasonID, teamID); err != nil {
		if errors.Is(err, ErrTeamNotInSeason) {
			return domainerr.New("SEASON_TEAM_NOT_FOUND", domainerr.NotFound, err.Error())
		}
		return err
	}
	_ = s.store.MarkStaleIfScheduled(ctx, seasonID)
	return nil
}

// UpdateTeam updates the season_name and captain_id for a team in a draft season.
// Returns domainerr.Unprocessable when the season is active.
// Returns domainerr.InvalidInput for validation failures (missing name, captain not on roster).
// Returns domainerr.NotFound when the team is not registered.
func (s *SeasonService) UpdateTeam(ctx context.Context, seasonID, teamID int64, req UpdateTeamRequest) (models.SeasonTeam, error) {
	draft, err := s.store.IsDraft(ctx, seasonID)
	if err != nil {
		return models.SeasonTeam{}, err
	}
	if !draft {
		return models.SeasonTeam{}, domainerr.New("SEASON_NOT_DRAFT", domainerr.Unprocessable,
			"cannot modify teams in an active season")
	}

	req.SeasonName = strings.TrimSpace(req.SeasonName)
	if req.SeasonName == "" {
		return models.SeasonTeam{}, domainerr.New("SEASON_NAME_REQUIRED", domainerr.InvalidInput,
			"season_name is required")
	}

	if req.CaptainID != nil {
		onRoster, err := s.store.CheckPlayerOnSeasonRoster(ctx, seasonID, teamID, *req.CaptainID)
		if err != nil {
			return models.SeasonTeam{}, err
		}
		if !onRoster {
			return models.SeasonTeam{}, domainerr.New(ChecklistCaptainNotOnRoster, domainerr.InvalidInput,
				"captain must be on this team's season roster")
		}
	}

	if err := s.store.UpdateSeasonTeamMeta(ctx, seasonID, teamID, req.SeasonName, req.CaptainID); err != nil {
		if errors.Is(err, ErrTeamNotInSeason) {
			return models.SeasonTeam{}, domainerr.New("SEASON_TEAM_NOT_FOUND", domainerr.NotFound, err.Error())
		}
		return models.SeasonTeam{}, err
	}
	return s.store.GetSeasonTeam(ctx, seasonID, teamID)
}

// ListSeasonTeams returns all teams registered in a season with full metadata.
func (s *SeasonService) ListSeasonTeams(ctx context.Context, seasonID int64) ([]models.SeasonTeam, error) {
	teams, err := s.store.ListSeasonTeams(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	if teams == nil {
		teams = []models.SeasonTeam{}
	}
	return teams, nil
}

// CreateByeRequest validates and inserts a bye request for the season.
// Returns ErrNotFound (wrapped) when the season does not exist.
// Returns domainerr.InvalidInput for validation failures.
func (s *SeasonService) CreateByeRequest(ctx context.Context, seasonID int64, req CreateByeRequestInput) (models.ByeRequest, error) {
	meta, err := s.getMeta(ctx, seasonID)
	if err != nil {
		return models.ByeRequest{}, err
	}

	count, err := s.store.CountParticipatingTeams(ctx, seasonID, meta.LeagueID, meta.TeamsManaged)
	if err != nil {
		return models.ByeRequest{}, err
	}
	if count%2 == 0 {
		return models.ByeRequest{}, domainerr.New("BYE_EVEN_TEAMS", domainerr.InvalidInput,
			fmt.Sprintf("bye requests require an odd number of teams (%d teams — even)", count))
	}

	teamLeagueID, err := s.store.GetTeamLeagueID(ctx, req.TeamID)
	if err != nil || teamLeagueID != meta.LeagueID {
		return models.ByeRequest{}, domainerr.New("BYE_TEAM_NOT_IN_LEAGUE", domainerr.InvalidInput,
			"team does not belong to this season's league")
	}

	if meta.TeamsManaged {
		inSeason, err := s.store.CheckTeamInSeason(ctx, seasonID, req.TeamID)
		if err != nil {
			return models.ByeRequest{}, err
		}
		if !inSeason {
			return models.ByeRequest{}, domainerr.New("BYE_TEAM_NOT_IN_SEASON", domainerr.InvalidInput,
				"team is not registered in this season")
		}
	}

	dup, err := s.store.HasDuplicateBye(ctx, seasonID, req.TeamID, req.WeekNumber)
	if err != nil {
		return models.ByeRequest{}, err
	}
	if dup {
		return models.ByeRequest{}, domainerr.New("BYE_DUPLICATE", domainerr.InvalidInput,
			"a bye request already exists for this team and week")
	}

	return s.store.InsertByeRequest(ctx, seasonID, req.TeamID, req.WeekNumber, req.Reason)
}

// UpdateByeRequest sets the approved flag on a bye request.
// Returns domainerr.NotFound when the bye does not exist in the season.
// Returns domainerr.InvalidInput when approving a week-0 (TBD) request or
// when another team already has an approved bye for the same week.
func (s *SeasonService) UpdateByeRequest(ctx context.Context, seasonID, byeID int64, approve bool) (models.ByeRequest, error) {
	bye, err := s.store.GetByeRequest(ctx, seasonID, byeID)
	if err != nil {
		if errors.Is(err, ErrByeNotFound) {
			return models.ByeRequest{}, domainerr.New("BYE_NOT_FOUND", domainerr.NotFound, "bye request not found")
		}
		return models.ByeRequest{}, err
	}

	if approve && bye.WeekNumber == 0 {
		return models.ByeRequest{}, domainerr.New("BYE_WEEK_ZERO", domainerr.InvalidInput,
			"cannot approve a TBD (week 0) request; set a specific week first")
	}

	if approve {
		conflict, err := s.store.HasByeConflict(ctx, seasonID, bye.WeekNumber, byeID)
		if err != nil {
			return models.ByeRequest{}, err
		}
		if conflict {
			return models.ByeRequest{}, domainerr.New("BYE_CONFLICT", domainerr.InvalidInput,
				fmt.Sprintf("another team already has an approved bye for week %d; unapprove it first", bye.WeekNumber))
		}
	}

	updated, err := s.store.SetByeApproval(ctx, seasonID, byeID, approve)
	if err != nil {
		if errors.Is(err, ErrByeNotFound) {
			return models.ByeRequest{}, domainerr.New("BYE_NOT_FOUND", domainerr.NotFound, "bye request not found")
		}
		return models.ByeRequest{}, err
	}
	return updated, nil
}

// getMeta fetches the season meta and converts sql.ErrNoRows to ErrNotFound.
func (s *SeasonService) getMeta(ctx context.Context, seasonID int64) (SeasonMeta, error) {
	meta, err := s.store.GetMeta(ctx, seasonID)
	if errors.Is(err, ErrNotFound) {
		return SeasonMeta{}, ErrNotFound
	}
	return meta, err
}

// computeChecklist evaluates checklist items given pre-fetched meta.
func (s *SeasonService) computeChecklist(ctx context.Context, seasonID int64, meta SeasonMeta) (models.SetupChecklist, error) {
	c := models.SetupChecklist{
		Blockers: []models.ChecklistItem{},
		Warnings: []models.ChecklistItem{},
	}

	// Legacy seasons bypass all enforcement.
	if !meta.TeamsManaged {
		c.CanActivate = true
		return c, nil
	}

	teams, err := s.store.GetTeamSummaries(ctx, seasonID)
	if err != nil {
		return c, err
	}

	if len(teams) < 2 {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    ChecklistTeamsTooFew,
			Message: fmt.Sprintf("season has %d participating team; at least 2 required", len(teams)),
		})
	}

	for _, t := range teams {
		if t.RosterCount == 0 {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    ChecklistTeamNoPlayers,
				Message: fmt.Sprintf("team %q has no rostered players", t.Name),
				TeamID:  t.TeamID,
			})
		} else if t.RosterCount < 3 {
			c.Warnings = append(c.Warnings, models.ChecklistItem{
				Code:    ChecklistTeamFewPlayers,
				Message: fmt.Sprintf("team %q has %d player(s); 3 or more recommended for match play", t.Name, t.RosterCount),
				TeamID:  t.TeamID,
			})
		}

		if t.CaptainID == nil {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    ChecklistTeamNoCaptain,
				Message: fmt.Sprintf("team %q has no captain assigned", t.Name),
				TeamID:  t.TeamID,
			})
		} else if !t.CaptainOnRoster {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    ChecklistCaptainNotOnRoster,
				Message: fmt.Sprintf("team %q captain is not on the season roster", t.Name),
				TeamID:  t.TeamID,
			})
		}
	}

	if meta.ScheduleStale {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    ChecklistScheduleStale,
			Message: "schedule is stale after team changes; regenerate before activating",
		})
	}

	matchCount, err := s.store.GetMatchCount(ctx, seasonID)
	if err != nil {
		return c, err
	}
	if matchCount == 0 {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    ChecklistNoSchedule,
			Message: "no schedule has been generated for this season",
		})
	} else if meta.EndDate == nil || *meta.EndDate == "" {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    ChecklistNoEndDate,
			Message: "season has no calculable end date; regenerate the schedule",
		})
	}

	c.CanActivate = len(c.Blockers) == 0
	return c, nil
}
