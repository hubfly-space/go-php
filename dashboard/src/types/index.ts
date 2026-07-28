export interface Site {
  id: string
  name: string
  port: number
  webroot: string
  domain: string
  php_version: string
  status: 'active' | 'stopped' | 'error'
  routes: number
  ssl: boolean
  created_at: string
  updated_at: string
}

export interface SiteCreateRequest {
  name: string
  port: number
  webroot: string
  php_version: string
}

export interface Runtime {
  version: string
  binary: string
  status: string
  active: boolean
  managed: boolean
}

export interface GatewayStatus {
  status: string
  uptime: string
  uptime_seconds: number
  version: string
  pid: number
  addr: string
  doc_root: string
  framework: string
  goroutines: number
  active_requests: number
  total_requests: number
  total_errors: number
  sites_count: number
  runtimes: string[]
}

export interface SystemInfo {
  hostname: string
  os: string
  arch: string
  go_version: string
  goroutines: number
  mem_alloc_mb: number
  mem_sys_mb: number
  num_cpu: number
  pid: number
}

export interface LogEntry {
  timestamp: string
  level: string
  message: string
  extra?: unknown
}

export interface GatewayConfig {
  schema: string
  server: {
    addr: string
    read_timeout: string
    write_timeout: string
  }
  php: {
    binary: string
    socket_path: string
    max_children: number
    request_timeout: string
  }
  logging: {
    format: string
    level: string
  }
  security: {
    symlink_mode: string
    max_body_size: string
    protected_patterns: string[]
  }
  sites: Site[]
}
