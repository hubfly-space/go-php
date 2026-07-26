// Dashboard page

window.DashboardPage = {
  async render() {
    const page = document.getElementById('page');
    page.innerHTML = '';
    page.appendChild(Components.loading());

    try {
      const [statusData, systemData] = await Promise.all([
        API.getStatus(),
        API.getSystem(),
      ]);

      Store.set('status', statusData);
      Store.set('system', systemData);

      page.innerHTML = '';
      this.renderDashboard(page, statusData, systemData);
    } catch (err) {
      page.innerHTML = '';
      page.appendChild(Components.emptyState('⚠️', 'Failed to load dashboard: ' + err.message));
    }
  },

  renderDashboard(page, status, system) {
    // Stats grid
    const stats = Utils.el('div', { className: 'stat-grid' });
    stats.appendChild(Components.statCard('⚡', status.uptime || '0m', 'Uptime', 'blue'));
    stats.appendChild(Components.statCard('📊', Utils.formatNumber(status.total_requests || 0), 'Total Requests', 'green'));
    stats.appendChild(Components.statCard('🔴', Utils.formatNumber(status.total_errors || 0), 'Errors', 'red'));
    stats.appendChild(Components.statCard('🌐', status.sites_count || 0, 'Active Sites', 'blue'));
    stats.appendChild(Components.statCard('🖥', system.num_cpu || 0, 'CPU Cores', 'yellow'));
    stats.appendChild(Components.statCard('📦', (system.mem_alloc_mb || 0).toFixed(1) + ' MB', 'Memory', 'blue'));
    page.appendChild(stats);

    // Two column layout
    const twoCol = Utils.el('div', { className: 'two-col', style: 'margin-top: 20px' });

    // Gateway info card
    const infoCard = Utils.el('div', { className: 'card' });
    infoCard.innerHTML = `
      <div class="card-header"><h3>Gateway</h3></div>
      <table>
        <tr><td style="color:var(--fg-3);width:140px">Version</td><td style="color:var(--fg-0);font-family:var(--mono)">${Utils.escapeHtml(status.version || '-')}</td></tr>
        <tr><td style="color:var(--fg-3)">PID</td><td style="color:var(--fg-0);font-family:var(--mono)">${status.pid || '-'}</td></tr>
        <tr><td style="color:var(--fg-3)">Listen</td><td style="color:var(--fg-0);font-family:var(--mono)">${Utils.escapeHtml(status.addr || '-')}</td></tr>
        <tr><td style="color:var(--fg-3)">Document Root</td><td style="color:var(--fg-0);font-family:var(--mono);font-size:0.8rem;word-break:break-all">${Utils.escapeHtml(status.doc_root || '-')}</td></tr>
        <tr><td style="color:var(--fg-3)">Framework</td><td>${status.framework ? Components.badge(status.framework, 'info') : '<span style="color:var(--fg-3)">-</span>'}</td></tr>
        <tr><td style="color:var(--fg-3)">Goroutines</td><td style="color:var(--fg-0);font-family:var(--mono)">${status.goroutines || 0}</td></tr>
      </table>`;
    twoCol.appendChild(infoCard);

    // System info card
    const sysCard = Utils.el('div', { className: 'card' });
    sysCard.innerHTML = `
      <div class="card-header"><h3>System</h3></div>
      <table>
        <tr><td style="color:var(--fg-3);width:140px">Hostname</td><td style="color:var(--fg-0);font-family:var(--mono)">${Utils.escapeHtml(system.hostname || '-')}</td></tr>
        <tr><td style="color:var(--fg-3)">OS</td><td style="color:var(--fg-0);font-family:var(--mono)">${Utils.escapeHtml(system.os || '-')} / ${Utils.escapeHtml(system.arch || '-')}</td></tr>
        <tr><td style="color:var(--fg-3)">Go</td><td style="color:var(--fg-0);font-family:var(--mono)">${Utils.escapeHtml(system.go_version || '-')}</td></tr>
        <tr><td style="color:var(--fg-3)">Memory (sys)</td><td style="color:var(--fg-0);font-family:var(--mono)">${(system.mem_sys_mb || 0).toFixed(1)} MB</td></tr>
        <tr><td style="color:var(--fg-3)">PHP Runtimes</td><td>${(status.runtimes || []).map(r => Components.badge(r, 'info')).join(' ') || '<span style="color:var(--fg-3)">-</span>'}</td></tr>
      </table>`;
    twoCol.appendChild(sysCard);

    page.appendChild(twoCol);

    // Quick actions
    const actions = Utils.el('div', { className: 'section', style: 'margin-top: 20px' });
    actions.innerHTML = `
      <div class="section-header"><h2>Quick Actions</h2></div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <a href="#/sites" class="btn btn-primary">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          Add Site
        </a>
        <a href="#/config" class="btn btn-secondary">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
          Edit Config
        </a>
        <a href="#/runtimes" class="btn btn-secondary">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 17V7l8-4.5L20 7v10l-8 4.5z"/></svg>
          Manage Runtimes
        </a>
        <a href="#/logs" class="btn btn-secondary">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          View Logs
        </a>
      </div>`;
    page.appendChild(actions);
  },
};
