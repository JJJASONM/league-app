// Users domain API service.

export async function fetchUsers() {
  return api('GET', '/users');
}

export async function createUser(body) {
  return api('POST', '/users', body);
}

export async function fetchCurrentUser() {
  return api('GET', '/users/me');
}

// --- Users/Roles Phase 1: email-identity account administration -----------

export async function provisionUser(body) {
  return api('POST', '/auth/admin/users', body);
}

export async function issueSetupToken(userId) {
  return api('POST', `/auth/admin/users/${userId}/setup-token`, {});
}

export async function deactivateUser(userId) {
  return api('POST', `/auth/admin/users/${userId}/deactivate`, {});
}

export async function reactivateUser(userId) {
  return api('POST', `/auth/admin/users/${userId}/reactivate`, {});
}

export async function revokeAPIKeys(userId) {
  return api('POST', `/auth/admin/users/${userId}/revoke-api-keys`, {});
}

export async function fetchRoleAssignments(userId) {
  return api('GET', `/auth/admin/users/${userId}/roles`);
}

export async function grantRole(userId, roleCode, leagueId) {
  const body = { role_code: roleCode };
  if (leagueId != null) body.league_id = leagueId;
  return api('POST', `/auth/admin/users/${userId}/roles`, body);
}

export async function revokeRole(userId, roleCode, leagueId) {
  const body = { role_code: roleCode };
  if (leagueId != null) body.league_id = leagueId;
  return api('POST', `/auth/admin/users/${userId}/roles/revoke`, body);
}
