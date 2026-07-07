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

export async function saveRounds(matchId, rounds) {
  return api('POST', `/matches/${matchId}/rounds`, { rounds });
}

export async function clearMatchResults(matchId) {
  return api('DELETE', `/matches/${matchId}/results`);
}
