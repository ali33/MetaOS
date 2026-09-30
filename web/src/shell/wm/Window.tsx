import { useRef, type PointerEvent as RPE, type ReactNode } from 'react'
import type { Action, Edge, Win } from './store'

const EDGES: Edge[] = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw']

type Props = {
  win: Win
  focused: boolean
  dispatch(a: Action): void
  toArea(clientX: number, clientY: number): { x: number; y: number }
  banners: ReactNode
  children: ReactNode
}

// Kéo: chuyển độ dời con trỏ thành action; luật (kẹp, bắt cạnh) nằm trong store.
function useDrag(onMove: (dx: number, dy: number) => void, onEnd?: (e: RPE) => void) {
  const last = useRef<{ x: number; y: number } | null>(null)
  return {
    onPointerDown(e: RPE) {
      if (e.button !== 0) return
      ;(e.currentTarget as Element).setPointerCapture?.(e.pointerId)
      last.current = { x: e.clientX, y: e.clientY }
    },
    onPointerMove(e: RPE) {
      if (!last.current) return
      onMove(e.clientX - last.current.x, e.clientY - last.current.y)
      last.current = { x: e.clientX, y: e.clientY }
    },
    onPointerUp(e: RPE) {
      if (!last.current) return
      last.current = null
      onEnd?.(e)
    },
  }
}

export function Window({ win, focused, dispatch, toArea, banners, children }: Props) {
  const drag = useDrag(
    (dx, dy) => dispatch({ type: 'move', id: win.id, dx, dy }),
    (e) => { const p = toArea(e.clientX, e.clientY); dispatch({ type: 'dragEnd', id: win.id, px: p.x, py: p.y }) },
  )
  const { x, y, w, h } = win.rect
  return (
    <section
      className={`win ${win.state} ${focused ? 'focused' : 'unfocused'}`}
      style={{ left: x, top: y, width: w, height: h, zIndex: win.z, display: win.minimized ? 'none' : undefined }}
      aria-label={win.title}
      onPointerDownCapture={() => { if (!focused) dispatch({ type: 'focus', id: win.id }) }}
    >
      <header className="titlebar" {...drag} onDoubleClick={() => dispatch({ type: 'toggleMaximize', id: win.id })}>
        <span className="title">{win.title}</span>
        <button aria-label="Thu nhỏ" onPointerDown={(e) => e.stopPropagation()} onClick={() => dispatch({ type: 'minimize', id: win.id })}>–</button>
        <button aria-label="Phóng to" onPointerDown={(e) => e.stopPropagation()} onClick={() => dispatch({ type: 'toggleMaximize', id: win.id })}>▢</button>
        <button aria-label="Đóng" onPointerDown={(e) => e.stopPropagation()} onClick={() => dispatch({ type: 'close', id: win.id })}>✕</button>
      </header>
      {banners}
      <div className="win-body">{children}</div>
      {win.state === 'normal' && EDGES.map((edge) => <ResizeHandle key={edge} edge={edge} id={win.id} dispatch={dispatch} />)}
    </section>
  )
}

function ResizeHandle({ edge, id, dispatch }: { edge: Edge; id: string; dispatch(a: Action): void }) {
  const drag = useDrag((dx, dy) => dispatch({ type: 'resize', id, edge, dx, dy }))
  return <div className={`rz rz-${edge}`} {...drag} />
}
