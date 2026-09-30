import { fireEvent, render, screen } from '@testing-library/react'
import { Window } from './Window'
import type { Win } from './store'

// jsdom không có PointerEvent: dùng MouseEvent để clientX/button đến được React.
if (typeof PointerEvent === 'undefined') {
  class PE extends MouseEvent { pointerId: number; constructor(t: string, i: PointerEventInit = {}) { super(t, i); this.pointerId = i.pointerId ?? 1 } }
  vi.stubGlobal('PointerEvent', PE)
}

const win: Win = { id: 'w1', appId: 'a', title: 'T', rect: { x: 0, y: 0, w: 400, h: 300 }, state: 'normal', minimized: false, z: 1, restore: null }

function setup() {
  const dispatch = vi.fn()
  render(<Window win={win} focused dispatch={dispatch} toArea={(x, y) => ({ x, y })} banners={null}>x</Window>)
  return { dispatch, bar: screen.getByRole('region', { name: 'T' }).querySelector('.titlebar')! }
}

test('kéo bình thường: move rồi dragEnd', () => {
  const { dispatch, bar } = setup()
  fireEvent.pointerDown(bar, { button: 0, clientX: 10, clientY: 10 })
  fireEvent.pointerMove(bar, { clientX: 15, clientY: 12 })
  fireEvent.pointerUp(bar, { clientX: 15, clientY: 12 })
  expect(dispatch.mock.calls.map((c) => c[0].type)).toEqual(['move', 'dragEnd'])
})

test.each(['pointerCancel', 'lostPointerCapture'] as const)('%s bỏ trạng thái kéo, di chuột sau đó không làm cửa sổ chạy', (ev) => {
  const { dispatch, bar } = setup()
  fireEvent.pointerDown(bar, { button: 0, clientX: 10, clientY: 10 })
  fireEvent[ev](bar)
  fireEvent.pointerMove(bar, { clientX: 50, clientY: 50 })
  fireEvent.pointerUp(bar, { clientX: 50, clientY: 50 })
  expect(dispatch).not.toHaveBeenCalled()
})
