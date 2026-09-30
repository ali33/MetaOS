import { act, fireEvent, render, screen } from '@testing-library/react'
import { BannerList } from '../lib/errors'
import { Banners } from './Banners'

test('băng đỏ và hổ phách, gộp trùng, đóng được', () => {
  const l = new BannerList()
  render(<Banners list={l} />)
  act(() => { l.add('error', 'access-denied: open /etc/shadow: permission denied'); l.add('warn', 'x'); l.add('warn', 'x') })
  expect(screen.getByRole('alert').className).toContain('banner-error')
  expect(screen.getByRole('status').textContent).toContain('x (×2)')
  fireEvent.click(screen.getAllByLabelText('Đóng thông báo')[0])
  expect(screen.queryByRole('alert')).toBeNull()
})
