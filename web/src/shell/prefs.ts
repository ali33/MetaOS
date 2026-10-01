export type ThemePref = 'system' | 'light' | 'dark'

// localStorage có thể ném lỗi (chế độ riêng tư, bị chặn): khi đó dùng mặc định.
export function loadPref<T extends string>(key: string, allowed: readonly T[], def: T): T {
  try {
    const v = localStorage.getItem(key)
    return v !== null && (allowed as readonly string[]).includes(v) ? (v as T) : def
  } catch {
    return def
  }
}

export function savePref(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    /* không lưu được thì chỉ mất tuỳ chọn, không mất chức năng */
  }
}

export function applyTheme(t: ThemePref) {
  if (t === 'system') delete document.documentElement.dataset.theme
  else document.documentElement.dataset.theme = t
}
