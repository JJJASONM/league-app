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
