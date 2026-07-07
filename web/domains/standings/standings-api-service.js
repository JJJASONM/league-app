// Standings domain API service.

export async function fetchStandings(seasonId) {
  return api('GET', `/standings?season_id=${seasonId}`);
}

export async function fetchPlayerStats(seasonId) {
  return api('GET', `/player-stats?season_id=${seasonId}`);
}
