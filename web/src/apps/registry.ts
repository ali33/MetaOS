import { TerminalApp } from './terminal/TerminalApp'
import type { JSX } from 'react'
import type { Connection, ConnState } from '../lib/channel'
import type { Level } from '../lib/errors'

export type AppProps = {
  winId: string
  conn: Connection
  connState: ConnState
  active: boolean
  report(level: Level, text: string): void
  clearReports(level?: Level): void
  setTitle(title: string): void
  requestClose(): void
  adopt?: string                                    // D17: id kênh có sẵn để tiếp quản thay vì mở mới
}

export type AppDef = { id: string; name: string; icon: string; keywords: string[]; component: (p: AppProps) => JSX.Element }

// M2–M4 thêm Quản lý file, Giám sát, Dịch vụ & log.
export const APPS: AppDef[] = [
  { id: 'terminal', name: 'Terminal', icon: '>_', keywords: ['terminal', 'shell', 'bash', 'console'], component: TerminalApp },
]
