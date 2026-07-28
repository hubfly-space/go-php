import { CheckCircle, AlertTriangle, XCircle, RefreshCw } from 'lucide-react'
import { Card } from '../components/StatCard'
import { useApi } from '../hooks/useApi'
import { api } from '../api/client'

export default function DoctorPage() {
  const { data: docReport, loading: docLoading, refetch: refetchDoc } = useApi(() => api.getDoctor(), [])
  const { data: compatReport } = useApi(() => api.getDoctorCompat(), [])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Diagnostics & System Doctor</h2>
          <p className="text-sm text-text-muted mt-0.5">Automated environment checks and framework compatibility analysis.</p>
        </div>
        <button
          onClick={() => refetchDoc()}
          className="flex items-center gap-2 px-3 py-2 text-sm text-text-secondary hover:text-text bg-surface border border-border hover:bg-surface-hover rounded-lg transition-colors"
        >
          <RefreshCw className="w-4 h-4" />
          Re-run Doctor
        </button>
      </div>

      {/* System Health Checks */}
      <Card title="System Readiness Checks">
        {docLoading ? (
          <p className="text-sm text-text-muted py-8 text-center">Running system doctor checks...</p>
        ) : (
          <div className="space-y-3">
            {(docReport?.checks || []).map((check, i) => (
              <div key={i} className="flex items-center justify-between p-3.5 bg-bg rounded-lg border border-border/60">
                <div className="flex items-center gap-3">
                  {check.status === 'ok' && <CheckCircle className="w-5 h-5 text-success shrink-0" />}
                  {check.status === 'warn' && <AlertTriangle className="w-5 h-5 text-warning shrink-0" />}
                  {check.status === 'fail' && <XCircle className="w-5 h-5 text-error shrink-0" />}
                  <div>
                    <div className="text-sm font-medium text-text">{check.name}</div>
                    <div className="text-xs text-text-muted">{check.message}</div>
                  </div>
                </div>
                <span className={`px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase ${
                  check.status === 'ok' ? 'bg-success/10 text-success' :
                  check.status === 'warn' ? 'bg-warning/10 text-warning' :
                  'bg-error/10 text-error'
                }`}>
                  {check.status}
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Framework Compatibility Scan */}
      {compatReport && (
        <Card title="Application Compatibility Scanner">
          <div className="space-y-4">
            <div className="flex items-center justify-between p-4 bg-bg rounded-lg border border-border">
              <div>
                <span className="text-xs text-text-muted block">Detected Framework</span>
                <span className="text-base font-semibold text-text">{compatReport.framework || 'Plain PHP'}</span>
              </div>
              <div className="text-right">
                <span className="text-xs text-text-muted block">Compatibility Score</span>
                <span className="text-2xl font-bold text-accent">{compatReport.score}/100</span>
              </div>
            </div>

            {compatReport.warnings && compatReport.warnings.length > 0 && (
              <div className="space-y-2">
                <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider">Recommendations</h4>
                {compatReport.warnings.map((w, i) => (
                  <div key={i} className="p-3 bg-bg rounded-lg border border-border text-xs space-y-1">
                    <div className="font-medium text-text flex items-center gap-2">
                      <AlertTriangle className="w-3.5 h-3.5 text-warning" />
                      [{w.category}] {w.message}
                    </div>
                    {w.suggestion && (
                      <div className="text-text-muted pl-5 font-sans">{w.suggestion}</div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>
      )}
    </div>
  )
}
