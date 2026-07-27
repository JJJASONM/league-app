package seasons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// seasonCloseState bundles validation output with the setInactive flag so
// computeClose can return everything CloseSeason needs in one call.
type seasonCloseState struct {
	preview     SeasonClosePreview
	setInactive bool
}

// computeClose runs close validation and returns the preview state plus whether
// the season must also be deactivated (active=0) when the close is committed.
// Returns ErrNotFound (wrapped) when the season does not exist.
func (s *SeasonService) computeClose(
	ctx context.Context,
	seasonID int64,
	weeks []models.WeekSummary,
	standings []models.Standing,
) (seasonCloseState, error) {
	state := seasonCloseState{
		preview: SeasonClosePreview{
			Blockers:      []CloseBlocker{},
			Warnings:      []CloseWarning{},
			Standings:     standings,
			UnclosedWeeks: []int{},
		},
	}
	if state.preview.Standings == nil {
		state.preview.Standings = []models.Standing{}
	}

	season, err := s.store.GetSeason(ctx, seasonID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return state, ErrNotFound
		}
		return state, fmt.Errorf("close season %d: get season: %w", seasonID, err)
	}

	// Draft seasons cannot be closed.
	if season.ActivatedAt == nil {
		state.preview.Blockers = append(state.preview.Blockers, CloseBlocker{
			Code:    CodeSeasonCloseDraft,
			Message: "draft seasons cannot be closed; activate the season first",
		})
		return state, nil
	}

	// Already-closed seasons are blocked.
	if season.ClosedAt != nil {
		state.preview.Blockers = append(state.preview.Blockers, CloseBlocker{
			Code:    CodeSeasonAlreadyClosed,
			Message: "this season is already closed",
		})
		return state, nil
	}

	state.setInactive = season.Active

	// No scheduled matches means the season has no play to close.
	matchCount, err := s.store.GetMatchCount(ctx, seasonID)
	if err != nil {
		return state, fmt.Errorf("close season %d: match count: %w", seasonID, err)
	}
	if matchCount == 0 {
		state.preview.Blockers = append(state.preview.Blockers, CloseBlocker{
			Code:    CodeSeasonCloseNoSchedule,
			Message: "no scheduled matches exist; generate a schedule before closing the season",
		})
		return state, nil
	}

	// All scheduled weeks must be officially closed.
	for _, w := range weeks {
		if w.Status != matches.WeekStatusClosed {
			state.preview.UnclosedWeeks = append(state.preview.UnclosedWeeks, w.WeekNumber)
		}
	}
	if len(state.preview.UnclosedWeeks) > 0 {
		state.preview.Blockers = append(state.preview.Blockers, CloseBlocker{
			Code: CodeSeasonCloseUnclosed,
			Message: fmt.Sprintf(
				"%d scheduled week(s) are not yet closed",
				len(state.preview.UnclosedWeeks),
			),
			UnclosedWeeks: state.preview.UnclosedWeeks,
		})
	}

	// Missing results are advisory; matches without results are already excluded
	// from standings through the week_closed filter applied during Close Week.
	missingCount, err := s.store.GetSeasonMissingCount(ctx, seasonID)
	if err != nil {
		return state, fmt.Errorf("close season %d: missing count: %w", seasonID, err)
	}
	state.preview.MissingCount = missingCount
	if missingCount > 0 {
		state.preview.Warnings = append(state.preview.Warnings, CloseWarning{
			Code: CodeMissingResults,
			Message: fmt.Sprintf(
				"%d match(es) have no entered results and are excluded from standings",
				missingCount,
			),
			Count: missingCount,
		})
	}

	state.preview.CanClose = len(state.preview.Blockers) == 0
	return state, nil
}

// ClosePreview returns the read-only close validation state for the season.
// Returns ErrNotFound (wrapped) when the season does not exist.
func (s *SeasonService) ClosePreview(
	ctx context.Context,
	seasonID int64,
	weeks []models.WeekSummary,
	standings []models.Standing,
) (SeasonClosePreview, error) {
	state, err := s.computeClose(ctx, seasonID, weeks, standings)
	return state.preview, err
}

// CloseSeason validates and commits the season close. Stores the final standings
// snapshot, sets closed_at, and deactivates the season when it is currently active.
// Returns ErrNotFound (wrapped) when the season does not exist.
// Returns domainerr.Err for blocker violations (see CodeSeason* constants).
func (s *SeasonService) CloseSeason(
	ctx context.Context,
	seasonID int64,
	weeks []models.WeekSummary,
	standings []models.Standing,
) (models.Season, error) {
	state, err := s.computeClose(ctx, seasonID, weeks, standings)
	if err != nil {
		return models.Season{}, err
	}
	if !state.preview.CanClose {
		if len(state.preview.Blockers) > 0 {
			b := state.preview.Blockers[0]
			cat := blockerCategory(b.Code)
			return models.Season{}, domainerr.New(b.Code, cat, b.Message)
		}
		return models.Season{}, domainerr.New("SEASON_CLOSE_BLOCKED", domainerr.Unprocessable,
			"season cannot be closed; resolve all blockers first")
	}

	type snapshotPayload struct {
		SchemaVersion int               `json:"schema_version"`
		CapturedAt    string            `json:"captured_at"`
		Standings     []models.Standing `json:"standings"`
	}
	payload := snapshotPayload{
		SchemaVersion: 1,
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Standings:     state.preview.Standings,
	}
	if payload.Standings == nil {
		payload.Standings = []models.Standing{}
	}

	snapshotJSON, err := json.Marshal(payload)
	if err != nil {
		return models.Season{}, fmt.Errorf("close season %d: marshal snapshot: %w", seasonID, err)
	}

	if err := s.store.CloseSeasonRecord(ctx, seasonID, string(snapshotJSON), state.setInactive); err != nil {
		return models.Season{}, fmt.Errorf("close season %d: store: %w", seasonID, err)
	}
	return s.store.GetSeason(ctx, seasonID)
}

// ReopenSeason clears closed_at on a closed season, returning it to Historical
// state (active=0, activated_at set, closed_at NULL). final_standings_snapshot
// is preserved as the last close record. Returns ErrNotFound (wrapped) when the
// season does not exist. Returns domainerr.Err with code SEASON_NOT_CLOSED (Conflict)
// when the season is not currently closed.
func (s *SeasonService) ReopenSeason(ctx context.Context, seasonID int64) (models.Season, error) {
	season, err := s.store.GetSeason(ctx, seasonID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return models.Season{}, ErrNotFound
		}
		return models.Season{}, fmt.Errorf("reopen season %d: get season: %w", seasonID, err)
	}
	if season.ClosedAt == nil {
		return models.Season{}, domainerr.New(CodeSeasonNotClosed, domainerr.Conflict,
			"season is not closed; only closed seasons can be reopened")
	}
	if err := s.store.ReopenSeasonRecord(ctx, seasonID); err != nil {
		return models.Season{}, fmt.Errorf("reopen season %d: store: %w", seasonID, err)
	}
	return s.store.GetSeason(ctx, seasonID)
}

// blockerCategory maps a close blocker code to its domainerr category for HTTP mapping.
func blockerCategory(code string) domainerr.Category {
	switch code {
	case CodeSeasonCloseDraft:
		return domainerr.Unprocessable
	case CodeSeasonAlreadyClosed:
		return domainerr.Conflict
	case CodeSeasonCloseNoSchedule:
		return domainerr.Unprocessable
	case CodeSeasonCloseUnclosed:
		return domainerr.Conflict
	default:
		return domainerr.Unprocessable
	}
}
