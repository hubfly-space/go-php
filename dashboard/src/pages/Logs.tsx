import { useState, useRef, useEffect } from 'react'
import { ScrollText, RefreshCw, Search, Radio } from 'lucide-react'
import { Card } from '../components/StatCard'
import { useApi, useInterval } from '../hooks/useApi'
import { api } from '../api/client'
import type { LogEntry } from '../types'

export default function LogsPage() {
  const { data, loading, refetch } = useApi(() => api.getLogs(), [])
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [search, setSearch] = useState('')
  const [filterLevel, setFilterLevel] = useState('all')
  const [wsConnected, setWsConnected] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (data?.entries) {
      setEntries(data.entries)
    }
  }, [data])

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/ws/logs`
    let ws: WebSocket | null = null

    try {
      ws = new WebSocket(wsUrl)
      ws.onopen = () => setWsConnected(true)
      ws.onclose = () => setWsConnected(false)
      ws.onerror = () => setWsConnected(false)
      ws.onmessage = (evt) => {
        try {
          const entry: LogEntry = JSON.parse(evt.data)
          setEntries((prev) => [...prev.slice(-499), entry])
        } catch {}
      }
    } catch {
      setWsConnected(false)
    }

    return () => {
      if (ws) ws.close()
    }
  }, [])

  useInterval(() => {
    if (!wsConnected) refetch()
  }, wsConnected ? null : 3000)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [entries.length])

  const filtered = entries.filter((e) => {
    const matchesSearch = e.message.toLowerCase().includes(search.toLowerCase())
    const matchesLevel = filterLevel === 'all' || e.level.toLowerCase() === filterLevel
    return matchesSearch && matchesLevel
  })

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-text">Real-Time System Logs</h2>
          <p className="text-sm text-text-muted mt-0.5">
            Showing {filtered.length} log entries. Streamed in real-time via WebSockets.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 px-2.5 py-1 bg-bg border border-border rounded-full text-xs font-medium">
            <Radio className={`w-3 h-3 ${wsConnected ? 'text-success animate-pulse' : 'text-text-muted'}`} />
            <span className={wsConnected ? 'text-success' : 'text-text-muted'}>
              {wsConnected ? 'WebSocket Live' : 'Polling (3s)'}
            </span>
          </div>
          <button onClick={() => refetch()} className="p-2 text-text-muted hover:text-text hover:bg-surface-hover rounded-lg transition-colors">
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Filters Bar */}
      <div className="flex items-center gap-3 p-3 bg-surface border border-border rounded-xl">
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-text-muted absolute left-3 top-2.5" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search log messages..."
            className="w-full bg-bg border border-border rounded-lg pl-9 pr-3 py-1.5 text-xs text-text focus:outline-none focus:border-accent"
          />
        </div>
        <select
          value={filterLevel}
          onChange={(e) => setFilterLevel(e.target.value)}
          className="bg-bg border border-border rounded-lg px-3 py-1.5 text-xs text-text focus:outline-none focus:border-accent"
        >
          <option value="all">All Levels</option>
          <option value="info">INFO</option>
          <option value="warn">WARN</option>
          <option value="error">ERROR</option>
          <option value="debug">DEBUG</option>
        </select>
      </div>

      <Card title="Live Terminal Feed">
        {loading && entries.length === 0 ? (
          <p className="text-sm text-text-muted py-8 text-center">Connecting to log stream...</p>
        ) : filtered.length === 0 ? (
          <div className="text-center py-12">
            <ScrollText className="w-10 h-10 text-text-muted mx-auto mb-3" />
            <p className="text-sm text-text-secondary">No log entries found</p>
          </div>
        ) : (
          <div className="bg-bg rounded-lg border border-border overflow-hidden">
            <div className="max-h-[520px] overflow-y-auto font-mono text-xs leading-relaxed">
              {filtered.map((entry: LogEntry, i: number) => (
                <div key={i} className="flex items-start gap-3 px-4 py-2 border-b border-border/30 last:border-0 hover:bg-surface-hover/30">
                  <span className="text-text-muted shrink-0 w-[170px]">{formatTime(entry.timestamp)}</span>
                  <span className={`shrink-0 w-12 font-semibold ${levelColor(entry.level)}`}>{entry.level.toUpperCase()}</span>
                  <span className="text-text break-all">{entry.message}</span>
                </div>
              ))}
              <div ref={bottomRef} />
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

function formatTime(ts: string): string {
  try {
    const d = new Date(ts)
    return d.toLocaleString('en-US', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return ts
  }
}

function levelColor(level: string): string {
  switch (level.toLowerCase()) {
    case 'error': return 'text-error'
    case 'warn': return 'text-warning'
    case 'info': return 'text-info'
    case 'debug': return 'text-text-muted'
    default: return 'text-text-secondary'
  }
}
