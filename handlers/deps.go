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

// ApplyAuthResolver resolves and manages application users for Apply attribution.
// This is a purpose-built interface — not a generic user store.
type ApplyAuthResolver interface {
	// ResolveApplyUserByAPIKey returns the active user matching SHA-256(apiKey),
	// or nil, nil when no match is found.
	ResolveApplyUserByAPIKey(ctx context.Context, apiKey string) (*models.User, error)
	// CreateApplyUser creates a new user and returns the user plus the one-time cleartext key.
	CreateApplyUser(ctx context.Context, username string) (models.User, string, error)
	// ListApplyUsers returns all users. The api_key_hash column is never exposed.
	ListApplyUsers(ctx context.Context) ([]models.User, error)
}

// applyUserIDKey is an unexported type used as a context key for the resolved
// user ID on authenticated Apply requests. A struct type avoids collisions with
// other packages that use string keys.
type applyUserIDKey struct{}

// Dependencies holds domain services injected into handlers at startup.
// Add new service fields here as additional domains are migrated.
type Dependencies struct {
	HandicapSvc     HandicapRecommender
	HandicapApplier HandicapApplier
	// AdminToken is the static bearer token for LEAGUE_ADMIN_TOKEN fallback auth.
	// When empty the Apply route is not mounted.
	// Personal API keys (via ApplyAuth) are checked first; this token is the fallback.
	AdminToken string
	// ApplyAuth resolves personal API keys for Apply attribution.
	// When nil, only the AdminToken static fallback is used.
	ApplyAuth ApplyAuthResolver
}
