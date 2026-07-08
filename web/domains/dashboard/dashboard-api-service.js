// Dashboard domain API service.

export async function fetchDashboardMatches(seasonId) {
  return api('GET', `/matches?season_id=${seasonId}`);
}

export async function fetchDashboardStandings(seasonId) {
  return api('GET', `/standings?season_id=${seasonId}`);
}
