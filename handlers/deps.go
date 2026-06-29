package handlers

import (
	"context"

	"league_app/models"
)

// HandicapRecommender is the subset of handicaps.Service used by the handler.
// Accepting an interface (rather than the concrete type) allows stub injection in tests.
type HandicapRecommender interface {
	Recommendations(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error)
}

// Dependencies holds domain services injected into handlers at startup.
// Add new service fields here as additional domains are migrated.
type Dependencies struct {
	HandicapSvc HandicapRecommender
}
