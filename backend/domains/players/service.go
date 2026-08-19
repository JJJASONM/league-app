package players

import (
	"context"
	"errors"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// PlayerService implements business logic for player CRUD operations.
type PlayerService struct {
	store PlayerStore
}

// NewPlayerService returns a PlayerService backed by the given store.
func NewPlayerService(store PlayerStore) *PlayerService {
	return &PlayerService{store: store}
}

func (s *PlayerService) ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error) {
	return s.store.ListPlayers(ctx, leagueID)
}

func (s *PlayerService) GetPlayer(ctx context.Context, id int64) (models.Player, error) {
	return s.store.GetPlayer(ctx, id)
}

// CreatePlayer validates that at least one of first name or last name is non-empty,
// then delegates to the store.
func (s *PlayerService) CreatePlayer(ctx context.Context, input CreatePlayerInput) (models.Player, error) {
	if strings.TrimSpace(input.FirstName) == "" && strings.TrimSpace(input.LastName) == "" {
		return models.Player{}, domainerr.New("PLAYER_NAME_REQUIRED", domainerr.InvalidInput, "first or last name is required")
	}
	return s.store.CreatePlayer(ctx, input)
}

// UpdatePlayer delegates to the store. PlayerNumber is locked at creation and not
// present in UpdatePlayerInput.
func (s *PlayerService) UpdatePlayer(ctx context.Context, id int64, input UpdatePlayerInput) error {
	return s.store.UpdatePlayer(ctx, id, input)
}

// DeletePlayer blocks deletion when handicap history exists for the player,
// returning a Conflict-category domain error in that case.
func (s *PlayerService) DeletePlayer(ctx context.Context, id int64) error {
	err := s.store.DeletePlayer(ctx, id)
	if errors.Is(err, ErrHasHistory) {
		return domainerr.New("PLAYER_HAS_HISTORY", domainerr.Conflict,
			"This player has handicap history records and cannot be deleted. Deactivate the player instead.")
	}
	return err
}

// MergePlayers merges sourceID into targetID: every supported reference to
// sourceID is repointed to targetID, then the source player is deleted. Both
// players must exist and be different players. See PlayerStore.MergePlayers
// for the full list of repointed references and the transaction guarantee.
//
// Error return contract:
//   - domainerr.InvalidInput -- sourceID == targetID
//   - domainerr.NotFound     -- source or target player does not exist
//   - domainerr.Conflict     -- an unsafe collision blocks the merge (see the
//     PLAYER_MERGE_*_CONFLICT codes below)
//   - domainerr.Internal     -- unexpected store error
func (s *PlayerService) MergePlayers(ctx context.Context, sourceID, targetID int64) error {
	if sourceID == targetID {
		return domainerr.New("PLAYER_MERGE_SAME_PLAYER", domainerr.InvalidInput,
			"source and target player must be different")
	}
	if _, err := s.store.GetPlayer(ctx, sourceID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return domainerr.New("PLAYER_MERGE_SOURCE_NOT_FOUND", domainerr.NotFound, "source player not found")
		}
		return domainerr.Wrap("PLAYER_MERGE_INTERNAL", domainerr.Internal, "internal error", err)
	}
	if _, err := s.store.GetPlayer(ctx, targetID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return domainerr.New("PLAYER_MERGE_TARGET_NOT_FOUND", domainerr.NotFound, "target player not found")
		}
		return domainerr.Wrap("PLAYER_MERGE_INTERNAL", domainerr.Internal, "internal error", err)
	}

	err := s.store.MergePlayers(ctx, sourceID, targetID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrMergeSeasonRosterConflict):
		return domainerr.New("PLAYER_MERGE_SEASON_ROSTER_CONFLICT", domainerr.Conflict,
			"source and target are both rostered in the same season; resolve the roster before merging")
	case errors.Is(err, ErrMergeRoundResultConflict):
		return domainerr.New("PLAYER_MERGE_ROUND_RESULT_CONFLICT", domainerr.Conflict,
			"source and target already both participate in the same scoresheet round; resolve before merging")
	case errors.Is(err, ErrMergeSelfOpponentConflict):
		return domainerr.New("PLAYER_MERGE_SELF_OPPONENT_CONFLICT", domainerr.Conflict,
			"source and target already played against each other in a recorded round; merging would make a player play themselves")
	case errors.Is(err, ErrMergeLineupPlanConflict):
		return domainerr.New("PLAYER_MERGE_LINEUP_PLAN_CONFLICT", domainerr.Conflict,
			"source and target are both already in the same lineup plan slot; resolve before merging")
	default:
		return domainerr.Wrap("PLAYER_MERGE_INTERNAL", domainerr.Internal, "internal error", err)
	}
}
