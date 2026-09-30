import { decodeBinary, encodeBinary, type Msg } from './protocol'

export type ConnState = 'connecting' | 'open' | 'reconnecting' | 'replaced' | 'expired'
export const BACKOFF_SECONDS = [1, 2, 4, 8, 16, 30]
export const CLOSE_REPLACED = 4001
export const CLOSE_SESSION_ENDED = 4401

export interface ChannelHandlers {
  onReady?(): void
  onReattach?(): object | void
  onText?(m: Msg): void
  onBinary?(p: Uint8Array): void
  onClose?(reason: string, exitCode?: number): void
  onError?(code: string, message: string): void
}

export interface WebSocketLike {
  binaryType: string
  readyState: number
  send(d: string | Uint8Array): void
  close(code?: number, reason?: string): void
  onopen: ((e: any) => void) | null
  onclose: ((e: { code: number; reason: string }) => void) | null
  onmessage: ((e: { data: any }) => void) | null
}

export interface ConnOptions {
  url?: string
  socketFactory?: (url: string) => WebSocketLike
  checkSession?: () => Promise<boolean>
  onState?(s: ConnState): void
  onControlError?(code: string, message: string): void
  onOrphans?(list: { ch: string; kind: string }[]): void
}

const enc = new TextEncoder()
let seq = 0

export class Channel {
  ready = false
  reattaching = false
  constructor(
    readonly id: string,
    readonly kind: string,
    readonly params: object,
    readonly reattachable: boolean,
    readonly h: ChannelHandlers,
    private conn: Connection,
  ) {}
  sendText(type: string, data?: unknown) { this.conn.sendRaw(JSON.stringify({ ch: this.id, type, data })) }
  sendBinary(p: Uint8Array | string) {
    this.conn.sendRaw(encodeBinary(this.id, typeof p === 'string' ? enc.encode(p) : p))
  }
  close() { this.conn.closeChannel(this) }
}

export class Connection {
  state: ConnState = 'connecting'
  private sock: WebSocketLike | null = null
  private chans = new Map<string, Channel>()
  // Kênh đã đóng khi socket chưa mở: gửi close cho bridge ngay khi nối lại, và không báo là kênh lạ.
  private closedIds = new Set<string>()
  // Cùng các id đó nhưng giữ đến khung `channels` đầu tiên sau khi nối lại (đã gửi close vẫn có thể còn trong danh sách).
  private closedSeen = new Set<string>()
  private attempt = 0
  private timer: ReturnType<typeof setTimeout> | null = null
  private disposed = false
  private url: string
  private factory: (url: string) => WebSocketLike

  constructor(private o: ConnOptions = {}) {
    this.url = o.url ?? `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`
    this.factory = o.socketFactory ?? ((u) => new WebSocket(u) as unknown as WebSocketLike)
  }

  private setState(s: ConnState) {
    if (this.state === s) return
    this.state = s
    this.o.onState?.(s)
  }

  connect() {
    if (this.disposed) return
    const s = this.factory(this.url)
    s.binaryType = 'arraybuffer'
    let opened = false
    s.onopen = () => {
      opened = true
      this.attempt = 0
      this.setState('open')
      for (const id of this.closedIds) {
        this.sendRaw(JSON.stringify({ ch: '', type: 'close', data: { ch: id } }))
      }
      this.closedIds.clear()
      for (const ch of this.chans.values()) this.sendOpen(ch)
    }
    s.onmessage = (e) => this.onMessage(e.data)
    s.onclose = (e) => {
      if (this.sock !== s) return
      this.sock = null
      if (e.code === CLOSE_REPLACED) return this.setState('replaced')
      if (e.code === CLOSE_SESSION_ENDED) return this.setState('expired')
      for (const ch of [...this.chans.values()]) {
        if (ch.reattachable && ch.ready) continue
        this.chans.delete(ch.id)
        ch.h.onClose?.('disconnected')
      }
      this.setState('reconnecting')
      if (!opened && this.o.checkSession) {
        this.o.checkSession().then((ok) => (ok ? this.schedule() : this.setState('expired')), () => this.schedule())
      } else {
        this.schedule()
      }
    }
    this.sock = s
  }

  private schedule() {
    if (this.disposed) return
    const sec = BACKOFF_SECONDS[Math.min(this.attempt, BACKOFF_SECONDS.length - 1)]
    this.attempt++
    this.timer = setTimeout(() => this.connect(), sec * 1000)
  }

  private isOpen() {
    return !!this.sock && this.sock.readyState === 1
  }

  open(kind: string, params: object, h: ChannelHandlers, o: { reattachable?: boolean } = {}): Channel {
    const id = `${kind}.${(++seq).toString(36)}.${Math.random().toString(36).slice(2, 6)}`
    const ch = new Channel(id, kind, params, !!o.reattachable, h, this)
    this.chans.set(id, ch)
    if (this.isOpen()) this.sendOpen(ch)
    return ch
  }

  // adopt tiếp quản một kênh đang sống trong bridge (tab mới / tải lại trang).
  // Đánh dấu ready để sendOpen đi nhánh gắn lại.
  adopt(id: string, kind: string, h: ChannelHandlers): Channel {
    const ch = new Channel(id, kind, {}, true, h, this)
    ch.ready = true
    this.chans.set(id, ch)
    if (this.isOpen()) this.sendOpen(ch)
    return ch
  }

  closeChannel(ch: Channel) {
    if (this.isOpen()) {
      this.sendRaw(JSON.stringify({ ch: '', type: 'close', data: { ch: ch.id } }))
      return
    }
    // Socket chưa mở: bỏ kênh ngay để không gắn lại. Chỉ khi bridge có thể đã biết kênh
    // (từng ready hoặc đang gắn lại) mới nhớ id để đóng phía bridge khi nối lại.
    this.chans.delete(ch.id)
    if (ch.ready || ch.reattaching) {
      this.closedIds.add(ch.id)
      this.closedSeen.add(ch.id)
    }
    ch.h.onClose?.('closed')
  }

  private sendOpen(ch: Channel) {
    let params: object = ch.params
    if (ch.ready) {
      ch.reattaching = true
      const extra = ch.h.onReattach?.() ?? {}
      params = { ...ch.params, ...extra, reattach: true }
    }
    this.sendRaw(JSON.stringify({ ch: '', type: 'open', data: { ch: ch.id, kind: ch.kind, params } }))
  }

  sendRaw(d: string | Uint8Array) {
    if (this.isOpen()) this.sock!.send(d)
  }

  private onMessage(data: unknown) {
    if (typeof data !== 'string') {
      let f
      try {
        f = decodeBinary(data as ArrayBuffer)
      } catch (err) {
        this.o.onControlError?.('internal', `frame nhị phân hỏng: ${(err as Error).message}`)
        return
      }
      this.chans.get(f.ch)?.h.onBinary?.(f.payload)
      return
    }
    let m: Msg
    try {
      m = JSON.parse(data)
    } catch {
      this.o.onControlError?.('internal', `frame văn bản không phải JSON: ${data.slice(0, 120)}`)
      return
    }
    if (m.ch) return void this.chans.get(m.ch)?.h.onText?.(m)
    const d = m.data ?? {}
    const ch = d.ch ? this.chans.get(d.ch) : undefined
    switch (m.type) {
      case 'ready':
        if (ch) { ch.ready = true; ch.reattaching = false; ch.h.onReady?.() }
        return
      case 'close':
        if (ch) { this.chans.delete(ch.id); ch.h.onClose?.(d.reason, d.exitCode) }
        return
      case 'error':
        if (!ch) return void this.o.onControlError?.(d.code, d.message)
        if (ch.reattaching && d.code === 'not-found') {
          this.chans.delete(ch.id)
          return void ch.h.onClose?.('gone')
        }
        if (!ch.ready) this.chans.delete(ch.id)
        ch.h.onError?.(d.code, d.message)
        return
      case 'pong':
        return
      case 'channels': {
        const list = (Array.isArray(m.data) ? m.data : []) as { ch: string; kind: string }[]
        const unknown = list.filter((c) => !this.chans.has(c.ch) && !this.closedSeen.has(c.ch))
        this.closedSeen.clear() // chỉ cần cho khung channels đầu tiên sau khi nối lại
        if (unknown.length) this.o.onOrphans?.(unknown)
        return
      }
      default:
        this.o.onControlError?.('unsupported', `thông điệp điều khiển lạ: ${m.type}`)
    }
  }

  dispose() {
    this.disposed = true
    if (this.timer) clearTimeout(this.timer)
    const s = this.sock
    this.sock = null
    s?.close(1000, 'dispose')
  }
}
