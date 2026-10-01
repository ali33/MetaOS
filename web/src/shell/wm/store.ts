export type Rect = { x: number; y: number; w: number; h: number }
export type WinState = 'normal' | 'maximized' | 'tiled-left' | 'tiled-right'
export type Edge = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'
export type Win = {
  id: string; appId: string; title: string; rect: Rect; state: WinState
  minimized: boolean; z: number; restore: Rect | null
}
export type WM = { wins: Win[]; focused: string | null; nextZ: number; area: { w: number; h: number }; opened: number }

export const MIN_W = 320
export const MIN_H = 200
export const SNAP = 8
export const KEEP_VISIBLE = 64
const TITLE_H = 32

export type Action =
  | { type: 'open'; id: string; appId: string; title: string }
  | { type: 'close'; id: string }
  | { type: 'focus'; id: string }
  | { type: 'move'; id: string; dx: number; dy: number }
  | { type: 'dragEnd'; id: string; px: number; py: number }
  | { type: 'resize'; id: string; edge: Edge; dx: number; dy: number }
  | { type: 'minimize'; id: string }
  | { type: 'toggleMaximize'; id: string }
  | { type: 'tile'; id: string; side: 'left' | 'right' }
  | { type: 'restore'; id: string }
  | { type: 'cycle'; dir: 1 | -1 }
  | { type: 'setArea'; w: number; h: number }
  | { type: 'setTitle'; id: string; title: string }

export const initialWM = (area: { w: number; h: number }): WM => ({ wins: [], focused: null, nextZ: 1, area, opened: 0 })
export const byZ = (s: WM) => [...s.wins].sort((a, b) => a.z - b.z)

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

function stateRect(state: WinState, area: WM['area'], fallback: Rect): Rect {
  const half = Math.floor(area.w / 2)
  switch (state) {
    case 'maximized': return { x: 0, y: 0, w: area.w, h: area.h }
    case 'tiled-left': return { x: 0, y: 0, w: half, h: area.h }
    case 'tiled-right': return { x: half, y: 0, w: area.w - half, h: area.h }
    default: return fallback
  }
}

function update(s: WM, id: string, f: (w: Win) => Win): WM {
  const w = s.wins.find((x) => x.id === id)
  if (!w) return s
  return { ...s, wins: s.wins.map((x) => (x === w ? f(w) : x)) }
}

function focusTop(s: WM, exclude?: string): string | null {
  const c = byZ(s).filter((w) => !w.minimized && w.id !== exclude)
  return c.length ? c[c.length - 1].id : null
}

function raise(s: WM, id: string): WM {
  const t = update(s, id, (w) => ({ ...w, z: s.nextZ, minimized: false }))
  return t === s ? s : { ...t, nextZ: s.nextZ + 1, focused: id }
}

// setState chuyển sang trạng thái mới, nhớ cỡ cũ khi rời 'normal'.
function setState(w: Win, state: WinState, area: WM['area']): Win {
  if (state === 'normal') return { ...w, state, rect: w.restore ?? w.rect, restore: null }
  const restore = w.state === 'normal' ? w.rect : w.restore
  return { ...w, state, restore, rect: stateRect(state, area, w.rect) }
}

function clampPos(r: Rect, area: WM['area']): Rect {
  return {
    ...r,
    x: clamp(r.x, -(r.w - KEEP_VISIBLE), area.w - KEEP_VISIBLE),
    y: clamp(r.y, 0, area.h - TITLE_H),
  }
}

function resizeRect(r: Rect, edge: Edge, dx: number, dy: number): Rect {
  let { x, y, w, h } = r
  if (edge.includes('e')) w = Math.max(MIN_W, w + dx)
  if (edge.includes('s')) h = Math.max(MIN_H, h + dy)
  if (edge.includes('w')) { const right = x + w; w = Math.max(MIN_W, w - dx); x = right - w }
  if (edge.includes('n')) { const bottom = y + h; h = Math.max(MIN_H, h - dy); y = bottom - h }
  return { x, y, w, h }
}

export function reduce(s: WM, a: Action): WM {
  switch (a.type) {
    case 'open': {
      const step = 24 * (s.opened % 8)
      const rect = { x: 40 + step, y: 40 + step, w: Math.min(800, s.area.w - 80), h: Math.min(520, s.area.h - 80) }
      const w: Win = { id: a.id, appId: a.appId, title: a.title, rect, state: 'normal', minimized: false, z: s.nextZ, restore: null }
      return { ...s, wins: [...s.wins, w], nextZ: s.nextZ + 1, focused: a.id, opened: s.opened + 1 }
    }
    case 'close': {
      if (!s.wins.some((w) => w.id === a.id)) return s
      const t = { ...s, wins: s.wins.filter((w) => w.id !== a.id) }
      return { ...t, focused: s.focused === a.id ? focusTop(t) : s.focused }
    }
    case 'focus':
      return raise(s, a.id)
    case 'move':
      return update(s, a.id, (w) => {
        const base = w.state === 'normal' ? w.rect : { ...(w.restore ?? w.rect), x: w.rect.x, y: w.rect.y }
        return { ...w, state: 'normal', restore: null, rect: clampPos({ ...base, x: base.x + a.dx, y: base.y + a.dy }, s.area) }
      })
    case 'dragEnd': {
      const target: WinState | null =
        a.px <= SNAP ? 'tiled-left' : a.px >= s.area.w - SNAP ? 'tiled-right' : a.py <= SNAP ? 'maximized' : null
      return target ? update(s, a.id, (w) => setState(w, target, s.area)) : s
    }
    case 'resize':
      return update(s, a.id, (w) => ({ ...w, state: 'normal', restore: null, rect: resizeRect(w.rect, a.edge, a.dx, a.dy) }))
    case 'minimize': {
      const t = update(s, a.id, (w) => ({ ...w, minimized: true }))
      return t === s ? s : { ...t, focused: s.focused === a.id ? focusTop(t, a.id) : s.focused }
    }
    case 'toggleMaximize':
      return update(s, a.id, (w) => setState(w, w.state === 'maximized' ? 'normal' : 'maximized', s.area))
    case 'tile': {
      const want: WinState = a.side === 'left' ? 'tiled-left' : 'tiled-right'
      return update(s, a.id, (w) => setState(w, w.state === want ? 'normal' : want, s.area))
    }
    case 'restore': {
      const w = s.wins.find((x) => x.id === a.id)
      if (!w) return s
      return w.state === 'normal' ? reduce(s, { type: 'minimize', id: a.id }) : update(s, a.id, (x) => setState(x, 'normal', s.area))
    }
    case 'cycle': {
      if (s.wins.length < 2) return s
      // Thứ tự dùng gần nhất: z giảm dần. dir=1 sang cửa sổ kế, dir=-1 về cửa sổ cũ nhất.
      const mru = [...byZ(s)].reverse()
      const i = Math.max(0, mru.findIndex((w) => w.id === s.focused))
      const next = mru[(i + (a.dir === 1 ? 1 : mru.length - 1)) % mru.length]
      return raise(s, next.id)
    }
    case 'setArea': {
      const area = { w: a.w, h: a.h }
      return { ...s, area, wins: s.wins.map((w) => ({ ...w, rect: stateRect(w.state, area, w.rect) })) }
    }
    case 'setTitle':
      return update(s, a.id, (w) => ({ ...w, title: a.title }))
  }
}
