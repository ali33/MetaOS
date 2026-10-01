#!/usr/bin/env bash
# Dựng container tích hợp, chạy test Go (và Playwright nếu --e2e), rồi dọn.
#   --keep  giữ container sau khi chạy (dùng cho scripts/dev.sh)
#   --e2e   build giao diện trước, chạy thêm Playwright
#   --up    chỉ dựng container rồi thoát (ngụ ý --keep)
# Cách chạy systemd đã kiểm: Docker Desktop 2026-09-30 trên WSL2 (kernel
# 5.15.167.4, cgroup v1, `docker info` ⇒ CgroupVersion 1) — cách mặc định dưới đây
# (--privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw) boot được
# systemd 252 của Debian 12, không cần phương án dự phòng --cgroupns=private.
set -euo pipefail
# Git Bash tự đổi mọi đối số bắt đầu bằng "/" thành đường dẫn Windows
# (/sys/fs/cgroup, /run, /usr/local/bin/…). Tắt hẳn cho cả script.
export MSYS_NO_PATHCONV=1
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ROOT_HOST="$(cd "$ROOT" && (pwd -W 2>/dev/null || pwd))"
KEEP=0; E2E=0; UP=0
for a in "$@"; do case "$a" in --keep) KEEP=1;; --e2e) E2E=1;; --up) UP=1; KEEP=1;; esac; done
NAME=metaos-it
if [ "$E2E" = 1 ]; then (cd "$ROOT/web" && { [ -d node_modules ] || npm ci; } && npm run build); fi
docker build -t metaos-it:dev -f "$ROOT_HOST/test/integration/Dockerfile" "$ROOT_HOST"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --privileged --cgroupns=host \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
  -p 127.0.0.1:9443:9443 metaos-it:dev >/dev/null
cleanup() { [ "$KEEP" = 1 ] || docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
for i in $(seq 1 60); do
  code=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:9443/api/session || true)
  [ "$code" = 401 ] && break
  sleep 1
done
[ "$code" = 401 ] || { docker exec "$NAME" journalctl -u metaos --no-pager | tail -50; echo "metaos không lên (http $code)"; exit 1; }
echo "metaos-it sẵn sàng tại https://127.0.0.1:9443"
[ "$UP" = 1 ] && exit 0
# Test tích hợp: build thành file chạy trong container Go, chép vào metaos-it và
# chạy ở đó (dưới root) — vừa không cần Go trên Windows, vừa gọi thẳng pgrep/systemctl.
"$ROOT/scripts/go.sh" sh -c 'CGO_ENABLED=0 go test -c -tags integration -o bin/it.test ./test/integration/'
docker cp "$ROOT_HOST/bin/it.test" "$NAME":/usr/local/bin/it.test
docker exec "$NAME" /usr/local/bin/it.test -test.v -test.count=1
if [ "$E2E" = 1 ]; then (cd "$ROOT/test/e2e" && { [ -d node_modules ] || npm ci; } && npx playwright install chromium && npx playwright test); fi
