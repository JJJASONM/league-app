// <users-management-page> - Users Admin Screen Phase 1: list existing users
// and create new ones with an explicit role.
//
// Public API:
//   refresh() -- (re)loads and renders the current user list. Called by the
//     app shell when the Users section activates.
//
// Visibility of the Users nav entry/section is gated by the shell on the
// resolved current identity (system_admin/admin). This component does not
// duplicate that check, but still handles a load failure gracefully in case
// it is reached directly (e.g. a stale tab, a role changed mid-session).
//
// New users may only be created as system_admin or league_admin. "admin" is
// a legacy alias kept valid on existing rows but is not offered here.

import { fetchUsers, createUser } from './users-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

const CREATABLE_ROLES = ['system_admin', 'league_admin'];
const MODAL_ID = 'user-modal';

class UsersManagementPage extends HTMLElement {
  #users = [];

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <h4 class="mb-0 fw-bold">Users</h4>
        <button class="btn btn-primary btn-sm" data-action="add-user">
          <i class="bi bi-plus-lg"></i> Add User
        </button>
      </div>
      <div class="card">
        <div class="card-body p-0">
          <table class="table table-hover mb-0">
            <thead><tr>
              <th>Username</th><th>Role</th><th>Active</th><th>Created</th>
            </tr></thead>
            <tbody class="um-tbody"></tbody>
          </table>
        </div>
      </div>
      <div class="um-new-key-alert alert alert-success d-none mt-3" role="alert"></div>`;

    this.#ensureModal();

    document.getElementById('um-save-btn')
      .addEventListener('click', () => this.#saveUser());

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="add-user"]')) { this.#openNewUser(); }
    });
  }

  refresh() {
    this.#load();
  }

  // -- Private ------------------------------------------------------------------

  #ensureModal() {
    if (document.getElementById(MODAL_ID)) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="user-modal" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Add User</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <div class="mb-3">
          <label class="form-label">Username *</label>
          <input type="text" class="form-control" id="um-username">
        </div>
        <div class="mb-1">
          <label class="form-label">Role *</label>
          <select class="form-select" id="um-role">
            <option value="league_admin">League Admin</option>
            <option value="system_admin">System Admin</option>
          </select>
          <div class="form-text">
            League Admin runs day-to-day league operations (close/reopen
            weeks, handicap apply, season setup). System Admin can also
            manage users, leagues, and global settings.
          </div>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="um-save-btn">Create</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  async #load() {
    this.querySelector('.um-new-key-alert')?.classList.add('d-none');
    try {
      this.#users = await fetchUsers();
      this.#renderList();
    } catch (e) {
      this.#renderError(e);
    }
  }

  #renderError(e) {
    const tbody = this.querySelector('.um-tbody');
    if (!tbody) return;
    tbody.innerHTML =
      `<tr><td colspan="4" class="text-center text-muted py-3">${esc(e.message)}</td></tr>`;
  }

  #renderList() {
    const tbody = this.querySelector('.um-tbody');
    if (!tbody) return;
    tbody.innerHTML = this.#users.map(u => `
      <tr>
        <td class="fw-semibold">${esc(u.username)}</td>
        <td><span class="badge bg-secondary">${esc(u.role)}</span></td>
        <td>${u.active
          ? '<span class="badge bg-success">Active</span>'
          : '<span class="badge bg-secondary">Inactive</span>'}</td>
        <td class="text-muted small">${esc(u.created_at)}</td>
      </tr>`).join('') ||
      '<tr><td colspan="4" class="text-center text-muted py-3">No users yet</td></tr>';
  }

  #openNewUser() {
    document.getElementById('um-username').value = '';
    document.getElementById('um-role').value = 'league_admin';
    this.querySelector('.um-new-key-alert')?.classList.add('d-none');
    new bootstrap.Modal(document.getElementById(MODAL_ID)).show();
  }

  async #saveUser() {
    const username = document.getElementById('um-username').value.trim();
    const role     = document.getElementById('um-role').value;
    if (!username) { toast('Username is required', 'warning'); return; }
    if (!CREATABLE_ROLES.includes(role)) { toast('Select a role', 'warning'); return; }
    try {
      const result = await createUser({ username, role });
      bootstrap.Modal.getInstance(document.getElementById(MODAL_ID))?.hide();
      toast('User created');
      this.#showNewKey(result);
      this.#users = await fetchUsers();
      this.#renderList();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  #showNewKey(result) {
    const alertEl = this.querySelector('.um-new-key-alert');
    if (!alertEl) return;
    alertEl.classList.remove('d-none');
    alertEl.innerHTML =
      `<strong>${esc(result.user.username)}</strong>'s one-time API key ` +
      `(copy it now -- it cannot be shown again):<br>` +
      `<code class="user-select-all">${esc(result.api_key)}</code>`;
  }
}

customElements.define('users-management-page', UsersManagementPage);
