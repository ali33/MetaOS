import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Dev: metaos-ws chạy trong container ở https://127.0.0.1:9443 (Task 12).
// ws kiểm Origin: scheme phải là https (listener TLS) và host phải trùng Host.
// changeOrigin để mặc định (false) nên Host giữ là host dev (vd. localhost:5173);
// trình duyệt gửi Origin http://<Host> nên proxy viết lại thành https://<Host> —
// chỉ ở dev, không nới lỏng kiểm Origin phía server.
type Req = { headers: { origin?: string; host?: string } }
type ProxyReq = { setHeader(k: string, v: string): void }
type ProxyLike = { on(ev: string, fn: (proxyReq: ProxyReq, req: Req) => void): void }
// ProxyServer của vite kế thừa EventEmitter của @types/node (web không cài) nên ép về dạng tối thiểu.
const httpsOrigin = (server: unknown) => {
  const proxy = server as ProxyLike
  const fix = (proxyReq: ProxyReq, req: Req) => {
    // Chỉ đổi Origin đúng của trang dev; Origin lạ giữ nguyên để server vẫn từ chối.
    const host = req.headers.host
    if (host && req.headers.origin === `http://${host}`) proxyReq.setHeader('origin', `https://${host}`)
  }
  proxy.on('proxyReq', fix)
  proxy.on('proxyReqWs', fix)
}

export default defineConfig({
  plugins: [react()],
  // assetsInlineLimit 0: không nhúng tài nguyên (font Inter) thành data: URI, vì CSP chỉ cho font-src 'self'.
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: true, assetsInlineLimit: 0 },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: 'https://127.0.0.1:9443', secure: false, configure: httpsOrigin },
      '/ws': { target: 'wss://127.0.0.1:9443', ws: true, secure: false, configure: httpsOrigin },
    },
  },
  test: { environment: 'jsdom', globals: true },
})
