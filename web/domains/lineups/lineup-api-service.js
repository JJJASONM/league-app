// Lineup Planning domain API service.
// Wraps the shell's global api() function with semantic method names.

export async function fetchMatches(seasonId) {
  return api('GET', `/matches?season_id=${seasonId}`);
}

export async function fetchLineupPlans(seasonId, weekNum) {
  return api('GET', `/lineup-plans?season_id=${seasonId}&week_number=${weekNum}`);
}

export async function saveLineupPlans(body) {
  return api('POST', '/lineup-plans', body);
}
