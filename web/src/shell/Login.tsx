import { useState, type FormEvent } from 'react'
import { ApiError, login, type SessionInfo } from '../lib/api'

const MESSAGES: Record<string, string> = {
  'auth-failed': 'Sai tên đăng nhập hoặc mật khẩu.',
  'password-expired': 'Mật khẩu đã hết hạn. Hãy đổi mật khẩu qua SSH rồi đăng nhập lại.',
  'root-disabled': 'Không cho phép đăng nhập trực tiếp bằng root. Hãy dùng tài khoản thường và sudo.',
}

export function loginErrorText(e: unknown): string {
  if (e instanceof ApiError) return MESSAGES[e.code] ?? `Máy chủ trả lỗi ${e.status} (${e.code}).`
  return `Không kết nối được máy chủ: ${(e as Error)?.message ?? String(e)}`
}

export function Login({ onLoggedIn, notice }: { onLoggedIn(s: SessionInfo): void; notice?: string }) {
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    setError('')
    try {
      onLoggedIn(await login(user, password))
    } catch (err) {
      console.error(err)
      setError(loginErrorText(err))
      setPassword('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login">
      <form onSubmit={submit} aria-busy={busy}>
        <h1 style={{ margin: 0, fontSize: 20 }}>MetaOS</h1>
        {notice && <p className="notice">{notice}</p>}
        <label>Tên đăng nhập<br />
          <input autoFocus autoComplete="username" value={user} onChange={(e) => setUser(e.target.value)} required disabled={busy} />
        </label>
        <label>Mật khẩu<br />
          <input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required disabled={busy} />
        </label>
        {error && <div className="banner banner-error" role="alert"><span className="text">{error}</span></div>}
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? <><span className="spin" aria-hidden /> Đang đăng nhập…</> : 'Đăng nhập'}
        </button>
      </form>
    </div>
  )
}
