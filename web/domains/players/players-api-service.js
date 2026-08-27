// Players domain API service.

export async function fetchPlayers(leagueId) {
  return api('GET', `/players?league_id=${leagueId}`);
}

export async function createPlayer(body) {
  return api('POST', '/players', body);
}

export async function updatePlayer(id, body) {
  return api('PUT', `/players/${id}`, body);
}

export async function removePlayer(id) {
  return api('DELETE', `/players/${id}`);
}

// fetchPlayerOverview loads the Player Overview aggregate for one player.
// seasonId is optional -- when omitted, the backend defaults to the
// player's league's active season.
export async function fetchPlayerOverview(id, seasonId) {
  const q = seasonId ? `?season_id=${seasonId}` : '';
  return api('GET', `/players/${id}/overview${q}`);
}
