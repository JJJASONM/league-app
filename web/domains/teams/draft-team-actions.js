// <draft-team-actions> - add/copy controls for draft seasons only.
// Fetches the previous season's teams and the current draft-season team list in
// parallel to build filtered copy choices.  Executes mutations directly and
// emits draft-team-mutated on success so the coordinator can refresh dependents.
//
// Public API:
//   load(seasonId) - fetch state for the given draft season and render controls.
//     Call again after any mutation to refresh available copy choices.
//   clear()        - cancel any in-flight fetch and wipe rendered content.
//     Called by <teams-page> when leaving draft mode so stale controls cannot
//     become visible if the user returns to this draft before the load resolves.
//
// Stale mutation-error suppression:
//   #ctx is a context token incremented by every load() and clear() call.
//   Each mutation captures `const ctx = this.#ctx` before its first await.
//   The catch block only writes inline errors or re-enables buttons when
//   this.#ctx === ctx, so a failed request begun in draft A cannot pollute
//   draft B's DOM after the user navigates between drafts mid-flight.

class DraftTeamActions extends HTMLElement {
  #seasonId   = null;
  #prevSeason = null;  // Season | null from /previous
  #prevTeams  = [];    // previousTeamEntry[] filtered to exclude current participants
  #loadSeq    = 0;
  #ctx        = 0;     // context token; incremented on every load() and clear()
  #submitting = false; // one guard for both copy and create; prevents concurrent submissions

  connectedCallback() {
    this.addEventListener('click',   e => this.#onClick(e));
    this.addEventListener('keydown', e => this.#onKeydown(e));
  }

  load(seasonId) {
    ++this.#ctx;
    this.#seasonId = seasonId ?? null;
    this.#fetch();
  }

  // Cancel any in-flight fetch and clear rendered content.
  clear() {
    ++this.#ctx;
    ++this.#loadSeq;
    this.#seasonId   = null;
    this.#prevSeason = null;
    this.#prevTeams  = [];
    this.innerHTML   = '';
  }

  // -- Fetch --------------------------------------------------------------------

  async #fetch() {
    const seq = ++this.#loadSeq;
    if (!this.#seasonId) return;

    this.innerHTML = this.#loadingHTML();

    let prevResp, currTeams;
    try {
      [prevResp, currTeams] = await Promise.all([
        api('GET', `/seasons/${this.#seasonId}/previous`),
        api('GET', `/seasons/${this.#seasonId}/teams`),
      ]);
    } catch (e) {
      if (seq !== this.#loadSeq) return;
      this.innerHTML = this.#errorHTML(e.message);
      return;
    }
    if (seq !== this.#loadSeq) return;

    this.#prevSeason = prevResp.season ?? null;
    const currIds    = new Set(currTeams.map(t => t.team_id));
    this.#prevTeams  = (prevResp.teams ?? []).filter(t => !currIds.has(t.team_id));

    this.innerHTML = this.#controlsHTML();
  }

  // -- Rendering ----------------------------------------------------------------

  #loadingHTML() {
    return `<div class="text-muted small py-2">
      <span class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>Loading...
    </div>`;
  }

  #errorHTML(msg) {
    return `<div class="alert alert-danger py-2 px-3 small mb-0">
      <i class="bi bi-exclamation-triangle me-1" aria-hidden="true"></i>Could not load draft controls: ${esc(msg)}
    </div>`;
  }

  #controlsHTML() {
    return `
      <div class="card mb-3">
        <div class="card-header small fw-semibold py-2">Add Teams to This Draft</div>
        <div class="card-body py-3">
          <div class="row g-4">
            <div class="col-md-6">${this.#copyHTML()}</div>
            <div class="col-md-6">${this.#createHTML()}</div>
          </div>
        </div>
      </div>`;
  }

  #copyHTML() {
    const header = `<div class="small fw-semibold mb-1">Copy Team from Previous Season</div>`;

    if (!this.#prevSeason) {
      return `${header}<p class="text-muted small mb-0">No previous season is available for this draft.</p>`;
    }
    if (this.#prevTeams.length === 0) {
      return `${header}<p class="text-muted small mb-0">
        All teams from <strong>${esc(this.#prevSeason.name)}</strong> are already participating in this draft.</p>`;
    }

    const options = this.#prevTeams
      .map(t => `<option value="${t.team_id}">${esc(t.season_name)}</option>`)
      .join('');
    return `
      ${header}
      <p class="text-muted small mb-2">
        Copies the team and roster from <strong>${esc(this.#prevSeason.name)}</strong>.
        The roster can be edited after adding.
      </p>
      <div class="d-flex gap-2 align-items-start flex-wrap">
        <select class="form-select form-select-sm copy-team-select"
                aria-label="Select team to copy"
                style="max-width:16rem">${options}</select>
        <button type="button" class="btn btn-sm btn-outline-primary copy-team-btn">Copy Team</button>
      </div>
      <div class="copy-team-error text-danger small mt-1" role="alert"></div>`;
  }

  #createHTML() {
    return `
      <div class="small fw-semibold mb-1">Create New Team</div>
      <p class="text-muted small mb-2">
        Creates a new team with an empty roster. Players can be added to the roster later.
      </p>
      <div class="d-flex gap-2 align-items-start flex-wrap">
        <input type="text"
               class="form-control form-control-sm create-team-input"
               placeholder="Team name"
               maxlength="120"
               aria-label="New team name"
               style="max-width:16rem">
        <button type="button" class="btn btn-sm btn-outline-success create-team-btn">Create Team</button>
      </div>
      <div class="create-team-error text-danger small mt-1" role="alert"></div>`;
  }

  // -- Mutations ----------------------------------------------------------------

  async #copyTeam() {
    if (this.#submitting) return;
    const sel = this.querySelector('.copy-team-select');
    const btn = this.querySelector('.copy-team-btn');
    if (!sel) return;
    const teamId = parseInt(sel.value, 10);
    if (!teamId) return;
    const team = this.#prevTeams.find(t => t.team_id === teamId);

    // Capture originating IDs and context token before any await so navigation
    // cannot change them mid-flight.
    const seasonId     = this.#seasonId;
    const fromSeasonId = this.#prevSeason.id;
    const ctx          = this.#ctx;

    this.#submitting = true;
    if (btn) btn.disabled = true;
    this.#clearError('.copy-team-error');
    try {
      await api('POST', `/seasons/${seasonId}/teams`, {
        from_team_id:   teamId,
        from_season_id: fromSeasonId,
      });
      this.#emit(seasonId, `${team?.season_name ?? 'Team'} added to draft`);
    } catch (e) {
      // Only update the DOM when still in the same season context where this
      // mutation began; suppress stale errors if the user navigated away.
      if (this.#ctx === ctx) {
        this.#showError('.copy-team-error', e.message);
        if (btn) btn.disabled = false;
      }
    } finally {
      this.#submitting = false;
    }
  }

  async #createTeam() {
    if (this.#submitting) return;
    const input = this.querySelector('.create-team-input');
    const btn   = this.querySelector('.create-team-btn');
    if (!input) return;
    const name = input.value.trim();
    if (!name) {
      this.#showError('.create-team-error', 'Team name is required.');
      return;
    }

    // Capture originating ID and context token before any await so navigation
    // cannot change them mid-flight.
    const seasonId = this.#seasonId;
    const ctx      = this.#ctx;

    this.#submitting = true;
    if (btn) btn.disabled = true;
    this.#clearError('.create-team-error');
    try {
      await api('POST', `/seasons/${seasonId}/teams`, { name });
      this.#emit(seasonId, `${name} added to draft`);
    } catch (e) {
      // Only update the DOM when still in the same season context where this
      // mutation began; suppress stale errors if the user navigated away.
      if (this.#ctx === ctx) {
        this.#showError('.create-team-error', e.message);
        if (btn) btn.disabled = false;
      }
    } finally {
      this.#submitting = false;
    }
  }

  #emit(seasonId, message) {
    this.dispatchEvent(new CustomEvent('draft-team-mutated', {
      bubbles: true,
      detail: { seasonId, message },
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

  // -- Interaction --------------------------------------------------------------

  #onClick(e) {
    if (e.target.closest('.copy-team-btn'))   this.#copyTeam();
    if (e.target.closest('.create-team-btn')) this.#createTeam();
  }

  #onKeydown(e) {
    if (e.key === 'Enter' && e.target.matches('.create-team-input')) {
      this.#createTeam();
    }
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('draft-team-actions', DraftTeamActions);
