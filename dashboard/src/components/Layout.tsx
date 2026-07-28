import { useState, useEffect } from 'react'
import { Outlet, useLocation, Link } from 'react-router'
import {
  LayoutDashboard,
  Globe,
  Rocket,
  GitFork,
  Cpu,
  Settings,
  ScrollText,
  Activity,
  Compass,
  Menu,
  X,
  Server,
  Sun,
  Moon,
  Search,
} from 'lucide-react'
import CommandPalette from './CommandPalette'
import { useApi, useInterval } from '../hooks/useApi'
import { api } from '../api/client'

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/sites', icon: Globe, label: 'Sites' },
  { to: '/deployments', icon: Rocket, label: 'Deployments' },
  { to: '/routes', icon: GitFork, label: 'Routes' },
  { to: '/runtimes', icon: Cpu, label: 'Runtimes' },
  { to: '/config', icon: Settings, label: 'Configuration' },
  { to: '/logs', icon: ScrollText, label: 'Logs' },
  { to: '/doctor', icon: Activity, label: 'Diagnostics' },
  { to: '/explorer', icon: Compass, label: 'Explorer' },
]

export default function Layout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [darkMode, setDarkMode] = useState(() => localStorage.getItem('theme') === 'dark')
  const location = useLocation()

  const { data: health, refetch } = useApi(() => api.healthCheck(), [])
  useInterval(() => { refetch() }, 10000)

  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('theme', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('theme', 'light')
    }
  }, [darkMode])

  const toggleTheme = () => setDarkMode(!darkMode)

  return (
    <div className="flex h-screen overflow-hidden bg-bg text-text">
      {sidebarOpen && (
        <div
          className="fixed inset-0 bg-black/40 z-40 lg:hidden backdrop-blur-xs"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <aside
        className={`fixed lg:static inset-y-0 left-0 z-50 w-64 bg-surface border-r border-border flex flex-col transform transition-transform duration-200 ease-out ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'
        }`}
      >
        <div className="flex items-center gap-3 px-5 h-16 border-b border-border shrink-0">
          <div className="w-8 h-8 rounded-lg bg-accent/10 flex items-center justify-center">
            <Server className="w-4 h-4 text-accent" />
          </div>
          <div>
            <span className="text-sm font-semibold tracking-tight text-text">Go-PHP Gateway</span>
            <span className="text-[10px] text-text-muted block leading-tight">Management Console</span>
          </div>
          <button
            onClick={() => setSidebarOpen(false)}
            className="ml-auto lg:hidden text-text-muted hover:text-text"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
          {navItems.map((item) => {
            const active = location.pathname === item.to ||
              (item.to !== '/' && location.pathname.startsWith(item.to))
            return (
              <Link
                key={item.to}
                to={item.to}
                onClick={() => setSidebarOpen(false)}
                className={`flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                  active
                    ? 'bg-accent/10 text-accent'
                    : 'text-text-secondary hover:text-text hover:bg-surface-hover'
                }`}
              >
                <item.icon className="w-4 h-4 shrink-0" />
                {item.label}
              </Link>
            )
          })}
        </nav>

        <div className="px-4 py-3 border-t border-border flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className={`w-2 h-2 rounded-full ${health?.status === 'ok' ? 'bg-success' : 'bg-error'}`} />
            <span className="text-xs text-text-muted">
              {health?.status === 'ok' ? 'Connected' : 'Offline'}
            </span>
          </div>
          <button
            onClick={toggleTheme}
            className="p-1.5 rounded-lg text-text-muted hover:text-text hover:bg-surface-hover transition-colors"
            title={darkMode ? 'Switch to Light Mode' : 'Switch to Dark Mode'}
          >
            {darkMode ? <Sun className="w-4 h-4 text-warning" /> : <Moon className="w-4 h-4 text-text-secondary" />}
          </button>
        </div>
      </aside>

      <main className="flex-1 flex flex-col overflow-hidden">
        <header className="h-16 border-b border-border flex items-center justify-between px-4 lg:px-6 shrink-0 bg-surface/80 backdrop-blur-md">
          <div className="flex items-center gap-3">
            <button
              onClick={() => setSidebarOpen(true)}
              className="lg:hidden text-text-secondary hover:text-text"
            >
              <Menu className="w-5 h-5" />
            </button>
            <h1 className="text-base font-semibold text-text">
              {navItems.find((n) => n.to === location.pathname)?.label || 'Dashboard'}
            </h1>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={() => setPaletteOpen(true)}
              className="flex items-center gap-2 px-3 py-1.5 bg-bg border border-border rounded-lg text-xs text-text-muted hover:text-text hover:border-border-light transition-colors"
            >
              <Search className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Quick Search...</span>
              <kbd className="hidden sm:inline px-1 py-0.2 bg-surface border border-border rounded text-[10px] font-mono">⌘K</kbd>
            </button>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto p-4 lg:p-6">
          <Outlet />
        </div>
      </main>

      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        darkMode={darkMode}
        onToggleTheme={toggleTheme}
      />
    </div>
  )
}
