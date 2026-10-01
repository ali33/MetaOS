import { BannerList, installGlobalHandlers, MAX_BANNERS } from './errors'

test('gộp trùng và đếm', () => {
  const l = new BannerList()
  l.add('error', 'mất kết nối')
  l.add('error', 'mất kết nối')
  expect(l.items).toHaveLength(1)
  expect(l.items[0].count).toBe(2)
})

test('tối đa 5 dòng, bỏ dòng cũ nhất', () => {
  const l = new BannerList()
  for (let i = 0; i < 8; i++) l.add('warn', `lỗi ${i}`)
  expect(l.items).toHaveLength(MAX_BANNERS)
  expect(l.items[0].text).toBe('lỗi 3')
})

test('clear theo mức và dismiss', () => {
  const l = new BannerList()
  l.add('error', 'a'); l.add('warn', 'b')
  l.clear('error')
  expect(l.items.map((b) => b.text)).toEqual(['b'])
  l.dismiss(l.items[0].id)
  expect(l.items).toHaveLength(0)
})

test('bắt window.error và unhandledrejection, vẫn ghi console', () => {
  const l = new BannerList()
  const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
  const off = installGlobalHandlers(l, window)
  window.dispatchEvent(new ErrorEvent('error', { message: 'x is undefined', filename: 'app.js', lineno: 12 }))
  const ev = new Event('unhandledrejection') as any
  ev.reason = new Error('fetch hỏng')
  window.dispatchEvent(ev)
  expect(l.items.map((b) => b.text)).toEqual(['x is undefined (app.js:12)', 'Promise bị bỏ rơi: fetch hỏng'])
  expect(l.items.every((b) => b.level === 'warn')).toBe(true)
  expect(spy).toHaveBeenCalled()
  off()
})
