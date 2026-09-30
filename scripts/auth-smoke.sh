#!/usr/bin/env bash
# Thử metaos-auth với PAM thật: đúng/sai mật khẩu, người gọi sai, --check-only, hạ quyền,
# hết hạn / bị khoá, và các thuộc tính bảo mật của tiến trình bridge.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
"$ROOT/scripts/go.sh" go build -o bin/metaos-auth ./cmd/metaos-auth
"$ROOT/scripts/go.sh" bash -euo pipefail -c '
  useradd --system --no-create-home --shell /usr/sbin/nologin metaos
  groupadd devs
  useradd --create-home --shell /bin/bash -G devs alice && echo "alice:Mật khẩu 1" | chpasswd
  useradd --create-home carol && echo "carol:pw-carol" | chpasswd && chage -d 0 carol
  useradd --create-home dave && echo "dave:pw-dave" | chpasswd && passwd -l dave >/dev/null
  install -D -m 0644 packaging/pam.d/metaos /etc/pam.d/metaos
  install -D -o root -g metaos -m 4750 bin/metaos-auth /usr/lib/metaos/metaos-auth
  # Bridge giả: in danh tính, cwd, umask, fd thừa hưởng (ls tự mở thêm fd 3), phần còn lại của stdin, env.
  printf "#!/bin/sh\nid -u; id -g; echo \"G:\$(id -G)\"; pwd; echo \"U:\$(umask)\"; echo \"FD:\$(ls /proc/self/fd | xargs)\"; echo \"IN:\$(head -c 5)\"; env | sort\n" > /usr/lib/metaos/metaos-bridge
  chmod 0755 /usr/lib/metaos/metaos-bridge
  export METAOS_LEAK_CANARY=1
  AS_METAOS="setpriv --reuid=metaos --regid=metaos --init-groups"
  A=/usr/lib/metaos/metaos-auth
  # Người gọi có umask 077 và fd 9 không CLOEXEC: bridge phải nhận umask 022 và chỉ fd 0/1/2.
  out=$(umask 077; printf "Mật khẩu 1\nFRAME" | $AS_METAOS $A alice 9</etc/hostname)
  echo "$out" | grep -qx "$(id -u alice)" || { echo "FAIL uid: $out"; exit 1; }
  echo "$out" | grep -qx "/home/alice"    || { echo "FAIL cwd"; exit 1; }
  echo "$out" | grep -q "^METAOS_LEAK_CANARY" && { echo "FAIL env lọt qua"; exit 1; }
  echo "$out" | grep -qx "G:$(id -G alice)" || { echo "FAIL nhóm phụ: $out"; exit 1; }
  echo "$out" | grep -qx "U:0022"         || { echo "FAIL umask: $out"; exit 1; }
  echo "$out" | grep -qx "FD:0 1 2 3"     || { echo "FAIL fd thừa hưởng: $out"; exit 1; }
  echo "$out" | grep -qx "IN:FRAME"       || { echo "FAIL stdin sau mật khẩu: $out"; exit 1; }
  rc=0; printf "sai\n" | $AS_METAOS $A alice >/dev/null 2>&1 || rc=$?
  [ "$rc" = 1 ] || { echo "FAIL sai mật khẩu rc=$rc"; exit 1; }
  rc=0; printf "Mật khẩu 1\n" | $A alice >/dev/null 2>&1 || rc=$?
  [ "$rc" = 2 ] || { echo "FAIL root gọi phải rc=2, nhận $rc"; exit 1; }
  rc=0; printf "Mật khẩu 1\n" | $AS_METAOS $A --check-only alice || rc=$?
  [ "$rc" = 0 ] || { echo "FAIL check-only rc=$rc"; exit 1; }
  n=$(printf "Mật khẩu 1\n" | $AS_METAOS $A --check-only alice | wc -c)
  [ "$n" = 0 ] || { echo "FAIL check-only ghi $n byte ra stdout"; exit 1; }
  rc=0; printf "pw-carol\n" | $AS_METAOS $A --check-only carol >/dev/null 2>&1 || rc=$?
  [ "$rc" = 4 ] || { echo "FAIL hết hạn phải rc=4, nhận $rc"; exit 1; }
  rc=0; printf "pw-dave\n" | $AS_METAOS $A --check-only dave >/dev/null 2>&1 || rc=$?
  [ "$rc" = 1 ] || { echo "FAIL bị khoá phải rc=1, nhận $rc"; exit 1; }
  # stdin hỏng (đọc thư mục ⇒ EISDIR) là lỗi hạ tầng (3), không phải sai mật khẩu; stdin rỗng vẫn là 1.
  rc=0; $AS_METAOS $A --check-only alice </ >/dev/null 2>&1 || rc=$?
  [ "$rc" = 3 ] || { echo "FAIL stdin lỗi đọc phải rc=3, nhận $rc"; exit 1; }
  rc=0; $AS_METAOS $A --check-only alice </dev/null >/dev/null 2>&1 || rc=$?
  [ "$rc" = 1 ] || { echo "FAIL stdin rỗng phải rc=1, nhận $rc"; exit 1; }
  echo "SMOKE OK"
'
