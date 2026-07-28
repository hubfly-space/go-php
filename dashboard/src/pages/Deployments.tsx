import { useState } from 'react'
import { Rocket, Plus, RotateCcw, CheckCircle, Clock, AlertCircle } from 'lucide-react'
import { Card } from '../components/StatCard'
import Modal, { Toast } from '../components/Modal'
import { useApi } from '../hooks/useApi'
import { api } from '../api/client'
import type { Release } from '../types'

export default function DeploymentsPage() {
  const { data, loading, refetch } = useApi(() => api.getDeployments(), [])
  const [modalOpen, setModalOpen] = useState(false)
  const [version, setVersion] = useState('1.0.1')
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' | 'info' } | null>(null)

  const releases = data?.releases || []

  const handleCreate = async () => {
    try {
      await api.createDeployment(version)
      setToast({ message: `Deployment release v${version} created`, type: 'success' })
      setModalOpen(false)
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Deployment failed', type: 'error' })
    }
  }

  const handleActivate = async (id: string) => {
    try {
      await api.activateDeployment(id)
      setToast({ message: `Release ${id} activated successfully`, type: 'success' })
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Activation failed', type: 'error' })
    }
  }

  const handleRollback = async () => {
    try {
      await api.rollbackDeployment()
      setToast({ message: 'Rolled back to previous active release', type: 'success' })
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Rollback failed', type: 'error' })
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Zero-Downtime Deployments</h2>
          <p className="text-sm text-text-muted mt-0.5">Manage immutable release snapshots and atomic symlink switches.</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleRollback}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-text-secondary hover:text-text bg-surface border border-border hover:bg-surface-hover rounded-lg transition-colors"
          >
            <RotateCcw className="w-4 h-4 text-warning" />
            Rollback
          </button>
          <button
            onClick={() => setModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-lg transition-colors"
          >
            <Plus className="w-4 h-4" />
            New Release
          </button>
        </div>
      </div>

      <Card title={`${releases.length} Release${releases.length !== 1 ? 's' : ''}`}>
        {loading ? (
          <p className="text-sm text-text-muted py-8 text-center">Loading releases...</p>
        ) : releases.length === 0 ? (
          <div className="text-center py-12">
            <Rocket className="w-10 h-10 text-text-muted mx-auto mb-3" />
            <p className="text-sm text-text-secondary">No deployments yet</p>
            <p className="text-xs text-text-muted mt-1">Create your first zero-downtime release snapshot</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Release ID</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Version</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">State</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Created At</th>
                  <th className="text-right py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody>
                {releases.map((rel: Release) => (
                  <tr key={rel.id} className="border-b border-border/50 last:border-0 hover:bg-surface-hover/50 transition-colors">
                    <td className="py-3 px-3 font-mono text-xs text-text font-medium">{rel.id}</td>
                    <td className="py-3 px-3">
                      <span className="px-2 py-0.5 bg-accent/10 text-accent font-mono text-xs rounded">{rel.version}</span>
                    </td>
                    <td className="py-3 px-3">
                      <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium ${
                        rel.state === 'active' ? 'bg-success/10 text-success' :
                        rel.state === 'failed' ? 'bg-error/10 text-error' :
                        'bg-bg text-text-muted'
                      }`}>
                        {rel.state === 'active' ? <CheckCircle className="w-3 h-3" /> : <Clock className="w-3 h-3" />}
                        {rel.state}
                      </span>
                    </td>
                    <td className="py-3 px-3 text-xs text-text-muted">
                      {new Date(rel.created_at).toLocaleString()}
                    </td>
                    <td className="py-3 px-3 text-right">
                      {rel.state !== 'active' && (
                        <button
                          onClick={() => handleActivate(rel.id)}
                          className="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs font-medium rounded transition-colors"
                        >
                          Activate
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Create New Release">
        <div className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-text-muted mb-1.5">Release Version</label>
            <input
              type="text"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder="1.0.1"
              className="w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm text-text focus:outline-none focus:border-accent"
            />
          </div>
          <div className="p-3 bg-bg rounded-lg border border-border text-xs text-text-muted flex items-start gap-2">
            <AlertCircle className="w-4 h-4 text-info shrink-0 mt-0.5" />
            <span>This will snapshot current application files into an immutable release directory. You can activate it immediately or later.</span>
          </div>
          <div className="flex justify-end gap-3 pt-2">
            <button onClick={() => setModalOpen(false)} className="px-4 py-2 text-sm text-text-secondary hover:text-text rounded-lg">Cancel</button>
            <button onClick={handleCreate} className="px-4 py-2 text-sm font-medium bg-accent hover:bg-accent-hover text-white rounded-lg">Create Release</button>
          </div>
        </div>
      </Modal>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </div>
  )
}
