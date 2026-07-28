import { useState, useRef, useEffect } from 'react'
import { ScrollText, RefreshCw } from 'lucide-react'
import { Card } from '../components/StatCard'
import { useApi, useInterval } from '../hooks/useApi'
import { api } from '../api/client'
import type { LogEntry } from '../types'

export default function LogsPage() {
  const { data, loading, refetch } = useApi(() => api.getLogs(), [])
  const [autoRefresh, setAutoRefresh] = useState(true)
  const bottomRef = useRef<HTMLDivElement>(null)

  useInterval(() => { if (autoRefresh) refetch() }, autoRefresh ? 3000 : null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [data?.entries?.length])

  const entries = data?.entries || []

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Logs</h2>
          <p className="text-sm text-text-muted mt-0.5">
            {data?.total || 0} total entries. Showing {entries.length} most recent.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer select-none">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              className="w-4 h-4 rounded border-border bg-bg text-accent focus:ring-accent focus:ring-offset-0"
            />
            Auto-refresh
          </label>
          <button onClick={() => refetch()} className="p-2 text-text-muted hover:text-text hover:bg-surface-hover rounded-lg transition-colors">
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>
      </div>

      <Card title="Log Stream">
        {loading && entries.length === 0 ? (
          <p className="text-sm text-text-muted py-8 text-center">Loading logs...</p>
        ) : entries.length === 0 ? (
          <div className="text-center py-12">
            <ScrollText className="w-10 h-10 text-text-muted mx-auto mb-3" />
            <p className="text-sm text-text-secondary">No log entries yet</p>
            <p className="text-xs text-text-muted mt-1">Activity will appear here as you use the gateway</p>
          </div>
        ) : (
          <div className="bg-bg rounded-lg border border-border overflow-hidden">
            <div className="max-h-[500px] overflow-y-auto font-mono text-xs leading-relaxed">
              {entries.map((entry: LogEntry, i: number) => (
                <div key={i} className="flex items-start gap-3 px-4 py-2 border-b border-border/30 last:border-0 hover:bg-surface-hover/30">
                  <span className="text-text-muted shrink-0 w-[180px]">{formatTime(entry.timestamp)}</span>
                  <span className={`shrink-0 w-12 font-medium ${levelColor(entry.level)}`}>{entry.level.toUpperCase()}</span>
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
  switch (level) {
    case 'error': return 'text-error'
    case 'warn': return 'text-warning'
    case 'info': return 'text-info'
    case 'debug': return 'text-text-muted'
    default: return 'text-text-secondary'
  }
}
