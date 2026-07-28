import { useState, useEffect } from 'react'
import { Save, RotateCcw } from 'lucide-react'
import { Card } from '../components/StatCard'
import { Toast } from '../components/Modal'
import { useApi } from '../hooks/useApi'
import { api } from '../api/client'
import type { GatewayConfig } from '../types'

export default function ConfigPage() {
  const { data: config, loading, refetch } = useApi(() => api.getConfig(), [])
  const [local, setLocal] = useState<GatewayConfig | null>(null)
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' | 'info' } | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => { if (config) setLocal(structuredClone(config)) }, [config])

  const handleSave = async () => {
    if (!local) return
    setSaving(true)
    try {
      await api.saveConfig(local)
      setToast({ message: 'Configuration saved', type: 'success' })
      refetch()
    } catch (e) {
      setToast({ message: e instanceof Error ? e.message : 'Failed to save', type: 'error' })
    } finally {
      setSaving(false)
    }
  }

  if (loading || !local) {
    return <div className="flex items-center justify-center h-64 text-text-muted text-sm">Loading configuration...</div>
  }

  const update = (path: string, value: string | number) => {
    const keys = path.split('.')
    const next = structuredClone(local)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let obj: any = next
    for (let i = 0; i < keys.length - 1; i++) obj = obj[keys[i]]
    obj[keys[keys.length - 1]] = value
    setLocal(next)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text">Configuration</h2>
          <p className="text-sm text-text-muted mt-0.5">Edit gateway configuration. Changes require a save to take effect.</p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => { if (config) setLocal(structuredClone(config)) }} className="flex items-center gap-1.5 px-3 py-2 text-sm text-text-secondary hover:text-text rounded-lg hover:bg-surface-hover transition-colors">
            <RotateCcw className="w-3.5 h-3.5" />
            Reset
          </button>
          <button onClick={handleSave} disabled={saving} className="flex items-center gap-1.5 px-4 py-2 bg-accent hover:bg-accent-hover text-black text-sm font-medium rounded-lg transition-colors disabled:opacity-40">
            <Save className="w-3.5 h-3.5" />
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card title="Server">
          <div className="space-y-3">
            <Field label="Listen Address" value={local.server.addr} onChange={(v) => update('server.addr', v)} />
            <Field label="Read Timeout" value={local.server.read_timeout} onChange={(v) => update('server.read_timeout', v)} />
            <Field label="Write Timeout" value={local.server.write_timeout} onChange={(v) => update('server.write_timeout', v)} />
          </div>
        </Card>

        <Card title="PHP-FPM">
          <div className="space-y-3">
            <Field label="Binary Path" value={local.php.binary} onChange={(v) => update('php.binary', v)} mono />
            <Field label="Socket Path" value={local.php.socket_path} onChange={(v) => update('php.socket_path', v)} mono />
            <Field label="Max Children" type="number" value={String(local.php.max_children)} onChange={(v) => update('php.max_children', parseInt(v) || 0)} />
            <Field label="Request Timeout" value={local.php.request_timeout} onChange={(v) => update('php.request_timeout', v)} />
          </div>
        </Card>

        <Card title="Logging">
          <div className="space-y-3">
            <SelectField label="Format" value={local.logging.format} options={['json', 'text']} onChange={(v) => update('logging.format', v)} />
            <SelectField label="Level" value={local.logging.level} options={['debug', 'info', 'warn', 'error']} onChange={(v) => update('logging.level', v)} />
          </div>
        </Card>

        <Card title="Security">
          <div className="space-y-3">
            <SelectField label="Symlink Mode" value={local.security.symlink_mode} options={['within_root', 'deny']} onChange={(v) => update('security.symlink_mode', v)} />
            <Field label="Max Body Size" value={local.security.max_body_size} onChange={(v) => update('security.max_body_size', v)} />
            <div>
              <label className="block text-xs font-medium text-text-muted mb-1.5">Protected Patterns</label>
              <div className="flex flex-wrap gap-1.5">
                {(local.security.protected_patterns || []).map((p, i) => (
                  <span key={i} className="px-2 py-0.5 bg-bg border border-border rounded text-xs text-text-secondary font-mono">{p}</span>
                ))}
              </div>
            </div>
          </div>
        </Card>
      </div>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </div>
  )
}

function Field({ label, value, onChange, type = 'text', mono }: {
  label: string; value: string; onChange: (v: string) => void; type?: string; mono?: boolean
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-text-muted mb-1.5">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm text-text placeholder:text-text-muted focus:outline-none focus:border-accent transition-colors ${mono ? 'font-mono text-xs' : ''}`}
      />
    </div>
  )
}

function SelectField({ label, value, options, onChange }: {
  label: string; value: string; options: string[]; onChange: (v: string) => void
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-text-muted mb-1.5">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm text-text focus:outline-none focus:border-accent"
      >
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </div>
  )
}
