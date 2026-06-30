package handlers

import (
	"context"

	"league_app/backend/domains/handicaps"
	"league_app/models"
)

// HandicapRecommender is the subset of handicaps.Service used by the read handler.
// Accepting an interface (rather than the concrete type) allows stub injection in tests.
type HandicapRecommender interface {
	Recommendations(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error)
}

// HandicapApplier is the subset of handicaps.Service used by the write handler.
// The route is registered in Register() when deps.AdminToken is non-empty.
type HandicapApplier interface {
	Apply(ctx context.Context, seasonID int64, req handicaps.ApplyRequest) (handicaps.ApplyResult, error)
}

// Dependencies holds domain services injected into handlers at startup.
// Add new service fields here as additional domains are migrated.
type Dependencies struct {
	HandicapSvc     HandicapRecommender
	HandicapApplier HandicapApplier
	// AdminToken is the bearer token required to call the Apply route.
	// When empty the Apply route is not mounted. Read from LEAGUE_ADMIN_TOKEN at startup.
	// AppliedByUserID stays nil in B2; it will be wired to a resolved users.id
	// in the future users/auth phase when a users table exists.
	AdminToken string
}
