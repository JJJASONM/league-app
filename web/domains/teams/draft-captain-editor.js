// <draft-captain-editor> - assign or clear the captain for a draft-season team.
//
// Fetches the team's season roster to populate captain choices.  Only rostered
// players are presented; backend validation remains authoritative.
//
// Public API:
//   load(seasonId, teamId, team) - fetch roster and render captain controls.
//     team is the SeasonTeam object; its season_name is forwarded in the PUT body
//     so the field is never inadvertently blanked.
//   clear()                      - cancel any in-flight fetch and wipe content.
//
// Emits (bubbling):
//   draft-captain-mutated - detail { seasonId, teamId, updatedTeam, message }
//     Fired on successful PUT.  updatedTeam is the SeasonTeam returned by the API
//     so the coordinator can update #selectedTeam and re-render dependents.
//
// Stale-response protection: #loadSeq guards the roster fetch.
// Stale mutation-error suppression: #ctx is incremented by load() and clear();
//   captured before the mutation await; catch block writes to the DOM only when
//   this.#ctx === ctx (same pattern as <draft-team-actions>).
// Unchanged guard: the Save button starts disabled and re-enables only when the
//   select value differs from the current captain_id.

class DraftCaptainEditor extends HTMLElement {
  #seasonId   = null;
  #teamId     = null;
  #team       = null;   // SeasonTeam -- carries season_name for the PUT body
  #loadSeq    = 0;
  #ctx        = 0;      // context token; incremented on every load() and clear()
  #submitting = false;
  #roster     = [];

  connectedCallback() {
    this.addEventListener('change', e => this.#onChange(e));
    this.addEventListener('click',  e => this.#onClick(e));
  }

  load(seasonId, teamId, team) {
    ++this.#ctx;
    this.#seasonId = seasonId ?? null;
    this.#teamId   = teamId   ?? null;
    this.#team     = team     ?? null;
    this.#fetch();
  }

  clear() {
    ++this.#ctx;
    ++this.#loadSeq;
    this.#seasonId = null;
    this.#teamId   = null;
    this.#team     = null;
    this.#roster   = [];
    this.innerHTML = '';
  }

  // -- Fetch ------------------------------------------------------------------

  async #fetch() {
    const seq = ++this.#loadSeq;
    if (!this.#seasonId || !this.#teamId) return;
    this.innerHTML = this.#loadingHTML();
    let roster;
    try {
      roster = await api('GET', `/seasons/${this.#seasonId}/teams/${this.#teamId}/roster`);
    } catch (e) {
      if (seq !== this.#loadSeq) return;
      this.innerHTML = this.#errorHTML(e.message);
      return;
    }
    if (seq !== this.#loadSeq) return;
    this.#roster   = roster;
    this.innerHTML = this.#controlsHTML();
  }

  // -- Rendering --------------------------------------------------------------

  #loadingHTML() {
    return `<div class="text-muted small py-2">
      <span class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>Loading captain options...
    </div>`;
  }

  #errorHTML(msg) {
    return `<div class="alert alert-danger py-2 px-3 small mb-0">
      <i class="bi bi-exclamation-triangle me-1" aria-hidden="true"></i>Could not load captain options: ${esc(msg)}
    </div>`;
  }

  #controlsHTML() {
    return `
      <div class="card">
        <div class="card-header small fw-semibold py-2">Captain Assignment</div>
        <div class="card-body py-3">${this.#formHTML()}</div>
      </div>`;
  }

  #formHTML() {
    if (this.#roster.length === 0) {
      return `<p class="text-muted small mb-0">
        <i class="bi bi-info-circle me-1" aria-hidden="true"></i>
        Add at least one player to the roster before assigning a captain.
      </p>`;
    }

    const currentId  = this.#team?.captain_id ?? null;
    const currentVal = currentId !== null ? String(currentId) : '';

    const options = this.#roster.map(p => {
      const num      = p.player_number ? `#${esc(p.player_number)} ` : '';
      const selected = p.player_id === currentId ? ' selected' : '';
      return `<option value="${p.player_id}"${selected}>${num}${esc(p.player_name)}</option>`;
    }).join('');

    return `
      <div class="d-flex gap-2 align-items-start flex-wrap">
        <select class="form-select form-select-sm captain-select"
                aria-label="Select captain"
                style="max-width:20rem">
          <option value=""${currentVal === '' ? ' selected' : ''}>-- No captain --</option>
          ${options}
        </select>
        <button type="button"
                class="btn btn-sm btn-outline-secondary captain-save-btn"
                disabled>Save Captain</button>
      </div>
      <div class="captain-error text-danger small mt-1" role="alert"></div>`;
  }

  // -- Mutation ---------------------------------------------------------------

  async #saveCaptain() {
    if (this.#submitting) return;
    const sel = this.querySelector('.captain-select');
    const btn = this.querySelector('.captain-save-btn');
    if (!sel) return;

    const newCaptainId = sel.value ? parseInt(sel.value, 10) : null;
    const currentId    = this.#team?.captain_id ?? null;
    if (newCaptainId === currentId) return; // unchanged -- Save should be disabled

    // Capture originating IDs and context token before any await so navigation
    // cannot change them mid-flight.
    const seasonId   = this.#seasonId;
    const teamId     = this.#teamId;
    const seasonName = this.#team?.season_name ?? '';
    const ctx        = this.#ctx;

    this.#submitting = true;
    if (btn) btn.disabled = true;
    this.#clearError('.captain-error');
    try {
      const updated = await api('PUT', `/seasons/${seasonId}/teams/${teamId}`, {
        season_name: seasonName,
        captain_id:  newCaptainId,
      });
      const msg = newCaptainId
        ? `${updated.captain_name ?? 'Player'} assigned as captain`
        : 'Captain cleared';
      this.#emit(seasonId, teamId, updated, msg);
    } catch (e) {
      // Only update the DOM when still in the same team context where this
      // mutation began; suppress stale errors if the user navigated away.
      if (this.#ctx === ctx) {
        this.#showError('.captain-error', e.message);
        if (btn) btn.disabled = false;
      }
    } finally {
      this.#submitting = false;
    }
  }

  #emit(seasonId, teamId, updatedTeam, message) {
    this.dispatchEvent(new CustomEvent('draft-captain-mutated', {
      bubbles: true,
      detail: { seasonId, teamId, updatedTeam, message },
    }));
  }

  #showError(selector, msg) {
    const el = this.querySelector(selector);
    if (el) el.textContent = msg;
  }

  #clearError(selector) {
    const el = this.querySelector(selector);
    if (el) el.textContent = '';
  }

  // -- Interaction ------------------------------------------------------------

  // Enable Save only when the select value differs from the current captain.
  #onChange(e) {
    if (!e.target.matches('.captain-select')) return;
    const btn = this.querySelector('.captain-save-btn');
    if (!btn) return;
    const currentVal = this.#team?.captain_id != null ? String(this.#team.captain_id) : '';
    btn.disabled = e.target.value === currentVal;
  }

  #onClick(e) {
    if (e.target.closest('.captain-save-btn')) this.#saveCaptain();
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('draft-captain-editor', DraftCaptainEditor);
