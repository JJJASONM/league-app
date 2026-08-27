// Weekly Summary domain API service.

export async function fetchSeasonWeeks(seasonId) {
  return api('GET', `/seasons/${seasonId}/weeks`);
}

export async function fetchWeekRecap(seasonId, weekNum) {
  return api('GET', `/seasons/${seasonId}/weeks/${weekNum}/recap`);
}

// processMatch is used by the "Process Approved Scores" bulk action, which
// loops this single-match call over every approved-but-unprocessed match
// in the displayed week (Weekly Summary Phase 1 -- no new bulk endpoint).
export async function processMatch(matchId) {
  return api('POST', `/matches/${matchId}/process`);
}
