import { decodeBinary, encodeBinary } from './protocol'

test('encode/decode khớp định dạng [len][id][payload]', () => {
  const b = encodeBinary('t1', new TextEncoder().encode('hi'))
  expect(Array.from(b)).toEqual([2, 116, 49, 104, 105])
  const { ch, payload } = decodeBinary(b.buffer.slice(0))
  expect(ch).toBe('t1')
  expect(new TextDecoder().decode(payload)).toBe('hi')
})

test('decode từ chối frame cụt', () => {
  expect(() => decodeBinary(new Uint8Array([]))).toThrow()
  expect(() => decodeBinary(new Uint8Array([5, 97]))).toThrow()
  expect(() => decodeBinary(new Uint8Array([0, 97]))).toThrow()
})

test('encode từ chối id sai', () => {
  expect(() => encodeBinary('a b', new Uint8Array())).toThrow()
  expect(() => encodeBinary('x'.repeat(65), new Uint8Array())).toThrow()
})
