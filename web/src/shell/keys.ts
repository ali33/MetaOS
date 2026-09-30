import { useEffect, useRef } from 'react'

export type Command = 'activities' | 'tile-left' | 'tile-right' | 'maximize' | 'restore' | 'cycle-next' | 'cycle-prev' | 'terminal'
export type KeyLike = { key: string; code: string; altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean; type: string }

export const SHORTCUTS: { command: Command; label: string; primary: string; alternate: string }[] = [
  { command: 'activities', label: 'Hoạt động', primary: 'Super', alternate: 'Alt+F1' },
  { command: 'tile-left', label: 'Chia đôi bên trái', primary: 'Super+←', alternate: 'Ctrl+Alt+←' },
  { command: 'tile-right', label: 'Chia đôi bên phải', primary: 'Super+→', alternate: 'Ctrl+Alt+→' },
  { command: 'maximize', label: 'Phóng to', primary: 'Super+↑', alternate: 'Ctrl+Alt+↑' },
  { command: 'restore', label: 'Bỏ phóng to / thu nhỏ', primary: 'Super+↓', alternate: 'Ctrl+Alt+↓' },
  { command: 'cycle-next', label: 'Cửa sổ kế tiếp', primary: 'Alt+Tab', alternate: 'Alt+`' },
  { command: 'cycle-prev', label: 'Cửa sổ trước', primary: 'Alt+Shift+Tab', alternate: 'Alt+Shift+`' },
  { command: 'terminal', label: 'Mở Terminal', primary: 'Ctrl+Alt+T', alternate: '' },
]

const ARROWS: Record<string, Command> = { ArrowLeft: 'tile-left', ArrowRight: 'tile-right', ArrowUp: 'maximize', ArrowDown: 'restore' }

export class ShortcutMatcher {
  private superDown = false
  private superUsed = false

  handle(e: KeyLike): Command | null {
    if (e.key === 'Meta') {
      if (e.type === 'keydown') { this.superDown = true; this.superUsed = false; return null }
      const tap = this.superDown && !this.superUsed
      this.superDown = false
      return tap ? 'activities' : null
    }
    if (e.type !== 'keydown') return null
    if (this.superDown) this.superUsed = true
    const superish = e.metaKey || (e.ctrlKey && e.altKey)
    if (superish && ARROWS[e.key]) return ARROWS[e.key]
    if (e.altKey && !e.ctrlKey && !e.metaKey) {
      if (e.key === 'Tab' || e.key === '`' || e.code === 'Backquote') return e.shiftKey ? 'cycle-prev' : 'cycle-next'
      if (e.key === 'F1') return 'activities'
    }
    if (e.ctrlKey && e.altKey && e.key.toLowerCase() === 't') return 'terminal'
    return null
  }
}

export function useShortcuts(onCommand: (c: Command) => void) {
  const cb = useRef(onCommand)
  cb.current = onCommand
  useEffect(() => {
    const m = new ShortcutMatcher()
    const on = (e: KeyboardEvent) => {
      const c = m.handle(e)
      if (c) {
        e.preventDefault()
        e.stopPropagation() // không để xterm nhận phím tắt của desktop
        cb.current(c)
      }
    }
    window.addEventListener('keydown', on, true)
    window.addEventListener('keyup', on, true)
    return () => {
      window.removeEventListener('keydown', on, true)
      window.removeEventListener('keyup', on, true)
    }
  }, [])
}
