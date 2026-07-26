// Main SPA application

window.App = (function () {
  const pages = {
    dashboard: { title: 'Dashboard',    page: DashboardPage },
    sites:     { title: 'Sites',        page: SitesPage },
    config:    { title: 'Configuration', page: ConfigPage },
    runtimes:  { title: 'Runtimes',     page: RuntimesPage },
    logs:      { title: 'Logs',         page: LogsPage },
  };

  let currentPage = null;

  function init() {
    window.addEventListener('hashchange', route);
    route();
    loadStatus();

    // Mobile menu toggle
    document.getElementById('menu-toggle').addEventListener('click', () => {
      document.getElementById('sidebar').classList.toggle('open');
    });

    // Close sidebar on nav click (mobile)
    document.querySelectorAll('.nav-item').forEach(item => {
      item.addEventListener('click', () => {
        document.getElementById('sidebar').classList.remove('open');
      });
    });

    // Search
    document.getElementById('search-input').addEventListener('input', Utils.debounce((e) => {
      const q = e.target.value.toLowerCase();
      // Simple filter: highlight matching nav items
      document.querySelectorAll('.nav-item').forEach(item => {
        const text = item.textContent.toLowerCase();
        item.style.opacity = !q || text.includes(q) ? '1' : '0.3';
      });
    }, 200));
  }

  function route() {
    const hash = window.location.hash || '#/';
    const path = hash.slice(1) || '/';

    let pageName = 'dashboard';
    if (path.startsWith('/sites')) pageName = 'sites';
    else if (path.startsWith('/config')) pageName = 'config';
    else if (path.startsWith('/runtimes')) pageName = 'runtimes';
    else if (path.startsWith('/logs')) pageName = 'logs';

    // Cleanup previous page
    if (currentPage && pages[currentPage]?.page.destroy) {
      pages[currentPage].page.destroy();
    }

    currentPage = pageName;
    const config = pages[pageName];
    if (!config) return;

    // Update title
    document.getElementById('page-title').textContent = config.title;

    // Update nav active state
    document.querySelectorAll('.nav-item').forEach(item => {
      item.classList.toggle('active', item.dataset.page === pageName);
    });

    // Render page
    config.page.render();
  }

  async function loadStatus() {
    try {
      const status = await API.getStatus();
      Store.set('status', status);
      document.getElementById('sidebar-version').textContent = 'v' + (status.version || '0.0.0');

      // Update status dot
      const dot = document.getElementById('status-dot');
      dot.style.background = status.status === 'ok' ? 'var(--success)' : 'var(--danger)';
    } catch (err) {
      document.getElementById('status-dot').style.background = 'var(--danger)';
      document.getElementById('sidebar-version').textContent = 'offline';
    }
  }

  return { init };
})();

// Boot
document.addEventListener('DOMContentLoaded', App.init);
