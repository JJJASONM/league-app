// Finances domain API service.
// All four routes require an Admin Key with league_admin/admin/system_admin
// role -- the shared api() client already attaches the Admin Key when one
// is set for this tab; no finances-specific auth handling is needed here.

export async function fetchSeasonDues(seasonId) {
  return api('GET', `/seasons/${seasonId}/finances/dues`);
}

export async function recordDuesPayment(seasonId, body) {
  return api('POST', `/seasons/${seasonId}/finances/dues-payments`, body);
}

export async function fetchSeasonPayouts(seasonId) {
  return api('GET', `/seasons/${seasonId}/finances/payouts`);
}

export async function recordPayout(seasonId, body) {
  return api('POST', `/seasons/${seasonId}/finances/payouts`, body);
}
