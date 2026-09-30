// Giao thức kênh MetaOS — xem docs/superpowers/specs mục 3.2.
export type Msg = { ch: string; type: string; data?: any }

const ID_RE = /^[A-Za-z0-9._-]{1,64}$/

export function encodeBinary(ch: string, payload: Uint8Array): Uint8Array {
  if (!ID_RE.test(ch)) throw new Error(`bad channel id: ${ch}`)
  const out = new Uint8Array(1 + ch.length + payload.length)
  out[0] = ch.length
  for (let i = 0; i < ch.length; i++) out[1 + i] = ch.charCodeAt(i)
  out.set(payload, 1 + ch.length)
  return out
}

export function decodeBinary(buf: ArrayBufferLike | Uint8Array): { ch: string; payload: Uint8Array } {
  const b = buf instanceof Uint8Array ? buf : new Uint8Array(buf)
  if (b.length < 1 || b[0] === 0 || b.length < 1 + b[0]) throw new Error('short binary frame')
  const ch = String.fromCharCode(...b.subarray(1, 1 + b[0]))
  if (!ID_RE.test(ch)) throw new Error(`bad channel id: ${ch}`)
  return { ch, payload: b.subarray(1 + b[0]) }
}
