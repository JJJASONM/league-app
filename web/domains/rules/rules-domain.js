// <rules-editor> — light DOM Web Component for the Rules tab.
// Public API:
//   loadSeason(seasonId)  — load and edit rules for an existing season
//   initNew()             — render defaults for a season not yet saved;
//                           changes are buffered until flushPending() is called
//   flushPending(seasonId)— save buffered changes after the season is created

const GROUP_ICONS = {
  handicap:   'bi-calculator',
  lineup:     'bi-people',
  scheduling: 'bi-calendar-week',
};

let _definitions = null;

async function fetchDefinitions() {
  if (_definitions) return _definitions;
  try {
    _definitions = await api('GET', '/rules/definitions');
  } catch (_) {
    _definitions = [];
  }
  return _definitions;
}

class RulesEditor extends HTMLElement {
  #seasonId    = null;
  #pendingRules = new Map(); // key → {label, value} — buffered when no season ID yet

  connectedCallback() {
    this.innerHTML = '<div class="text-muted text-center py-4 small">Loading rules…</div>';
    this.addEventListener('change', e => this.#onChange(e));
    this.addEventListener('click',  e => this.#onClick(e));
  }

  // Load and edit rules for an existing season.
  async loadSeason(seasonId) {
    this.#seasonId = seasonId;
    this.#pendingRules.clear();
    this.innerHTML = '<div class="text-muted text-center py-4 small">Loading rules…</div>';
    await this.#render();
  }

  // Render rule defaults for a season that hasn't been saved yet.
  // Rule changes are buffered in #pendingRules until flushPending() is called.
  async initNew() {
    this.#seasonId = null;
    this.#pendingRules.clear();
    this.innerHTML = '<div class="text-muted text-center py-4 small">Loading rules…</div>';
    await this.#render();
  }

  // Save all buffered rule changes to a newly created season, then bind to it.
  async flushPending(seasonId) {
    if (this.#pendingRules.size === 0) {
      this.#seasonId = seasonId;
      return;
    }
    this.#seasonId = seasonId;
    await Promise.all([...this.#pendingRules.entries()].map(([key, v]) =>
      api('POST', `/seasons/${seasonId}/rules`,
        { rule_key: key, rule_label: v.label, rule_value: String(v.value) })
        .catch(() => {})
    ));
    this.#pendingRules.clear();
  }

  // ── Rendering ────────────────────────────────────────────────────────────────

  async #render() {
    const seasonId = this.#seasonId;
    let allRules = [];
    if (seasonId) {
      try { allRules = await api('GET', `/seasons/${seasonId}/rules`); } catch (_) {}
    }
    const defs = await fetchDefinitions();

    // Build ruleMap: stored API values take precedence; pending values override
    // defaults when no season exists yet.
    const ruleMap = {};
    allRules.forEach(r => { ruleMap[r.rule_key] = r; });
    this.#pendingRules.forEach((v, k) => {
      ruleMap[k] = { rule_key: k, rule_label: v.label, rule_value: v.value };
    });

    const knownKeys = new Set(defs.map(d => d.key));
    const customRules = allRules.filter(r => !knownKeys.has(r.rule_key));

    const groupIndex = {};
    for (const def of defs) {
      if (!groupIndex[def.group]) {
        groupIndex[def.group] = {
          group: def.group, group_label: def.group_label,
          group_order: def.group_order, defs: [],
        };
      }
      groupIndex[def.group].defs.push(def);
    }
    const groups = Object.values(groupIndex).sort((a, b) => a.group_order - b.group_order);
    groups.forEach(g => g.defs.sort((a, b) => a.order - b.order));

    let html = '';
    if (!seasonId) {
      html += `<div class="alert alert-info py-2 px-3 mb-2" style="font-size:.82rem">
        <i class="bi bi-info-circle me-1"></i>
        Showing defaults — changes are saved when you create the season.
      </div>`;
    }
    for (const grp of groups) html += this.#renderGroup(grp, ruleMap);
    html += this.#renderCustomSection(customRules);

    this.innerHTML = html;
  }

  #renderGroup(grp, ruleMap) {
    const icon = GROUP_ICONS[grp.group] || 'bi-gear';
    const rows = grp.defs.map(def => {
      const stored  = ruleMap[def.key];
      const curVal  = stored ? stored.rule_value : def.default_value;
      const inputId = `sysrule_${def.key}`;
      let ctrl = '';

      if (def.type === 'choice') {
        const opts = (def.options || []).map(o =>
          `<option value="${o.value}"${curVal === o.value ? ' selected' : ''}>${o.label}</option>`
        ).join('');
        ctrl = `<select class="form-select form-select-sm rule-ctrl" id="${inputId}"
          data-action="save-system"
          data-key="${def.key}"
          data-label="${def.label.replace(/"/g, '&quot;')}">${opts}</select>`;

      } else if (def.type === 'boolean') {
        const chk = curVal === 'true' ? 'checked' : '';
        ctrl = `<div class="form-check form-switch ms-1 mt-1">
          <input class="form-check-input" type="checkbox" id="${inputId}" ${chk}
            data-action="save-system-bool"
            data-key="${def.key}"
            data-label="${def.label.replace(/"/g, '&quot;')}">
          <label class="form-check-label small text-muted" for="${inputId}">
            ${curVal === 'true' ? 'Yes' : 'No'}
          </label>
        </div>`;

      } else {
        const step = def.step != null ? def.step : (def.type === 'integer' ? 1 : 'any');
        const minA = def.minimum != null ? ` min="${def.minimum}"` : '';
        const maxA = def.maximum != null ? ` max="${def.maximum}"` : '';
        ctrl = `<input type="number" class="form-control form-control-sm rule-ctrl"
          id="${inputId}" value="${curVal}" step="${step}"${minA}${maxA}
          data-action="save-system"
          data-key="${def.key}"
          data-label="${def.label.replace(/"/g, '&quot;')}">`;
      }

      return `<tr class="rule-row">
        <td>
          <div class="rule-label">${def.label}</div>
          ${def.help ? `<div class="rule-help">${def.help}</div>` : ''}
        </td>
        <td style="width:220px">${ctrl}</td>
      </tr>`;
    }).join('');

    return `<div class="mb-3">
      <div class="rule-group-hdr">
        <i class="bi ${icon}" aria-hidden="true"></i> ${grp.group_label}
      </div>
      <table class="table table-sm table-borderless mb-0">
        <tbody>${rows}</tbody>
      </table>
    </div>`;
  }

  #renderCustomSection(customRules) {
    // Custom rules require a saved season ID — hide the add form in pending mode.
    if (!this.#seasonId) {
      return `<div class="text-muted small fst-italic py-1">
        <i class="bi bi-info-circle me-1"></i>Custom rules can be added after saving the season.
      </div>`;
    }

    const rows = customRules.length
      ? customRules.map(r => `<tr>
          <td class="small">${r.rule_label}</td>
          <td style="width:130px">
            <input type="text" class="form-control form-control-sm"
              value="${r.rule_value.replace(/"/g, '&quot;')}"
              data-action="update-custom"
              data-rule-id="${r.id}">
          </td>
          <td class="text-end" style="width:40px">
            <button class="btn btn-outline-danger btn-sm py-0"
              data-action="delete-custom"
              data-rule-id="${r.id}">
              <i class="bi bi-trash" aria-hidden="true"></i>
            </button>
          </td>
        </tr>`).join('')
      : '<tr><td colspan="3" class="text-muted text-center py-2 small">No custom rules yet</td></tr>';

    return `<div class="mb-1">
      <div class="rule-group-hdr">
        <i class="bi bi-list-ul" aria-hidden="true"></i> Custom League Rules
        <span class="text-muted fw-normal ms-1" style="font-size:.7rem;text-transform:none;letter-spacing:0">
          (informational only)
        </span>
      </div>
      <div class="d-flex gap-2 mb-2">
        <input type="text" class="form-control form-control-sm" id="rule-label-input"
          placeholder="Rule name (e.g. Break requirements)">
        <input type="text" class="form-control form-control-sm" id="rule-value-input"
          placeholder="Value" style="width:110px">
        <button class="btn btn-primary btn-sm text-nowrap" data-action="add-custom">
          <i class="bi bi-plus-lg" aria-hidden="true"></i> Add
        </button>
      </div>
      <table class="table table-sm mb-0" id="rules-table">
        <thead><tr><th>Rule</th><th>Value</th><th></th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
  }

  // ── Event delegation ─────────────────────────────────────────────────────────

  async #onChange(e) {
    const el = e.target;
    const action = el.dataset.action;
    if (!action) return;

    if (action === 'save-system') {
      await this.#saveSystemRule(el.dataset.key, el.dataset.label, el.value);
    } else if (action === 'save-system-bool') {
      const value = el.checked ? 'true' : 'false';
      await this.#saveSystemRule(el.dataset.key, el.dataset.label, value);
      const lbl = el.nextElementSibling;
      if (lbl) lbl.textContent = el.checked ? 'Yes' : 'No';
    } else if (action === 'update-custom') {
      await this.#updateCustomRule(parseInt(el.dataset.ruleId), el.value);
    }
  }

  async #onClick(e) {
    const btn = e.target.closest('[data-action]');
    if (!btn) return;
    const action = btn.dataset.action;

    if (action === 'delete-custom') {
      await this.#deleteCustomRule(parseInt(btn.dataset.ruleId));
    } else if (action === 'add-custom') {
      await this.#addCustomRule();
    }
  }

  // ── API calls ────────────────────────────────────────────────────────────────

  async #saveSystemRule(key, label, value) {
    if (!this.#seasonId) {
      // No season yet — buffer the change; it will be saved by flushPending().
      this.#pendingRules.set(key, { label, value });
      return;
    }
    try {
      await api('POST', `/seasons/${this.#seasonId}/rules`,
        { rule_key: key, rule_label: label, rule_value: String(value) });
      toast('Saved', 'success');
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #addCustomRule() {
    if (!this.#seasonId) return;
    const labelInput = this.querySelector('#rule-label-input');
    const valueInput = this.querySelector('#rule-value-input');
    const label = labelInput.value.trim();
    const value = valueInput.value.trim();
    if (!label) { toast('Rule description is required', 'warning'); return; }
    const key = 'custom_' + label.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '');
    try {
      await api('POST', `/seasons/${this.#seasonId}/rules`,
        { rule_key: key, rule_label: label, rule_value: value });
      labelInput.value = '';
      valueInput.value = '';
      toast('Rule added');
      await this.#render();
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #updateCustomRule(id, value) {
    try {
      await api('PUT', `/seasons/${this.#seasonId}/rules/${id}`, { rule_value: value });
      toast('Rule updated');
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #deleteCustomRule(id) {
    if (!confirm('Remove this rule?')) return;
    try {
      await api('DELETE', `/seasons/${this.#seasonId}/rules/${id}`);
      toast('Rule removed');
      await this.#render();
    } catch (e) { toast(e.message, 'danger'); }
  }
}

customElements.define('rules-editor', RulesEditor);
