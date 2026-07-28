import { useState } from 'react'
import { Search, ArrowRight, CheckCircle2 } from 'lucide-react'
import { Card } from '../components/StatCard'
import { useApi } from '../hooks/useApi'
import { api } from '../api/client'

export default function RoutesPage() {
  const { data: config } = useApi(() => api.getConfig(), [])
  const [testUrl, setTestUrl] = useState('/index.php')
  const [testResult, setTestResult] = useState<import('../types').Explanation | null>(null)
  const [testing, setTesting] = useState(false)

  const routes = config?.routes || [
    { host: '*', path: '/*', target: 'index.php', type: 'php_front_controller' },
    { host: '*', path: '/assets/*', target: 'public/assets', type: 'static' }
  ]

  const handleTest = async () => {
    setTesting(true)
    try {
      const res = await api.explainRequest(testUrl)
      setTestResult(res)
    } catch {
      setTestResult(null)
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-text">Route Inspector & Tester</h2>
        <p className="text-sm text-text-muted mt-0.5">View configured routes and test request evaluation rules.</p>
      </div>

      {/* Interactive Tester */}
      <Card title="Interactive Route Tester">
        <div className="space-y-4">
          <div className="flex gap-2">
            <div className="relative flex-1">
              <Search className="w-4 h-4 text-text-muted absolute left-3 top-3" />
              <input
                type="text"
                value={testUrl}
                onChange={(e) => setTestUrl(e.target.value)}
                placeholder="/api/users or http://localhost/blog"
                className="w-full bg-bg border border-border rounded-lg pl-9 pr-3 py-2 text-sm text-text font-mono focus:outline-none focus:border-accent"
              />
            </div>
            <button
              onClick={handleTest}
              disabled={testing}
              className="flex items-center gap-2 px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50"
            >
              Test Route
              <ArrowRight className="w-4 h-4" />
            </button>
          </div>

          {testResult && (
            <div className="p-4 bg-bg rounded-lg border border-border space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-text uppercase tracking-wider">Evaluation Trace Result</span>
                <span className="px-2 py-0.5 bg-success/10 text-success text-xs font-medium rounded flex items-center gap-1">
                  <CheckCircle2 className="w-3 h-3" />
                  {testResult.summary}
                </span>
              </div>
              <div className="grid grid-cols-2 gap-4 text-xs font-mono text-text-secondary">
                <div>
                  <span className="text-text-muted block">Normalized Path:</span>
                  <span className="text-text">{testResult.path_normalization.normalized}</span>
                </div>
                <div>
                  <span className="text-text-muted block">Policy Decision:</span>
                  <span className="text-success">{testResult.policy_check.decision}</span>
                </div>
              </div>
            </div>
          )}
        </div>
      </Card>

      {/* Configured Routes Table */}
      <Card title={`Configured Routes (${routes.length})`}>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Host</th>
                <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Path Pattern</th>
                <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Target</th>
                <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Type</th>
              </tr>
            </thead>
            <tbody>
              {routes.map((r: Record<string, string>, i: number) => (
                <tr key={i} className="border-b border-border/50 last:border-0 hover:bg-surface-hover/50 transition-colors">
                  <td className="py-3 px-3 font-mono text-xs text-text">{r.host || '*'}</td>
                  <td className="py-3 px-3 font-mono text-xs text-accent">{r.path || r.path_prefix || '/*'}</td>
                  <td className="py-3 px-3 font-mono text-xs text-text-secondary">{r.target}</td>
                  <td className="py-3 px-3">
                    <span className="px-2 py-0.5 bg-surface border border-border text-xs text-text-muted rounded">
                      {r.type || 'front_controller'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}
