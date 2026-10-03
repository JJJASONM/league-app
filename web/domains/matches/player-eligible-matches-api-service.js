// Player Eligible Matches API service (Player Score Approval Phase 1C):
// "My Matches" list. seasonId is optional -- when omitted, the backend
// resolves the player's own league's active season server-side, never the
// shell's admin-selected league/season.

export async function fetchPlayerEligibleMatches(playerId, seasonId) {
  const q = seasonId != null ? `?season_id=${seasonId}` : '';
  return api('GET', `/players/${playerId}/eligible-matches${q}`);
}
