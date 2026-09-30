import { act, fireEvent, render, screen } from '@testing-library/react'
import type { ChannelHandlers } from '../../lib/channel'
import type { AppProps } from '../registry'
import { TerminalApp } from './TerminalApp'

// vi.mock được đưa lên đầu file khi chạy, nên lớp giả phải tạo trong vi.hoisted.
const { FakeTerminal, terms } = vi.hoisted(() => {
  const terms: any[] = []
  class FakeTerminal {
    cols = 80; rows = 24
    written: (string | Uint8Array)[] = []
    reset = vi.fn(); dispose = vi.fn(); focus = vi.fn()
    dataCb: (d: string) => void = () => {}
    constructor() { terms.push(this) }
    loadAddon() {}
    open() {}
    write(d: string | Uint8Array) { this.written.push(d) }
    onData(cb: (d: string) => void) { this.dataCb = cb; return { dispose() {} } }
    onBinary() { return { dispose() {} } }
    onTitleChange() { return { dispose() {} } }
    attachCustomKeyEventHandler() {}
  }
  return { FakeTerminal, terms }
})
vi.mock('@xterm/xterm', () => ({ Terminal: FakeTerminal }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))
vi.mock('@xterm/addon-search', () => ({ SearchAddon: class { findNext() {} findPrevious() {} } }))
vi.mock('@xterm/xterm/css/xterm.css', () => ({}))

type Opened = { kind: string; params: any; h: ChannelHandlers; opts: any; ch: { sendBinary: any; sendText: any; close: any } }

function setup(adopt?: string) {
  terms.length = 0
  const opened: Opened[] = []
  const conn = {
    open: (kind: string, params: any, h: ChannelHandlers, opts: any) => {
      const ch = { id: `pty.${opened.length}`, sendBinary: vi.fn(), sendText: vi.fn(), close: vi.fn() }
      opened.push({ kind, params, h, opts, ch })
      return ch
    },
    adopt: (id: string, kind: string, h: ChannelHandlers) => {
      const ch = { id, sendBinary: vi.fn(), sendText: vi.fn(), close: vi.fn() }
      opened.push({ kind, params: { adopt: id }, h, opts: null, ch })
      return ch
    },
  }
  const props: AppProps = {
    winId: 'w1', conn: conn as any, connState: 'open', active: true,
    report: vi.fn(), clearReports: vi.fn(), setTitle: vi.fn(), requestClose: vi.fn(), adopt,
  }
  const r = render(<TerminalApp {...props} />)
  return { opened, props, ...r }
}

beforeAll(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})

test('mở kênh pty reattachable với cỡ của xterm', () => {
  const { opened } = setup()
  expect(opened).toHaveLength(1)
  expect(opened[0].kind).toBe('pty')
  expect(opened[0].params).toEqual({ cols: 80, rows: 24 })
  expect(opened[0].opts).toEqual({ reattachable: true })
})

test('tiếp quản kênh có sẵn thay vì mở mới, rồi báo cỡ', () => {
  const { opened } = setup('pty.old.1')
  expect(opened).toHaveLength(1)
  expect(opened[0].params).toEqual({ adopt: 'pty.old.1' })
  expect(opened[0].ch.sendText).toHaveBeenCalledWith('resize', { cols: 80, rows: 24 })
})

test('phím gõ đi xuống, đầu ra đi lên', () => {
  const { opened } = setup()
  terms[0].dataCb('ls\r')
  expect(opened[0].ch.sendBinary).toHaveBeenCalledWith('ls\r')
  const out = new TextEncoder().encode('xin chào')
  act(() => opened[0].h.onBinary!(out))
  expect(terms[0].written).toContain(out)
})

test('đang mở thì phủ mờ có vòng quay; ready thì bỏ phủ', () => {
  const { opened, container } = setup()
  expect(container.querySelector('.phu-lop .spin')).not.toBeNull()
  act(() => opened[0].h.onReady!())
  expect(container.querySelector('.phu-lop')).toBeNull()
})

test('gắn lại: gửi offset đã nhận; chỉ xoá màn khi bridge yêu cầu reset', () => {
  const { opened } = setup()
  act(() => opened[0].h.onReady!())
  act(() => opened[0].h.onBinary!(new Uint8Array(10)))
  let extra: unknown
  act(() => { extra = opened[0].h.onReattach!() })
  expect(extra).toEqual({ offset: 10 })
  act(() => opened[0].h.onText!({ ch: 'pty.0', type: 'replay', data: { reset: false, offset: 10 } }))
  expect(terms[0].reset).not.toHaveBeenCalled()
  act(() => opened[0].h.onText!({ ch: 'pty.0', type: 'replay', data: { reset: true, offset: 4000 } }))
  expect(terms[0].reset).toHaveBeenCalled()
})

test('shell thoát thì đóng tab; tab cuối thì đóng cửa sổ', () => {
  const { opened, props } = setup()
  act(() => opened[0].h.onClose!('exit', 0))
  expect(props.requestClose).toHaveBeenCalled()
})

test('nhiều tab: thêm tab mở kênh mới; thoát một tab không đóng cửa sổ', () => {
  const { opened, props } = setup()
  fireEvent.click(screen.getByLabelText('Tab mới'))
  expect(opened).toHaveLength(2)
  act(() => opened[1].h.onClose!('exit', 0))
  expect(props.requestClose).not.toHaveBeenCalled()
  expect(screen.getAllByRole('tab')).toHaveLength(1)
})

test('lỗi mở kênh và mất phiên sau 60 giây lên băng đỏ', () => {
  const { opened, props } = setup()
  act(() => opened[0].h.onError!('invalid-params', 'shell /bin/zsh is not listed in /etc/shells'))
  expect(props.report).toHaveBeenCalledWith('error', 'invalid-params: shell /bin/zsh is not listed in /etc/shells')
  const again = setup()
  act(() => again.opened[0].h.onClose!('gone'))
  expect(again.props.report).toHaveBeenCalledWith('error', expect.stringContaining('60 giây'))
})
