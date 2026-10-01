import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Login } from './Login'

const fetchMock = vi.fn()
beforeEach(() => { fetchMock.mockReset(); vi.stubGlobal('fetch', fetchMock) })

function fill() {
  fireEvent.change(screen.getByLabelText('Tên đăng nhập'), { target: { value: 'alice' } })
  fireEvent.change(screen.getByLabelText('Mật khẩu'), { target: { value: 'Mật khẩu 1' } })
  fireEvent.click(screen.getByRole('button', { name: 'Đăng nhập' }))
}

test('đang chờ thì hiện vòng quay và khoá nút', async () => {
  let resolve!: (r: Response) => void
  fetchMock.mockReturnValue(new Promise<Response>((r) => (resolve = r)))
  const onLoggedIn = vi.fn()
  render(<Login onLoggedIn={onLoggedIn} />)
  fill()
  const btn = await screen.findByRole('button', { name: /Đang đăng nhập/ })
  expect((btn as HTMLButtonElement).disabled).toBe(true) // không cài jest-dom: kiểm thuộc tính trực tiếp
  expect(btn.querySelector('.spin')).not.toBeNull()
  resolve(new Response(JSON.stringify({ user: 'alice', csrf: 'c', hostname: 'h' }), { status: 200 }))
  await waitFor(() => expect(onLoggedIn).toHaveBeenCalledWith({ user: 'alice', csrf: 'c', hostname: 'h' }))
})

test.each([
  [401, 'auth-failed', 'Sai tên đăng nhập hoặc mật khẩu.'],
  [403, 'root-disabled', 'Không cho phép đăng nhập trực tiếp bằng root'],
  [500, 'internal', 'Máy chủ trả lỗi 500 (internal).'],
])('lỗi %i %s hiện lên màn hình', async (status, code, text) => {
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ error: code }), { status }))
  vi.spyOn(console, 'error').mockImplementation(() => {})
  render(<Login onLoggedIn={vi.fn()} />)
  fill()
  expect((await screen.findByRole('alert')).textContent).toContain(text)
})

test('mất mạng: hiện nguyên văn lỗi', async () => {
  fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
  vi.spyOn(console, 'error').mockImplementation(() => {})
  render(<Login onLoggedIn={vi.fn()} />)
  fill()
  expect((await screen.findByRole('alert')).textContent).toContain('Failed to fetch')
})
