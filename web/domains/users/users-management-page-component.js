// <users-management-page> - Users Admin Screen Phase 1: list existing users
// and create new ones with an explicit role.
//
// Public API:
//   refresh(allPlayers, allLeagues) -- (re)loads and renders the current
//     user list. Called by the app shell when the Users section activates.
//     allPlayers (Player Account Access Phase 1) populates the
//     linked-player picker shown when role=player is selected. allLeagues
//     (Users/Roles Phase 1 UI correction) resolves a league_admin
//     assignment's league_id to a display name in the Access column.
//
// Visibility of the Users nav entry/section is gated by the shell on the
// resolved current identity (system_admin/admin). This component does not
// duplicate that check, but still handles a load failure gracefully in case
// it is reached directly (e.g. a stale tab, a role changed mid-session).
//
// New users may be created as system_admin, league_admin, or player
// (Player Account Access Phase 1 -- requires linking an existing player),
// via the legacy API-key path ("Add User (API Key)"). "admin" is a legacy
// alias kept valid on existing rows but is not offered here.
//
// Users/Roles Phase 1 adds a second creation path, "Provision Email
// Login" -- an email-identity account with no password and no API key,
// paired with a one-time password setup token (issued from the row
// menu) the account holder uses to choose their own password. The row
// menu also supports deactivate/reactivate (deactivation immediately
// revokes every session and API key for that account) and revoking API
// keys without deactivating. Scoped role assignment (grant/revoke
// league_admin for a specific league, or system_admin) is not yet
// surfaced in this UI -- use the API directly
// (POST /api/auth/admin/users/{id}/roles) until a later phase adds it
// here.

import {
  fetchUsers, createUser,
  provisionUser, issueSetupToken, deactivateUser, reactivateUser, revokeAPIKeys,
} from './users-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

const CREATABLE_ROLES = ['system_admin', 'league_admin', 'player'];
const MODAL_ID = 'user-modal';

class UsersManagementPage extends HTMLElement {
  #users      = [];
  #allPlayers = [];
  #allLeagues = [];

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <h4 class="mb-0 fw-bold">Users</h4>
        <div>
          <button class="btn btn-outline-primary btn-sm me-2" data-action="provision-account">
            <i class="bi bi-envelope-plus"></i> Provision Email Login
          </button>
          <button class="btn btn-primary btn-sm" data-action="add-user">
            <i class="bi bi-plus-lg"></i> Add User (API Key)
          </button>
        </div>
      </div>
      <div class="card">
        <div class="card-body p-0">
          <table class="table table-hover mb-0">
            <thead><tr>
              <th>Username</th><th>Email</th><th>Access</th><th>Linked Player</th><th>Active</th><th>Created</th><th></th>
            </tr></thead>
            <tbody class="um-tbody"></tbody>
          </table>
        </div>
      </div>
      <div class="um-new-key-alert alert alert-success d-none mt-3" role="alert"></div>`;

    this.#ensureModal();
    this.#ensureProvisionModal();

    document.getElementById('um-save-btn')
      .addEventListener('click', () => this.#saveUser());
    document.getElementById('um-role')
      .addEventListener('change', () => this.#toggleLinkedPlayerRow());
    document.getElementById('um-provision-save-btn')
      .addEventListener('click', () => this.#saveProvisionedAccount());

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="add-user"]')) { this.#openNewUser(); }
      if (e.target.closest('[data-action="provision-account"]')) { this.#openProvisionAccount(); }

      const row = e.target.closest('[data-user-id]');
      if (!row) return;
      const userId = parseInt(row.dataset.userId, 10);
      if (e.target.closest('[data-row-action="setup-token"]')) this.#issueSetupToken(userId);
      if (e.target.closest('[data-row-action="deactivate"]')) this.#toggleActive(userId, false);
      if (e.target.closest('[data-row-action="reactivate"]')) this.#toggleActive(userId, true);
      if (e.target.closest('[data-row-action="revoke-keys"]')) this.#revokeKeys(userId);
    });
  }

  refresh(allPlayers, allLeagues) {
    this.#allPlayers = allPlayers ?? [];
    this.#allLeagues = allLeagues ?? [];
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
        <div class="mb-3">
          <label class="form-label">Role *</label>
          <select class="form-select" id="um-role">
            <option value="league_admin">League Admin</option>
            <option value="system_admin">System Admin</option>
            <option value="player">Player</option>
          </select>
          <div class="form-text">
            League Admin runs day-to-day league operations (close/reopen
            weeks, handicap apply, season setup). System Admin can also
            manage users, leagues, and global settings. Player is a
            read-only account limited to that one player's own Player
            Overview (schedule, stats, dues) -- no admin access.
          </div>
        </div>
        <div class="mb-1 um-linked-player-row d-none">
          <label class="form-label">Linked Player *</label>
          <select class="form-select" id="um-player-id"></select>
          <div class="form-text">
            Required for role=player. The account can view only this
            player's own overview.
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
      `<tr><td colspan="5" class="text-center text-muted py-3">${esc(e.message)}</td></tr>`;
  }

  // #formatAccess renders the Access column from the account's REAL,
  // current access rather than the legacy flat role field (Users/Roles
  // Phase 1 UI correction, staging-UI-isolation round): a session/email
  // account's actual authorization comes entirely from its
  // role_assignments rows (u.role_assignments, populated by
  // ListApplyUsers's single query -- see backend/storage/sqlite/
  // apply_auth_store.go) or, for role=player, from its linked player_id --
  // never from u.role, which can disagree with reality (e.g. a
  // league_admin created via the legacy endpoint with zero grants shows
  // role="league_admin" but has no access at all; every new email-login
  // account shows the legacy "admin" role regardless of its real scoped
  // access). One badge per assignment (a league_admin may hold more than
  // one league). Falls back to an explicit NO-ACCESS label (PM
  // correction: "Legacy: <role>" alone could still read as granted
  // access) only when there are no assignments AND no linked player --
  // this covers both a genuine pre-Phase-1 account the migration
  // backfill did not reach, and a newly created legacy league_admin
  // account still awaiting its first scoped role grant. The legacy role
  // string is shown only as parenthetical context, never as if it were
  // current access.
  #formatAccess(u) {
    const assignments = u.role_assignments ?? [];
    if (assignments.length > 0) {
      return assignments.map(a => {
        if (a.role_code === 'system_admin') {
          return '<span class="badge bg-primary me-1">System Admin</span>';
        }
        if (a.role_code === 'league_admin') {
          return `<span class="badge bg-info text-dark me-1">League Admin -- ${esc(this.#leagueName(a.league_id))}</span>`;
        }
        return `<span class="badge bg-secondary me-1">${esc(a.role_code)}</span>`;
      }).join('');
    }
    if (u.player_id != null) {
      return '<span class="badge bg-success">Player</span>';
    }
    return `<span class="badge bg-light text-muted border">No scoped access (legacy role: ${esc(u.role)})</span>`;
  }

  #leagueName(leagueId) {
    const league = this.#allLeagues.find(l => l.id === leagueId);
    return league ? league.name : `League #${leagueId}`;
  }

  #renderList() {
    const tbody = this.querySelector('.um-tbody');
    if (!tbody) return;
    tbody.innerHTML = this.#users.map(u => `
      <tr data-user-id="${u.id}">
        <td class="fw-semibold">${esc(u.username)}</td>
        <td class="text-muted small">${u.email ? esc(u.email) : '<span class="text-muted">-</span>'}</td>
        <td>${this.#formatAccess(u)}</td>
        <td class="text-muted small">${u.player_name ? esc(u.player_name) : '<span class="text-muted">-</span>'}</td>
        <td>${u.active
          ? '<span class="badge bg-success">Active</span>'
          : '<span class="badge bg-secondary">Inactive</span>'}</td>
        <td class="text-muted small">${esc(u.created_at)}</td>
        <td class="text-end">
          <div class="dropdown">
            <button class="btn btn-sm btn-outline-secondary" type="button" data-bs-toggle="dropdown">
              <i class="bi bi-three-dots"></i>
            </button>
            <ul class="dropdown-menu dropdown-menu-end">
              <li><a class="dropdown-item" href="#" data-row-action="setup-token">Issue Password Setup Token</a></li>
              <li><a class="dropdown-item" href="#" data-row-action="revoke-keys">Revoke API Keys</a></li>
              ${u.active
                ? '<li><a class="dropdown-item text-danger" href="#" data-row-action="deactivate">Deactivate</a></li>'
                : '<li><a class="dropdown-item" href="#" data-row-action="reactivate">Reactivate</a></li>'}
            </ul>
          </div>
        </td>
      </tr>`).join('') ||
      '<tr><td colspan="7" class="text-center text-muted py-3">No users yet</td></tr>';
  }

  #ensureProvisionModal() {
    if (document.getElementById('user-provision-modal')) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="user-provision-modal" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Provision Email Login Account</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <p class="small text-muted">
          Creates an account with an email identity but no password. Issue a
          one-time setup token afterward (from this account's row menu) and
          share it with the person out of band so they can choose their own
          password.
        </p>
        <div class="mb-3">
          <label class="form-label">Email *</label>
          <input type="email" class="form-control" id="um-provision-email" autocomplete="off">
        </div>
        <div class="mb-1">
          <label class="form-label">Linked Player (optional)</label>
          <select class="form-select" id="um-provision-player-id">
            <option value="">(none)</option>
          </select>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="um-provision-save-btn">Provision</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  #openProvisionAccount() {
    document.getElementById('um-provision-email').value = '';
    const sorted = [...this.#allPlayers].sort((a, b) => a.name.localeCompare(b.name));
    document.getElementById('um-provision-player-id').innerHTML =
      '<option value="">(none)</option>' +
      sorted.map(p => `<option value="${p.id}">${esc(p.name)}${p.team_name ? ' - ' + esc(p.team_name) : ''}</option>`).join('');
    this.querySelector('.um-new-key-alert')?.classList.add('d-none');
    new bootstrap.Modal(document.getElementById('user-provision-modal')).show();
  }

  async #saveProvisionedAccount() {
    const email = document.getElementById('um-provision-email').value.trim();
    if (!email) { toast('Email is required', 'warning'); return; }
    const body = { email };
    const playerId = parseInt(document.getElementById('um-provision-player-id')?.value, 10);
    if (playerId) body.player_id = playerId;
    try {
      await provisionUser(body);
      bootstrap.Modal.getInstance(document.getElementById('user-provision-modal'))?.hide();
      toast('Account provisioned -- issue a password setup token from its row menu');
      this.#users = await fetchUsers();
      this.#renderList();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  async #issueSetupToken(userId) {
    try {
      const result = await issueSetupToken(userId);
      const alertEl = this.querySelector('.um-new-key-alert');
      if (alertEl) {
        alertEl.classList.remove('d-none');
        alertEl.innerHTML =
          `One-time password setup token (copy it now -- it cannot be shown again; ` +
          `share it out of band):<br><code class="user-select-all">${esc(result.setup_token)}</code>`;
      }
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  async #toggleActive(userId, active) {
    try {
      await (active ? reactivateUser(userId) : deactivateUser(userId));
      toast(active ? 'Account reactivated' : 'Account deactivated -- all sessions and API keys were revoked');
      this.#users = await fetchUsers();
      this.#renderList();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  async #revokeKeys(userId) {
    try {
      await revokeAPIKeys(userId);
      toast('API keys revoked for this account');
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  #openNewUser() {
    document.getElementById('um-username').value = '';
    document.getElementById('um-role').value = 'league_admin';
    this.#toggleLinkedPlayerRow();
    this.querySelector('.um-new-key-alert')?.classList.add('d-none');
    new bootstrap.Modal(document.getElementById(MODAL_ID)).show();
  }

  // Shows/hides and (re)populates the linked-player picker based on the
  // currently-selected role. Only role=player needs a linked player.
  #toggleLinkedPlayerRow() {
    const role = document.getElementById('um-role').value;
    const row  = this.querySelector('.um-linked-player-row');
    const sel  = document.getElementById('um-player-id');
    const isPlayerRole = role === 'player';
    row?.classList.toggle('d-none', !isPlayerRole);
    if (isPlayerRole && sel) {
      const sorted = [...this.#allPlayers].sort((a, b) => a.name.localeCompare(b.name));
      sel.innerHTML = sorted.map(p =>
        `<option value="${p.id}">${esc(p.name)}${p.team_name ? ' - ' + esc(p.team_name) : ''}</option>`
      ).join('') || '<option value="">No players</option>';
    }
  }

  async #saveUser() {
    const username = document.getElementById('um-username').value.trim();
    const role     = document.getElementById('um-role').value;
    if (!username) { toast('Username is required', 'warning'); return; }
    if (!CREATABLE_ROLES.includes(role)) { toast('Select a role', 'warning'); return; }
    const body = { username, role };
    if (role === 'player') {
      const playerId = parseInt(document.getElementById('um-player-id')?.value, 10);
      if (!playerId) { toast('Select a player to link', 'warning'); return; }
      body.player_id = playerId;
    }
    try {
      const result = await createUser(body);
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
