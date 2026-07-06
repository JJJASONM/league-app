// Seasons domain API service.
// Wraps the shell's global api() function with semantic method names.
// All methods are async and throw on HTTP error (propagated from api()).

export async function listSeasons(leagueId) {
  return api('GET', `/seasons?league_id=${leagueId}`);
}

export async function createSeason(body) {
  return api('POST', '/seasons', body);
}

export async function updateSeason(id, body) {
  return api('PUT', `/seasons/${id}`, body);
}

export async function deleteSeason(id) {
  return api('DELETE', `/seasons/${id}`);
}

export async function activateSeason(id) {
  return api('POST', `/seasons/${id}/activate`);
}

export async function generateSchedule(body) {
  return api('POST', '/matches/generate', body);
}

export async function listSkippedWeeks(seasonId) {
  return api('GET', `/seasons/${seasonId}/skipped-weeks`);
}

export async function addSkippedWeek(seasonId, body) {
  return api('POST', `/seasons/${seasonId}/skipped-weeks`, body);
}

export async function removeSkippedWeek(seasonId, skipId) {
  return api('DELETE', `/seasons/${seasonId}/skipped-weeks/${skipId}`);
}

export async function listByeRequests(seasonId) {
  return api('GET', `/seasons/${seasonId}/bye-requests`);
}

export async function addByeRequest(seasonId, body) {
  return api('POST', `/seasons/${seasonId}/bye-requests`, body);
}

export async function updateByeRequest(seasonId, byeId, body) {
  return api('PUT', `/seasons/${seasonId}/bye-requests/${byeId}`, body);
}

export async function removeByeRequest(seasonId, byeId) {
  return api('DELETE', `/seasons/${seasonId}/bye-requests/${byeId}`);
}

export async function listSeasonTeams(seasonId) {
  return api('GET', `/seasons/${seasonId}/teams`);
}

export async function getSeasonChecklist(seasonId) {
  return api('GET', `/seasons/${seasonId}/checklist`);
}
