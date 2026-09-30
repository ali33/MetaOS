import { useState } from 'react'
import type { AppDef } from '../apps/registry'
import { SHORTCUTS } from './keys'
import type { Win } from './wm/store'
import { Dock } from './Dock'

type Props = { wins: Win[]; apps: AppDef[]; onPickWindow(id: string): void; onLaunch(id: string): void; onClose(): void }

export function Activities({ wins, apps, onPickWindow, onLaunch, onClose }: Props) {
  const [q, setQ] = useState('')
  const needle = q.trim().toLowerCase()
  const matches = apps.filter((a) => !needle || a.name.toLowerCase().includes(needle) || a.keywords.some((k) => k.includes(needle)))
  const running = new Set(wins.map((w) => w.appId))
  return (
    <div className="activities" role="dialog" aria-label="Hoạt động"
      onKeyDown={(e) => { if (e.key === 'Escape') onClose() }}
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}>
      <input autoFocus placeholder="Tìm ứng dụng…" aria-label="Tìm ứng dụng" value={q} onChange={(e) => setQ(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter' && matches[0]) onLaunch(matches[0].id) }} />
      {needle ? (
        <div className="thumbs">
          {matches.length === 0 && <p>Không có ứng dụng nào khớp “{q}”.</p>}
          {matches.map((a) => (
            <button key={a.id} className="thumb" onClick={() => onLaunch(a.id)}>{a.icon} {a.name}</button>
          ))}
        </div>
      ) : (
        <div className="thumbs">
          {wins.length === 0 && <p>Chưa có cửa sổ nào. Chọn một ứng dụng trong dock.</p>}
          {wins.map((w) => (
            <button key={w.id} className="thumb" onClick={() => onPickWindow(w.id)}>{w.title}{w.minimized ? ' (đã thu nhỏ)' : ''}</button>
          ))}
        </div>
      )}
      <Dock apps={apps} running={running} onLaunch={onLaunch} />
      <div className="shortcut-help" aria-label="Phím tắt">
        {SHORTCUTS.map((s) => (
          <span key={s.command}>{s.label}: {s.primary}{s.alternate ? ` / ${s.alternate}` : ''}</span>
        ))}
      </div>
    </div>
  )
}
