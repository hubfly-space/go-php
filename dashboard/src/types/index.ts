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
  routes?: Array<{ host?: string; path?: string; path_prefix?: string; target?: string; type?: string }>
  sites: Site[]
}

export interface Release {
  id: string
  version: string
  runtime_id: string
  state: 'created' | 'active' | 'draining' | 'stopped' | 'failed'
  created_at: string
  activated_at?: string
  deactivated_at?: string
  dir: string
  metadata?: Record<string, string>
}

export interface CheckResult {
  name: string
  status: 'ok' | 'warn' | 'fail'
  message: string
}

export interface DoctorReport {
  checks: CheckResult[]
  os: string
  arch: string
  go_version: string
  hostname: string
}

export interface CompatIssue {
  category: string
  severity: 'error' | 'warning' | 'info'
  file?: string
  message: string
  suggestion?: string
}

export interface CompatReport {
  root: string
  scanned_at: string
  framework?: string
  php_version?: string
  issues?: CompatIssue[]
  warnings?: CompatIssue[]
  info?: string[]
  score: number
}

export interface Explanation {
  request: {
    method: string
    host: string
    path: string
    query: string
    headers: Record<string, string>
    tls: boolean
    remote: string
  }
  path_normalization: {
    raw: string
    decoded: string
    normalized: string
    valid: boolean
  }
  policy_check: {
    decision: string
    matched_rules?: string[]
  }
  route_match: {
    matched: boolean
    target?: string
  }
  file_check: {
    found: boolean
    is_php: boolean
    protected: boolean
    error?: string
  }
  script_check: {
    found: boolean
    script_name: string
    script_path: string
  }
  summary: string
  duration: string
}

export interface MetricPoint {
  timestamp: string
  requests: number
  errors: number
  latency_ms: number
}
