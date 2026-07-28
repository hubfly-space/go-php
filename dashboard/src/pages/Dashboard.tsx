import { Activity, Globe, Clock, AlertTriangle } from 'lucide-react'
import StatCard from '../components/StatCard'
import { Card } from '../components/StatCard'
import { useApi, useInterval } from '../hooks/useApi'
import { api } from '../api/client'

export default function DashboardPage() {
  const { data: status, loading, refetch } = useApi(() => api.getStatus(), [])
  const { data: system } = useApi(() => api.getSystem(), [])

  useInterval(() => { refetch() }, 5000)

  if (loading || !status) {
    return <div className="flex items-center justify-center h-64 text-text-muted text-sm">Loading dashboard...</div>
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard label="Uptime" value={formatUptime(status.uptime_seconds)} icon={Clock} color="info" />
        <StatCard label="Total Requests" value={status.total_requests.toLocaleString()} icon={Activity} trend={`${status.active_requests} active`} color="accent" />
        <StatCard label="Sites" value={status.sites_count} icon={Globe} color="success" />
        <StatCard
          label="Errors"
          value={status.total_errors.toLocaleString()}
          icon={AlertTriangle}
          color={status.total_errors > 0 ? 'error' : 'success'}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card title="Gateway">
          <table className="w-full text-sm">
            <tbody>
              <Row label="Version" value={status.version} />
              <Row label="PID" value={String(status.pid)} />
              <Row label="Address" value={status.addr} />
              <Row label="Document Root" value={status.doc_root} mono />
              {status.framework && <Row label="Framework" value={status.framework} />}
              <Row label="Runtimes" value={status.runtimes.join(', ') || 'None detected'} />
              <Row label="Goroutines" value={String(status.goroutines)} />
            </tbody>
          </table>
        </Card>

        {system && (
          <Card title="System">
            <table className="w-full text-sm">
              <tbody>
                <Row label="Hostname" value={system.hostname} />
                <Row label="OS / Arch" value={`${system.os} / ${system.arch}`} />
                <Row label="Go" value={system.go_version} />
                <Row label="CPU Cores" value={String(system.num_cpu)} />
                <Row label="Memory (Alloc)" value={`${system.mem_alloc_mb.toFixed(1)} MB`} />
                <Row label="Memory (Sys)" value={`${system.mem_sys_mb.toFixed(1)} MB`} />
                <Row label="PID" value={String(system.pid)} />
              </tbody>
            </table>
          </Card>
        )}
      </div>

      <Card title="Quick Start">
        <div className="text-sm text-text-secondary space-y-3">
          <p>Create a new site from the <strong className="text-text">Sites</strong> page. Each site gets its own port and webroot directory.</p>
          <div className="bg-bg rounded-lg p-4 font-mono text-xs text-text-muted space-y-1">
            <p><span className="text-accent">$</span> gateway serve . --php-fpm /usr/sbin/php-fpm</p>
            <p><span className="text-accent">$</span> open http://127.0.0.1:30200</p>
          </div>
        </div>
      </Card>
    </div>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <tr className="border-b border-border/50 last:border-0">
      <td className="py-2.5 text-text-muted font-medium">{label}</td>
      <td className={`py-2.5 text-right text-text ${mono ? 'font-mono text-xs break-all' : ''}`}>{value}</td>
    </tr>
  )
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const mins = Math.floor(seconds / 60)
  if (mins < 60) return `${mins}m ${seconds % 60}s`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ${mins % 60}m`
  const days = Math.floor(hours / 24)
  return `${days}d ${hours % 24}h`
}
