// API client

window.API = (function () {
  const BASE = '';

  async function request(path, opts = {}) {
    const url = BASE + path;
    const config = {
      headers: { 'Content-Type': 'application/json' },
      ...opts,
    };
    if (config.body && typeof config.body === 'object') {
      config.body = JSON.stringify(config.body);
    }

    try {
      const res = await fetch(url, config);
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Request failed');
      return data;
    } catch (err) {
      console.error('API error:', path, err);
      throw err;
    }
  }

  return {
    getStatus:    ()      => request('/api/status'),
    getSystem:     ()      => request('/api/system'),
    getSites:      ()      => request('/api/sites'),
    createSite:    (site)  => request('/api/sites', { method: 'POST', body: site }),
    updateSite:    (id, s) => request('/api/sites/' + id, { method: 'PUT', body: s }),
    deleteSite:    (id)    => request('/api/sites/' + id, { method: 'DELETE' }),
    getConfig:     ()      => request('/api/config'),
    saveConfig:    (cfg)   => request('/api/config/save', { method: 'POST', body: cfg }),
    validateConfig:()      => request('/api/config/validate'),
    getRuntimes:   ()      => request('/api/runtimes'),
    getLogs:       ()      => request('/api/logs'),
    getRecentLogs: ()      => request('/api/logs/recent'),
    health:        ()      => request('/api/health'),
  };
})();
