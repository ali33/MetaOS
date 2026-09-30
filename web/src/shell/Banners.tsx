import { useSyncExternalStore } from 'react'
import type { BannerList } from '../lib/errors'

export function Banners({ list, className = '' }: { list: BannerList; className?: string }) {
  const items = useSyncExternalStore((f) => list.subscribe(f), () => list.items)
  if (items.length === 0) return null
  return (
    <div className={`banners ${className}`}>
      {items.map((b) => (
        <div key={b.id} className={`banner banner-${b.level}`} role={b.level === 'error' ? 'alert' : 'status'}>
          <span aria-hidden>{b.level === 'error' ? '⛔' : '⚠'}</span>
          <span className="text">{b.text}{b.count > 1 ? ` (×${b.count})` : ''}</span>
          <button aria-label="Đóng thông báo" onClick={() => list.dismiss(b.id)}>✕</button>
        </div>
      ))}
    </div>
  )
}
