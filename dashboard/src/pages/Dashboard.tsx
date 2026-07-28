import { useNavigate } from 'react-router'
import { Activity, Globe, Clock, AlertTriangle, ArrowRight, Zap, CheckCircle2, Terminal } from 'lucide-react'
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import StatCard, { Card } from '../components/StatCard'
import { useApi, useInterval } from '../hooks/useApi'
import { api } from '../api/client'

export default function DashboardPage() {
  const navigate = useNavigate()
  const { data: status, loading, refetch } = useApi(() => api.getStatus(), [])
  const { data: system } = useApi(() => api.getSystem(), [])
  const { data: metricsData, refetch: refetchMetrics } = useApi(() => api.getMetricsHistory(), [])

  useInterval(() => {
    refetch()
    refetchMetrics()
  }, 5000)

  if (loading || !status) {
    return <div className="flex items-center justify-center h-64 text-text-muted text-sm">Loading dashboard...</div>
  }

  const history = metricsData?.history || []

  return (
    <div className="space-y-6">
      {/* Quick Action Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 p-4 bg-surface border border-border rounded-xl shadow-xs">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-lg bg-accent/10 flex items-center justify-center">
            <Zap className="w-5 h-5 text-accent" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-text">Gateway Control Center</h3>
            <p className="text-xs text-text-muted">Quick actions and real-time monitoring</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => navigate('/doctor')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium bg-bg border border-border hover:border-accent rounded-lg text-text transition-colors"
          >
            <CheckCircle2 className="w-3.5 h-3.5 text-success" />
            Run Doctor
          </button>
          <button
            onClick={() => navigate('/explorer')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium bg-bg border border-border hover:border-accent rounded-lg text-text transition-colors"
          >
            <Terminal className="w-3.5 h-3.5 text-info" />
            Trace Request
          </button>
          <button
            onClick={() => navigate('/sites')}
            className="flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium bg-accent hover:bg-accent-hover text-white rounded-lg transition-colors"
          >
            Manage Sites
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Top Stat Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard label="Uptime" value={formatUptime(status.uptime_seconds)} icon={Clock} color="info" />
        <StatCard label="Total Requests" value={status.total_requests.toLocaleString()} icon={Activity} trend={`${status.active_requests} active`} color="accent" />
        <StatCard label="Configured Sites" value={status.sites_count} icon={Globe} color="success" />
        <StatCard
          label="Total Errors"
          value={status.total_errors.toLocaleString()}
          icon={AlertTriangle}
          color={status.total_errors > 0 ? 'error' : 'success'}
        />
      </div>

      {/* Interactive Recharts Analytics */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card title="Request Throughput (Req/sec)">
          <div className="h-56 pt-2">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={history}>
                <defs>
                  <linearGradient id="reqGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#2563eb" stopOpacity={0.4} />
                    <stop offset="95%" stopColor="#2563eb" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <XAxis dataKey="timestamp" stroke="#94a3b8" fontSize={11} tickLine={false} />
                <YAxis stroke="#94a3b8" fontSize={11} tickLine={false} axisLine={false} />
                <Tooltip
                  contentStyle={{ backgroundColor: '#ffffff', borderColor: '#e2e8f0', borderRadius: '8px', fontSize: '12px', color: '#0f172a' }}
                />
                <Area type="monotone" dataKey="requests" stroke="#2563eb" strokeWidth={2} fillOpacity={1} fill="url(#reqGradient)" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </Card>

        <Card title="Latency Distribution (ms)">
          <div className="h-56 pt-2">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={history}>
                <XAxis dataKey="timestamp" stroke="#94a3b8" fontSize={11} tickLine={false} />
                <YAxis stroke="#94a3b8" fontSize={11} tickLine={false} axisLine={false} />
                <Tooltip
                  contentStyle={{ backgroundColor: '#ffffff', borderColor: '#e2e8f0', borderRadius: '8px', fontSize: '12px', color: '#0f172a' }}
                />
                <Bar dataKey="latency_ms" fill="#10b981" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>
      </div>

      {/* Gateway Info & System Stats */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card title="Gateway Runtime Engine">
          <table className="w-full text-sm">
            <tbody>
              <Row label="Gateway Version" value={status.version} />
              <Row label="Process PID" value={String(status.pid)} />
              <Row label="Listening Address" value={status.addr} />
              <Row label="Document Root" value={status.doc_root} mono />
              {status.framework && <Row label="Detected Framework" value={status.framework} />}
              <Row label="PHP Runtimes" value={status.runtimes.join(', ') || 'None detected'} />
              <Row label="Active Goroutines" value={String(status.goroutines)} />
            </tbody>
          </table>
        </Card>

        {system && (
          <Card title="Host System Environment">
            <table className="w-full text-sm">
              <tbody>
                <Row label="Hostname" value={system.hostname} />
                <Row label="OS / Architecture" value={`${system.os} / ${system.arch}`} />
                <Row label="Go Runtime" value={system.go_version} />
                <Row label="CPU Cores" value={String(system.num_cpu)} />
                <Row label="Memory (Allocated)" value={`${system.mem_alloc_mb.toFixed(1)} MB`} />
                <Row label="Memory (System Sys)" value={`${system.mem_sys_mb.toFixed(1)} MB`} />
                <Row label="PID" value={String(system.pid)} />
              </tbody>
            </table>
          </Card>
        )}
      </div>
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
