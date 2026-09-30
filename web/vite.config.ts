import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Dev: metaos-ws chạy trong container ở https://127.0.0.1:9443 (Task 12).
// changeOrigin để mặc định (false) để Host trùng Origin — ws kiểm Origin == Host.
export default defineConfig({
  plugins: [react()],
  // assetsInlineLimit 0: không nhúng tài nguyên (font Inter) thành data: URI, vì CSP chỉ cho font-src 'self'.
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: true, assetsInlineLimit: 0 },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: 'https://127.0.0.1:9443', secure: false },
      '/ws': { target: 'wss://127.0.0.1:9443', ws: true, secure: false },
    },
  },
  test: { environment: 'jsdom', globals: true },
})
