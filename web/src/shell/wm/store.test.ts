import { byZ, initialWM, MIN_H, MIN_W, reduce, type Action, type WM } from './store'

const area = { w: 1600, h: 900 }
const run = (...acts: Action[]) => acts.reduce(reduce, initialWM(area))
const win = (s: WM, id: string) => s.wins.find((w) => w.id === id)!
const open = (id: string): Action => ({ type: 'open', id, appId: 'terminal', title: id })

test('mở cửa sổ: xếp bậc thang, được focus, z tăng', () => {
  const s = run(open('a'), open('b'))
  expect(win(s, 'a').rect).toEqual({ x: 40, y: 40, w: 800, h: 520 })
  expect(win(s, 'b').rect).toEqual({ x: 64, y: 64, w: 800, h: 520 })
  expect(s.focused).toBe('b')
  expect(win(s, 'b').z).toBeGreaterThan(win(s, 'a').z)
})

test('cửa sổ mặc định không to hơn vùng làm việc nhỏ', () => {
  const s = [open('a')].reduce(reduce, initialWM({ w: 700, h: 400 }))
  expect(win(s, 'a').rect.w).toBe(620)
  expect(win(s, 'a').rect.h).toBe(320)
})

test('focus đưa lên trên cùng và bỏ thu nhỏ', () => {
  const s = run(open('a'), open('b'), { type: 'minimize', id: 'a' }, { type: 'focus', id: 'a' })
  expect(s.focused).toBe('a')
  expect(win(s, 'a').minimized).toBe(false)
  expect(byZ(s).at(-1)!.id).toBe('a')
})

test('đóng thì focus cửa sổ cao nhất còn lại chưa thu nhỏ', () => {
  const s = run(open('a'), open('b'), open('c'), { type: 'minimize', id: 'b' }, { type: 'close', id: 'c' })
  expect(s.wins.map((w) => w.id)).toEqual(['a', 'b'])
  expect(s.focused).toBe('a')
})

test('kéo bị kẹp để thanh tiêu đề không ra khỏi vùng làm việc', () => {
  const s = run(open('a'), { type: 'move', id: 'a', dx: -5000, dy: -5000 })
  expect(win(s, 'a').rect.x).toBe(-(800 - 64))
  expect(win(s, 'a').rect.y).toBe(0)
  const t = run(open('a'), { type: 'move', id: 'a', dx: 5000, dy: 5000 })
  expect(win(t, 'a').rect.x).toBe(1600 - 64)
  expect(win(t, 'a').rect.y).toBe(900 - 32)
})

test('thả sát cạnh trái/phải thì chia đôi, sát mép trên thì phóng to', () => {
  const l = run(open('a'), { type: 'dragEnd', id: 'a', px: 3, py: 400 })
  expect(win(l, 'a').state).toBe('tiled-left')
  expect(win(l, 'a').rect).toEqual({ x: 0, y: 0, w: 800, h: 900 })
  const r = run(open('a'), { type: 'dragEnd', id: 'a', px: 1598, py: 400 })
  expect(win(r, 'a').rect).toEqual({ x: 800, y: 0, w: 800, h: 900 })
  const m = run(open('a'), { type: 'dragEnd', id: 'a', px: 700, py: 2 })
  expect(win(m, 'a').state).toBe('maximized')
  const n = run(open('a'), { type: 'dragEnd', id: 'a', px: 700, py: 400 })
  expect(win(n, 'a').state).toBe('normal')
})

test('kéo cửa sổ đang phóng to thì trả về cỡ cũ', () => {
  const s = run(open('a'), { type: 'toggleMaximize', id: 'a' }, { type: 'move', id: 'a', dx: 10, dy: 20 })
  expect(win(s, 'a').state).toBe('normal')
  expect(win(s, 'a').rect).toEqual({ x: 10, y: 20, w: 800, h: 520 })
})

test('đổi cỡ theo 8 hướng, cạnh đối diện đứng yên', () => {
  const base = run(open('a'))
  const r = (edge: any, dx: number, dy: number) => win(reduce(base, { type: 'resize', id: 'a', edge, dx, dy }), 'a').rect
  expect(r('e', 100, 0)).toEqual({ x: 40, y: 40, w: 900, h: 520 })
  expect(r('s', 0, 50)).toEqual({ x: 40, y: 40, w: 800, h: 570 })
  expect(r('w', 30, 0)).toEqual({ x: 70, y: 40, w: 770, h: 520 })
  expect(r('n', 0, -20)).toEqual({ x: 40, y: 20, w: 800, h: 540 })
  expect(r('se', 10, 10)).toEqual({ x: 40, y: 40, w: 810, h: 530 })
  expect(r('nw', -10, -10)).toEqual({ x: 30, y: 30, w: 810, h: 530 })
  expect(r('ne', 10, -10)).toEqual({ x: 40, y: 30, w: 810, h: 530 })
  expect(r('sw', -10, 10)).toEqual({ x: 30, y: 40, w: 810, h: 530 })
})

test('không nhỏ hơn cỡ tối thiểu; kéo cạnh trái quá mức thì cạnh phải vẫn giữ', () => {
  const s = run(open('a'), { type: 'resize', id: 'a', edge: 'w', dx: 5000, dy: 0 })
  expect(win(s, 'a').rect.w).toBe(MIN_W)
  expect(win(s, 'a').rect.x + win(s, 'a').rect.w).toBe(40 + 800)
  const t = run(open('a'), { type: 'resize', id: 'a', edge: 'n', dx: 0, dy: 5000 })
  expect(win(t, 'a').rect.h).toBe(MIN_H)
  expect(win(t, 'a').rect.y + win(t, 'a').rect.h).toBe(40 + 520)
})

test('phóng to rồi bỏ phóng to trả đúng cỡ cũ', () => {
  const s = run(open('a'), { type: 'toggleMaximize', id: 'a' })
  expect(win(s, 'a').rect).toEqual({ x: 0, y: 0, w: 1600, h: 900 })
  const t = reduce(s, { type: 'toggleMaximize', id: 'a' })
  expect(win(t, 'a').state).toBe('normal')
  expect(win(t, 'a').rect).toEqual({ x: 40, y: 40, w: 800, h: 520 })
})

test('chia đôi cùng một bên lần nữa thì trả về bình thường', () => {
  const s = run(open('a'), { type: 'tile', id: 'a', side: 'left' }, { type: 'tile', id: 'a', side: 'left' })
  expect(win(s, 'a').state).toBe('normal')
  expect(win(s, 'a').rect).toEqual({ x: 40, y: 40, w: 800, h: 520 })
  const t = run(open('a'), { type: 'tile', id: 'a', side: 'left' }, { type: 'tile', id: 'a', side: 'right' })
  expect(win(t, 'a').rect).toEqual({ x: 800, y: 0, w: 800, h: 900 })
})

test('restore: bỏ phóng to / chia đôi, cửa sổ thường thì thu nhỏ (như GNOME Super+↓)', () => {
  const s = run(open('a'), { type: 'toggleMaximize', id: 'a' }, { type: 'restore', id: 'a' })
  expect(win(s, 'a').state).toBe('normal')
  const t = reduce(s, { type: 'restore', id: 'a' })
  expect(win(t, 'a').minimized).toBe(true)
})

test('Alt+Tab đi theo thứ tự dùng gần nhất và vòng lại', () => {
  let s = run(open('a'), open('b'), open('c'))
  s = reduce(s, { type: 'cycle', dir: 1 })
  expect(s.focused).toBe('b')
  s = reduce(s, { type: 'cycle', dir: 1 })
  expect(s.focused).toBe('c')
  s = reduce(s, { type: 'cycle', dir: -1 }) // MRU [c, b, a]: lùi một bước là cửa sổ lâu nhất
  expect(s.focused).toBe('a')
})

test('đổi cỡ vùng làm việc thì cửa sổ phóng to/chia đôi theo kịp', () => {
  const s = run(open('a'), open('b'), { type: 'toggleMaximize', id: 'a' }, { type: 'tile', id: 'b', side: 'right' },
    { type: 'setArea', w: 1000, h: 700 })
  expect(win(s, 'a').rect).toEqual({ x: 0, y: 0, w: 1000, h: 700 })
  expect(win(s, 'b').rect).toEqual({ x: 500, y: 0, w: 500, h: 700 })
})

test('id lạ thì trạng thái giữ nguyên', () => {
  const s = run(open('a'))
  expect(reduce(s, { type: 'close', id: 'zz' })).toBe(s)
  expect(reduce(s, { type: 'move', id: 'zz', dx: 1, dy: 1 })).toBe(s)
})
