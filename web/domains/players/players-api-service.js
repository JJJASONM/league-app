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
