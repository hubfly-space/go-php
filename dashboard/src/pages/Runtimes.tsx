import { Cpu, CheckCircle, XCircle, Wrench } from 'lucide-react'
import { Card } from '../components/StatCard'
import { useApi } from '../hooks/useApi'
import { api } from '../api/client'
import type { Runtime } from '../types'

export default function RuntimesPage() {
  const { data, loading } = useApi(() => api.getRuntimes(), [])
  const runtimes = data?.runtimes || []

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold text-text">PHP Runtimes</h2>
        <p className="text-sm text-text-muted mt-0.5">Detected PHP-FPM installations on this system.</p>
      </div>

      <Card title={`${runtimes.length} runtime${runtimes.length !== 1 ? 's' : ''} detected`}>
        {loading ? (
          <p className="text-sm text-text-muted py-8 text-center">Scanning for PHP runtimes...</p>
        ) : runtimes.length === 0 ? (
          <div className="text-center py-12">
            <Cpu className="w-10 h-10 text-text-muted mx-auto mb-3" />
            <p className="text-sm text-text-secondary">No PHP runtimes found</p>
            <p className="text-xs text-text-muted mt-1">Install PHP-FPM to get started</p>
          </div>
        ) : (
          <div className="space-y-2">
            {runtimes.map((rt: Runtime) => (
              <div key={rt.version} className="flex items-center justify-between p-4 bg-bg rounded-lg border border-border/50">
                <div className="flex items-center gap-3">
                  <div className={`w-9 h-9 rounded-lg flex items-center justify-center ${rt.active ? 'bg-accent/10 text-accent' : 'bg-bg text-text-muted'}`}>
                    <Cpu className="w-4 h-4" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-text">PHP {rt.version}</span>
                      {rt.active && (
                        <span className="px-1.5 py-0.5 bg-accent/10 text-accent text-[10px] font-medium rounded">Active</span>
                      )}
                    </div>
                    <span className="text-xs text-text-muted font-mono">{rt.binary}</span>
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <span className="flex items-center gap-1 text-xs text-text-muted">
                    {rt.managed ? <Wrench className="w-3 h-3" /> : null}
                    {rt.managed ? 'Managed' : 'System'}
                  </span>
                  <span className={`flex items-center gap-1 text-xs ${rt.status === 'ready' ? 'text-success' : 'text-error'}`}>
                    {rt.status === 'ready' ? <CheckCircle className="w-3 h-3" /> : <XCircle className="w-3 h-3" />}
                    {rt.status}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card title="About PHP Runtimes">
        <div className="text-sm text-text-secondary space-y-2">
          <p>Gateway scans common PHP-FPM binary locations at startup. Each site can be assigned a specific PHP version.</p>
          <p>To install additional runtimes, use your system package manager or the <code className="px-1.5 py-0.5 bg-bg rounded text-xs text-accent font-mono">gateway runtime install</code> command.</p>
        </div>
      </Card>
    </div>
  )
}
