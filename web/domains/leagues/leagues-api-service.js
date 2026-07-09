// Leagues domain API service.

export async function fetchAllLeagues() {
  return api('GET', '/leagues');
}

export async function fetchLeagueTeams(leagueId) {
  return api('GET', `/teams?league_id=${leagueId}`);
}

export async function createLeague(name, gameFormat, dayOfWeek) {
  return api('POST', '/leagues', { name, game_format: gameFormat, day_of_week: dayOfWeek || null });
}

export async function removeLeague(id) {
  return api('DELETE', `/leagues/${id}`);
}
