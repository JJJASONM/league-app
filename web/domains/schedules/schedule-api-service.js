// Schedule domain API service.

export async function fetchSeasonMatches(seasonId) {
  return api('GET', `/matches?season_id=${seasonId}`);
}

export async function fetchSeasonWeeks(seasonId) {
  return api('GET', `/seasons/${seasonId}/weeks`);
}

export async function fetchWeekValidation(seasonId, weekNum) {
  return api('GET', `/seasons/${seasonId}/weeks/${weekNum}/validate`);
}

export async function fetchAdvancePreview(seasonId, weekNum) {
  return api('GET', `/seasons/${seasonId}/weeks/${weekNum}/advance-preview`);
}

export async function fetchWeekAcknowledgments(seasonId, weekNum) {
  return api('GET', `/seasons/${seasonId}/weeks/${weekNum}/acknowledgments`);
}

export async function closeWeek(seasonId, weekNum, body) {
  return api('POST', `/seasons/${seasonId}/weeks/${weekNum}/close`, body);
}

export async function reopenWeek(seasonId, weekNum) {
  return api('POST', `/seasons/${seasonId}/weeks/${weekNum}/reopen`);
}

export async function assignMatchTeams(matchId, body) {
  return api('PATCH', `/matches/${matchId}/assign`, body);
}

export async function fetchLeaguePlayers(leagueId) {
  return api('GET', `/players?league_id=${leagueId}`);
}
