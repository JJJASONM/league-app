// <draft-roster-editor> - add eligible players to a draft-season team roster.
//
// Displays active unrostered players in a select and adds one at a time via
// POST /api/seasons/{id}/teams/{tid}/roster.  Remove buttons live in
// <season-team-detail>; this component emits draft-roster-mutated on add success
// so the coordinator can refresh both the detail and the available-player list.
//
// Public API:
//   load(seasonId, teamId) - fetch available players for the given draft team.
//   clear()                - cancel any in-flight fetch and wipe rendered content.
//
// Emits (bubbling):
//   draft-roster-mutated - detail { seasonId, teamId, message } on successful add.
//
// Stale-response protection: #loadSeq guards the available-player fetch.
// Stale mutation-error suppression: #ctx is incremented by load() and clear();
//   captured as a local constant before the mutation's first await; the catch
//   block only writes to the DOM when this.#ctx === ctx (same pattern as
//   <draft-team-actions>).

class DraftRosterEditor extends HTMLElement {
  #seasonId   = null;
  #teamId     = null;
  #loadSeq    = 0;
  #ctx        = 0;     // context token; incremented on every load() and clear()
  #submitting = false;
  #available  = [];

  connectedCallback() {
    this.addEventListener('click', e => this.#onClick(e));
  }

  load(seasonId, teamId) {
    ++this.#ctx;
    this.#seasonId = seasonId ?? null;
    this.#teamId   = teamId   ?? null;
    this.#fetch();
  }

  clear() {
    ++this.#ctx;
    ++this.#loadSeq;
    this.#seasonId  = null;
    this.#teamId    = null;
    this.#available = [];
    this.innerHTML  = '';
  }

  // -- Fetch ------------------------------------------------------------------

  async #fetch() {
    const seq = ++this.#loadSeq;
    if (!this.#seasonId || !this.#teamId) return;
    this.innerHTML = this.#loadingHTML();
    let players;
    try {
      players = await api('GET', `/seasons/${this.#seasonId}/players/available`);
    } catch (e) {
      if (seq !== this.#loadSeq) return;
      this.innerHTML = this.#errorHTML(e.message);
      return;
    }
    if (seq !== this.#loadSeq) return;
    this.#available = players;
    this.innerHTML  = this.#controlsHTML();
  }

  // -- Rendering --------------------------------------------------------------

  #loadingHTML() {
    return `<div class="text-muted small py-2">
      <span class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>Loading available players...
    </div>`;
  }

  #errorHTML(msg) {
    return `<div class="alert alert-danger py-2 px-3 small mb-0">
      <i class="bi bi-exclamation-triangle me-1" aria-hidden="true"></i>Could not load available players: ${esc(msg)}
    </div>`;
  }

  #controlsHTML() {
    return `
      <div class="card">
        <div class="card-header small fw-semibold py-2">Add Player to Roster</div>
        <div class="card-body py-3">${this.#addFormHTML()}</div>
      </div>`;
  }

  #addFormHTML() {
    if (this.#available.length === 0) {
      return `<p class="text-muted small mb-0">
        <i class="bi bi-info-circle me-1" aria-hidden="true"></i>
        No eligible active players are available for this season.
      </p>`;
    }

    const options = this.#available.map(p => {
      const num  = p.player_number ? `#${esc(p.player_number)} ` : '';
      const team = p.team_name     ? ` - ${esc(p.team_name)}` : '';
      return `<option value="${p.id}">${num}${esc(p.name)} (HC ${p.handicap})${team}</option>`;
    }).join('');

    return `
      <div class="d-flex gap-2 align-items-start flex-wrap">
        <select class="form-select form-select-sm add-player-select"
                aria-label="Select player to add to roster"
                style="max-width:24rem">
          <option value="">-- select player --</option>
          ${options}
        </select>
        <button type="button" class="btn btn-sm btn-outline-primary add-player-btn">Add to Roster</button>
      </div>
      <div class="add-player-error text-danger small mt-1" role="alert"></div>`;
  }

  // -- Mutation ---------------------------------------------------------------

  async #addPlayer() {
    if (this.#submitting) return;
    const sel = this.querySelector('.add-player-select');
    const btn = this.querySelector('.add-player-btn');
    if (!sel) return;
    const playerId = parseInt(sel.value, 10);
    if (!playerId) {
      this.#showError('.add-player-error', 'Select a player to add.');
      return;
    }
    const player = this.#available.find(p => p.id === playerId);

    // Capture originating IDs and context token before any await so navigation
    // cannot change them mid-flight.
    const seasonId = this.#seasonId;
    const teamId   = this.#teamId;
    const ctx      = this.#ctx;

    this.#submitting = true;
    if (btn) btn.disabled = true;
    this.#clearError('.add-player-error');
    try {
      await api('POST', `/seasons/${seasonId}/teams/${teamId}/roster`, { player_id: playerId });
      this.#emit(seasonId, teamId, `${player?.name ?? 'Player'} added to roster`);
    } catch (e) {
      // Only update the DOM when still in the same team context where this
      // mutation began; suppress stale errors if the user navigated away.
      if (this.#ctx === ctx) {
        this.#showError('.add-player-error', e.message);
        if (btn) btn.disabled = false;
      }
    } finally {
      this.#submitting = false;
    }
  }

  #emit(seasonId, teamId, message) {
    this.dispatchEvent(new CustomEvent('draft-roster-mutated', {
      bubbles: true,
      detail: { seasonId, teamId, message },
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

  #onClick(e) {
    if (e.target.closest('.add-player-btn')) this.#addPlayer();
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('draft-roster-editor', DraftRosterEditor);
