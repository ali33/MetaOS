import { fireEvent, render, screen } from '@testing-library/react'
import type { AppDef } from '../apps/registry'
import { Activities } from './Activities'

const app: AppDef = { id: 'terminal', name: 'Terminal', icon: '>_', keywords: ['shell'], component: () => <div /> }

function setup() {
  const onLaunch = vi.fn(), onClose = vi.fn()
  const r = render(<Activities wins={[]} apps={[app]} onPickWindow={vi.fn()} onLaunch={onLaunch} onClose={onClose} />)
  return { onLaunch, onClose, r }
}

test('Enter khi ô tìm trống không mở gì; có từ khoá thì mở kết quả đầu', () => {
  const { onLaunch } = setup()
  const input = screen.getByLabelText('Tìm ứng dụng')
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(onLaunch).not.toHaveBeenCalled()
  fireEvent.change(input, { target: { value: 'she' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(onLaunch).toHaveBeenCalledWith('terminal')
})

test('bấm vào chỗ trống (lớp phủ hoặc nền .thumbs) thì đóng; bấm vào nút thì không', () => {
  const { onClose, r } = setup()
  fireEvent.click(screen.getByRole('dialog'))
  expect(onClose).toHaveBeenCalledTimes(1)
  fireEvent.click(r.container.querySelector('.thumbs')!)
  expect(onClose).toHaveBeenCalledTimes(2)
  fireEvent.click(screen.getByLabelText('Terminal'))
  expect(onClose).toHaveBeenCalledTimes(2)
})
