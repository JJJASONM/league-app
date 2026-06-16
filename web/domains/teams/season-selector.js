// <season-selector> - league-scoped season picker for the Teams screen.
// Fetches all seasons for a league, defaults to the active season,
// and provides an Active Season button and optional Previous Season shortcut.
//
// Public API:
//   load(leagueId, activeSeasonId) - fetch seasons; reset to the active-season default.
//     Pass activeSeasonId=null when no active season exists.
//     Call again when the league changes to fully reset the component.
//
// Emits:
//   season-changed (bubbling) -- detail { season: Season | null }
//     Fired once after load() completes and again on every user selection.

class SeasonSelector extends HTMLElement {
  #leagueId       = null;
  #activeSeasonId = null;
  #seasons        = [];
  #prevSeason     = null;
  #selected       = null;
  #loadSeq        = 0;

  connectedCallback() {
    this.innerHTML = '';
    this.addEventListener('click',  e => this.#onClick(e));
    this.addEventListener('change', e => this.#onChange(e));
  }

  load(leagueId, activeSeasonId) {
    this.#leagueId       = leagueId       ?? null;
    this.#activeSeasonId = activeSeasonId ?? null;
    this.#seasons        = [];
    this.#prevSeason     = null;
    this.#selected       = null;
    this.#fetch();
  }

  get selectedSeason() { return this.#selected; }

  // -- Async fetch --------------------------------------------------------------

  async #fetch() {
    const seq = ++this.#loadSeq;

    if (!this.#leagueId) {
      this.innerHTML = this.#noLeagueHTML();
      this.#emit();
      return;
    }

    this.innerHTML = this.#loadingHTML();

    let seasons, prevResp;
    try {
      const p1 = api('GET', `/seasons?league_id=${this.#leagueId}`);
      // Tolerate /previous failures (stale or deleted activeSeasonId).
      const p2 = this.#activeSeasonId
        ? api('GET', `/seasons/${this.#activeSeasonId}/previous`).catch(() => null)
        : Promise.resolve(null);
      [seasons, prevResp] = await Promise.all([p1, p2]);
    } catch (e) {
      if (seq !== this.#loadSeq) return;
      this.innerHTML = this.#errorHTML(e.message);
      return;
    }
    if (seq !== this.#loadSeq) return;

    this.#seasons    = seasons;
    this.#prevSeason = prevResp?.season ?? null;
    this.#selected   = seasons.find(s => s.active) ?? null;
    this.#render();
    this.#emit();
  }

  // -- Rendering ----------------------------------------------------------------

  #render() {
    this.innerHTML = this.#seasons.length === 0
      ? this.#noSeasonsHTML()
      : this.#controlsHTML();
  }

  #loadingHTML() {
    return `<span class="text-muted small">
      <span class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>Loading seasons...
    </span>`;
  }

  #errorHTML(msg) {
    return `<span class="text-danger small">
      <i class="bi bi-exclamation-triangle me-1" aria-hidden="true"></i>Could not load seasons: ${esc(msg)}
    </span>`;
  }

  #noLeagueHTML() {
    return `<span class="text-muted small fst-italic">No league selected.</span>`;
  }

  #noSeasonsHTML() {
    return `<span class="text-muted small fst-italic">No seasons for this league.</span>`;
  }

  #controlsHTML() {
    const active         = this.#seasons.find(s => s.active) ?? null;
    const isViewingActive = !!(active && this.#selected?.id === active.id);

    const activeBtn = active
      ? `<button type="button"
                 class="btn btn-sm ${isViewingActive ? 'btn-primary' : 'btn-outline-primary'} season-active-btn"
                 data-season-id="${active.id}"
                 aria-pressed="${isViewingActive}">Active Season</button>`
      : `<button type="button" class="btn btn-sm btn-outline-secondary" disabled aria-disabled="true">No active season</button>`;

    const prevBtn = this.#prevSeason
      ? `<button type="button"
                 class="btn btn-sm btn-outline-secondary season-prev-btn"
                 data-season-id="${this.#prevSeason.id}"
                 aria-label="Select previous season: ${esc(this.#prevSeason.name)}">Previous Season</button>`
      : '';

    const historical = this.#seasons
      .filter(s => !s.active && s.activated_at)
      .sort((a, b) => {
        if (!a.end_date && !b.end_date) return 0;
        if (!a.end_date) return 1;
        if (!b.end_date) return -1;
        return b.end_date.localeCompare(a.end_date);
      });
    const drafts = this.#seasons.filter(s => !s.active && !s.activated_at);

    const activeGroup = active
      ? `<optgroup label="Active"><option value="${active.id}">${esc(active.name)}</option></optgroup>`
      : '';
    const histGroup = historical.length
      ? `<optgroup label="Historical">${historical.map(s =>
          `<option value="${s.id}">${esc(s.name)}</option>`).join('')}</optgroup>`
      : '';
    const draftGroup = drafts.length
      ? `<optgroup label="Draft">${drafts.map(s =>
          `<option value="${s.id}">${esc(s.name)}</option>`).join('')}</optgroup>`
      : '';

    const noSelOpt = !this.#selected
      ? `<option value="" disabled selected>-- No active season --</option>`
      : '';

    return `
      <div class="d-flex align-items-center gap-2 flex-wrap">
        ${activeBtn}${prevBtn}
        <select class="form-select form-select-sm season-select"
                aria-label="Select season"
                style="width:auto;max-width:14rem">
          ${noSelOpt}${activeGroup}${histGroup}${draftGroup}
        </select>
      </div>`;
  }

  // -- Selection ----------------------------------------------------------------

  #selectSeason(season) {
    this.#selected = season;
    this.#syncSelectionDOM();
    this.#emit();
  }

  // Targeted DOM update -- avoids full re-render flicker on selection change.
  #syncSelectionDOM() {
    const sel = this.querySelector('.season-select');
    if (sel) sel.value = this.#selected ? String(this.#selected.id) : '';

    const activeBtn = this.querySelector('.season-active-btn');
    if (activeBtn) {
      const activeSeason = this.#seasons.find(s => s.active);
      const isActive     = !!(activeSeason && this.#selected?.id === activeSeason.id);
      activeBtn.disabled = isActive;
      activeBtn.setAttribute('aria-pressed', String(isActive));
      activeBtn.className = `btn btn-sm ${isActive ? 'btn-primary' : 'btn-outline-primary'} season-active-btn`;
    }
  }

  #emit() {
    this.dispatchEvent(new CustomEvent('season-changed', {
      bubbles: true,
      detail: { season: this.#selected },
    }));
  }

  // -- Interaction --------------------------------------------------------------

  #onClick(e) {
    const btn = e.target.closest('[data-season-id]');
    if (!btn) return;
    const id     = parseInt(btn.dataset.seasonId, 10);
    const season = this.#seasons.find(s => s.id === id)
                ?? (this.#prevSeason?.id === id ? this.#prevSeason : null);
    if (season) this.#selectSeason(season);
  }

  #onChange(e) {
    if (!e.target.matches('.season-select')) return;
    const id     = parseInt(e.target.value, 10);
    const season = this.#seasons.find(s => s.id === id) ?? null;
    if (season) this.#selectSeason(season);
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('season-selector', SeasonSelector);
