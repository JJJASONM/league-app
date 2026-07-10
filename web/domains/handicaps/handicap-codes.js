// Handicap reason codes returned by the recommendations API.
// These values drive UI branching and must remain stable with the backend
// (see backend/domains/handicaps/codes.go for the authoritative definitions).
export const REASON_ADMIN_HOLD      = 'admin_hold';
export const REASON_NO_DATA         = 'no_data';
export const REASON_BELOW_THRESHOLD = 'below_threshold';
export const REASON_NO_CHANGE       = 'no_change';
export const REASON_CAPPED          = 'capped';
