import { useEffect, useMemo, useState } from 'react'
import { currentSession, type SessionInfo } from './lib/api'
import { BannerList, installGlobalHandlers } from './lib/errors'
import { Banners } from './shell/Banners'
import { Desktop } from './shell/Desktop'
import { Login } from './shell/Login'
import { applyTheme, loadPref } from './shell/prefs'

type Phase = { kind: 'checking' } | { kind: 'login'; notice?: string } | { kind: 'desktop'; session: SessionInfo }

export function App() {
  const global = useMemo(() => new BannerList(), [])
  const [phase, setPhase] = useState<Phase>({ kind: 'checking' })
  useEffect(() => installGlobalHandlers(global), [global])
  useEffect(() => {
    applyTheme(loadPref('metaos.theme', ['system', 'light', 'dark'] as const, 'system'))
    currentSession().then(
      (s) => setPhase(s ? { kind: 'desktop', session: s } : { kind: 'login' }),
      (e) => { console.error(e); global.add('error', `Không kiểm được phiên: ${(e as Error).message}`); setPhase({ kind: 'login' }) },
    )
  }, [global])

  return (
    <>
      <Banners list={global} className="global-banners" />
      {phase.kind === 'checking' && (
        <div className="login" role="status"><span className="spin" aria-hidden /> Đang kiểm tra phiên…</div>
      )}
      {phase.kind === 'login' && (
        <Login notice={phase.notice} onLoggedIn={(session) => { global.clear(); setPhase({ kind: 'desktop', session }) }} />
      )}
      {phase.kind === 'desktop' && (
        <Desktop session={phase.session} globalBanners={global}
          onLoggedOut={() => setPhase({ kind: 'login', notice: 'Đã đăng xuất.' })}
          onExpired={() => setPhase({ kind: 'login', notice: 'Phiên đã hết hạn. Hãy đăng nhập lại.' })} />
      )}
    </>
  )
}
