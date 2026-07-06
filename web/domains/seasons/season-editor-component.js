// <season-editor> — Bootstrap modal Web Component for season create/edit.
//
// Public API:
//   openNew(league, allSeasons)          — open modal to create a new season
//   openEdit(season, league, allSeasons) — open modal to edit an existing season
//
// Fires (bubbles: true):
//   season-editor-saved — { saved, isNew, copyFromSeasonId }

import { createSeason, updateSeason } from './season-api-service.js';

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

class SeasonEditor extends HTMLElement {
  #bsModal    = null;
  #isNew      = true;
  #season     = null;
  #league     = null;
  #allSeasons = [];

  connectedCallback() {
    this.innerHTML = this.#modalHTML();
    this.#bsModal = new bootstrap.Modal(this.querySelector('.sdx-editor-modal'));
    this.querySelector('.sdx-season-save-btn').addEventListener('click', () => this.#onSave());
  }

  openNew(league, allSeasons) {
    this.#isNew      = true;
    this.#season     = null;
    this.#league     = league;
    this.#allSeasons = allSeasons ?? [];
    this.#populate();
    this.#bsModal.show();
  }

  openEdit(season, league, allSeasons) {
    this.#isNew      = false;
    this.#season     = season;
    this.#league     = league;
    this.#allSeasons = allSeasons ?? [];
    this.#populate();
    this.#bsModal.show();
  }

  #modalHTML() {
    return `
    <div class="modal fade sdx-editor-modal" tabindex="-1">
      <div class="modal-dialog modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title sdx-editor-title">Season</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body" style="max-height:75vh;overflow-y:auto">
            <input type="hidden" class="sdx-season-id">
            <div class="row g-3 mb-3">
              <div class="col-md-6">
                <label class="form-label">Season Name *</label>
                <input type="text" class="form-control sdx-season-name" placeholder="e.g. Fall 2026">
              </div>
              <div class="col-md-6">
                <label class="form-label">Start Date <span class="text-muted small">(first match date)</span></label>
                <input type="date" class="form-control sdx-season-start">
              </div>
            </div>
            <div class="row g-3 mb-3">
              <div class="col-md-6">
                <label class="form-label">Schedule Type</label>
                <select class="form-select sdx-season-type">
                  <option value="single_rr">Single Round Robin</option>
                  <option value="double_rr">Double Round Robin</option>
                  <option value="split">Split Season</option>
                  <option value="custom">Custom (Fixed Weeks)</option>
                  <option value="blanket">Blanket (Empty Slots)</option>
                </select>
              </div>
              <div class="col-md-6">
                <label class="form-label">Use Teams From Prior Season</label>
                <select class="form-select sdx-season-copy-from">
                  <option value="">— All teams in league —</option>
                </select>
                <div class="form-text">Pre-selects which season's teams to use when generating the schedule.</div>
              </div>
            </div>
            <div class="text-muted small mb-3">
              <i class="bi bi-info-circle"></i> End date is calculated automatically from the last scheduled match after you generate a schedule.
            </div>
            <div class="sdx-rules-section d-none">
              <hr class="my-2">
              <div class="accordion">
                <div class="accordion-item">
                  <h6 class="accordion-header">
                    <button class="accordion-button collapsed sdx-rules-toggle" type="button"
                      data-bs-toggle="collapse" data-bs-target=".sdx-rules-collapse"
                      aria-expanded="false">
                      <i class="bi bi-sliders me-2"></i> Season Rules (optional)
                    </button>
                  </h6>
                  <div class="sdx-rules-collapse accordion-collapse collapse">
                    <div class="accordion-body">
                      <rules-editor class="sdx-rules-editor"></rules-editor>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
            <button type="button" class="btn btn-primary sdx-season-save-btn">Save &amp; Continue</button>
          </div>
        </div>
      </div>
    </div>`;
  }

  #populate() {
    const s = this.#season;
    const isNew = !s;

    this.querySelector('.sdx-editor-title').textContent = isNew ? 'New Season' : 'Edit Season Details';
    this.querySelector('.sdx-season-id').value        = s?.id ?? '';
    this.querySelector('.sdx-season-name').value      = s?.name ?? '';
    this.querySelector('.sdx-season-start').value     = s?.start_date ? s.start_date.slice(0, 10) : '';
    this.querySelector('.sdx-season-type').value      = s?.schedule_type ?? 'double_rr';

    const sel = this.querySelector('.sdx-season-copy-from');
    sel.innerHTML = '<option value="">— All teams in league —</option>' +
      this.#allSeasons
        .filter(x => !s || x.id !== s.id)
        .map(x => `<option value="${x.id}">${esc(x.name)}</option>`)
        .join('');

    const rulesSection = this.querySelector('.sdx-rules-section');
    const rulesCollapse = this.querySelector('.sdx-rules-collapse');
    const rulesToggle  = this.querySelector('.sdx-rules-toggle');
    const rulesEditor  = this.querySelector('.sdx-rules-editor');
    rulesSection.classList.remove('d-none');

    if (isNew) {
      rulesCollapse.classList.remove('show');
      rulesToggle?.classList.add('collapsed');
      rulesToggle?.setAttribute('aria-expanded', 'false');
      rulesEditor.initNew();
    } else {
      rulesCollapse.classList.add('show');
      rulesToggle?.classList.remove('collapsed');
      rulesToggle?.setAttribute('aria-expanded', 'true');
      rulesEditor.loadSeason(s.id);
    }
  }

  async #onSave() {
    const id       = this.querySelector('.sdx-season-id').value;
    const copyFrom = parseInt(this.querySelector('.sdx-season-copy-from').value) || 0;
    const body = {
      name:          this.querySelector('.sdx-season-name').value.trim(),
      start_date:    this.querySelector('.sdx-season-start').value || null,
      schedule_type: this.querySelector('.sdx-season-type').value || 'double_rr',
      league_id:     this.#league?.id,
    };
    if (!body.name)      { toast('Name is required', 'warning'); return; }
    if (!body.league_id) { toast('No active league selected', 'warning'); return; }

    try {
      let saved;
      if (id) {
        saved = await updateSeason(id, body);
      } else {
        saved = await createSeason(body);
        if (saved?.id) {
          await this.querySelector('.sdx-rules-editor').flushPending(saved.id);
        }
      }
      this.#bsModal.hide();
      toast('Season saved');
      this.dispatchEvent(new CustomEvent('season-editor-saved', {
        bubbles: true,
        detail: { saved, isNew: !id, copyFromSeasonId: copyFrom },
      }));
    } catch (e) {
      toast(e.message, 'danger');
    }
  }
}

customElements.define('season-editor', SeasonEditor);
