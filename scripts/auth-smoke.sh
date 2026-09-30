#!/usr/bin/env bash
# Thử metaos-auth với PAM thật: đúng/sai mật khẩu, người gọi sai, --check-only, hạ quyền.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
"$ROOT/scripts/go.sh" go build -o bin/metaos-auth ./cmd/metaos-auth
"$ROOT/scripts/go.sh" bash -euo pipefail -c '
  useradd --system --no-create-home --shell /usr/sbin/nologin metaos
  useradd --create-home --shell /bin/bash alice && echo "alice:Mật khẩu 1" | chpasswd
  install -D -m 0644 packaging/pam.d/metaos /etc/pam.d/metaos
  install -D -o root -g metaos -m 4750 bin/metaos-auth /usr/lib/metaos/metaos-auth
  printf "#!/bin/sh\nid -u; id -g; pwd; env | sort\n" > /usr/lib/metaos/metaos-bridge
  chmod 0755 /usr/lib/metaos/metaos-bridge
  export METAOS_LEAK_CANARY=1
  AS_METAOS="setpriv --reuid=metaos --regid=metaos --init-groups"
  A=/usr/lib/metaos/metaos-auth
  out=$(printf "Mật khẩu 1\n" | $AS_METAOS $A alice)
  echo "$out" | grep -qx "$(id -u alice)" || { echo "FAIL uid: $out"; exit 1; }
  echo "$out" | grep -qx "/home/alice"    || { echo "FAIL cwd"; exit 1; }
  echo "$out" | grep -q "^METAOS_LEAK_CANARY" && { echo "FAIL env lọt qua"; exit 1; }
  rc=0; printf "sai\n" | $AS_METAOS $A alice >/dev/null 2>&1 || rc=$?
  [ "$rc" = 1 ] || { echo "FAIL sai mật khẩu rc=$rc"; exit 1; }
  rc=0; printf "Mật khẩu 1\n" | $A alice >/dev/null 2>&1 || rc=$?
  [ "$rc" = 2 ] || { echo "FAIL root gọi phải rc=2, nhận $rc"; exit 1; }
  rc=0; printf "Mật khẩu 1\n" | $AS_METAOS $A --check-only alice || rc=$?
  [ "$rc" = 0 ] || { echo "FAIL check-only rc=$rc"; exit 1; }
  echo "SMOKE OK"
'
