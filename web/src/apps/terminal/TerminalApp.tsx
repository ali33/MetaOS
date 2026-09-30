import '@xterm/xterm/css/xterm.css'
import './terminal.css'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import { useEffect, useRef, useState } from 'react'
import type { ChannelHandlers } from '../../lib/channel'
import type { AppProps } from '../registry'

type Tab = { key: number; title: string; adopt?: string }
type Phase = 'opening' | 'ready' | 'reattaching' | 'closed'
let tabSeq = 0

export function TerminalApp(p: AppProps) {
  const [tabs, setTabs] = useState<Tab[]>(() => [{ key: ++tabSeq, title: 'Terminal', adopt: p.adopt }])
  const [active, setActive] = useState(tabs[0].key)

  const addTab = () => {
    const t = { key: ++tabSeq, title: 'Terminal' }
    setTabs((ts) => [...ts, t])
    setActive(t.key)
  }
  const tabsRef = useRef(tabs)
  tabsRef.current = tabs
  const removeTab = (key: number) => {
    const rest = tabsRef.current.filter((t) => t.key !== key)
    if (rest.length === 0) return p.requestClose()
    setTabs(rest)
    setActive((a) => (a === key ? rest[rest.length - 1].key : a))
  }
  const activeTitle = tabs.find((t) => t.key === active)?.title ?? 'Terminal'
  useEffect(() => p.setTitle(activeTitle), [activeTitle]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="term-app">
      <div className="term-tabs" role="tablist">
        {tabs.map((t) => (
          <button key={t.key} role="tab" aria-selected={t.key === active} onClick={() => setActive(t.key)}>{t.title}</button>
        ))}
        <button className="add" aria-label="Tab mới" title="Tab mới (Ctrl+Shift+T)" onClick={addTab}>+</button>
      </div>
      {tabs.map((t) => (
        <TermTab key={t.key} app={p} adopt={t.adopt} visible={t.key === active} onNewTab={addTab}
          onTitle={(title) => setTabs((ts) => ts.map((x) => (x.key === t.key ? { ...x, title } : x)))}
          onExit={() => removeTab(t.key)} />
      ))}
    </div>
  )
}

type TabProps = { app: AppProps; adopt?: string; visible: boolean; onTitle(t: string): void; onExit(): void; onNewTab(): void }

function TermTab({ app, adopt, visible, onTitle, onExit, onNewTab }: TabProps) {
  const host = useRef<HTMLDivElement>(null)
  const api = useRef<{ term: Terminal; fit: FitAddon; search: SearchAddon } | null>(null)
  const [phase, setPhase] = useState<Phase>('opening')
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState('')
  const cb = useRef({ onTitle, onExit, onNewTab, report: app.report })
  cb.current = { onTitle, onExit, onNewTab, report: app.report }

  useEffect(() => {
    const term = new Terminal({
      fontFamily: 'ui-monospace, "Cascadia Mono", "DejaVu Sans Mono", monospace',
      fontSize: 14, cursorBlink: true, scrollback: 5000,
      theme: { background: '#1e1e1e', foreground: '#e6e6e6' },
    })
    const fit = new FitAddon()
    const search = new SearchAddon()
    term.loadAddon(fit)
    term.loadAddon(new WebLinksAddon())
    term.loadAddon(search)
    term.open(host.current!)
    const safeFit = () => {
      try {
        fit.fit()
      } catch (e) {
        console.warn('xterm fit', e)
        cb.current.report('warn', `Không đo được cỡ terminal: ${(e as Error).message}`)
      }
    }
    let received = 0 // tổng byte đầu ra đã nhận — gửi lên khi gắn lại
    safeFit()
    const handlers: ChannelHandlers = {
      onReady: () => setPhase('ready'),
      onReattach: () => { setPhase('reattaching'); return { offset: received } },
      onText: (m) => {
        if (m.type !== 'replay') return
        if (m.data?.reset) term.reset()
        received = m.data?.offset ?? 0
      },
      onBinary: (d) => { received += d.length; term.write(d) },
      onError: (code, message) => cb.current.report('error', `${code}: ${message}`),
      onClose: (reason, code) => {
        setPhase('closed')
        if (reason === 'exit') return cb.current.onExit()
        if (reason === 'gone' || reason === 'detach-timeout') {
          cb.current.report('error', 'Phiên terminal đã mất vì mất kết nối quá 60 giây. Mở tab mới để tiếp tục.')
        } else if (reason !== 'client' && reason !== 'shutdown' && reason !== 'closed') {
          cb.current.report('warn', `Terminal đã đóng: ${reason}${code !== undefined ? ` (mã ${code})` : ''}`)
        }
      },
    }
    // D17: cửa sổ mở để tiếp quản thì gắn vào kênh có sẵn, rồi báo cỡ hiện tại.
    const ch = adopt
      ? app.conn.adopt(adopt, 'pty', handlers)
      : app.conn.open('pty', { cols: term.cols, rows: term.rows }, handlers, { reattachable: true })
    if (adopt) ch.sendText('resize', { cols: term.cols, rows: term.rows })
    const subs = [
      term.onData((d) => ch.sendBinary(d)),
      term.onBinary((d) => ch.sendBinary(Uint8Array.from(d, (c) => c.charCodeAt(0)))),
      term.onTitleChange((t) => cb.current.onTitle(t)),
    ]
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown' || !e.ctrlKey || !e.shiftKey) return true
      if (e.key === 'T') { cb.current.onNewTab(); return false }
      if (e.key === 'F') { setSearchOpen((v) => !v); return false }
      return true
    })
    let last = { cols: term.cols, rows: term.rows }
    let timer: ReturnType<typeof setTimeout> | undefined
    const ro = new ResizeObserver(() => {
      clearTimeout(timer)
      timer = setTimeout(() => {
        safeFit()
        if (term.cols !== last.cols || term.rows !== last.rows) {
          last = { cols: term.cols, rows: term.rows }
          ch.sendText('resize', last)
        }
      }, 80)
    })
    ro.observe(host.current!)
    api.current = { term, fit, search }
    return () => {
      clearTimeout(timer)
      ro.disconnect()
      subs.forEach((s) => s.dispose())
      ch.close()
      term.dispose()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (visible && app.active) api.current?.term.focus()
  }, [visible, app.active])

  const waiting = phase === 'opening' || phase === 'reattaching' || app.connState === 'reconnecting'
  const label = phase === 'opening' ? 'Đang mở terminal…' : 'Đang nối lại…'
  return (
    <div className="term-pane" style={{ display: visible ? undefined : 'none' }}>
      <div className="xterm-host" ref={host} />
      {searchOpen && (
        <div className="term-search">
          <input autoFocus aria-label="Tìm trong terminal" value={query} onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') e.shiftKey ? api.current?.search.findPrevious(query) : api.current?.search.findNext(query)
              if (e.key === 'Escape') { setSearchOpen(false); api.current?.term.focus() }
            }} />
        </div>
      )}
      {waiting && phase !== 'closed' && (
        <div className="phu-lop" role="status"><span className="spin" aria-hidden /> {label}</div>
      )}
    </div>
  )
}
