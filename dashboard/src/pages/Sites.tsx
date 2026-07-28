import { useState } from 'react'
import { Plus, Pencil, Trash2, ExternalLink, Square, Play } from 'lucide-react'
import { Card } from '../components/StatCard'
import Modal, { Toast } from '../components/Modal'
import { useApi } from '../hooks/useApi'
import { api } from '../api/client'
import type { Site, SiteCreateRequest } from '../types'

export default function SitesPage() {
  const { data, loading, refetch } = useApi(() => api.getSites(), [])
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Site | null>(null)
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' | 'info' } | null>(null)
  const [form, setForm] = useState<SiteCreateRequest>({ name: '', port: 8081, webroot: '', php_version: '8.3' })

  const resetForm = () => {
    setForm({ name: '', port: nextPort(data?.sites || []), webroot: '', php_version: '8.3' })
    setEditing(null)
  }

  const openCreate = () => {
    resetForm()
    setForm(f => ({ ...f, port: nextPort(data?.sites || []) }))
    setModalOpen(true)
  }

  const openEdit = (site: Site) => {
    setEditing(site)
    setForm({ name: site.name, port: site.port, webroot: site.webroot, php_version: site.php_version })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    try {
      if (editing) {
        await api.updateSite(editing.id, { ...editing, ...form, updated_at: new Date().toISOString() })
        setToast({ message: `Site "${form.name}" updated`, type: 'success' })
      } else {
        await api.createSite(form)
        setToast({ message: `Site "${form.name}" created on port ${form.port}`, type: 'success' })
      }
      setModalOpen(false)
      resetForm()
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Failed', type: 'error' })
    }
  }

  const handleDelete = async (site: Site) => {
    if (!confirm(`Delete site "${site.name}"?`)) return
    try {
      await api.deleteSite(site.id)
      setToast({ message: `Site "${site.name}" deleted`, type: 'success' })
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Failed', type: 'error' })
    }
  }

  const handleToggle = async (site: Site) => {
    const newStatus = site.status === 'active' ? 'stopped' : 'active'
    try {
      await api.updateSite(site.id, { ...site, status: newStatus, updated_at: new Date().toISOString() })
      setToast({ message: `Site "${site.name}" ${newStatus === 'active' ? 'started' : 'stopped'}`, type: 'success' })
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Failed', type: 'error' })
    }
  }

  const sites = data?.sites || []

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Sites</h2>
          <p className="text-sm text-text-muted mt-0.5">Manage your PHP sites. Each site runs on its own port with a dedicated webroot.</p>
        </div>
        <button onClick={openCreate} className="flex items-center gap-2 px-4 py-2 bg-accent hover:bg-accent-hover text-black text-sm font-medium rounded-lg transition-colors">
          <Plus className="w-4 h-4" />
          New Site
        </button>
      </div>

      <Card title={`${sites.length} site${sites.length !== 1 ? 's' : ''}`}>
        {loading ? (
          <p className="text-sm text-text-muted py-8 text-center">Loading sites...</p>
        ) : sites.length === 0 ? (
          <div className="text-center py-12">
            <Globe className="w-10 h-10 text-text-muted mx-auto mb-3" />
            <p className="text-sm text-text-secondary">No sites yet</p>
            <p className="text-xs text-text-muted mt-1">Create your first site to get started</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Name</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Port</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Webroot</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">PHP</th>
                  <th className="text-left py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Status</th>
                  <th className="text-right py-3 px-3 text-xs font-medium text-text-muted uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody>
                {sites.map((site) => (
                  <tr key={site.id} className="border-b border-border/50 last:border-0 hover:bg-surface-hover/50 transition-colors">
                    <td className="py-3 px-3">
                      <div className="font-medium text-text">{site.name}</div>
                      <div className="text-xs text-text-muted">{site.id}</div>
                    </td>
                    <td className="py-3 px-3">
                      <code className="px-2 py-0.5 bg-bg rounded text-xs text-accent font-mono">{site.port}</code>
                    </td>
                    <td className="py-3 px-3 font-mono text-xs text-text-secondary break-all max-w-[200px] truncate" title={site.webroot}>
                      {site.webroot}
                    </td>
                    <td className="py-3 px-3 text-text-secondary">{site.php_version}</td>
                    <td className="py-3 px-3">
                      <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
                        site.status === 'active' ? 'bg-success/10 text-success' :
                        site.status === 'error' ? 'bg-error/10 text-error' :
                        'bg-bg text-text-muted'
                      }`}>
                        <span className={`w-1.5 h-1.5 rounded-full ${
                          site.status === 'active' ? 'bg-success' :
                          site.status === 'error' ? 'bg-error' :
                          'bg-text-muted'
                        }`} />
                        {site.status}
                      </span>
                    </td>
                    <td className="py-3 px-3">
                      <div className="flex items-center justify-end gap-1">
                        <a
                          href={`http://127.0.0.1:${site.port}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="p-1.5 rounded-md text-text-muted hover:text-info hover:bg-bg transition-colors"
                          title="Open site"
                        >
                          <ExternalLink className="w-3.5 h-3.5" />
                        </a>
                        <button
                          onClick={() => handleToggle(site)}
                          className="p-1.5 rounded-md text-text-muted hover:text-accent hover:bg-bg transition-colors"
                          title={site.status === 'active' ? 'Stop' : 'Start'}
                        >
                          {site.status === 'active' ? <Square className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
                        </button>
                        <button
                          onClick={() => openEdit(site)}
                          className="p-1.5 rounded-md text-text-muted hover:text-text-secondary hover:bg-bg transition-colors"
                          title="Edit"
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => handleDelete(site)}
                          className="p-1.5 rounded-md text-text-muted hover:text-error hover:bg-bg transition-colors"
                          title="Delete"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal open={modalOpen} onClose={() => { setModalOpen(false); resetForm() }} title={editing ? 'Edit Site' : 'Create Site'} wide>
        <div className="space-y-4">
          <Field label="Site Name" value={form.name} onChange={(v) => setForm({ ...form, name: v })} placeholder="my-app" />
          <div className="grid grid-cols-2 gap-4">
            <Field label="Port" type="number" value={String(form.port)} onChange={(v) => setForm({ ...form, port: parseInt(v) || 0 })} placeholder="8081" />
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1.5">PHP Version</label>
              <select
                value={form.php_version}
                onChange={(e) => setForm({ ...form, php_version: e.target.value })}
                className="w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm text-text focus:outline-none focus:border-accent"
              >
                <option value="8.3">PHP 8.3</option>
                <option value="8.2">PHP 8.2</option>
                <option value="8.1">PHP 8.1</option>
                <option value="8.0">PHP 8.0</option>
              </select>
            </div>
          </div>
          <Field label="Webroot Directory" value={form.webroot} onChange={(v) => setForm({ ...form, webroot: v })} placeholder="/var/www/my-app" hint="The content directory for this site. If empty, a directory will be created automatically." />
          <div className="flex justify-end gap-3 pt-2">
            <button onClick={() => { setModalOpen(false); resetForm() }} className="px-4 py-2 text-sm text-text-secondary hover:text-text rounded-lg hover:bg-surface-hover transition-colors">
              Cancel
            </button>
            <button onClick={handleSubmit} disabled={!form.name || !form.port} className="px-4 py-2 text-sm font-medium bg-accent hover:bg-accent-hover text-black rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed">
              {editing ? 'Save Changes' : 'Create Site'}
            </button>
          </div>
        </div>
      </Modal>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </div>
  )
}

function Field({ label, value, onChange, placeholder, type = 'text', hint }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string; hint?: string
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-text-muted mb-1.5">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm text-text placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors"
      />
      {hint && <p className="text-[11px] text-text-muted mt-1">{hint}</p>}
    </div>
  )
}

function nextPort(sites: Site[]): number {
  if (sites.length === 0) return 8081
  const max = Math.max(...sites.map((s) => s.port))
  return max + 1
}

function Globe(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <circle cx="12" cy="12" r="10" /><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" /><path d="M2 12h20" />
    </svg>
  )
}
