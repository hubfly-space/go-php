import { useState, useEffect } from 'react'
import { Puzzle, CheckCircle, XCircle, RefreshCw } from 'lucide-react'
import { Card } from '../components/StatCard'

const ALL_EXTENSIONS = [
  { name: 'curl', type: 'extension', description: 'cURL client library', version: 'built-in' },
  { name: 'json', type: 'extension', description: 'JSON serialization support', version: 'built-in' },
  { name: 'mbstring', type: 'extension', description: 'Multibyte string functions', version: 'built-in' },
  { name: 'pdo', type: 'extension', description: 'PHP Data Objects interface', version: 'built-in' },
  { name: 'pdo_sqlite', type: 'extension', description: 'PDO driver for SQLite', version: 'built-in' },
  { name: 'pdo_mysql', type: 'extension', description: 'PDO driver for MySQL', version: 'built-in' },
  { name: 'pdo_pgsql', type: 'extension', description: 'PDO driver for PostgreSQL', version: 'built-in' },
  { name: 'opcache', type: 'extension', description: 'OPcache bytecode cache', version: 'built-in' },
  { name: 'intl', type: 'extension', description: 'Internationalization extension', version: 'built-in' },
  { name: 'xml', type: 'extension', description: 'XML parsing and processing', version: 'built-in' },
  { name: 'gd', type: 'extension', description: 'Image processing library', version: 'built-in' },
  { name: 'mysqlnd', type: 'extension', description: 'MySQL native driver', version: 'built-in' },
  { name: 'redis', type: 'extension', description: 'Redis key-value store client', version: 'built-in' },
  { name: 'memcached', type: 'extension', description: 'Memcached client library', version: 'built-in' },
  { name: 'xdebug', type: 'zend_extension', description: 'PHP debugger and profiler', version: 'built-in' },
  { name: 'imagick', type: 'extension', description: 'ImageMagick image processing', version: 'built-in' },
  { name: 'zip', type: 'extension', description: 'ZIP file management', version: 'built-in' },
  { name: 'bz2', type: 'extension', description: 'BZip2 compression support', version: 'built-in' },
  { name: 'bcmath', type: 'extension', description: 'Arbitrary precision mathematics', version: 'built-in' },
  { name: 'gmp', type: 'extension', description: 'GNU Multiple Precision arithmetic', version: 'built-in' },
  { name: 'sockets', type: 'extension', description: 'Socket communication', version: 'built-in' },
]

const BUILT_IN_PROFILES = [
  { name: 'minimal', description: 'Minimal set for basic PHP scripts', extensions: ['curl', 'json', 'mbstring', 'pdo', 'pdo_sqlite', 'opcache', 'xml'] },
  { name: 'web-standard', description: 'Standard set for most web applications', extensions: ['curl', 'json', 'mbstring', 'pdo', 'pdo_sqlite', 'pdo_mysql', 'pdo_pgsql', 'opcache', 'intl', 'xml', 'gd', 'zip', 'bcmath'] },
  { name: 'wordpress', description: 'Optimized for WordPress', extensions: ['curl', 'json', 'mbstring', 'pdo', 'pdo_mysql', 'mysqlnd', 'opcache', 'intl', 'xml', 'gd', 'zip', 'bcmath', 'sockets'] },
  { name: 'laravel', description: 'Optimized for Laravel', extensions: ['curl', 'json', 'mbstring', 'pdo', 'pdo_mysql', 'pdo_pgsql', 'opcache', 'intl', 'xml', 'gd', 'zip', 'bcmath', 'sockets'] },
  { name: 'development', description: 'Full feature set for development environments', extensions: ['curl', 'json', 'mbstring', 'pdo', 'pdo_sqlite', 'pdo_mysql', 'pdo_pgsql', 'opcache', 'intl', 'xml', 'gd', 'zip', 'bcmath', 'xdebug', 'imagick', 'memcached', 'redis'] },
]

export default function ExtensionsPage() {
  const [selectedProfile, setSelectedProfile] = useState('web-standard')
  const [enabledExts, setEnabledExts] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    const profile = BUILT_IN_PROFILES.find(p => p.name === selectedProfile)
    if (profile) {
      setEnabledExts(profile.extensions)
    }
  }, [selectedProfile])

  const handleToggle = (name: string) => {
    setEnabledExts(prev =>
      prev.includes(name) ? prev.filter(e => e !== name) : [...prev, name]
    )
  }

  const handleSave = async () => {
    setSaving(true)
    setSaved(false)
    try {
      await new Promise(r => setTimeout(r, 300))
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold text-text">PHP Extensions</h2>
        <p className="text-sm text-text-muted mt-0.5">Manage PHP extension profiles and individual extension toggles.</p>
      </div>

      <Card title="Extension Profile">
        <div className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-text-muted mb-1.5">Profile</label>
            <select
              value={selectedProfile}
              onChange={(e) => setSelectedProfile(e.target.value)}
              className="w-full max-w-xs bg-bg border border-border rounded-lg px-3 py-2 text-sm text-text focus:outline-none focus:border-accent"
            >
              {BUILT_IN_PROFILES.map(p => (
                <option key={p.name} value={p.name}>{p.name}</option>
              ))}
            </select>
            <p className="text-xs text-text-muted mt-1">
              {BUILT_IN_PROFILES.find(p => p.name === selectedProfile)?.description}
            </p>
          </div>

          <div className="flex items-center gap-3 pt-2">
            <button
              onClick={handleSave}
              disabled={saving}
              className="px-4 py-2 text-sm font-medium bg-accent hover:bg-accent-hover text-black rounded-lg transition-colors disabled:opacity-40 flex items-center gap-2"
            >
              {saving ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : null}
              Apply Profile
            </button>
            {saved && (
              <span className="text-xs text-success flex items-center gap-1">
                <CheckCircle className="w-3.5 h-3.5" /> Profile applied
              </span>
            )}
          </div>
        </div>
      </Card>

      <Card title={`Extensions (${enabledExts.length} of ${ALL_EXTENSIONS.length} enabled)`}>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {ALL_EXTENSIONS.map(ext => {
            const isEnabled = enabledExts.includes(ext.name)
            return (
              <button
                key={ext.name}
                onClick={() => handleToggle(ext.name)}
                className={`flex items-center gap-3 p-3 rounded-lg border text-left transition-colors ${
                  isEnabled
                    ? 'border-accent/30 bg-accent/5'
                    : 'border-border/50 bg-bg hover:border-border-light'
                }`}
              >
                <div className={`shrink-0 w-8 h-8 rounded-lg flex items-center justify-center ${
                  isEnabled ? 'bg-accent/10 text-accent' : 'bg-surface text-text-muted'
                }`}>
                  <Puzzle className="w-4 h-4" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className={`text-sm font-medium ${isEnabled ? 'text-text' : 'text-text-secondary'}`}>
                      {ext.name}
                    </span>
                    {ext.type === 'zend_extension' && (
                      <span className="px-1 py-0.5 bg-warning/10 text-warning text-[10px] font-medium rounded">Zend</span>
                    )}
                  </div>
                  <p className="text-xs text-text-muted truncate">{ext.description}</p>
                </div>
                {isEnabled ? (
                  <CheckCircle className="w-4 h-4 text-success shrink-0" />
                ) : (
                  <XCircle className="w-4 h-4 text-text-muted/40 shrink-0" />
                )}
              </button>
            )
          })}
        </div>
      </Card>
    </div>
  )
}
