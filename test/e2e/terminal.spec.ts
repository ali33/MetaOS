import { expect, test, type Page } from '@playwright/test'

async function loginAs(page: Page, user: string, pw: string) {
  await page.goto('/')
  await page.getByLabel('Tên đăng nhập').fill(user)
  await page.getByLabel('Mật khẩu').fill(pw)
  await page.getByRole('button', { name: 'Đăng nhập' }).click()
}

async function logout(page: Page) {
  await page.getByRole('button', { name: 'alice ▾' }).click()
  await page.getByRole('menuitem', { name: 'Đăng xuất' }).click()
  await expect(page.getByText('Đã đăng xuất.')).toBeVisible()
}

test('sai mật khẩu hiện thông báo', async ({ page }) => {
  await loginAs(page, 'alice', 'sai')
  await expect(page.getByRole('alert')).toContainText('Sai tên đăng nhập hoặc mật khẩu')
})

test('đăng nhập, mở Terminal, whoami ra alice, đăng xuất', async ({ page }) => {
  const errors: string[] = []
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()) })
  await loginAs(page, 'alice', 'alice-pass-1')
  await expect(page.getByRole('button', { name: 'Hoạt động' })).toBeVisible()
  await page.getByRole('button', { name: 'Hoạt động' }).click()
  await page.getByRole('button', { name: 'Terminal' }).first().click()
  const win = page.getByRole('region', { name: /Terminal|alice@/ })
  await expect(win).toBeVisible()
  await expect(win.locator('.phu-lop')).toHaveCount(0, { timeout: 15_000 })
  await win.locator('.xterm').click()
  await page.keyboard.type('whoami\n')
  // Mỗi dòng terminal là một <div> trong .xterm-rows; tìm dòng chỉ có "alice".
  await expect(win.locator('.xterm-rows > div').filter({ hasText: /^alice\s*$/ })).toHaveCount(1, { timeout: 15_000 })
  await logout(page)
  expect(errors.filter((e) => /Content Security Policy/i.test(e))).toEqual([])
})

test('tải lại trang: tiếp quản terminal đang chạy (Q2)', async ({ page }) => {
  await loginAs(page, 'alice', 'alice-pass-1')
  await expect(page.getByRole('button', { name: 'Hoạt động' })).toBeVisible()
  await page.keyboard.press('Control+Alt+T')
  const win = page.getByRole('region', { name: /Terminal|alice@/ })
  await expect(win.locator('.phu-lop')).toHaveCount(0, { timeout: 15_000 })
  await win.locator('.xterm').click()
  await page.keyboard.type('echo KEEP-$((20+22))\n')
  await expect(win.locator('.xterm-rows > div').filter({ hasText: /^KEEP-42\s*$/ })).toHaveCount(1, { timeout: 15_000 })
  await page.reload()
  const again = page.getByRole('region', { name: /Terminal|alice@/ })
  await expect(again.locator('.xterm-rows > div').filter({ hasText: /^KEEP-42\s*$/ })).toHaveCount(1, { timeout: 15_000 })
  await logout(page)
})

test('tab thứ hai giành phiên, tab cũ báo "Đã mở ở nơi khác"', async ({ page, context }) => {
  await loginAs(page, 'alice', 'alice-pass-1')
  await expect(page.getByRole('button', { name: 'Hoạt động' })).toBeVisible()
  const second = await context.newPage()
  await second.goto('/')
  await expect(second.getByRole('button', { name: 'Hoạt động' })).toBeVisible()
  await expect(page.getByText('Đã mở ở nơi khác')).toBeVisible()
  await logout(second)
})
