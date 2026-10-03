// Match Entry domain API service.

export async function fetchSeasonMatches(seasonId) {
  return api('GET', `/matches?season_id=${seasonId}`);
}

export async function fetchMatch(matchId) {
  return api('GET', `/matches/${matchId}`);
}

export async function fetchSeasonRules(seasonId) {
  return api('GET', `/seasons/${seasonId}/rules`);
}

export async function fetchRounds(matchId) {
  return api('GET', `/matches/${matchId}/rounds`);
}

export async function fetchLineupPlans(seasonId, weekNumber) {
  return api('GET', `/lineup-plans?season_id=${seasonId}&week_number=${weekNumber}`);
}

// expectedRevision (Player Score Approval Phase 1C) is omitted from the
// body entirely when not given -- the existing admin write path stays
// byte-for-byte unchanged; a player-mode caller always passes the
// score_revision from the match's last-fetched approval snapshot, which
// the backend requires unconditionally for a non-admin write.
export async function saveRounds(matchId, rounds, expectedRevision) {
  const body = expectedRevision != null ? { rounds, expected_revision: expectedRevision } : { rounds };
  return api('POST', `/matches/${matchId}/rounds`, body);
}

export async function clearMatchResults(matchId, expectedRevision) {
  const body = expectedRevision != null ? { expected_revision: expectedRevision } : undefined;
  return api('DELETE', `/matches/${matchId}/results`, body);
}

// Weekly Score Processing Phase 1A/1B admin actions. note is optional and
// only meaningful for approveMatch (admin-attested team/captain approval).
export async function approveMatch(matchId, note) {
  return api('POST', `/matches/${matchId}/approve`, note ? { note } : undefined);
}

export async function processMatch(matchId) {
  return api('POST', `/matches/${matchId}/process`);
}

export async function unprocessMatch(matchId) {
  return api('POST', `/matches/${matchId}/unprocess`);
}

export async function unapproveMatch(matchId) {
  return api('POST', `/matches/${matchId}/unapprove`);
}

// Substitute Workflow Phase 1: set/clear a substitute for an existing
// lineup_plans slot. lineupPlanId is the row's id, not a player id.
export async function setLineupSubstitute(lineupPlanId, substitutePlayerId) {
  return api('POST', `/lineup-plans/${lineupPlanId}/substitute`, { substitute_player_id: substitutePlayerId });
}

export async function clearLineupSubstitute(lineupPlanId) {
  return api('DELETE', `/lineup-plans/${lineupPlanId}/substitute`);
}
