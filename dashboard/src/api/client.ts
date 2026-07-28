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
  getSystem: () => request<{ hostname: string; os: string; arch: string; go_version: string; goroutines: number; mem_alloc_mb: number; mem_sys_mb: number; num_cpu: number; pid: number }>('/system'),
  getSites: () => request<{ sites: import('../types').Site[] }>('/sites'),
  createSite: (data: import('../types').SiteCreateRequest) => request<{ site: import('../types').Site }>('/sites', { method: 'POST', body: JSON.stringify(data) }),
  updateSite: (id: string, data: import('../types').Site) => request<{ site: import('../types').Site }>(`/sites/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteSite: (id: string) => request<{ status: string }>(`/sites/${id}`, { method: 'DELETE' }),
  getRuntimes: () => request<{ runtimes: import('../types').Runtime[] }>('/runtimes'),
  getLogs: () => request<{ entries: import('../types').LogEntry[]; total: number }>('/logs'),
  getConfig: () => request<import('../types').GatewayConfig>('/config'),
  saveConfig: (cfg: import('../types').GatewayConfig) => request<{ status: string }>('/config/save', { method: 'POST', body: JSON.stringify(cfg) }),
  healthCheck: () => request<{ status: string }>('/health'),
}
