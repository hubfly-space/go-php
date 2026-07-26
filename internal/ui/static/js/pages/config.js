// Configuration editor page

window.ConfigPage = {
  async render() {
    const page = document.getElementById('page');
    page.innerHTML = '';
    page.appendChild(Components.loading());

    try {
      const config = await API.getConfig();
      Store.set('config', config);
      page.innerHTML = '';
      this.renderConfig(page, config);
    } catch (err) {
      page.innerHTML = '';
      page.appendChild(Components.emptyState('⚠️', 'Failed to load configuration: ' + err.message));
    }
  },

  renderConfig(page, config) {
    // Header
    const header = Utils.el('div', { className: 'section-header' });
    header.innerHTML = `<h2>Configuration</h2>`;
    const actions = Utils.el('div', { style: 'display:flex;gap:8px' });

    const validateBtn = Utils.el('button', { className: 'btn btn-secondary', id: 'validate-btn' });
    validateBtn.textContent = 'Validate';
    validateBtn.addEventListener('click', () => this.validate());

    const saveBtn = Utils.el('button', { className: 'btn btn-primary', id: 'save-config-btn' });
    saveBtn.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg> Save`;
    saveBtn.addEventListener('click', () => this.save());

    actions.appendChild(validateBtn);
    actions.appendChild(saveBtn);
    header.appendChild(actions);
    page.appendChild(header);

    // Two column: visual editor + raw editor
    const twoCol = Utils.el('div', { className: 'two-col' });

    // Visual editor
    const visualCard = Utils.el('div', { className: 'card' });
    visualCard.innerHTML = `
      <div class="card-header"><h3>Server Settings</h3></div>
      <div class="form-group">
        <label class="form-label">Listen Address</label>
        <input class="form-input" id="cfg-addr" value="${Utils.escapeHtml(config.server?.addr || ':8080')}">
      </div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label">Read Timeout</label>
          <input class="form-input" id="cfg-read-timeout" value="${Utils.escapeHtml(config.server?.read_timeout || '30s')}">
        </div>
        <div class="form-group">
          <label class="form-label">Write Timeout</label>
          <input class="form-input" id="cfg-write-timeout" value="${Utils.escapeHtml(config.server?.write_timeout || '60s')}">
        </div>
      </div>
      <div class="card-header" style="margin-top:16px"><h3>PHP-FPM</h3></div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label">Max Children</label>
          <input class="form-input" type="number" id="cfg-max-children" value="${config.php?.max_children || 20}">
        </div>
        <div class="form-group">
          <label class="form-label">Request Timeout</label>
          <input class="form-input" id="cfg-req-timeout" value="${Utils.escapeHtml(config.php?.request_timeout || '60s')}">
        </div>
      </div>
      <div class="form-group">
        <label class="form-label">Socket Path</label>
        <input class="form-input" id="cfg-socket" value="${Utils.escapeHtml(config.php?.socket_path || '')}" placeholder="/tmp/gateway-fpm.sock">
      </div>
      <div class="card-header" style="margin-top:16px"><h3>Logging</h3></div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label">Format</label>
          <select class="form-input" id="cfg-log-format">
            <option value="json" ${config.logging?.format === 'json' ? 'selected' : ''}>JSON</option>
            <option value="text" ${config.logging?.format === 'text' ? 'selected' : ''}>Text</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Level</label>
          <select class="form-input" id="cfg-log-level">
            <option value="debug" ${config.logging?.level === 'debug' ? 'selected' : ''}>Debug</option>
            <option value="info" ${config.logging?.level === 'info' ? 'selected' : ''}>Info</option>
            <option value="warn" ${config.logging?.level === 'warn' ? 'selected' : ''}>Warn</option>
            <option value="error" ${config.logging?.level === 'error' ? 'selected' : ''}>Error</option>
          </select>
        </div>
      </div>
      <div class="card-header" style="margin-top:16px"><h3>Security</h3></div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label">Symlink Mode</label>
          <select class="form-input" id="cfg-symlink">
            <option value="within_root" ${config.security?.symlink_mode === 'within_root' ? 'selected' : ''}>Within Root</option>
            <option value="deny" ${config.security?.symlink_mode === 'deny' ? 'selected' : ''}>Deny</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Max Body Size</label>
          <input class="form-input" id="cfg-body-size" value="${Utils.escapeHtml(config.security?.max_body_size || '20MB')}">
        </div>
      </div>`;
    twoCol.appendChild(visualCard);

    // Raw JSON editor
    const rawCard = Utils.el('div', { className: 'card' });
    rawCard.innerHTML = `
      <div class="card-header"><h3>Raw Configuration</h3></div>
      <div class="config-editor">
        <textarea id="config-raw" spellcheck="false">${Utils.escapeHtml(JSON.stringify(config, null, 2))}</textarea>
      </div>`;
    twoCol.appendChild(rawCard);

    page.appendChild(twoCol);
  },

  async validate() {
    try {
      const result = await API.validateConfig();
      Components.toast('Configuration is valid', 'success');
    } catch (err) {
      Components.toast('Validation failed: ' + err.message, 'error');
    }
  },

  async save() {
    const raw = document.getElementById('config-raw')?.value;
    let config;
    try {
      config = JSON.parse(raw);
    } catch (e) {
      Components.toast('Invalid JSON: ' + e.message, 'error');
      return;
    }

    try {
      await API.saveConfig(config);
      Store.set('config', config);
      Components.toast('Configuration saved', 'success');
    } catch (err) {
      Components.toast('Failed to save: ' + err.message, 'error');
    }
  },
};
