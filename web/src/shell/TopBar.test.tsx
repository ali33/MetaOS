import { render, screen } from '@testing-library/react'
import type { ConnState } from '../lib/channel'
import { TopBar } from './TopBar'

function bar(connState: ConnState) {
  return render(
    <TopBar hostname="h" user="u" connState={connState} loggingOut={false} theme="system" onTheme={() => {}}
      dockAlways={false} onDockAlways={() => {}} onActivities={() => {}} onLogout={() => {}} />,
  )
}

test.each([
  ['reconnecting', '⚠', true],
  ['replaced', '⚠', false],
  ['expired', '✕', false],
] as const)('trạng thái %s có biểu tượng %s (vòng quay: %s)', (state, glyph, spin) => {
  const { container } = bar(state)
  expect(container.querySelector('.conn-glyph')?.textContent).toBe(glyph)
  expect(container.querySelector('.spin') !== null).toBe(spin)
  expect(screen.getByRole('status').getAttribute('title')).toBeTruthy()
})

test.each(['open', 'connecting'] as const)('trạng thái %s không có biểu tượng lỗi', (state) => {
  const { container } = bar(state)
  expect(container.querySelector('.conn-glyph')).toBeNull()
})
