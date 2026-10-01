import type { AppDef } from '../apps/registry'

export function Dock({ apps, running, onLaunch, always }: { apps: AppDef[]; running: Set<string>; onLaunch(id: string): void; always?: boolean }) {
  return (
    <nav className={`dock ${always ? 'always' : ''}`} aria-label="Dock">
      {apps.map((a) => (
        <button key={a.id} title={a.name} aria-label={a.name} className={running.has(a.id) ? 'running' : ''} onClick={() => onLaunch(a.id)}>
          {a.icon}
        </button>
      ))}
    </nav>
  )
}
