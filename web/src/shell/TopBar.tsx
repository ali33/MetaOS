import { useEffect, useRef, useState } from 'react'
import type { ConnState } from '../lib/channel'
import type { ThemePref } from './prefs'

const CONN: Record<ConnState, { cls: string; text: string; glyph: string }> = {
  connecting: { cls: 'conn-wait', text: 'Đang kết nối…', glyph: '' },
  open: { cls: 'conn-open', text: 'Đã kết nối', glyph: '' },
  reconnecting: { cls: 'conn-bad', text: 'Mất kết nối — đang nối lại…', glyph: '⚠' },
  replaced: { cls: 'conn-bad', text: 'Phiên đang mở ở tab khác', glyph: '⚠' },
  expired: { cls: 'conn-bad', text: 'Phiên đã hết hạn', glyph: '✕' },
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
  const menuBox = useRef<HTMLSpanElement>(null)
  const toggle = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 15_000)
    return () => clearInterval(t)
  }, [])
  // PH-002: menu mở thì Escape (tiêu điểm ở đâu cũng vậy) hoặc bấm ra ngoài sẽ đóng menu.
  // Bắt ở pha capture trên window và chặn lan truyền: Escape ấy chỉ đóng menu, không tới
  // Hoạt động hay terminal. Menu đóng thì không nghe gì — Escape vẫn về với vim.
  useEffect(() => {
    if (!menu) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.preventDefault()
      e.stopPropagation()
      setMenu(false)
      toggle.current?.focus()
    }
    const onPointer = (e: PointerEvent) => {
      if (!menuBox.current?.contains(e.target as Node)) setMenu(false)
    }
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('pointerdown', onPointer, true)
    return () => {
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('pointerdown', onPointer, true)
    }
  }, [menu])
  const c = CONN[p.connState]
  const waiting = p.connState === 'connecting' || p.connState === 'reconnecting'
  return (
    <header className="topbar">
      <div><button onClick={p.onActivities}>Hoạt động</button></div>
      <time>{fmt.format(now)}</time>
      <div className="right">
        <span title={c.text} aria-label={c.text} role="status">
          {waiting && <span className="spin" style={{ width: 10, height: 10 }} aria-hidden />}
          {c.glyph ? <span className="conn-glyph" style={{ color: '#ff6b6b' }} aria-hidden>{c.glyph}</span> : !waiting && <span className={`conn-dot ${c.cls}`} />}
        </span>
        <span>{p.hostname}</span>
        <span ref={menuBox}>
          <button ref={toggle} aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu(!menu)}>{p.user} ▾</button>
          {menu && (
            <div className="menu" role="menu">
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
        </span>
      </div>
    </header>
  )
}
