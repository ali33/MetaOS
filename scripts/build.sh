#!/usr/bin/env bash
# Build giao diện (trên host, cần Node 22) rồi ba binary Go (trong container, cần cgo + libpam).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(git -C "$ROOT" describe --always --dirty 2>/dev/null || echo dev)"
(cd "$ROOT/web" && npm ci && npm run build)
"$ROOT/scripts/go.sh" go build -trimpath -ldflags "-X main.version=$VERSION" -o bin/ ./cmd/...
ls -l "$ROOT/bin"
