// Reusable UI components

window.Components = {
  statCard(icon, value, label, colorClass) {
    const div = Utils.el('div', { className: 'stat-card' });
    div.innerHTML = `
      <div class="stat-icon ${colorClass}">${icon}</div>
      <div class="stat-value">${Utils.escapeHtml(String(value))}</div>
      <div class="stat-label">${Utils.escapeHtml(label)}</div>
    `;
    return div;
  },

  badge(text, variant) {
    return Utils.el('span', { className: `badge badge-${variant || 'neutral'}` }, text);
  },

  toast(message, type) {
    const container = document.getElementById('toast-container');
    const toast = Utils.el('div', { className: `toast ${type || 'info'}` }, message);
    container.appendChild(toast);
    setTimeout(() => { toast.style.opacity = '0'; setTimeout(() => toast.remove(), 200); }, 3000);
  },

  confirm(message) {
    return new Promise((resolve) => {
      const overlay = document.getElementById('modal-overlay');
      overlay.classList.remove('hidden');
      overlay.innerHTML = `
        <div class="modal">
          <div class="modal-header">
            <h3>Confirm</h3>
            <button class="modal-close" data-action="cancel">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
            </button>
          </div>
          <div class="modal-body"><p>${Utils.escapeHtml(message)}</p></div>
          <div class="modal-footer">
            <button class="btn btn-secondary" data-action="cancel">Cancel</button>
            <button class="btn btn-danger" data-action="confirm">Delete</button>
          </div>
        </div>`;

      const close = (val) => { overlay.classList.add('hidden'); resolve(val); };
      overlay.querySelectorAll('[data-action]').forEach(btn => {
        btn.addEventListener('click', () => close(btn.dataset.action === 'confirm'));
      });
      overlay.addEventListener('click', (e) => { if (e.target === overlay) close(false); }, { once: true });
    });
  },

  modal(title, bodyHtml, footerHtml) {
    const overlay = document.getElementById('modal-overlay');
    overlay.classList.remove('hidden');
    overlay.innerHTML = `
      <div class="modal">
        <div class="modal-header">
          <h3>${Utils.escapeHtml(title)}</h3>
          <button class="modal-close" id="modal-close-btn">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <div class="modal-body">${bodyHtml}</div>
        ${footerHtml ? `<div class="modal-footer">${footerHtml}</div>` : ''}
      </div>`;

    const close = () => overlay.classList.add('hidden');
    document.getElementById('modal-close-btn').addEventListener('click', close);
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); }, { once: true });
    return { close };
  },

  closeModal() {
    document.getElementById('modal-overlay').classList.add('hidden');
  },

  loading() {
    return Utils.el('div', { className: 'loading', html: '<div class="spinner"></div> Loading...' });
  },

  emptyState(icon, text, actionHtml) {
    const div = Utils.el('div', { className: 'empty-state' });
    div.innerHTML = `
      <div class="empty-state-icon">${icon}</div>
      <div class="empty-state-text">${Utils.escapeHtml(text)}</div>
      ${actionHtml || ''}`;
    return div;
  },

  logLine(entry) {
    const div = Utils.el('div', { className: 'log-line' });
    div.innerHTML = `
      <span class="log-time">${Utils.formatTime(entry.timestamp)}</span>
      <span class="log-level ${entry.level}">${Utils.escapeHtml(entry.level)}</span>
      <span class="log-msg">${Utils.escapeHtml(entry.message)}</span>`;
    return div;
  },
};
