package seasons

import (
	"context"
	"errors"
	"fmt"

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
			Code:    "TEAMS_TOO_FEW",
			Message: fmt.Sprintf("season has %d participating team; at least 2 required", len(teams)),
		})
	}

	for _, t := range teams {
		if t.RosterCount == 0 {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    "TEAM_NO_PLAYERS",
				Message: fmt.Sprintf("team %q has no rostered players", t.Name),
				TeamID:  t.TeamID,
			})
		} else if t.RosterCount < 3 {
			c.Warnings = append(c.Warnings, models.ChecklistItem{
				Code:    "TEAM_FEW_PLAYERS",
				Message: fmt.Sprintf("team %q has %d player(s); 3 or more recommended for match play", t.Name, t.RosterCount),
				TeamID:  t.TeamID,
			})
		}

		if t.CaptainID == nil {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    "TEAM_NO_CAPTAIN",
				Message: fmt.Sprintf("team %q has no captain assigned", t.Name),
				TeamID:  t.TeamID,
			})
		} else if !t.CaptainOnRoster {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    "CAPTAIN_NOT_ON_ROSTER",
				Message: fmt.Sprintf("team %q captain is not on the season roster", t.Name),
				TeamID:  t.TeamID,
			})
		}
	}

	if meta.ScheduleStale {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "SCHEDULE_STALE",
			Message: "schedule is stale after team changes; regenerate before activating",
		})
	}

	matchCount, err := s.store.GetMatchCount(ctx, seasonID)
	if err != nil {
		return c, err
	}
	if matchCount == 0 {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "NO_SCHEDULE",
			Message: "no schedule has been generated for this season",
		})
	} else if meta.EndDate == nil || *meta.EndDate == "" {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "NO_END_DATE",
			Message: "season has no calculable end date; regenerate the schedule",
		})
	}

	c.CanActivate = len(c.Blockers) == 0
	return c, nil
}
