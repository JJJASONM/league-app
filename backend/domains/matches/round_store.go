package matches

import (
	"context"
	"database/sql"
	"errors"

	"league_app/models"
)

// Guard sentinel errors, returned by every transaction-scoped conditional
// write in this package's SQLite implementation (AdvanceScoreRevisionForEdit,
// ApproveTeamSide, WithdrawTeamApproval, RequestCorrection, GuardedAdminAccept)
// when their conditional UPDATE affects zero rows. Each write's WHERE clause
// re-verifies every condition atomically at write time -- these sentinels
// classify WHY it matched nothing, via a read-only follow-up query, never a
// separate pre-check relied on for correctness. Defined here rather than in
// domainerr because backend/storage/sqlite must not import domainerr (see
// that package's doc comment) but may import this domain package, mirroring
// how backend/domains/players' merge sentinels are shared today.
var (
	// ErrApprovalRevisionStale fires when the caller's expected score
	// revision no longer matches matches.score_revision.
	ErrApprovalRevisionStale = errors.New("approval revision stale")

	// ErrGuardMatchNotFound fires when matchID does not exist.
	ErrGuardMatchNotFound = errors.New("match not found")

	// ErrGuardSeasonClosed fires when the match's season has closed_at set.
	ErrGuardSeasonClosed = errors.New("season is closed")

	// ErrGuardWeekClosed fires when matches.week_closed=1.
	ErrGuardWeekClosed = errors.New("week is closed")

	// ErrGuardAdminAccepted fires when matches.approved_at is already set --
	// blocks score edits, team actions, and a second admin acceptance alike.
	ErrGuardAdminAccepted = errors.New("match already admin-accepted")

	// ErrGuardProcessed fires when matches.processed_at is already set.
	ErrGuardProcessed = errors.New("match already processed")

	// ErrGuardNotScored fires when matches.completed=0 for an operation
	// that requires scores to already exist (admin acceptance, team-side
	// approve/request-correction -- never withdraw, which can only apply
	// to an already-approved, and therefore already-scored, side).
	ErrGuardNotScored = errors.New("match not scored")

	// ErrGuardTeamStateChanged fires only for GuardedAdminAccept, when
	// either side's approval_state no longer matches the state the caller's
	// accept-or-override decision was based on.
	ErrGuardTeamStateChanged = errors.New("team approval state changed")

	// ErrGuardInvalidTransition fires when a team action requires the
	// side's current state to be a specific value (Withdraw requires
	// "approved") and it no longer is.
	ErrGuardInvalidTransition = errors.New("invalid approval state transition")
)

// RoundStore is the persistence interface for the round read/write service.
// Implementations must be safe for concurrent use by multiple goroutines.
// The tx-scoped variant (returned by RunTx) operates within a single transaction.
type RoundStore interface {
	// IsWeekClosed returns true when matches.week_closed=1 for the given matchID.
	// Returns false when no match row exists.
	IsWeekClosed(ctx context.Context, matchID int64) (bool, error)

	// IsSeasonClosedForMatch returns true when the season containing matchID has
	// closed_at set. Returns false when the match or season does not exist.
	IsSeasonClosedForMatch(ctx context.Context, matchID int64) (bool, error)

	// RunTx executes fn inside a single read/write transaction.
	// The RoundStore passed to fn is tx-scoped. Panics and errors both roll back.
	RunTx(ctx context.Context, fn func(RoundStore) error) error

	// LoadMatchContext returns season_id, home_team_id, and away_team_id for a match.
	LoadMatchContext(ctx context.Context, matchID int64) (MatchContext, error)

	// LoadPlayerHandicap returns the current handicap for the given player.
	LoadPlayerHandicap(ctx context.Context, playerID int64) (float64, error)

	// LoadPriorSnapshots returns the stored HC snapshots for existing round_results rows,
	// grouped by round_number. Used to preserve history when a scoresheet is re-saved.
	LoadPriorSnapshots(ctx context.Context, matchID int64) ([]PriorSnapshotRow, error)

	// DeleteRoundResults deletes all round_results rows for the match.
	DeleteRoundResults(ctx context.Context, matchID int64) error

	// InsertRoundResult inserts one round_results row with full snapshot columns.
	InsertRoundResult(ctx context.Context, row RoundResultRow) error

	// DeleteMatchResults deletes all match_results rows for the match.
	DeleteMatchResults(ctx context.Context, matchID int64) error

	// InsertMatchResult inserts one match_results row.
	InsertMatchResult(ctx context.Context, row MatchResultRow) error

	// MarkMatchCompleted sets matches.completed=1 for the match.
	MarkMatchCompleted(ctx context.Context, matchID int64) error

	// MarkMatchIncomplete sets matches.completed=0 for the match.
	MarkMatchIncomplete(ctx context.Context, matchID int64) error

	// GetRoundResults returns all round_results rows for the match joined to player
	// names and current handicaps, ordered by round_number then id.
	GetRoundResults(ctx context.Context, matchID int64) ([]models.RoundResult, error)

	// GetStandingsData returns teams, completed+closed matches, and per-match results
	// for the given season in the shape needed by logic.ComputeStandings.
	GetStandingsData(ctx context.Context, seasonID int64) (StandingsData, error)

	// GetPlayerStats returns aggregated match_results for the given season or league scope.
	GetPlayerStats(ctx context.Context, req PlayerStatsRequest) ([]models.PlayerStat, error)

	// SubmitMatchResults replaces match_results for a match and marks it completed,
	// wrapped in a transaction.
	SubmitMatchResults(ctx context.Context, matchID int64, results []models.MatchResult) error

	// ClearMatchResults deletes match_results for a match and marks it incomplete.
	ClearMatchResults(ctx context.Context, matchID int64) error

	// GetMatchApprovalState returns the match's completed/approved/processed
	// state for Weekly Score Processing Phase 1A. Exists is false when no
	// match row matches matchID.
	GetMatchApprovalState(ctx context.Context, matchID int64) (MatchApprovalState, error)

	// ApproveMatch sets approved_at (now), approved_by_user_id, and
	// approval_note for the match. approvedByUserID may be nil (admin-attested
	// approval does not require a personal-key user in Phase 1A).
	ApproveMatch(ctx context.Context, matchID int64, approvedByUserID *int64, note string) error

	// ProcessMatch sets processed_at (now) and processed_by_user_id for the
	// match. processedByUserID may be nil, same as ApproveMatch.
	ProcessMatch(ctx context.Context, matchID int64, processedByUserID *int64) error

	// UnapproveMatch clears approved_at, approved_by_user_id, and approval_note.
	UnapproveMatch(ctx context.Context, matchID int64) error

	// UnprocessMatch clears processed_at and processed_by_user_id. Approval
	// fields are left untouched.
	UnprocessMatch(ctx context.Context, matchID int64) error

	// AdvanceScoreRevisionForEdit is the transaction-scoped, authoritative
	// guard for every scoresheet-affecting mutation (SaveRounds,
	// SubmitMatchResults, ClearMatchResults, and the SQLite lineup store's
	// substitute-swap methods on an already-scored match). It performs one
	// conditional UPDATE that atomically verifies, at write time, that: the
	// match exists; expectedRevision matches matches.score_revision, when
	// non-nil; the season is not closed; the week is not closed; the match
	// is not admin-accepted (approved_at IS NULL); and the match is not
	// processed (processed_at IS NULL). On success it increments
	// score_revision, resets any team side that is not already "pending"
	// back to "pending" (recording one "cleared_by_edit" event per side
	// actually reset), and returns the new revision. On failure it performs
	// a read-only follow-up query to classify the exact reason and returns
	// the matching sentinel (ErrGuardMatchNotFound, ErrApprovalRevisionStale,
	// ErrGuardSeasonClosed, ErrGuardWeekClosed, ErrGuardAdminAccepted, or
	// ErrGuardProcessed) with zero writes.
	//
	// Callers must invoke this as the FIRST statement inside the exact same
	// transaction as the destructive mutation that follows it (never as a
	// separate commit), so that a later failure in that mutation rolls back
	// this call's revision bump and clears along with it.
	AdvanceScoreRevisionForEdit(ctx context.Context, matchID int64, expectedRevision *int) (newRevision int, err error)

	// ResolveApprovalEligibility reports whether playerID is eligible to
	// act (approve, withdraw, or request correction) for the given side
	// ("home" or "away") of matchID: either currently rostered to that
	// side's team for the match's season (season_rosters), or an actual
	// round_results participant on that side for this specific match.
	// Either condition alone is sufficient; there is no captain
	// requirement and no requirement to have personally played when
	// rostered.
	ResolveApprovalEligibility(ctx context.Context, matchID int64, side string, playerID int64) (bool, error)

	// ApproveTeamSide sets the given side's approval_state to 'approved'
	// for expectedRevision via one conditional UPDATE that atomically
	// re-verifies, at write time: the match exists; score_revision equals
	// expectedRevision; the match is completed; the season is not closed;
	// the week is not closed; approved_at IS NULL; and processed_at IS
	// NULL. On success it records an 'approved' match_approval_events row.
	// On failure (zero rows matched) it classifies the reason via a
	// read-only follow-up query and returns the matching sentinel
	// (ErrGuardMatchNotFound, ErrApprovalRevisionStale, ErrGuardNotScored,
	// ErrGuardSeasonClosed, ErrGuardWeekClosed, ErrGuardAdminAccepted, or
	// ErrGuardProcessed), with zero writes either way.
	// actorUserID/actorPlayerID/actorNameSnapshot identify who approved.
	ApproveTeamSide(ctx context.Context, matchID int64, side string, expectedRevision int, actorUserID, actorPlayerID *int64, actorNameSnapshot string) error

	// WithdrawTeamApproval resets the given side's approval_state from
	// 'approved' back to 'pending' for expectedRevision via one conditional
	// UPDATE additionally requiring the side's current state to already be
	// 'approved'. Records a 'withdrawn' event on success. On failure,
	// classifies and returns ErrGuardMatchNotFound, ErrApprovalRevisionStale,
	// ErrGuardSeasonClosed, ErrGuardWeekClosed, ErrGuardAdminAccepted,
	// ErrGuardProcessed, or ErrGuardInvalidTransition, with zero writes.
	WithdrawTeamApproval(ctx context.Context, matchID int64, side string, expectedRevision int, actorUserID, actorPlayerID *int64, actorNameSnapshot string) error

	// RequestCorrection sets the given side's approval_state to
	// 'correction_requested' for expectedRevision with the given note via
	// one conditional UPDATE requiring the same match-must-be-editable
	// conditions as ApproveTeamSide (no FROM-state restriction -- a
	// correction may be requested from 'pending' or 'approved' alike).
	// Records a 'correction_requested' event on success. On failure,
	// classifies and returns the matching sentinel, with zero writes.
	RequestCorrection(ctx context.Context, matchID int64, side string, expectedRevision int, note string, actorUserID, actorPlayerID *int64, actorNameSnapshot string) error

	// ListApprovalEvents returns the full match_approval_events history for
	// matchID, ordered oldest first. Returns a non-nil empty slice when no
	// events exist.
	ListApprovalEvents(ctx context.Context, matchID int64) ([]models.MatchApprovalEvent, error)

	// GuardedAdminAccept atomically performs the admin-acceptance write for
	// the Phase 1B-ready AdminAcceptMatch primitive (never the existing,
	// externally-reachable ApproveMatch, which is unchanged). Its
	// conditional UPDATE pins the COMPLETE decision snapshot
	// RoundService.AdminAcceptMatch's accept-vs-override choice was based
	// on, not just the state labels: pinnedHome.State/pinnedAway.State,
	// pinnedHome.StateScoreRevision/pinnedAway.StateScoreRevision
	// (NULL-safe -- a nil pinned revision only matches a currently-nil
	// column), and pinnedHome.CorrectionNote/pinnedAway.CorrectionNote --
	// alongside match-exists/expectedRevision/completed/season-open/
	// week-open/approved_at-IS-NULL/processed_at-IS-NULL. Pinning the note
	// and revision (not just the label) matters because a correction
	// request can be rewritten at the same revision with the same
	// "correction_requested" label but a different note -- without pinning
	// the note too, the guard could accept the match while the recorded
	// event snapshot describes a note that was already superseded. On
	// success it sets approved_at/approved_by_user_id/approval_note (the
	// same fields ApproveMatch sets) and returns nil; the caller records
	// the admin_accepted/admin_override event afterward, in the same
	// transaction, using these SAME pinned values, so the guarded write
	// and the event describe one consistent snapshot. On failure,
	// classifies and returns ErrGuardMatchNotFound, ErrApprovalRevisionStale,
	// ErrGuardNotScored, ErrGuardSeasonClosed, ErrGuardWeekClosed,
	// ErrGuardProcessed, ErrGuardAdminAccepted (already accepted), or
	// ErrGuardTeamStateChanged (state, revision, or note changed), with
	// zero writes.
	GuardedAdminAccept(ctx context.Context, matchID int64, expectedRevision int, pinnedHome, pinnedAway TeamSideApprovalState, approvedByUserID *int64, note string) error

	// RecordAdminAcceptanceEvent inserts one match-scope
	// ('admin_accepted' or 'admin_override') match_approval_events row,
	// capturing a structured snapshot of both sides' state/note at the
	// moment of the admin's decision.
	RecordAdminAcceptanceEvent(ctx context.Context, ev AdminAcceptanceEvent) error
}

// MatchApprovalState holds one match's Weekly Score Processing Phase 1A
// state plus Player Score Approval Phase 1A's score revision and per-side
// team-approval state, used to validate every state transition in this
// package.
type MatchApprovalState struct {
	Exists      bool
	Completed   bool
	ApprovedAt  *string
	ProcessedAt *string

	ScoreRevision int
	HomeSide      TeamSideApprovalState
	AwaySide      TeamSideApprovalState
}

// TeamSideApprovalState holds one team side's current approval state.
// State is always exactly "pending", "approved", or "correction_requested"
// -- never inferred from timestamp presence alone, per the schema's CHECK
// constraint. StateScoreRevision is meaningful whenever State is "approved"
// or "correction_requested" -- the exact score_revision that state
// describes (both states have an actor and a revision they apply to; only
// "pending" has neither, and always carries a nil StateScoreRevision).
type TeamSideApprovalState struct {
	State              string
	StateAt            *string
	ActorUserID        *int64
	ActorPlayerID      *int64
	ActorNameSnapshot  string
	StateScoreRevision *int
	CorrectionNote     string
}

// Team-approval state constants. Mirrors the CHECK constraint on
// matches.home_approval_state / away_approval_state exactly.
const (
	ApprovalStatePending             = "pending"
	ApprovalStateApproved            = "approved"
	ApprovalStateCorrectionRequested = "correction_requested"
)

// Approval side constants, matching match_approval_events.event_scope's
// "home"/"away" values (the third possible value, "match", is used only by
// admin-level acceptance/override events, never as a side value here).
const (
	ApprovalSideHome = "home"
	ApprovalSideAway = "away"
)

// AdminAcceptanceEvent is the structured input for
// RoundStore.RecordAdminAcceptanceEvent.
type AdminAcceptanceEvent struct {
	MatchID       int64
	EventType     string // "admin_accepted" | "admin_override"
	ActorUserID   *int64
	Note          string
	ScoreRevision int
	HomeState     string
	HomeNote      string
	AwayState     string
	AwayNote      string
}

// MatchContext holds the season and team IDs for a match.
type MatchContext struct {
	SeasonID   int64
	HomeTeamID int64
	AwayTeamID int64
}

// PriorSnapshotRow holds the HC snapshot from an existing round_results row.
// Used by SaveRounds to preserve handicap history on re-save.
type PriorSnapshotRow struct {
	RoundNumber      int
	HomePlayerID     int64
	AwayPlayerID     int64
	HomeHandicapUsed sql.NullFloat64
	AwayHandicapUsed sql.NullFloat64
}

// RoundResultRow is one row to be inserted into round_results, including full HC snapshots.
type RoundResultRow struct {
	MatchID         int64
	RoundNumber     int
	HomePlayerID    int64
	AwayPlayerID    int64
	Game1Home       int
	Game1Away       int
	Game2Home       int
	Game2Away       int
	Game3Home       int
	Game3Away       int
	HomeHCUsed      float64
	AwayHCUsed      float64
	HandicapPtsUsed int
	HandicapTo      string
}

// MatchResultRow is one row to be inserted into match_results.
type MatchResultRow struct {
	MatchID   int64
	PlayerID  int64
	TeamID    int64
	GamesWon  int
	GamesLost int
	Diff      float64
	SetsWon   int
	SetsLost  int
}

// StandingsData is the raw data returned from GetStandingsData,
// ready to be passed to logic.ComputeStandings.
type StandingsData struct {
	Teams     []models.Team
	Matches   []models.Match
	ResultMap map[int64][]models.MatchResult
}

// PlayerStatsRequest parameterizes GetPlayerStats.
// Exactly one of SeasonID or LeagueID should be non-zero.
type PlayerStatsRequest struct {
	SeasonID int64
	LeagueID int64
}

// SaveRoundsInput is the domain-level input for RoundService.SaveRounds.
// ExpectedRevision is optional (Player Score Approval Phase 1A): when set,
// SaveRounds fails with CodeApprovalRevisionStale and performs zero writes
// if it no longer matches the match's current score_revision. The existing
// HTTP handler never sets it, so existing admin behavior is unchanged --
// this exists so Phase 1B can populate it from a request body field without
// another struct-shape change.
type SaveRoundsInput struct {
	MatchID          int64
	Rounds           []models.RoundResult
	ExpectedRevision *int
}
