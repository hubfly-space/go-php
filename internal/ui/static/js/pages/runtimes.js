// Runtimes management page

window.RuntimesPage = {
  async render() {
    const page = document.getElementById('page');
    page.innerHTML = '';
    page.appendChild(Components.loading());

    try {
      const data = await API.getRuntimes();
      Store.set('runtimes', data.runtimes || []);
      page.innerHTML = '';
      this.renderRuntimes(page, data.runtimes || []);
    } catch (err) {
      page.innerHTML = '';
      page.appendChild(Components.emptyState('⚠️', 'Failed to load runtimes: ' + err.message));
    }
  },

  renderRuntimes(page, runtimes) {
    const header = Utils.el('div', { className: 'section-header' });
    header.innerHTML = `<h2>PHP Runtimes</h2>`;
    page.appendChild(header);

    if (runtimes.length === 0) {
      page.appendChild(Components.emptyState('📦', 'No PHP runtimes installed'));
      return;
    }

    const wrap = Utils.el('div', { className: 'table-wrap' });
    const table = Utils.el('table');
    table.innerHTML = `
      <thead>
        <tr>
          <th>Version</th>
          <th>Binary</th>
          <th>Status</th>
          <th>Active</th>
          <th>Managed</th>
        </tr>
      </thead>
      <tbody></tbody>`;
    wrap.appendChild(table);
    page.appendChild(wrap);

    const tbody = table.querySelector('tbody');
    runtimes.forEach(rt => {
      const tr = Utils.el('tr');
      tr.innerHTML = `
        <td style="color:var(--fg-0);font-weight:500;font-family:var(--mono)">PHP ${Utils.escapeHtml(rt.version)}</td>
        <td style="font-family:var(--mono);font-size:0.82rem;color:var(--fg-2)">${Utils.escapeHtml(rt.binary)}</td>
        <td>${Components.badge(rt.status, rt.status === 'ready' ? 'success' : 'warning')}</td>
        <td>${rt.active ? Components.badge('Active', 'info') : Components.badge('Inactive', 'neutral')}</td>
        <td>${rt.managed ? '✓' : '—'}</td>`;
      tbody.appendChild(tr);
    });

    // Info card
    const info = Utils.el('div', { className: 'card', style: 'margin-top:20px' });
    info.innerHTML = `
      <div class="card-header"><h3>About Runtimes</h3></div>
      <p style="color:var(--fg-2);font-size:0.85rem;line-height:1.6">
        Managed runtimes are installed and controlled by the gateway.
        Each runtime runs its own PHP-FPM pool. The active runtime handles incoming PHP requests.
        Switch runtimes with zero downtime using the deploy manager.
      </p>`;
    page.appendChild(info);
  },
};
