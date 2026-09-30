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
