import { fireEvent, render, screen } from '@testing-library/react'
import type { ConnState } from '../lib/channel'
import { TopBar } from './TopBar'

function bar(connState: ConnState) {
  return render(
    <TopBar hostname="h" user="u" connState={connState} loggingOut={false} theme="system" onTheme={() => {}}
      dockAlways={false} onDockAlways={() => {}} onActivities={() => {}} onLogout={() => {}} />,
  )
}

test.each([
  ['reconnecting', '⚠', true],
  ['replaced', '⚠', false],
  ['expired', '✕', false],
] as const)('trạng thái %s có biểu tượng %s (vòng quay: %s)', (state, glyph, spin) => {
  const { container } = bar(state)
  expect(container.querySelector('.conn-glyph')?.textContent).toBe(glyph)
  expect(container.querySelector('.spin') !== null).toBe(spin)
  expect(screen.getByRole('status').getAttribute('title')).toBeTruthy()
})

test.each(['open', 'connecting'] as const)('trạng thái %s không có biểu tượng lỗi', (state) => {
  const { container } = bar(state)
  expect(container.querySelector('.conn-glyph')).toBeNull()
})

// PH-002: menu người dùng phải đóng được bằng Escape (dù tiêu điểm đang ở đâu) và bằng bấm ra ngoài.
describe('PH-002: menu người dùng', () => {
  function withMenu(extra: Partial<Parameters<typeof TopBar>[0]> = {}) {
    const p = { onTheme: vi.fn(), onLogout: vi.fn(), onActivities: vi.fn(), ...extra }
    render(
      <div>
        <TopBar hostname="h" user="alice" connState="open" loggingOut={false} theme="system"
          dockAlways={false} onDockAlways={() => {}} {...p} />
        <p>ngoài menu</p>
      </div>,
    )
    const toggle = screen.getByText('alice ▾')
    fireEvent.click(toggle)
    expect(screen.getByRole('menu')).toBeTruthy()
    return { p, toggle }
  }

  test('Escape đóng menu kể cả khi tiêu điểm không nằm trong menu', () => {
    const { toggle } = withMenu()
    fireEvent.keyDown(toggle, { key: 'Escape' })
    expect(screen.queryByRole('menu')).toBeNull()
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
  })

  test('Escape gõ ở chỗ khác khi menu mở thì chỉ đóng menu, không chuyển tiếp', () => {
    const outer = vi.fn()
    withMenu()
    const el = screen.getByText('ngoài menu')
    el.addEventListener('keydown', outer)
    fireEvent.keyDown(el, { key: 'Escape' })
    expect(screen.queryByRole('menu')).toBeNull()
    expect(outer).not.toHaveBeenCalled()
  })

  test('menu đóng thì không nuốt Escape (vim trong terminal cần nó)', () => {
    const outer = vi.fn()
    render(<><TopBar hostname="h" user="alice" connState="open" loggingOut={false} theme="system" onTheme={() => {}}
      dockAlways={false} onDockAlways={() => {}} onActivities={() => {}} onLogout={() => {}} /><p>ngoài menu</p></>)
    const el = screen.getByText('ngoài menu')
    el.addEventListener('keydown', outer)
    fireEvent.keyDown(el, { key: 'Escape' })
    expect(outer).toHaveBeenCalledOnce()
  })

  test('bấm (pointerdown) ra ngoài menu thì đóng menu', () => {
    withMenu()
    fireEvent.pointerDown(screen.getByText('ngoài menu'))
    expect(screen.queryByRole('menu')).toBeNull()
  })

  test('bấm mục trong menu vẫn chạy (pointerdown bên trong không đóng menu trước)', () => {
    const { p } = withMenu()
    const item = screen.getByText(/Giao diện tối/)
    fireEvent.pointerDown(item)
    expect(screen.getByRole('menu')).toBeTruthy()
    fireEvent.click(item)
    expect(p.onTheme).toHaveBeenCalledWith('dark')
    fireEvent.pointerDown(screen.getByText('Đăng xuất'))
    fireEvent.click(screen.getByText('Đăng xuất'))
    expect(p.onLogout).toHaveBeenCalledOnce()
  })

  test('bấm lại nút alice ▾ thì đóng menu (pointerdown trên nút không đóng rồi mở lại)', () => {
    const { toggle } = withMenu()
    fireEvent.pointerDown(toggle)
    fireEvent.click(toggle)
    expect(screen.queryByRole('menu')).toBeNull()
  })
})
