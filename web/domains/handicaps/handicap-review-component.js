// <handicap-review> — light DOM Web Component for the Handicap Review tab.
//
// Public API (called by app.js shell via DOM reference):
//   loadSeason(seasonId)  — fetch and render recommendations for the given season
//   reset()               — clear content (no season selected)
//
// Token handling:
//   Bearer token is stored in the #adminToken private field (session memory only).
//   It is never written to localStorage, sessionStorage, cookies, or URL params.
//   Cleared on 401 or 403. Lost on page reload.
//
// Apply flow:
//   1. Admin checks rows (or uses Select All)
//   2. Clicks "Apply Changes" in the apply bar
//   3. If no token: token-entry modal → store token in #adminToken
//   4. Confirmation modal lists exact changes
//   5. Admin clicks "Apply N Changes" → POST handicap-apply → reload on success

import {
  escapeHTML,
  fmtHC,
  isSelectableRec,
  buildApplyEntries,
  makeApplyRequestId,
  describeConflict,
  describeRejection,
  fetchRecommendations,
  applyHandicaps,
} from './handicap-api-service.js';

class HandicapReview extends HTMLElement {
  #seasonId   = null;
  #data       = null;    // HandicapReviewResponse from last successful fetch
  #selected   = new Set(); // Set<player_id> — selectable rows checked by admin
  #adminToken = null;    // session memory only — cleared on reload, 401, 403
  #loadedAt   = null;    // Date of last successful fetch (for "Last loaded" display)

  connectedCallback() {
    this.innerHTML = `
      <div class="hc-main-content"></div>
      ${this.#tokenModalHtml()}
      ${this.#confirmModalHtml()}
    `;
    this._contentEl = this.querySelector('.hc-main-content');

    // Bootstrap Modal instances — persistent across content re-renders
    this._tokenModal   = new bootstrap.Modal(this.querySelector('.hc-token-modal'));
    this._confirmModal = new bootstrap.Modal(this.querySelector('.hc-confirm-modal'));

    // Token modal: "Set Token" button and Enter key
    const tokenInput  = this.querySelector('.hc-token-input');
    const tokenSubmit = this.querySelector('.hc-token-submit-btn');
    tokenSubmit.addEventListener('click', () => this.#onTokenSubmit());
    tokenInput.addEventListener('keydown', e => {
      if (e.key === 'Enter') this.#onTokenSubmit();
    });

    // Confirm modal: "Apply N Changes" button
    this.querySelector('.hc-confirm-apply-btn').addEventListener('click', () => this.#onConfirmApply());

    // Event delegation on the content area (table checkboxes, apply bar buttons)
    this._contentEl.addEventListener('click',  e => this.#onClickEvt(e));
    this._contentEl.addEventListener('change', e => this.#onChangeEvt(e));

    this._contentEl.innerHTML = '<p class="text-muted text-center py-4 small">Select a season to review handicaps.</p>';
  }

  disconnectedCallback() {
    this._tokenModal?.dispose();
    this._confirmModal?.dispose();
  }

  // ─── Public API ───────────────────────────────────────────────────────────────

  async loadSeason(seasonId) {
    this.#seasonId = String(seasonId);
    this.#selected.clear();
    this.#data = null;
    await this.#load();
  }

  reset() {
    this.#seasonId = null;
    this.#data = null;
    this.#selected.clear();
    if (this._contentEl) this._contentEl.innerHTML = '';
  }

  // ─── Loading ─────────────────────────────────────────────────────────────────

  async #load() {
    const sid = this.#seasonId;
    if (!sid) { this.reset(); return; }

    this._contentEl.innerHTML = '<p class="text-muted text-center py-4 small"><span class="spinner-border spinner-border-sm me-2" role="status"></span>Loading…</p>';
    try {
      const data = await fetchRecommendations(sid);
      if (this.#seasonId !== sid) return; // season changed while fetching
      this.#data    = data;
      this.#loadedAt = new Date();
      this.#selected.clear();
      this.#render();
    } catch (e) {
      if (this.#seasonId !== sid) return;
      this._contentEl.innerHTML = `<div class="alert alert-danger small">Failed to load recommendations: ${escapeHTML(e.message)}</div>`;
    }
  }

  // ─── Rendering ───────────────────────────────────────────────────────────────

  #render() {
    const data  = this.#data;
    const recs  = data?.recommendations ?? [];
    const hasActionable = recs.some(isSelectableRec);

    let html = '';

    // Status/message alert
    const wcNote = data.weeks_closed > 0
      ? `<span class="text-muted ms-2">Based on ${data.weeks_closed} closed week${data.weeks_closed !== 1 ? 's' : ''}</span>`
      : '';
    html += `<div class="alert alert-info py-2 mb-3 small">
      <i class="bi bi-info-circle me-1"></i>${escapeHTML(data.message || '')}${wcNote}
    </div>`;

    // Apply bar — shown only when there are selectable rows with changes
    if (hasActionable) {
      html += this.#applyBarHtml();
    }

    // Alert area — always present so #showAlert can insert into it after reload
    html += '<div class="hc-alert-area mb-2"></div>';

    if (recs.length === 0) {
      this._contentEl.innerHTML = html;
      return;
    }

    if (!hasActionable) {
      html += `<p class="small text-muted mb-2">
        <i class="bi bi-info-circle me-1"></i>No actionable changes — all players are at recommended handicap or have insufficient data.
      </p>`;
    }

    html += this.#tableHtml(recs, hasActionable);

    this._contentEl.innerHTML = html;
    if (hasActionable) this.#updateApplyBar();
  }

  #applyBarHtml() {
    const ts = this.#loadedAt
      ? this.#loadedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      : '';
    return `<div class="hc-apply-bar d-flex align-items-center flex-wrap gap-2 mb-3 px-3 py-2 border rounded bg-light">
      <span class="hc-selected-count small fw-semibold text-muted">0 selected</span>
      <span class="ms-auto"></span>
      ${ts ? `<span class="small text-muted">Loaded&nbsp;${escapeHTML(ts)}</span>` : ''}
      <button type="button" class="btn btn-outline-secondary btn-sm hc-reload-btn" title="Reload recommendations">
        <i class="bi bi-arrow-clockwise" aria-hidden="true"></i>
      </button>
      <button type="button" class="btn btn-primary btn-sm hc-apply-btn" disabled>
        <i class="bi bi-check2-circle me-1" aria-hidden="true"></i>Apply Changes
      </button>
    </div>`;
  }

  #tableHtml(recs, hasActionable) {
    const windowSize = recs[0]?.window_size ?? 15;
    const rows = recs.map(r => this.#rowHtml(r, hasActionable)).join('');

    const cbTh = hasActionable
      ? `<th class="hc-cb-col text-center"><input type="checkbox" class="form-check-input hc-select-all" title="Select all with changes"></th>`
      : `<th class="hc-cb-col"></th>`;

    return `<div class="card"><div class="card-body p-0">
      <table class="table table-hover table-sm mb-0" id="handicap-review-table">
        <thead><tr>
          ${cbTh}
          <th class="text-muted fw-normal small">Team</th>
          <th class="text-muted fw-normal small">Player</th>
          <th class="text-muted fw-normal small text-end">Assigned</th>
          <th class="text-muted fw-normal small text-end">Lifetime HC</th>
          <th class="text-muted fw-normal small text-end">Window HC (W=${windowSize})</th>
          <th class="text-muted fw-normal small text-end">Recommended</th>
          <th class="text-muted fw-normal small text-end">Change</th>
          <th class="text-muted fw-normal small text-center">Excl.</th>
          <th class="text-muted fw-normal small">Status</th>
        </tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div></div>`;
  }

  #rowHtml(r, hasActionable) {
    const selectable = isSelectableRec(r);
    const nonActionable = r.recommended_hc == null;

    // Lifetime HC cell
    let lifetimeCell;
    if (r.lifetime_hc == null) {
      lifetimeCell = '<span class="text-muted">--</span>';
    } else {
      lifetimeCell = `${escapeHTML(fmtHC(r.lifetime_hc))}<br><span class="text-muted" style="font-size:0.75em">(${r.lifetime_racks} racks)</span>`;
    }

    // Window HC cell
    let windowCell;
    if (r.window_hc == null) {
      windowCell = '<span class="text-muted">--</span>';
    } else {
      windowCell = `${escapeHTML(fmtHC(r.window_hc))}<br><span class="text-muted" style="font-size:0.75em">(${r.window_racks}/${r.window_size})</span>`;
    }

    // Recommended HC and Change cells
    let recCell, changeCell;
    if (nonActionable) {
      recCell    = '<span class="text-muted">N/A</span>';
      changeCell = '<span class="text-muted">--</span>';
    } else {
      recCell = escapeHTML(fmtHC(r.recommended_hc));
      const chg = r.change_amount;
      if (!chg || chg === 0) {
        changeCell = '<span class="text-muted">0</span>';
      } else {
        const cls = chg > 0 ? 'text-success' : 'text-danger';
        changeCell = `<span class="${cls}">${chg > 0 ? '+' : ''}${escapeHTML(String(chg))}</span>`;
      }
    }

    // Missing snapshots cell
    const missing = r.missing_snapshot_racks || 0;
    const missingCell = missing > 0
      ? `<span class="badge bg-warning text-dark">${missing}</span>`
      : '<span class="text-muted">--</span>';

    // Status/reason cell
    let noteCell;
    if      (r.reason === 'admin_hold')      noteCell = '<span class="badge bg-secondary">Admin Hold</span>';
    else if (r.reason === 'no_data')         noteCell = '<span class="text-muted fst-italic">No data</span>';
    else if (r.reason === 'below_threshold') noteCell = `<span class="text-muted fst-italic">Below threshold (${r.eligibility_threshold})</span>`;
    else if (r.reason === 'no_change')       noteCell = '<span class="text-muted fst-italic">No change</span>';
    else if (r.reason === 'capped')          noteCell = '<span class="badge bg-warning text-dark">Capped</span>';
    else                                     noteCell = '';

    const trClass = nonActionable ? ' class="text-muted"' : '';
    const isChecked = this.#selected.has(r.player_id);

    let cbCell;
    if (hasActionable && selectable) {
      cbCell = `<td class="hc-cb-col text-center">
        <input type="checkbox" class="form-check-input hc-row-cb"
          data-player-id="${r.player_id}"${isChecked ? ' checked' : ''}>
      </td>`;
    } else {
      cbCell = '<td class="hc-cb-col"></td>';
    }

    return `<tr${trClass}>
      ${cbCell}
      <td class="text-muted small">${escapeHTML(r.team_name || '')}</td>
      <td>${escapeHTML(r.player_name)}</td>
      <td class="text-end">${escapeHTML(fmtHC(r.assigned_hc))}</td>
      <td class="text-end">${lifetimeCell}</td>
      <td class="text-end">${windowCell}</td>
      <td class="text-end">${recCell}</td>
      <td class="text-end">${changeCell}</td>
      <td class="text-center">${missingCell}</td>
      <td>${noteCell}</td>
    </tr>`;
  }

  #updateApplyBar() {
    const countEl    = this._contentEl.querySelector('.hc-selected-count');
    const applyBtn   = this._contentEl.querySelector('.hc-apply-btn');
    const selectAllCb = this._contentEl.querySelector('.hc-select-all');
    if (!countEl || !applyBtn) return;

    const n = this.#selected.size;
    countEl.textContent = `${n} player${n !== 1 ? 's' : ''} selected`;
    applyBtn.disabled = (n === 0);

    if (selectAllCb && this.#data) {
      const allSelectable = this.#data.recommendations.filter(isSelectableRec);
      const allChecked  = allSelectable.length > 0 && allSelectable.every(r => this.#selected.has(r.player_id));
      const someChecked = allSelectable.some(r => this.#selected.has(r.player_id));
      selectAllCb.checked       = allChecked;
      selectAllCb.indeterminate = someChecked && !allChecked;
    }
  }

  // ─── Event delegation ─────────────────────────────────────────────────────────

  #onChangeEvt(e) {
    const el = e.target;

    if (el.classList.contains('hc-row-cb')) {
      const pid = parseInt(el.dataset.playerId, 10);
      if (el.checked) this.#selected.add(pid);
      else            this.#selected.delete(pid);
      this.#updateApplyBar();

    } else if (el.classList.contains('hc-select-all')) {
      const recs = this.#data?.recommendations ?? [];
      const allSelectable = recs.filter(isSelectableRec);
      if (el.checked) allSelectable.forEach(r => this.#selected.add(r.player_id));
      else            this.#selected.clear();

      // Refresh tbody checkboxes to reflect updated selection state
      const tbody = this._contentEl.querySelector('#handicap-review-table tbody');
      if (tbody) tbody.innerHTML = recs.map(r => this.#rowHtml(r, true)).join('');
      this.#updateApplyBar();
    }
  }

  #onClickEvt(e) {
    const btn = e.target.closest('button');
    if (!btn) return;
    if (btn.classList.contains('hc-apply-btn'))  this.#onApplyClick();
    if (btn.classList.contains('hc-reload-btn')) this.#load();
  }

  // ─── Apply flow ───────────────────────────────────────────────────────────────

  #onApplyClick() {
    if (this.#selected.size === 0) return;
    if (!this.#adminToken) {
      this.querySelector('.hc-token-input').value = '';
      this.querySelector('.hc-token-input').classList.remove('is-invalid');
      this._tokenModal.show();
    } else {
      this.#showConfirmModal();
    }
  }

  #onTokenSubmit() {
    const input = this.querySelector('.hc-token-input');
    const val   = input.value.trim();
    if (!val) {
      input.classList.add('is-invalid');
      return;
    }
    input.classList.remove('is-invalid');
    this.#adminToken = val;
    this._tokenModal.hide();
    this.#showConfirmModal();
  }

  #showConfirmModal() {
    const recs      = this.#data?.recommendations ?? [];
    const selected  = recs.filter(r => this.#selected.has(r.player_id) && isSelectableRec(r));
    if (selected.length === 0) return;

    const n = selected.length;
    this.querySelector('.hc-confirm-title').textContent = `Apply Handicap Changes (${n})`;
    this.querySelector('.hc-confirm-apply-btn').textContent = `Apply ${n} Change${n !== 1 ? 's' : ''}`;
    this.querySelector('.hc-confirm-apply-btn').disabled = false;
    this.querySelector('.hc-confirm-cancel-btn').disabled = false;

    const rows = selected.map(r => {
      const chg    = r.change_amount;
      const chgStr = chg > 0 ? `+${chg}` : String(chg);
      const chgCls = chg > 0 ? 'text-success' : 'text-danger';
      return `<tr>
        <td class="small text-muted">${escapeHTML(r.team_name || '')}</td>
        <td class="small fw-semibold">${escapeHTML(r.player_name)}</td>
        <td class="small text-end">${escapeHTML(fmtHC(r.assigned_hc))}</td>
        <td class="small text-end fw-semibold">${escapeHTML(fmtHC(r.recommended_hc))}</td>
        <td class="small text-end ${chgCls}">${escapeHTML(chgStr)}</td>
      </tr>`;
    }).join('');

    this.querySelector('.hc-confirm-tbody').innerHTML = rows;
    this._confirmModal.show();
  }

  async #onConfirmApply() {
    const recs        = this.#data?.recommendations ?? [];
    const selectedRecs = recs.filter(r => this.#selected.has(r.player_id) && isSelectableRec(r));
    if (selectedRecs.length === 0) return;

    const confirmBtn = this.querySelector('.hc-confirm-apply-btn');
    const cancelBtn  = this.querySelector('.hc-confirm-cancel-btn');
    confirmBtn.disabled = true;
    cancelBtn.disabled  = true;

    const body = {
      apply_request_id: makeApplyRequestId(),
      entries:          buildApplyEntries(selectedRecs),
    };

    try {
      const result = await applyHandicaps(this.#seasonId, this.#adminToken, body);
      this._confirmModal.hide();
      this.#selected.clear();

      // Reload first so #showAlert finds a fresh hc-alert-area in the new render
      await this.#load();

      if (result.replayed) {
        this.#showAlert('info', 'This request was already applied — no new changes were made.');
      } else {
        const n = result.applied?.length ?? 0;
        this.#showAlert('success', `Applied ${n} handicap change${n !== 1 ? 's' : ''} successfully.`);
      }
    } catch (err) {
      this._confirmModal.hide();
      this.#handleApplyError(err);
    }
  }

  #handleApplyError(err) {
    const status  = err.status;
    const payload = err.payload;
    const recs    = this.#data?.recommendations ?? [];
    const recMap  = Object.fromEntries(recs.map(r => [r.player_id, r]));

    if (status === 401 || status === 403) {
      this.#adminToken = null;
      this.#showAlert('danger',
        'Authorization failed — verify your admin token. You will be prompted again on the next apply attempt.');

    } else if (status === 404) {
      this.#showAlert('warning', 'Apply is not available on this server.');

    } else if (status === 409) {
      const conflicts = payload?.conflicts ?? [];
      const items = conflicts.map(c =>
        `<li>${escapeHTML(describeConflict(c, recMap))}</li>`).join('');
      this.#showAlert('warning',
        `Stale recommendations detected for ${conflicts.length} player${conflicts.length !== 1 ? 's' : ''}. ` +
        `Reload and re-review before retrying.<ul class="mb-0 mt-1 ps-3">${items}</ul>`,
        true);

    } else if (status === 422) {
      const rejections = payload?.rejections ?? [];
      const items = rejections.map(rj =>
        `<li>${escapeHTML(describeRejection(rj, recMap))}</li>`).join('');
      this.#showAlert('warning',
        `${rejections.length} player${rejections.length !== 1 ? 's were' : ' was'} not eligible for apply.` +
        `<ul class="mb-0 mt-1 ps-3">${items}</ul>`,
        true);

    } else if (status === 400) {
      this.#showAlert('danger', `Invalid request: ${escapeHTML(err.message)}.`);

    } else {
      this.#showAlert('danger', 'Server error — no handicaps were changed. Please try again.');
    }
  }

  // withReload=true adds a "Reload Recommendations" button inside the alert.
  #showAlert(type, message, withReload = false) {
    const alertArea = this._contentEl.querySelector('.hc-alert-area');
    if (!alertArea) return;
    const reloadHtml = withReload
      ? `<div class="mt-2"><button type="button" class="btn btn-sm btn-outline-secondary hc-reload-btn">
           <i class="bi bi-arrow-clockwise me-1" aria-hidden="true"></i>Reload Recommendations
         </button></div>`
      : '';
    alertArea.innerHTML = `<div class="alert alert-${type} py-2 small" role="alert">${message}${reloadHtml}</div>`;
  }

  // ─── Modal HTML (rendered once in connectedCallback) ─────────────────────────

  #tokenModalHtml() {
    return `<div class="modal fade hc-token-modal" tabindex="-1"
        aria-labelledby="hc-token-modal-label" aria-hidden="true">
      <div class="modal-dialog modal-sm modal-dialog-centered">
        <div class="modal-content">
          <div class="modal-header py-2">
            <h6 class="modal-title" id="hc-token-modal-label">
              <i class="bi bi-key me-1" aria-hidden="true"></i>Admin Token Required
            </h6>
            <button type="button" class="btn-close btn-sm"
              data-bs-dismiss="modal" aria-label="Close"></button>
          </div>
          <div class="modal-body">
            <label for="hc-token-field" class="form-label small mb-1">Enter admin token</label>
            <input type="password" class="form-control form-control-sm hc-token-input"
              id="hc-token-field" autocomplete="off" placeholder="Bearer token…">
            <div class="invalid-feedback">Token is required.</div>
          </div>
          <div class="modal-footer py-2">
            <button type="button" class="btn btn-secondary btn-sm"
              data-bs-dismiss="modal">Cancel</button>
            <button type="button" class="btn btn-primary btn-sm hc-token-submit-btn">
              Set Token
            </button>
          </div>
        </div>
      </div>
    </div>`;
  }

  #confirmModalHtml() {
    return `<div class="modal fade hc-confirm-modal" tabindex="-1"
        aria-labelledby="hc-confirm-modal-label" aria-hidden="true">
      <div class="modal-dialog modal-dialog-centered">
        <div class="modal-content">
          <div class="modal-header py-2">
            <h6 class="modal-title hc-confirm-title" id="hc-confirm-modal-label">
              Apply Handicap Changes
            </h6>
            <button type="button" class="btn-close btn-sm"
              data-bs-dismiss="modal" aria-label="Close"></button>
          </div>
          <div class="modal-body">
            <p class="small text-muted mb-2">The following handicap changes will be applied:</p>
            <table class="table table-sm mb-0">
              <thead>
                <tr>
                  <th class="fw-normal small text-muted">Team</th>
                  <th class="fw-normal small text-muted">Player</th>
                  <th class="fw-normal small text-muted text-end">Current</th>
                  <th class="fw-normal small text-muted text-end">New</th>
                  <th class="fw-normal small text-muted text-end">Delta</th>
                </tr>
              </thead>
              <tbody class="hc-confirm-tbody"></tbody>
            </table>
          </div>
          <div class="modal-footer py-2">
            <button type="button" class="btn btn-secondary btn-sm hc-confirm-cancel-btn"
              data-bs-dismiss="modal">Cancel</button>
            <button type="button" class="btn btn-primary btn-sm hc-confirm-apply-btn">
              Apply Changes
            </button>
          </div>
        </div>
      </div>
    </div>`;
  }
}

customElements.define('handicap-review', HandicapReview);
