export type SessionInfo = { user: string; csrf: string; hostname: string }

export class ApiError extends Error {
  constructor(public status: number, public code: string) {
    super(`${status} ${code}`)
  }
}

async function call(path: string, init: RequestInit & { headers?: Record<string, string> } = {}) {
  const r = await fetch(path, { credentials: 'same-origin', ...init })
  if (r.ok) return r
  let code = `http-${r.status}`
  try {
    const b = await r.json()
    if (typeof b?.error === 'string') code = b.error
  } catch {
    /* thân không phải JSON: giữ http-<mã> */
  }
  throw new ApiError(r.status, code)
}

export async function login(user: string, password: string): Promise<SessionInfo> {
  const r = await call('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user, password }),
  })
  return r.json()
}

export async function currentSession(): Promise<SessionInfo | null> {
  try {
    return await (await call('/api/session')).json()
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null
    throw e
  }
}

export async function logout(csrf: string): Promise<void> {
  await call('/api/logout', { method: 'POST', headers: { 'X-MetaOS-CSRF': csrf } })
}
