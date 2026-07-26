// Logs viewer page

window.LogsPage = {
  refreshTimer: null,

  async render() {
    const page = document.getElementById('page');
    page.innerHTML = '';

    // Header with controls
    const header = Utils.el('div', { className: 'section-header' });
    header.innerHTML = `<h2>Logs</h2>`;
    const controls = Utils.el('div', { style: 'display:flex;gap:8px;align-items:center' });

    const autoRefresh = Utils.el('label', { style: 'display:flex;align-items:center;gap:6px;font-size:0.82rem;color:var(--fg-2);cursor:pointer' });
    autoRefresh.innerHTML = `<input type="checkbox" id="auto-refresh" checked style="accent-color:var(--accent)"> Auto-refresh`;
    controls.appendChild(autoRefresh);

    const refreshBtn = Utils.el('button', { className: 'btn btn-secondary btn-sm', id: 'refresh-logs' });
    refreshBtn.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:14px;height:14px"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg> Refresh`;
    refreshBtn.addEventListener('click', () => this.loadLogs());
    controls.appendChild(refreshBtn);

    header.appendChild(controls);
    page.appendChild(header);

    // Log stream
    const stream = Utils.el('div', { className: 'log-stream', id: 'log-stream' });
    stream.innerHTML = '<div class="loading"><div class="spinner"></div> Loading logs...</div>';
    page.appendChild(stream);

    // Load logs
    await this.loadLogs();

    // Auto refresh
    if (this.refreshTimer) clearInterval(this.refreshTimer);
    document.getElementById('auto-refresh')?.addEventListener('change', (e) => {
      if (e.target.checked) {
        this.refreshTimer = setInterval(() => this.loadLogs(), 3000);
      } else {
        clearInterval(this.refreshTimer);
      }
    });
    this.refreshTimer = setInterval(() => {
      if (document.getElementById('auto-refresh')?.checked) this.loadLogs();
    }, 3000);
  },

  async loadLogs() {
    const stream = document.getElementById('log-stream');
    if (!stream) return;

    try {
      const data = await API.getRecentLogs();
      const entries = data.entries || [];

      if (entries.length === 0) {
        stream.innerHTML = '<div style="color:var(--fg-3);text-align:center;padding:24px;font-size:0.85rem">No logs yet. Activity will appear here.</div>';
        return;
      }

      stream.innerHTML = '';
      entries.forEach(entry => {
        stream.appendChild(Components.logLine(entry));
      });

      // Auto-scroll to bottom
      stream.scrollTop = stream.scrollHeight;
    } catch (err) {
      stream.innerHTML = `<div style="color:var(--danger);padding:16px;font-size:0.85rem">Failed to load logs: ${Utils.escapeHtml(err.message)}</div>`;
    }
  },

  destroy() {
    if (this.refreshTimer) {
      clearInterval(this.refreshTimer);
      this.refreshTimer = null;
    }
  },
};
