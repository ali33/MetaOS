import { useEffect, useState } from 'react'
import type { ConnState } from '../lib/channel'
import type { ThemePref } from './prefs'

const CONN: Record<ConnState, { cls: string; text: string }> = {
  connecting: { cls: 'conn-wait', text: 'Đang kết nối…' },
  open: { cls: 'conn-open', text: 'Đã kết nối' },
  reconnecting: { cls: 'conn-bad', text: 'Mất kết nối — đang nối lại…' },
  replaced: { cls: 'conn-bad', text: 'Phiên đang mở ở tab khác' },
  expired: { cls: 'conn-bad', text: 'Phiên đã hết hạn' },
}

const fmt = new Intl.DateTimeFormat('vi-VN', { weekday: 'short', day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })

type Props = {
  hostname: string; user: string; connState: ConnState; loggingOut: boolean
  theme: ThemePref; onTheme(t: ThemePref): void; dockAlways: boolean; onDockAlways(v: boolean): void
  onActivities(): void; onLogout(): void
}

export function TopBar(p: Props) {
  const [now, setNow] = useState(() => new Date())
  const [menu, setMenu] = useState(false)
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 15_000)
    return () => clearInterval(t)
  }, [])
  const c = CONN[p.connState]
  const waiting = p.connState === 'connecting' || p.connState === 'reconnecting'
  return (
    <header className="topbar">
      <div><button onClick={p.onActivities}>Hoạt động</button></div>
      <time>{fmt.format(now)}</time>
      <div className="right">
        <span title={c.text} aria-label={c.text} role="status">
          {waiting ? <span className="spin" style={{ width: 10, height: 10 }} aria-hidden /> : <span className={`conn-dot ${c.cls}`} />}
        </span>
        <span>{p.hostname}</span>
        <button aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu(!menu)}>{p.user} ▾</button>
        {menu && (
          <div className="menu" role="menu" onKeyDown={(e) => { if (e.key === 'Escape') setMenu(false) }}>
            {(['system', 'light', 'dark'] as ThemePref[]).map((t) => (
              <button key={t} role="menuitemradio" aria-checked={p.theme === t} onClick={() => p.onTheme(t)}>
                {p.theme === t ? '●' : '○'} Giao diện {t === 'system' ? 'theo hệ thống' : t === 'light' ? 'sáng' : 'tối'}
              </button>
            ))}
            <button role="menuitemcheckbox" aria-checked={p.dockAlways} onClick={() => p.onDockAlways(!p.dockAlways)}>
              {p.dockAlways ? '☑' : '☐'} Luôn hiện dock
            </button>
            <button role="menuitem" disabled={p.loggingOut} onClick={p.onLogout}>
              {p.loggingOut ? <><span className="spin" aria-hidden /> Đang đăng xuất…</> : 'Đăng xuất'}
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
