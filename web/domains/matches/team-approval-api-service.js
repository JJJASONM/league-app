// Team Approval API service (Player Score Approval Phase 1C).
//
// Every mutation here requires the score_revision it applies to -- the
// backend rejects a stale one with 409, which the caller must treat as
// "reload the match and let the player review the refreshed scores before
// trying again," never as something to retry automatically. Each call
// resolves with the updated MatchApprovalSnapshot, so callers never need a
// separate read to see the result of their own action.

export async function fetchMatchApproval(matchId) {
  return api('GET', `/matches/${matchId}/approval`);
}

export async function fetchMatchApprovalEvents(matchId) {
  return api('GET', `/matches/${matchId}/approval-events`);
}

export async function approveTeamSide(matchId, side, expectedRevision) {
  return api('POST', `/matches/${matchId}/approve-${side}`, { expected_revision: expectedRevision });
}

export async function withdrawTeamApproval(matchId, side, expectedRevision) {
  return api('POST', `/matches/${matchId}/withdraw-approval`, { side, expected_revision: expectedRevision });
}

export async function requestTeamCorrection(matchId, side, expectedRevision, note) {
  return api('POST', `/matches/${matchId}/request-correction`, { side, expected_revision: expectedRevision, note });
}
