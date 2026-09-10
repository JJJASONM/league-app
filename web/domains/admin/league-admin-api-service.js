// League Admin hub API service.
//
// League Admin Screen Phase 1 is a read-only operational command center. It
// adds no backend routes -- these are thin wrappers over endpoints Weekly
// Summary, Schedule, Lineups, and Financial already use, duplicated here
// rather than importing those domains' api-service modules directly,
// matching this codebase's convention of each frontend domain owning its
// own API calls even when the underlying route is shared.

export async function fetchSeasonWeeks(seasonId) {
  return api('GET', `/seasons/${seasonId}/weeks`);
}

export async function fetchWeekRecap(seasonId, weekNum) {
  return api('GET', `/seasons/${seasonId}/weeks/${weekNum}/recap`);
}

export async function fetchLineupPlans(seasonId, weekNum) {
  return api('GET', `/lineup-plans?season_id=${seasonId}&week_number=${weekNum}`);
}

// Requires an Admin Key with league_admin/admin/system_admin role, same as
// the Financial screen. The hub is already gated to those roles, so an
// admin viewing it has a key; a failed call degrades that one card to a
// link rather than blocking the whole screen.
export async function fetchSeasonDues(seasonId) {
  return api('GET', `/seasons/${seasonId}/finances/dues`);
}
