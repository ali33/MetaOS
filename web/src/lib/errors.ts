export type Level = 'error' | 'warn'
export type Banner = { id: number; level: Level; text: string; count: number }
export const MAX_BANNERS = 5

let nextId = 1

// BannerList: băng lỗi đỏ (chặn) và hổ phách (không chặn). Gộp trùng, tối đa 5 dòng.
export class BannerList {
  items: Banner[] = []
  private subs = new Set<() => void>()

  add(level: Level, text: string) {
    const same = this.items.find((b) => b.level === level && b.text === text)
    if (same) {
      this.items = this.items.map((b) => (b === same ? { ...b, count: b.count + 1 } : b))
    } else {
      this.items = [...this.items, { id: nextId++, level, text, count: 1 }].slice(-MAX_BANNERS)
    }
    this.emit()
  }
  clear(level?: Level) {
    this.items = level ? this.items.filter((b) => b.level !== level) : []
    this.emit()
  }
  dismiss(id: number) {
    this.items = this.items.filter((b) => b.id !== id)
    this.emit()
  }
  subscribe(fn: () => void) {
    this.subs.add(fn)
    return () => void this.subs.delete(fn)
  }
  private emit() {
    for (const f of this.subs) f()
  }
}

export function installGlobalHandlers(list: BannerList, target: Window = window): () => void {
  const onError = (e: ErrorEvent) => {
    console.error(e.error ?? e.message)
    const where = e.filename ? ` (${e.filename.split('/').pop()}:${e.lineno})` : ''
    list.add('warn', `${e.message}${where}`)
  }
  const onRejection = (e: PromiseRejectionEvent) => {
    console.error(e.reason)
    const msg = e.reason instanceof Error ? e.reason.message : String(e.reason)
    list.add('warn', `Promise bị bỏ rơi: ${msg}`)
  }
  target.addEventListener('error', onError)
  target.addEventListener('unhandledrejection', onRejection as EventListener)
  return () => {
    target.removeEventListener('error', onError)
    target.removeEventListener('unhandledrejection', onRejection as EventListener)
  }
}
