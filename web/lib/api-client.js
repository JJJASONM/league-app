// Methods where a bodyless request can be rejected by a reverse proxy in
// front of the app (e.g. staging's IIS returns 411 Length Required for a
// bodyless POST) before it ever reaches the Go server. GET and DELETE are
// unaffected -- confirmed against staging -- so only these three get a
// synthesized empty body when the caller passes none.
const BODY_REQUIRED_METHODS = ['POST', 'PUT', 'PATCH'];

async function api(method, path, body) {
  const opts = { method, headers: {'Content-Type': 'application/json'} };
  const adminKey = typeof getAdminKey === 'function' ? getAdminKey() : '';
  if (adminKey) opts.headers['Authorization'] = `Bearer ${adminKey}`;
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  } else if (BODY_REQUIRED_METHODS.includes(method)) {
    opts.body = '{}';
  }
  const res = await fetch('/api' + path, opts);
  const data = await res.json();
  if (!res.ok) {
    if (res.status === 401) {
      throw new Error('Admin key required for this action. Set one from the sidebar (Admin Key).');
    }
    if (res.status === 403) {
      throw new Error('Admin key was rejected for this action (missing role or invalid key). Set a valid key from the sidebar (Admin Key).');
    }
    if (Array.isArray(data.messages) && data.messages.length > 0) {
      const errs = data.messages.filter(m => m.level === 'error');
      const list = (errs.length ? errs : data.messages).map(m => m.message).join('; ');
      throw new Error(list);
    }
    throw new Error(data.error || 'Request failed');
  }
  return data;
}
