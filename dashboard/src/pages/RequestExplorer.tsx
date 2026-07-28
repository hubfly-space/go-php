import { useState } from 'react'
import { Compass, Search, ArrowRight, ShieldCheck } from 'lucide-react'
import { Card } from '../components/StatCard'
import { api } from '../api/client'
import type { Explanation } from '../types'

export default function RequestExplorerPage() {
  const [url, setUrl] = useState('/index.php')
  const [result, setResult] = useState<Explanation | null>(null)
  const [loading, setLoading] = useState(false)

  const handleTrace = async () => {
    setLoading(true)
    try {
      const res = await api.explainRequest(url)
      setResult(res)
    } catch {
      setResult(null)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-text">Request Decision Explorer</h2>
        <p className="text-sm text-text-muted mt-0.5">Trace any request URL through the 5-step gateway decision pipeline.</p>
      </div>

      <Card title="Inspect Request Execution">
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Search className="w-4 h-4 text-text-muted absolute left-3 top-3" />
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="/index.php or /api/v1/users"
              className="w-full bg-bg border border-border rounded-lg pl-9 pr-3 py-2 text-sm text-text font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <button
            onClick={handleTrace}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50"
          >
            Trace Pipeline
            <ArrowRight className="w-4 h-4" />
          </button>
        </div>
      </Card>

      {result && (
        <div className="space-y-4">
          <div className="flex items-center justify-between p-4 bg-surface border border-border rounded-xl">
            <div className="flex items-center gap-3">
              <Compass className="w-6 h-6 text-accent" />
              <div>
                <h3 className="text-sm font-semibold text-text">Decision Pipeline Summary</h3>
                <p className="text-xs text-text-muted">Total execution time: {result.duration}</p>
              </div>
            </div>
            <span className="px-3 py-1 bg-success/10 text-success text-xs font-semibold rounded-full">
              {result.summary}
            </span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Step 1: Normalization */}
            <Card title="1. Path Normalization">
              <div className="space-y-2 text-xs font-mono">
                <div className="flex justify-between border-b border-border/40 pb-1">
                  <span className="text-text-muted">Raw Target:</span>
                  <span className="text-text">{result.path_normalization.raw}</span>
                </div>
                <div className="flex justify-between border-b border-border/40 pb-1">
                  <span className="text-text-muted">Normalized:</span>
                  <span className="text-accent">{result.path_normalization.normalized}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-text-muted">Status:</span>
                  <span className={result.path_normalization.valid ? 'text-success' : 'text-error'}>
                    {result.path_normalization.valid ? 'Valid (No Traversal)' : 'Rejected'}
                  </span>
                </div>
              </div>
            </Card>

            {/* Step 2: Policy */}
            <Card title="2. Policy Inspection">
              <div className="space-y-2 text-xs font-mono">
                <div className="flex justify-between border-b border-border/40 pb-1">
                  <span className="text-text-muted">WAF Decision:</span>
                  <span className="text-success font-semibold flex items-center gap-1">
                    <ShieldCheck className="w-3.5 h-3.5" />
                    {result.policy_check.decision}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-text-muted">Matched Rules:</span>
                  <span className="text-text">{result.policy_check.matched_rules?.length || 0} rules</span>
                </div>
              </div>
            </Card>

            {/* Step 3: File Check */}
            <Card title="3. Filesystem Resolution">
              <div className="space-y-2 text-xs font-mono">
                <div className="flex justify-between border-b border-border/40 pb-1">
                  <span className="text-text-muted">Protected File Check:</span>
                  <span className={result.file_check.protected ? 'text-error' : 'text-success'}>
                    {result.file_check.protected ? 'Protected (Denied)' : 'Allowed'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-text-muted">File Target Found:</span>
                  <span className="text-text">{String(result.file_check.found)}</span>
                </div>
              </div>
            </Card>

            {/* Step 4: Script Resolution */}
            <Card title="4. FastCGI Script Target">
              <div className="space-y-2 text-xs font-mono">
                <div className="flex justify-between border-b border-border/40 pb-1">
                  <span className="text-text-muted">SCRIPT_NAME:</span>
                  <span className="text-accent">{result.script_check.script_name}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-text-muted">SCRIPT_FILENAME:</span>
                  <span className="text-text break-all">{result.script_check.script_path}</span>
                </div>
              </div>
            </Card>
          </div>
        </div>
      )}
    </div>
  )
}
