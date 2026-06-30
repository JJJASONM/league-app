// Pure helpers and API calls for the Handicap Apply workflow.
// All functions are exported; none carry state.

// escapeHTML prevents XSS when inserting user-controlled text into innerHTML.
// Domain-local copy — the component must not depend on the shell global.
export function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, ch => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[ch]));
}

// fmtHC formats a handicap float for display: "+2.5", "-1", "+0".
// Domain-local copy — the component must not depend on the shell global.
export function fmtHC(v) { return (v >= 0 ? '+' : '') + v; }

// isSelectableRec returns true when a recommendation row is eligible for Apply.
// Requires a rec_token, a non-null change_amount, and change_amount !== 0.
// no_change rows (change_amount === 0) are excluded per B3 product spec.
export function isSelectableRec(rec) {
  return !!(rec.rec_token && rec.change_amount != null && rec.change_amount !== 0);
}

// buildApplyEntries converts selected recommendation rows to the Apply entry payload shape.
// Callers must pass only rows satisfying isSelectableRec.
export function buildApplyEntries(selectedRecs) {
  return selectedRecs.map(r => ({
    player_id:               r.player_id,
    expected_assigned_hc:    r.assigned_hc,
    expected_recommended_hc: r.recommended_hc,
    rec_token:               r.rec_token,
  }));
}

// makeApplyRequestId generates a UUID v4.
// Uses crypto.randomUUID() in secure contexts; falls back for http:// origins
// (e.g. http://league-staging.local where the Crypto API may be unavailable).
export function makeApplyRequestId() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = Math.random() * 16 | 0;
    return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16);
  });
}

// describeConflict returns a human-readable message for one Apply conflict entry.
// recsByPlayerId maps player_id → HandicapReviewRec for name lookup.
export function describeConflict(conflict, recsByPlayerId) {
  const rec  = recsByPlayerId[conflict.player_id];
  const name = rec ? rec.player_name : `Player #${conflict.player_id}`;
  const labels = {
    token_mismatch:         'recommendation has changed since load',
    assigned_hc_changed:    'assigned handicap changed since load',
    recommended_hc_changed: 'recommended handicap changed since load',
    not_in_roster:          "player is not in this season's roster",
    concurrent_write:       'concurrent update — reload and retry',
    idempotency_key_reused: 'request ID already used for a different payload',
  };
  return `${name}: ${labels[conflict.code] || conflict.code}`;
}

// describeRejection returns a human-readable message for one Apply rejection entry.
export function describeRejection(rejection, recsByPlayerId) {
  const rec  = recsByPlayerId[rejection.player_id];
  const name = rec ? rec.player_name : `Player #${rejection.player_id}`;
  const labels = {
    admin_hold:      'player is on admin hold',
    below_threshold: 'insufficient match data',
    no_data:         'no rack data available',
  };
  return `${name}: ${labels[rejection.code] || rejection.code}`;
}

// fetchRecommendations loads the HandicapReviewResponse for a season.
// Throws with .status on HTTP error.
export async function fetchRecommendations(seasonId) {
  const res  = await fetch(`/api/seasons/${seasonId}/handicap-recommendations`);
  const data = await res.json();
  if (!res.ok) {
    const err = new Error(data.error || 'Failed to load recommendations');
    err.status = res.status;
    throw err;
  }
  return data;
}

// applyHandicaps submits a handicap apply request.
// Returns ApplyResult on success.
// Throws with .status and .payload on HTTP error.
// Handles non-JSON responses (e.g. Go router 404 plain-text "page not found").
export async function applyHandicaps(seasonId, token, body) {
  const res = await fetch(`/api/seasons/${seasonId}/handicap-apply`, {
    method:  'POST',
    headers: {
      'Content-Type':  'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  });

  let data = null;
  const ct = res.headers.get('Content-Type') || '';
  if (ct.includes('application/json')) {
    data = await res.json();
  }

  if (!res.ok) {
    const msg  = data?.error || (res.status === 404 ? 'not found' : 'Apply failed');
    const err  = new Error(msg);
    err.status  = res.status;
    err.payload = data;
    throw err;
  }
  return data;
}
