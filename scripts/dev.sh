#!/usr/bin/env bash
# Môi trường dev: backend chạy trong container metaos-it (HTTPS 127.0.0.1:9443),
# giao diện chạy vite dev ở http://localhost:5173 và proxy /api, /ws sang backend.
# Đăng nhập bằng alice / alice-pass-1 hoặc bob / bob-pass-1 (tài khoản giả trong container).
# Sửa mã Go ⇒ chạy lại scripts/dev.sh để dựng lại container.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
"$ROOT/scripts/it.sh" --up
cd "$ROOT/web"
[ -d node_modules ] || npm ci
exec npx vite
