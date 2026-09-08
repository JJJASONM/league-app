// Communication domain API service.
//
// League Communication Screen Phase 1 is presentation-only: it generates
// copy/paste message text from data other domains already expose. No new
// backend routes were added for this phase -- these are thin wrappers over
// the exact same routes weekly-summary, finances, and players already call,
// duplicated here rather than importing those domains' api-service modules
// directly, matching this codebase's convention of each frontend domain
// owning its own API calls even when another domain reads the same route.

export async function fetchSeasonWeeks(seasonId) {
  return api('GET', `/seasons/${seasonId}/weeks`);
}

export async function fetchWeekRecap(seasonId, weekNum) {
  return api('GET', `/seasons/${seasonId}/weeks/${weekNum}/recap`);
}

// Requires an Admin Key with league_admin/admin/system_admin role, same as
// the Financial screen -- this screen has no fallback for viewers without
// one; a failed load shows the api() client's own 401/403 message inline.
export async function fetchSeasonDues(seasonId) {
  return api('GET', `/seasons/${seasonId}/finances/dues`);
}

// seasonId is optional -- when omitted, the backend defaults to the
// player's league's active season.
export async function fetchPlayerOverview(playerId, seasonId) {
  const q = seasonId ? `?season_id=${seasonId}` : '';
  return api('GET', `/players/${playerId}/overview${q}`);
}
