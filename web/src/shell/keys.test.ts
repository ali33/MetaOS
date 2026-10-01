import { ShortcutMatcher, type KeyLike } from './keys'

const k = (key: string, mods: Partial<KeyLike> = {}, type = 'keydown'): KeyLike => ({
  key, code: '', altKey: false, ctrlKey: false, metaKey: false, shiftKey: false, type, ...mods,
})

test.each([
  [k('ArrowLeft', { metaKey: true }), 'tile-left'],
  [k('ArrowRight', { ctrlKey: true, altKey: true }), 'tile-right'],
  [k('ArrowUp', { metaKey: true }), 'maximize'],
  [k('ArrowDown', { ctrlKey: true, altKey: true }), 'restore'],
  [k('Tab', { altKey: true }), 'cycle-next'],
  [k('Tab', { altKey: true, shiftKey: true }), 'cycle-prev'],
  [k('`', { altKey: true }), 'cycle-next'],
  [k('~', { altKey: true, shiftKey: true, code: 'Backquote' }), 'cycle-prev'],
  [k('t', { ctrlKey: true, altKey: true }), 'terminal'],
  [k('T', { ctrlKey: true, altKey: true }), 'terminal'],
  [k('F1', { altKey: true }), 'activities'],
])('%o ⇒ %s', (e, want) => {
  expect(new ShortcutMatcher().handle(e)).toBe(want)
})

test.each([
  k('a'), k('ArrowLeft'), k('t', { ctrlKey: true }), k('c', { ctrlKey: true }), k('Tab'),
])('phím thường không bị nuốt: %o', (e) => {
  expect(new ShortcutMatcher().handle(e)).toBeNull()
})

test('chạm Super (nhấn rồi thả, không kèm phím khác) mở Hoạt động', () => {
  const m = new ShortcutMatcher()
  expect(m.handle(k('Meta', { metaKey: true }))).toBeNull()
  expect(m.handle(k('Meta', {}, 'keyup'))).toBe('activities')
})

test('Super+← không kích hoạt Hoạt động khi thả Super', () => {
  const m = new ShortcutMatcher()
  m.handle(k('Meta', { metaKey: true }))
  expect(m.handle(k('ArrowLeft', { metaKey: true }))).toBe('tile-left')
  expect(m.handle(k('Meta', {}, 'keyup'))).toBeNull()
})
