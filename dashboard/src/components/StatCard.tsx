import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

interface StatCardProps {
  label: string
  value: string | number
  icon: LucideIcon
  trend?: string
  color?: 'accent' | 'success' | 'error' | 'info'
}

const colorMap = {
  accent: 'text-accent bg-accent/10',
  success: 'text-success bg-success/10',
  error: 'text-error bg-error/10',
  info: 'text-info bg-info/10',
}

export default function StatCard({ label, value, icon: Icon, trend, color = 'accent' }: StatCardProps) {
  return (
    <div className="bg-surface border border-border rounded-xl p-5 hover:border-border-light transition-colors">
      <div className="flex items-start justify-between">
        <div>
          <p className="text-xs font-medium text-text-muted uppercase tracking-wider">{label}</p>
          <p className="text-2xl font-semibold text-text mt-1.5">{value}</p>
          {trend && <p className="text-xs text-text-muted mt-1">{trend}</p>}
        </div>
        <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${colorMap[color]}`}>
          <Icon className="w-5 h-5" />
        </div>
      </div>
    </div>
  )
}

interface CardProps {
  title: string
  action?: ReactNode
  children: ReactNode
}

export function Card({ title, action, children }: CardProps) {
  return (
    <div className="bg-surface border border-border rounded-xl">
      <div className="flex items-center justify-between px-5 py-4 border-b border-border">
        <h3 className="text-sm font-medium text-text">{title}</h3>
        {action}
      </div>
      <div className="p-5">{children}</div>
    </div>
  )
}
