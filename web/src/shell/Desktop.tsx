import { useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { APPS, type AppDef } from '../apps/registry'
import { ApiError, currentSession, logout, type SessionInfo } from '../lib/api'
import { Connection, type ConnOptions, type ConnState } from '../lib/channel'
import { BannerList } from '../lib/errors'
import { Activities } from './Activities'
import { Banners } from './Banners'
import { Dock } from './Dock'
import { useShortcuts } from './keys'
import { applyTheme, loadPref, savePref, type ThemePref } from './prefs'
import { TopBar } from './TopBar'
import { byZ, initialWM, reduce } from './wm/store'
import { Window } from './wm/Window'

type Props = {
  session: SessionInfo
  apps?: AppDef[]
  connOptions?: ConnOptions
  globalBanners: BannerList
  onLoggedOut(): void
  onExpired(): void
}

let winSeq = 0

export function Desktop({ session, apps = APPS, connOptions, globalBanners, onLoggedOut, onExpired }: Props) {
  const [connState, setConnState] = useState<ConnState>('connecting')
  const launchRef = useRef<(appId: string, adopt?: string) => void>(() => {})
  const loggingOutRef = useRef(false)
  const conn = useMemo(() => new Connection({
    checkSession: async () => (await currentSession()) !== null,
    onControlError: (code, msg) => globalBanners.add('warn', `${code}: ${msg}`),
    // D17 (Q2): tab mới tiếp quản terminal đang chạy — mỗi kênh pty thành một cửa sổ Terminal.
    onOrphans: (list) => list.filter((c) => c.kind === 'pty').forEach((c) => launchRef.current('terminal', c.ch)),
    ...connOptions,
    onState: setConnState,
  }), []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { conn.connect(); return () => conn.dispose() }, [conn])

  useEffect(() => {
    if (connState === 'expired') {
      // I2: máy chủ đóng WS 4401 "logout" trước khi trả 204 — đang đăng xuất thì để doLogout báo;
      // tab khác đăng xuất thì báo đã đăng xuất, không phải hết hạn.
      if (loggingOutRef.current) return
      if (conn.endReason === 'logout') { conn.dispose(); onLoggedOut() } else onExpired()
    }
    if (connState === 'replaced') globalBanners.add('warn', 'Đã mở ở nơi khác. Tải lại trang để dùng ở đây.')
  }, [connState]) // eslint-disable-line react-hooks/exhaustive-deps

  const [wm, dispatch] = useReducer(reduce, undefined, () => initialWM({ w: window.innerWidth, h: window.innerHeight - 32 }))
  const workspace = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = workspace.current!
    const measure = () => dispatch({ type: 'setArea', w: el.clientWidth || window.innerWidth, h: el.clientHeight || window.innerHeight - 32 })
    measure()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure)
      return () => window.removeEventListener('resize', measure)
    }
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const toArea = (cx: number, cy: number) => {
    const r = workspace.current!.getBoundingClientRect()
    return { x: cx - r.left, y: cy - r.top }
  }

  const winBanners = useRef(new Map<string, BannerList>())
  const bannersFor = (id: string) => {
    let l = winBanners.current.get(id)
    if (!l) winBanners.current.set(id, (l = new BannerList()))
    return l
  }

  const adoptFor = useRef(new Map<string, string>()) // winId → id kênh cần tiếp quản
  // Dọn băng lỗi và id tiếp quản của cửa sổ đã đóng, dù đóng bằng đường nào.
  useEffect(() => {
    const alive = new Set(wm.wins.map((w) => w.id))
    for (const id of [...winBanners.current.keys()]) if (!alive.has(id)) winBanners.current.delete(id)
    for (const id of [...adoptFor.current.keys()]) if (!alive.has(id)) adoptFor.current.delete(id)
  }, [wm.wins])

  const [activities, setActivities] = useState(false)
  const [theme, setTheme] = useState<ThemePref>(() => loadPref('metaos.theme', ['system', 'light', 'dark'] as const, 'system'))
  const [dockAlways, setDockAlways] = useState(() => loadPref('metaos.dockAlways', ['0', '1'] as const, '0') === '1')
  useEffect(() => { applyTheme(theme); savePref('metaos.theme', theme) }, [theme])
  useEffect(() => savePref('metaos.dockAlways', dockAlways ? '1' : '0'), [dockAlways])

  const launch = (appId: string, adopt?: string) => {
    const app = apps.find((a) => a.id === appId)
    if (!app) return globalBanners.add('warn', `Không có ứng dụng "${appId}".`)
    const id = `w${++winSeq}`
    if (adopt) adoptFor.current.set(id, adopt)
    dispatch({ type: 'open', id, appId, title: app.name })
    setActivities(false)
  }
  launchRef.current = launch

  useShortcuts((c) => {
    const f = wm.focused
    switch (c) {
      case 'activities': return setActivities((v) => !v)
      case 'terminal': return launch('terminal')
      case 'cycle-next': return dispatch({ type: 'cycle', dir: 1 })
      case 'cycle-prev': return dispatch({ type: 'cycle', dir: -1 })
    }
    if (!f) return
    if (c === 'tile-left') dispatch({ type: 'tile', id: f, side: 'left' })
    if (c === 'tile-right') dispatch({ type: 'tile', id: f, side: 'right' })
    if (c === 'maximize') dispatch({ type: 'toggleMaximize', id: f })
    if (c === 'restore') dispatch({ type: 'restore', id: f })
  })

  const [loggingOut, setLoggingOut] = useState(false)
  async function doLogout() {
    if (loggingOut) return
    loggingOutRef.current = true
    setLoggingOut(true)
    try {
      await logout(session.csrf)
      conn.dispose()
      onLoggedOut()
    } catch (e) {
      // phiên đã mất sẵn (401) hoặc máy chủ đã đóng WS 4401 "logout": coi như đã đăng xuất
      if ((e instanceof ApiError && e.status === 401) || (conn.state === 'expired' && conn.endReason === 'logout')) {
        conn.dispose()
        onLoggedOut()
        return
      }
      console.error(e)
      globalBanners.add('error', `Đăng xuất không thành công: ${(e as Error).message}`)
      loggingOutRef.current = false
      setLoggingOut(false)
      if (conn.state === 'expired') onExpired() // 4401 khác "logout" tới trong lúc chờ
    }
  }

  return (
    <div className="desktop">
      <TopBar hostname={session.hostname} user={session.user} connState={connState} loggingOut={loggingOut}
        theme={theme} onTheme={setTheme} dockAlways={dockAlways} onDockAlways={setDockAlways}
        onActivities={() => setActivities((v) => !v)} onLogout={doLogout} />
      <main className="workspace" ref={workspace}>
        {byZ(wm).map((w) => {
          const app = apps.find((a) => a.id === w.appId)!
          const App = app.component
          const list = bannersFor(w.id)
          return (
            <Window key={w.id} win={w} focused={wm.focused === w.id} dispatch={dispatch} toArea={toArea}
              banners={<Banners list={list} />}>
              <App winId={w.id} conn={conn} connState={connState} active={wm.focused === w.id} adopt={adoptFor.current.get(w.id)}
                report={(level, text) => list.add(level, text)} clearReports={(level) => list.clear(level)}
                setTitle={(title) => dispatch({ type: 'setTitle', id: w.id, title })}
                requestClose={() => dispatch({ type: 'close', id: w.id })} />
            </Window>
          )
        })}
        {dockAlways && !activities && <Dock apps={apps} running={new Set(wm.wins.map((w) => w.appId))} onLaunch={launch} always />}
        {activities && (
          <Activities wins={byZ(wm).reverse()} apps={apps} onLaunch={launch} onClose={() => setActivities(false)}
            onPickWindow={(id) => { dispatch({ type: 'focus', id }); setActivities(false) }} />
        )}
      </main>
    </div>
  )
}
