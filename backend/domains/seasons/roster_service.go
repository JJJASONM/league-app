package seasons

import (
	"context"
	"errors"

	"league_app/backend/domainerr"
	"league_app/models"
)

// ListRoster returns all players on a team's season roster.
func (s *SeasonService) ListRoster(ctx context.Context, seasonID, teamID int64) ([]models.SeasonRosterEntry, error) {
	entries, err := s.store.ListRoster(ctx, seasonID, teamID)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []models.SeasonRosterEntry{}
	}
	return entries, nil
}

// AddRosterPlayer adds a player to a team's season roster.
// Returns domainerr.Unprocessable when the season is active.
// Returns domainerr.InvalidInput when the team is not in the season or the
// player is already rostered on a different team.
func (s *SeasonService) AddRosterPlayer(ctx context.Context, seasonID, teamID, playerID int64) (models.SeasonRosterEntry, error) {
	draft, err := s.store.IsDraft(ctx, seasonID)
	if err != nil {
		return models.SeasonRosterEntry{}, err
	}
	if !draft {
		return models.SeasonRosterEntry{}, domainerr.New("SEASON_NOT_DRAFT", domainerr.Unprocessable,
			"cannot modify rosters in an active season")
	}

	inSeason, err := s.store.CheckTeamInSeason(ctx, seasonID, teamID)
	if err != nil {
		return models.SeasonRosterEntry{}, err
	}
	if !inSeason {
		return models.SeasonRosterEntry{}, domainerr.New("ROSTER_TEAM_NOT_IN_SEASON", domainerr.InvalidInput,
			"team is not in this season")
	}

	existingTeam, found, err := s.store.GetPlayerRosterTeam(ctx, seasonID, playerID)
	if err != nil {
		return models.SeasonRosterEntry{}, err
	}
	if found && existingTeam != teamID {
		return models.SeasonRosterEntry{}, domainerr.New("ROSTER_PLAYER_ON_OTHER_TEAM", domainerr.InvalidInput,
			"player is already on another team in this season")
	}

	return s.store.InsertOrGetRosterPlayer(ctx, seasonID, teamID, playerID)
}

// RemoveRosterPlayer removes a player from a team's season roster.
// Returns domainerr.Unprocessable when the season is active.
// Returns domainerr.NotFound when the roster entry does not exist.
func (s *SeasonService) RemoveRosterPlayer(ctx context.Context, seasonID, teamID, playerID int64) error {
	draft, err := s.store.IsDraft(ctx, seasonID)
	if err != nil {
		return err
	}
	if !draft {
		return domainerr.New("SEASON_NOT_DRAFT", domainerr.Unprocessable,
			"cannot modify rosters in an active season")
	}

	if err := s.store.DeleteRosterPlayer(ctx, seasonID, teamID, playerID); err != nil {
		if errors.Is(err, ErrRosterEntryNotFound) {
			return domainerr.New("ROSTER_ENTRY_NOT_FOUND", domainerr.NotFound, "roster entry not found")
		}
		return err
	}
	return nil
}

// ListAvailablePlayers returns all active players not already rostered in this season.
// Returns ErrNotFound when the season does not exist.
func (s *SeasonService) ListAvailablePlayers(ctx context.Context, seasonID int64) ([]models.Player, error) {
	players, err := s.store.ListAvailablePlayers(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	if players == nil {
		players = []models.Player{}
	}
	return players, nil
}
