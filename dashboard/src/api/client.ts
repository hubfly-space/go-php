const BASE = '/api'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || 'Request failed')
  }
  return res.json()
}

export const api = {
  getStatus: () => request<{ status: string; uptime: string; uptime_seconds: number; version: string; pid: number; addr: string; doc_root: string; framework: string; goroutines: number; active_requests: number; total_requests: number; total_errors: number; sites_count: number; runtimes: string[] }>('/status'),
  getSystem: () => request<import('../types').SystemInfo>('/system'),
  getSites: () => request<{ sites: import('../types').Site[] }>('/sites'),
  createSite: (data: import('../types').SiteCreateRequest) => request<{ site: import('../types').Site }>('/sites', { method: 'POST', body: JSON.stringify(data) }),
  updateSite: (id: string, data: import('../types').Site) => request<{ site: import('../types').Site }>(`/sites/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteSite: (id: string) => request<{ status: string }>(`/sites/${id}`, { method: 'DELETE' }),
  getRuntimes: () => request<{ runtimes: import('../types').Runtime[] }>('/runtimes'),
  getLogs: () => request<{ entries: import('../types').LogEntry[]; total: number }>('/logs'),
  getConfig: () => request<import('../types').GatewayConfig>('/config'),
  saveConfig: (cfg: import('../types').GatewayConfig) => request<{ status: string }>('/config/save', { method: 'POST', body: JSON.stringify(cfg) }),
  healthCheck: () => request<{ status: string }>('/health'),

  // Deployments
  getDeployments: () => request<{ releases: import('../types').Release[] }>('/deploy/list'),
  createDeployment: (version: string, srcDir?: string) => request<{ release: import('../types').Release }>('/deploy/create', { method: 'POST', body: JSON.stringify({ version, src_dir: srcDir }) }),
  activateDeployment: (id: string) => request<{ status: string; id: string }>('/deploy/activate', { method: 'POST', body: JSON.stringify({ id }) }),
  rollbackDeployment: () => request<{ status: string; release: import('../types').Release }>('/deploy/rollback', { method: 'POST' }),

  // Diagnostics & Explorer
  getDoctor: () => request<import('../types').DoctorReport>('/doctor'),
  getDoctorCompat: (path?: string) => request<import('../types').CompatReport>(`/doctor/compat${path ? `?path=${encodeURIComponent(path)}` : ''}`),
  explainRequest: (url: string) => request<import('../types').Explanation>(`/explain?url=${encodeURIComponent(url)}`),
  getMetricsHistory: () => request<{ history: import('../types').MetricPoint[] }>('/metrics/history'),

  // Extensions
  getExtensions: () => request<{ extensions: import('../types').ExtensionInfo[]; enabled: string[] }>('/extensions'),
  getProfiles: () => request<{ profiles: import('../types').ExtensionProfile[] }>('/profiles'),
  getSiteExtensions: (id: string) => request<import('../types').SiteExtensions>(`/sites/${id}/extensions`),
  updateSiteExtensions: (id: string, data: { extensions: string[]; profile: string }) => request<{ status: string }>(`/sites/${id}/extensions`, { method: 'PUT', body: JSON.stringify(data) }),
}
