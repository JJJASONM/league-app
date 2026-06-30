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
// The route is not yet registered (Phase B2); the field is wired in main.go
// so the schema migration runs without requiring a route change.
type HandicapApplier interface {
	Apply(ctx context.Context, seasonID int64, req handicaps.ApplyRequest) (handicaps.ApplyResult, error)
}

// Dependencies holds domain services injected into handlers at startup.
// Add new service fields here as additional domains are migrated.
type Dependencies struct {
	HandicapSvc     HandicapRecommender
	HandicapApplier HandicapApplier
}
