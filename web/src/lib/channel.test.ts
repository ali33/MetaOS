import { Connection, type WebSocketLike, type ConnState } from './channel'
import { encodeBinary, decodeBinary } from './protocol'

class FakeSocket implements WebSocketLike {
  binaryType = 'blob'
  readyState = 0
  sent: (string | Uint8Array)[] = []
  onopen: ((e: any) => void) | null = null
  onclose: ((e: { code: number; reason: string }) => void) | null = null
  onmessage: ((e: { data: any }) => void) | null = null
  send(d: string | Uint8Array) { this.sent.push(d) }
  close() {}
  open() { this.readyState = 1; this.onopen?.({}) }
  drop(code = 1006, reason = '') { this.readyState = 3; this.onclose?.({ code, reason }) }
  text(m: object) { this.onmessage?.({ data: JSON.stringify(m) }) }
  bin(ch: string, s: string) { this.onmessage?.({ data: encodeBinary(ch, new TextEncoder().encode(s)).buffer }) }
  json() { return this.sent.filter((x): x is string => typeof x === 'string').map((x) => JSON.parse(x)) }
}

function setup(checkSession = async () => true) {
  const sockets: FakeSocket[] = []
  const states: ConnState[] = []
  const conn = new Connection({
    url: 'wss://h/ws',
    socketFactory: () => { const s = new FakeSocket(); sockets.push(s); return s },
    checkSession,
    onState: (s) => states.push(s),
  })
  conn.connect()
  return { conn, sockets, states, last: () => sockets[sockets.length - 1] }
}

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

test('open trước khi socket mở thì xếp hàng, mở xong mới gửi', () => {
  const { conn, last } = setup()
  const ch = conn.open('pty', { cols: 80, rows: 24 }, {})
  expect(last().sent).toHaveLength(0)
  last().open()
  expect(last().json()).toEqual([{ ch: '', type: 'open', data: { ch: ch.id, kind: 'pty', params: { cols: 80, rows: 24 } } }])
  expect(last().binaryType).toBe('arraybuffer')
})

test('ready, dữ liệu nhị phân, text và close được chuyển đúng kênh', () => {
  const { conn, last } = setup()
  last().open()
  const h = { onReady: vi.fn(), onBinary: vi.fn(), onText: vi.fn(), onClose: vi.fn() }
  const ch = conn.open('pty', {}, h)
  last().text({ ch: '', type: 'ready', data: { ch: ch.id } })
  last().bin(ch.id, 'xin chào')
  last().text({ ch: ch.id, type: 'x', data: { a: 1 } })
  last().text({ ch: '', type: 'close', data: { ch: ch.id, reason: 'exit', exitCode: 3 } })
  expect(h.onReady).toHaveBeenCalledOnce()
  expect(new TextDecoder().decode(h.onBinary.mock.calls[0][0])).toBe('xin chào')
  expect(h.onText).toHaveBeenCalledWith({ ch: ch.id, type: 'x', data: { a: 1 } })
  expect(h.onClose).toHaveBeenCalledWith('exit', 3)
})

test('sendBinary mã hoá chuỗi thành UTF-8 kèm id kênh', () => {
  const { conn, last } = setup()
  last().open()
  const ch = conn.open('pty', {}, {})
  ch.sendBinary('ls\r')
  const frame = last().sent.find((x) => x instanceof Uint8Array) as Uint8Array
  const { ch: id, payload } = decodeBinary(frame)
  expect(id).toBe(ch.id)
  expect(new TextDecoder().decode(payload)).toBe('ls\r')
})

test('error khi mở kênh: gọi onError và bỏ kênh', () => {
  const { conn, last } = setup()
  last().open()
  const h = { onError: vi.fn(), onBinary: vi.fn() }
  const ch = conn.open('pty', {}, h)
  last().text({ ch: '', type: 'error', data: { ch: ch.id, code: 'invalid-params', message: 'shell /x is not listed' } })
  expect(h.onError).toHaveBeenCalledWith('invalid-params', 'shell /x is not listed')
  last().bin(ch.id, 'x')
  expect(h.onBinary).not.toHaveBeenCalled()
})

test('rớt kết nối: nối lại theo 1, 2, 4… tối đa 30 giây', async () => {
  const { sockets, states } = setup()
  sockets[0].open()
  sockets[0].drop()
  expect(states.at(-1)).toBe('reconnecting')
  const waits = [1, 2, 4, 8, 16, 30, 30]
  for (const [i, sec] of waits.entries()) {
    await vi.advanceTimersByTimeAsync(sec * 1000 - 1)
    expect(sockets).toHaveLength(i + 1)
    await vi.advanceTimersByTimeAsync(1)
    expect(sockets).toHaveLength(i + 2)
    sockets[i + 1].drop()
  }
})

test('nối lại thành công thì độ trễ quay về 1 giây', async () => {
  const { sockets } = setup()
  sockets[0].open(); sockets[0].drop()
  await vi.advanceTimersByTimeAsync(1000); sockets[1].drop()
  await vi.advanceTimersByTimeAsync(2000); sockets[2].open(); sockets[2].drop()
  await vi.advanceTimersByTimeAsync(1000)
  expect(sockets).toHaveLength(4)
})

test('gắn lại kênh reattachable, đóng kênh thường', async () => {
  const { conn, sockets, last } = setup()
  sockets[0].open()
  const order: string[] = []
  const onReattach = () => { order.push('reattach'); return { offset: 5 } }
  const pty = conn.open('pty', { cols: 80 }, { onReattach, onClose: vi.fn() }, { reattachable: true })
  const other = { onClose: vi.fn() }
  const o = conn.open('misc', {}, other)
  sockets[0].text({ ch: '', type: 'ready', data: { ch: pty.id } })
  sockets[0].text({ ch: '', type: 'ready', data: { ch: o.id } })
  sockets[0].drop()
  expect(other.onClose).toHaveBeenCalledWith('disconnected')
  await vi.advanceTimersByTimeAsync(1000)
  last().open()
  expect(order).toEqual(['reattach'])
  expect(last().json()).toEqual([{ ch: '', type: 'open', data: { ch: pty.id, kind: 'pty', params: { cols: 80, offset: 5, reattach: true } } }])
})

test('gắn lại thất bại (not-found) thì onClose("gone")', async () => {
  const { conn, sockets, last } = setup()
  sockets[0].open()
  const h = { onClose: vi.fn() }
  const pty = conn.open('pty', {}, h, { reattachable: true })
  sockets[0].text({ ch: '', type: 'ready', data: { ch: pty.id } })
  sockets[0].drop()
  await vi.advanceTimersByTimeAsync(1000)
  last().open()
  last().text({ ch: '', type: 'error', data: { ch: pty.id, code: 'not-found', message: 'channel is gone' } })
  expect(h.onClose).toHaveBeenCalledWith('gone')
})

test('channels: báo các kênh lạ để tiếp quản, bỏ qua kênh đã biết', () => {
  const orphans = vi.fn()
  const sockets: FakeSocket[] = []
  const conn = new Connection({ url: 'wss://h/ws', socketFactory: () => { const s = new FakeSocket(); sockets.push(s); return s }, onOrphans: orphans })
  conn.connect()
  sockets[0].open()
  const mine = conn.open('pty', {}, {}, { reattachable: true })
  sockets[0].text({ ch: '', type: 'channels', data: [{ ch: mine.id, kind: 'pty' }, { ch: 'pty.old.1', kind: 'pty' }] })
  expect(orphans).toHaveBeenCalledWith([{ ch: 'pty.old.1', kind: 'pty' }])
  const h = { onReattach: () => ({ offset: 0 }) }
  conn.adopt('pty.old.1', 'pty', h)
  expect(sockets[0].json().at(-1)).toEqual({ ch: '', type: 'open', data: { ch: 'pty.old.1', kind: 'pty', params: { offset: 0, reattach: true } } })
})

test('4001: bị tab khác thay, không tự nối lại', async () => {
  const { sockets, states } = setup()
  sockets[0].open()
  sockets[0].drop(4001, 'replaced')
  await vi.advanceTimersByTimeAsync(120_000)
  expect(sockets).toHaveLength(1)
  expect(states.at(-1)).toBe('replaced')
})

test('4401: phiên hết hạn', async () => {
  const { sockets, states } = setup()
  sockets[0].open()
  sockets[0].drop(4401, 'expired')
  await vi.advanceTimersByTimeAsync(120_000)
  expect(sockets).toHaveLength(1)
  expect(states.at(-1)).toBe('expired')
})

test('nối lại thất bại và checkSession=false thì expired', async () => {
  const { sockets, states } = setup(async () => false)
  sockets[0].open()
  sockets[0].drop()
  await vi.advanceTimersByTimeAsync(1000)
  sockets[1].drop() // chưa từng mở: nâng cấp bị 401
  await vi.advanceTimersByTimeAsync(0)
  expect(states.at(-1)).toBe('expired')
  await vi.advanceTimersByTimeAsync(120_000)
  expect(sockets).toHaveLength(2)
})

test('F3: close khi mất kết nối ⇒ sau nối lại gửi close, không gửi open gắn lại', async () => {
  const { conn, sockets, last } = setup()
  sockets[0].open()
  const h = { onClose: vi.fn() }
  const pty = conn.open('pty', {}, h, { reattachable: true })
  sockets[0].text({ ch: '', type: 'ready', data: { ch: pty.id } })
  sockets[0].drop()
  pty.close()
  await vi.advanceTimersByTimeAsync(1000)
  last().open()
  expect(last().json()).toEqual([{ ch: '', type: 'close', data: { ch: pty.id } }])
  // chỉ gửi một lần: lần nối lại kế tiếp không còn close
  last().drop()
  await vi.advanceTimersByTimeAsync(1000)
  last().open()
  expect(last().json()).toEqual([])
})

test('F3: channels chứa kênh vừa đóng lúc mất kết nối thì không báo onOrphans', async () => {
  const orphans = vi.fn()
  const sockets: FakeSocket[] = []
  const conn = new Connection({ url: 'wss://h/ws', socketFactory: () => { const s = new FakeSocket(); sockets.push(s); return s }, onOrphans: orphans })
  conn.connect()
  sockets[0].open()
  const pty = conn.open('pty', {}, {}, { reattachable: true })
  sockets[0].text({ ch: '', type: 'ready', data: { ch: pty.id } })
  sockets[0].drop()
  pty.close()
  await vi.advanceTimersByTimeAsync(1000)
  sockets[1].open()
  sockets[1].text({ ch: '', type: 'channels', data: [{ ch: pty.id, kind: 'pty' }, { ch: 'pty.old.1', kind: 'pty' }] })
  expect(orphans).toHaveBeenCalledWith([{ ch: 'pty.old.1', kind: 'pty' }])
})

test('F3: close trước khi socket mở lần đầu ⇒ không gửi open cũng không gửi close', () => {
  const { conn, last } = setup()
  const h = { onClose: vi.fn() }
  const ch = conn.open('pty', {}, h)
  ch.close()
  last().open()
  expect(last().json()).toEqual([])
  expect(h.onClose).toHaveBeenCalledWith('closed')
})

test('F3: close khi mất kết nối gọi onClose("closed")', async () => {
  const { conn, sockets } = setup()
  sockets[0].open()
  const h = { onClose: vi.fn() }
  const pty = conn.open('pty', {}, h, { reattachable: true })
  sockets[0].text({ ch: '', type: 'ready', data: { ch: pty.id } })
  sockets[0].drop()
  pty.close()
  expect(h.onClose).toHaveBeenCalledWith('closed')
})

test('F3: closedSeen được dọn sau khung channels đầu tiên', async () => {
  const orphans = vi.fn()
  const sockets: FakeSocket[] = []
  const conn = new Connection({ url: 'wss://h/ws', socketFactory: () => { const s = new FakeSocket(); sockets.push(s); return s }, onOrphans: orphans })
  conn.connect()
  sockets[0].open()
  const pty = conn.open('pty', {}, {}, { reattachable: true })
  sockets[0].text({ ch: '', type: 'ready', data: { ch: pty.id } })
  sockets[0].drop()
  pty.close()
  await vi.advanceTimersByTimeAsync(1000)
  sockets[1].open()
  sockets[1].text({ ch: '', type: 'channels', data: [] })
  expect(orphans).not.toHaveBeenCalled()
  // id không còn được nhớ: nếu bridge liệt kê lại thì là kênh lạ
  sockets[1].text({ ch: '', type: 'channels', data: [{ ch: pty.id, kind: 'pty' }] })
  expect(orphans).toHaveBeenCalledWith([{ ch: pty.id, kind: 'pty' }])
})
