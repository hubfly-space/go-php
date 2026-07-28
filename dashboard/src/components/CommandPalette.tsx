import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router'
import {
  Search,
  LayoutDashboard,
  Globe,
  Rocket,
  GitFork,
  Cpu,
  Settings,
  ScrollText,
  Activity,
  Compass,
  Moon,
  Sun,
  X,
} from 'lucide-react'

interface CommandPaletteProps {
  open: boolean
  onClose: () => void
  darkMode: boolean
  onToggleTheme: () => void
}

export default function CommandPalette({ open, onClose, darkMode, onToggleTheme }: CommandPaletteProps) {
  const [query, setQuery] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        if (open) onClose()
        else {
          setQuery('')
          // trigger open from parent
        }
      }
      if (e.key === 'Escape' && open) {
        onClose()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  if (!open) return null

  const items = [
    { label: 'Go to Dashboard', icon: LayoutDashboard, action: () => navigate('/') },
    { label: 'Manage Sites', icon: Globe, action: () => navigate('/sites') },
    { label: 'Deployments & Releases', icon: Rocket, action: () => navigate('/deployments') },
    { label: 'Route Inspector', icon: GitFork, action: () => navigate('/routes') },
    { label: 'PHP Runtimes', icon: Cpu, action: () => navigate('/runtimes') },
    { label: 'Configuration', icon: Settings, action: () => navigate('/config') },
    { label: 'System Logs', icon: ScrollText, action: () => navigate('/logs') },
    { label: 'System Doctor & Health', icon: Activity, action: () => navigate('/doctor') },
    { label: 'Request Decision Explorer', icon: Compass, action: () => navigate('/explorer') },
    { label: darkMode ? 'Switch to Light Theme' : 'Switch to Dark Theme', icon: darkMode ? Sun : Moon, action: onToggleTheme },
  ]

  const filtered = items.filter((i) => i.label.toLowerCase().includes(query.toLowerCase()))

  const handleSelect = (action: () => void) => {
    action()
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-20 bg-black/40 backdrop-blur-xs p-4">
      <div className="w-full max-w-lg bg-surface border border-border rounded-xl shadow-2xl overflow-hidden animate-toast-slide-in">
        <div className="flex items-center px-4 border-b border-border">
          <Search className="w-4 h-4 text-text-muted shrink-0 mr-3" />
          <input
            autoFocus
            type="text"
            placeholder="Type a command or search..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full py-3 bg-transparent text-sm text-text placeholder:text-text-muted focus:outline-none"
          />
          <button onClick={onClose} className="text-text-muted hover:text-text p-1">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="max-h-72 overflow-y-auto p-2 space-y-1">
          {filtered.length === 0 ? (
            <div className="py-6 text-center text-xs text-text-muted">No matching commands found</div>
          ) : (
            filtered.map((item, i) => (
              <button
                key={i}
                onClick={() => handleSelect(item.action)}
                className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-text-secondary hover:text-text hover:bg-surface-hover transition-colors text-left"
              >
                <item.icon className="w-4 h-4 text-accent shrink-0" />
                <span>{item.label}</span>
              </button>
            ))
          )}
        </div>

        <div className="px-4 py-2 border-t border-border bg-bg/50 flex items-center justify-between text-[11px] text-text-muted">
          <span>Navigation shortcut</span>
          <kbd className="px-1.5 py-0.5 bg-surface border border-border rounded text-[10px] font-mono">ESC to close</kbd>
        </div>
      </div>
    </div>
  )
}
