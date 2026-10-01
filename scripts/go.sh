#!/usr/bin/env bash
# Chạy lệnh Go trong container Linux. Dùng được từ Git Bash (Windows) và WSL.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ROOT_HOST="$(cd "$ROOT" && (pwd -W 2>/dev/null || pwd))"   # Git Bash: D:/META/MetaOS
IMG=metaos-gobuild:1
if ! docker image inspect "$IMG" >/dev/null 2>&1; then
  docker build -t "$IMG" -f "$ROOT_HOST/build/Dockerfile.go" "$ROOT_HOST/build"
fi
# go:embed cần web/dist tồn tại; khi chưa build giao diện thì đặt một trang tạm
# (scripts/build.sh ở Task 18 ghi đè bằng bản thật; web/dist nằm trong .gitignore).
mkdir -p "$ROOT/web/dist"
[ -e "$ROOT/web/dist/index.html" ] || printf '<!doctype html><title>MetaOS</title><p>Giao diện chưa build: chạy scripts/build.sh
' > "$ROOT/web/dist/index.html"
TTY=""; [ -t 1 ] && TTY="-t"
# --init: PID 1 là tini, thu dọn tiến trình mồ côi — không có nó thì tiến trình
# con của test pty thành zombie và kill(pid, 0) vẫn báo "còn sống".
MSYS_NO_PATHCONV=1 docker run --rm --init $TTY \
  -v "$ROOT_HOST":/src -w /src \
  -v metaos-gomod:/go/pkg/mod -v metaos-gocache:/root/.cache/go-build \
  -e CGO_ENABLED=1 \
  "$IMG" "$@"
