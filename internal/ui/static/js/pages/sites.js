// Sites management page

window.SitesPage = {
  async render() {
    const page = document.getElementById('page');
    page.innerHTML = '';
    page.appendChild(Components.loading());

    try {
      const data = await API.getSites();
      Store.set('sites', data.sites || []);
      page.innerHTML = '';
      this.renderSites(page, data.sites || []);
    } catch (err) {
      page.innerHTML = '';
      page.appendChild(Components.emptyState('⚠️', 'Failed to load sites: ' + err.message));
    }
  },

  renderSites(page, sites) {
    // Header
    const header = Utils.el('div', { className: 'section-header' });
    header.innerHTML = `<h2>Sites</h2>`;
    const addBtn = Utils.el('button', { className: 'btn btn-primary', id: 'add-site-btn' });
    addBtn.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg> Add Site`;
    addBtn.addEventListener('click', () => this.showAddModal());
    header.appendChild(addBtn);
    page.appendChild(header);

    if (sites.length === 0) {
      page.appendChild(Components.emptyState('🌐', 'No sites configured yet', '<button class="btn btn-primary" onclick="SitesPage.showAddModal()">Add your first site</button>'));
      return;
    }

    // Table
    const wrap = Utils.el('div', { className: 'table-wrap' });
    const table = Utils.el('table');
    table.innerHTML = `
      <thead>
        <tr>
          <th>Site</th>
          <th>Domain</th>
          <th>PHP</th>
          <th>Status</th>
          <th>Routes</th>
          <th style="width:100px">Actions</th>
        </tr>
      </thead>
      <tbody id="sites-tbody"></tbody>`;
    wrap.appendChild(table);
    page.appendChild(wrap);

    const tbody = table.querySelector('#sites-tbody');
    sites.forEach(site => {
      const tr = Utils.el('tr');
      const statusBadge = site.status === 'active'
        ? Components.badge('Active', 'success')
        : Components.badge(site.status, 'neutral');

      tr.innerHTML = `
        <td style="color:var(--fg-0);font-weight:500">${Utils.escapeHtml(site.name)}</td>
        <td style="font-family:var(--mono);font-size:0.82rem">${Utils.escapeHtml(site.domain || '-')}</td>
        <td>${site.php_version ? Components.badge(site.php_version, 'info') : '<span style="color:var(--fg-3)">-</span>'}</td>
        <td>${statusBadge}</td>
        <td style="font-family:var(--mono)">${site.routes || 0}</td>
        <td>
          <div style="display:flex;gap:4px">
            <button class="btn btn-ghost btn-sm edit-btn" data-id="${site.id}" title="Edit">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </button>
            <button class="btn btn-ghost btn-sm delete-btn" data-id="${site.id}" data-name="${site.name}" title="Delete">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            </button>
          </div>
        </td>`;
      tbody.appendChild(tr);
    });

    // Event delegation
    tbody.addEventListener('click', async (e) => {
      const editBtn = e.target.closest('.edit-btn');
      const deleteBtn = e.target.closest('.delete-btn');
      if (editBtn) this.showEditModal(editBtn.dataset.id);
      if (deleteBtn) {
        const confirmed = await Components.confirm(`Delete site "${deleteBtn.dataset.name}"?`);
        if (confirmed) {
          try {
            await API.deleteSite(deleteBtn.dataset.id);
            Components.toast('Site deleted', 'success');
            this.render();
          } catch (err) {
            Components.toast('Failed to delete: ' + err.message, 'error');
          }
        }
      }
    });
  },

  showAddModal() {
    const modal = Components.modal('Add Site', `
      <div class="form-group">
        <label class="form-label">Site Name</label>
        <input class="form-input" id="site-name" placeholder="my-site">
      </div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label">Domain</label>
          <input class="form-input" id="site-domain" placeholder="example.com">
        </div>
        <div class="form-group">
          <label class="form-label">PHP Version</label>
          <input class="form-input" id="site-php" placeholder="8.3">
        </div>
      </div>
      <div class="form-group">
        <label class="form-label">Document Root</label>
        <input class="form-input" id="site-root" placeholder="/var/www/html">
      </div>`, `
      <button class="btn btn-secondary" data-action="cancel">Cancel</button>
      <button class="btn btn-primary" id="save-site-btn">Create Site</button>
    `);

    document.getElementById('save-site-btn').addEventListener('click', async () => {
      const site = {
        name: document.getElementById('site-name').value,
        domain: document.getElementById('site-domain').value,
        php_version: document.getElementById('site-php').value,
        root: document.getElementById('site-root').value,
      };
      if (!site.name) { Components.toast('Name is required', 'error'); return; }
      try {
        await API.createSite(site);
        Components.toast('Site created', 'success');
        modal.close();
        this.render();
      } catch (err) {
        Components.toast('Failed: ' + err.message, 'error');
      }
    });

    document.querySelector('[data-action="cancel"]').addEventListener('click', () => modal.close());
  },

  async showEditModal(id) {
    const sites = Store.get('sites');
    const site = sites.find(s => s.id === id);
    if (!site) return;

    const modal = Components.modal('Edit Site', `
      <div class="form-group">
        <label class="form-label">Site Name</label>
        <input class="form-input" id="edit-name" value="${Utils.escapeHtml(site.name)}">
      </div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label">Domain</label>
          <input class="form-input" id="edit-domain" value="${Utils.escapeHtml(site.domain || '')}">
        </div>
        <div class="form-group">
          <label class="form-label">PHP Version</label>
          <input class="form-input" id="edit-php" value="${Utils.escapeHtml(site.php_version || '')}">
        </div>
      </div>
      <div class="form-group">
        <label class="form-label">Document Root</label>
        <input class="form-input" id="edit-root" value="${Utils.escapeHtml(site.root || '')}">
      </div>`, `
      <button class="btn btn-secondary" data-action="cancel">Cancel</button>
      <button class="btn btn-primary" id="update-site-btn">Save Changes</button>
    `);

    document.getElementById('update-site-btn').addEventListener('click', async () => {
      const updated = {
        name: document.getElementById('edit-name').value,
        domain: document.getElementById('edit-domain').value,
        php_version: document.getElementById('edit-php').value,
        root: document.getElementById('edit-root').value,
        status: site.status,
      };
      try {
        await API.updateSite(id, updated);
        Components.toast('Site updated', 'success');
        modal.close();
        this.render();
      } catch (err) {
        Components.toast('Failed: ' + err.message, 'error');
      }
    });

    document.querySelector('[data-action="cancel"]').addEventListener('click', () => modal.close());
  },
};
