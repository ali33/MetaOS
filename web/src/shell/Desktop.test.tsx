import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { AppDef } from '../apps/registry'
import { BannerList } from '../lib/errors'
import { Desktop } from './Desktop'

const fakeApp: AppDef = {
  id: 'terminal', name: 'Terminal', icon: '>_', keywords: ['shell'],
  component: ({ report }) => <button onClick={() => report('error', 'not-found: /x')}>báo lỗi</button>,
}
const socket = () => ({ binaryType: '', readyState: 0, send() {}, close() {}, onopen: null, onclose: null, onmessage: null })

function renderDesktop(extra: Partial<Parameters<typeof Desktop>[0]> = {}) {
  return render(
    <Desktop session={{ user: 'alice', csrf: 'tok', hostname: 'srv1' }} apps={[fakeApp]} globalBanners={new BannerList()}
      connOptions={{ socketFactory: socket as any }} onLoggedOut={vi.fn()} onExpired={vi.fn()} {...extra} />,
  )
}

test('Hoạt động → tìm → mở ứng dụng thành cửa sổ; đóng được', () => {
  renderDesktop()
  expect(screen.getByText('srv1')).toBeTruthy()
  fireEvent.click(screen.getByText('Hoạt động'))
  fireEvent.change(screen.getByLabelText('Tìm ứng dụng'), { target: { value: 'she' } })
  fireEvent.click(screen.getByText(/Terminal/, { selector: '.thumb' }))
  expect(screen.getByRole('region', { name: 'Terminal' })).toBeTruthy()
  expect(screen.queryByRole('dialog', { name: 'Hoạt động' })).toBeNull()
  fireEvent.click(screen.getByLabelText('Đóng'))
  expect(screen.queryByRole('region', { name: 'Terminal' })).toBeNull()
})

test('lỗi của ứng dụng hiện trên băng của chính cửa sổ đó', () => {
  renderDesktop()
  fireEvent.keyDown(window, { key: 't', ctrlKey: true, altKey: true })
  fireEvent.click(screen.getByText('báo lỗi'))
  expect(screen.getByRole('alert').textContent).toContain('not-found: /x')
})

test('tab mới tiếp quản: kênh pty lạ mở thành cửa sổ Terminal', () => {
  let sock: any
  render(
    <Desktop session={{ user: 'alice', csrf: 'tok', hostname: 'srv1' }} apps={[fakeApp]} globalBanners={new BannerList()}
      connOptions={{ socketFactory: (() => (sock = socket())) as any }} onLoggedOut={vi.fn()} onExpired={vi.fn()} />,
  )
  act(() => { sock.readyState = 1; sock.onopen({}) })
  act(() => sock.onmessage({ data: JSON.stringify({ ch: '', type: 'channels', data: [{ ch: 'pty.old.1', kind: 'pty' }] }) }))
  expect(screen.getByRole('region', { name: 'Terminal' })).toBeTruthy()
})

test('đăng xuất gọi API có CSRF rồi báo lên', async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetchMock)
  const onLoggedOut = vi.fn()
  renderDesktop({ onLoggedOut })
  fireEvent.click(screen.getByText('alice ▾'))
  fireEvent.click(screen.getByText('Đăng xuất'))
  await waitFor(() => expect(onLoggedOut).toHaveBeenCalled())
  expect(fetchMock.mock.calls[0][1].headers['X-MetaOS-CSRF']).toBe('tok')
})

function openSocket() {
  let sock: any
  const factory = (() => (sock = socket())) as any
  return { factory, get: () => sock, open() { act(() => { sock.readyState = 1; sock.onopen({}) }) } }
}

test('đăng xuất mà máy chủ trả 401: coi như đã đăng xuất, không có băng đỏ', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'unauthorized' }), { status: 401 })))
  const onLoggedOut = vi.fn()
  renderDesktop({ onLoggedOut })
  fireEvent.click(screen.getByText('alice ▾'))
  fireEvent.click(screen.getByText('Đăng xuất'))
  await waitFor(() => expect(onLoggedOut).toHaveBeenCalled())
  expect(screen.queryByRole('alert')).toBeNull()
})

test('đăng xuất lỗi khác 401: hiện băng đỏ, không báo đã đăng xuất', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'internal' }), { status: 500 })))
  vi.spyOn(console, 'error').mockImplementation(() => {})
  const onLoggedOut = vi.fn()
  const banners = new BannerList()
  renderDesktop({ onLoggedOut, globalBanners: banners })
  fireEvent.click(screen.getByText('alice ▾'))
  fireEvent.click(screen.getByText('Đăng xuất'))
  await waitFor(() => expect(banners.items.length).toBe(1))
  expect(banners.items[0].level).toBe('error')
  expect(onLoggedOut).not.toHaveBeenCalled()
})

test('WebSocket đóng 4401 ⇒ onExpired', () => {
  const s = openSocket()
  const onExpired = vi.fn()
  renderDesktop({ connOptions: { socketFactory: s.factory }, onExpired })
  s.open()
  act(() => s.get().onclose({ code: 4401, reason: '' }))
  expect(onExpired).toHaveBeenCalled()
})

test('WebSocket đóng 4001 ⇒ băng hổ phách "Đã mở ở nơi khác"', () => {
  const s = openSocket()
  const banners = new BannerList()
  renderDesktop({ connOptions: { socketFactory: s.factory }, globalBanners: banners })
  s.open()
  act(() => s.get().onclose({ code: 4001, reason: '' }))
  expect(banners.items.map((b) => [b.level, b.text.startsWith('Đã mở ở nơi khác')])).toEqual([['warn', true]])
})

test('đóng bằng nút ✕ của khung: cửa sổ mở lại không thừa hưởng băng lỗi cũ', () => {
  renderDesktop()
  fireEvent.keyDown(window, { key: 't', ctrlKey: true, altKey: true })
  fireEvent.click(screen.getByText('báo lỗi'))
  expect(screen.getByRole('alert')).toBeTruthy()
  fireEvent.click(screen.getByLabelText('Đóng'))
  expect(screen.queryByRole('alert')).toBeNull()
  fireEvent.keyDown(window, { key: 't', ctrlKey: true, altKey: true })
  expect(screen.queryByRole('alert')).toBeNull()
})

test('I2: 4401 "logout" tới khi đang đăng xuất ⇒ chỉ "Đã đăng xuất.", không bao giờ báo hết hạn', async () => {
  let resolve!: (r: Response) => void
  vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise<Response>((r) => { resolve = r })))
  const notices: string[] = []
  const s = openSocket()
  renderDesktop({
    connOptions: { socketFactory: s.factory },
    onLoggedOut: () => notices.push('Đã đăng xuất.'),
    onExpired: () => notices.push('Phiên đã hết hạn. Hãy đăng nhập lại.'),
  })
  s.open()
  fireEvent.click(screen.getByText('alice ▾'))
  fireEvent.click(screen.getByText('Đăng xuất'))
  act(() => s.get().onclose({ code: 4401, reason: 'logout' })) // máy chủ đóng WS trước khi trả 204
  expect(notices).toEqual([])
  await act(async () => { resolve(new Response(null, { status: 204 })) })
  await waitFor(() => expect(notices).toEqual(['Đã đăng xuất.']))
})

test('I2: 4401 "logout" do tab khác đăng xuất ⇒ onLoggedOut, không onExpired', () => {
  const s = openSocket()
  const onExpired = vi.fn()
  const onLoggedOut = vi.fn()
  renderDesktop({ connOptions: { socketFactory: s.factory }, onExpired, onLoggedOut })
  s.open()
  act(() => s.get().onclose({ code: 4401, reason: 'logout' }))
  expect(onLoggedOut).toHaveBeenCalledOnce()
  expect(onExpired).not.toHaveBeenCalled()
})
