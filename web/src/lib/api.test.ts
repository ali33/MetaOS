import { ApiError, currentSession, login, logout } from './api'

const fetchMock = vi.fn()
beforeEach(() => { fetchMock.mockReset(); vi.stubGlobal('fetch', fetchMock) })

const res = (status: number, body?: object) =>
  new Response(body ? JSON.stringify(body) : null, { status, headers: { 'Content-Type': 'application/json' } })

test('login gửi JSON và trả SessionInfo', async () => {
  fetchMock.mockResolvedValue(res(200, { user: 'alice', csrf: 'c', hostname: 'srv1' }))
  expect(await login('alice', 'Mật khẩu 1')).toEqual({ user: 'alice', csrf: 'c', hostname: 'srv1' })
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/login')
  expect(init.method).toBe('POST')
  expect(init.headers['Content-Type']).toBe('application/json')
  expect(JSON.parse(init.body)).toEqual({ user: 'alice', password: 'Mật khẩu 1' })
})

test('login lỗi thành ApiError có mã', async () => {
  fetchMock.mockResolvedValue(res(401, { error: 'auth-failed' }))
  await expect(login('alice', 'x')).rejects.toMatchObject({ status: 401, code: 'auth-failed' })
  fetchMock.mockResolvedValue(new Response('<html>502</html>', { status: 502 }))
  const e = await login('alice', 'x').catch((x) => x)
  expect(e).toBeInstanceOf(ApiError)
  expect(e.code).toBe('http-502')
})

test('currentSession: 401 ⇒ null', async () => {
  fetchMock.mockResolvedValue(res(401, { error: 'no-session' }))
  expect(await currentSession()).toBeNull()
})

test('logout gửi header CSRF', async () => {
  fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
  await logout('tok')
  expect(fetchMock.mock.calls[0][1].headers['X-MetaOS-CSRF']).toBe('tok')
})
