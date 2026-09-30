# MetaOS — Desktop web kiểu GNOME để quản trị máy chủ Linux

- Ngày: 2026-09-30
- Trạng thái: Đã duyệt (2026-09-30); cập nhật quyết định phiên và phạm vi M1 cùng ngày
- Repo: https://github.com/ali33/MetaOS

## 1. Mục tiêu

MetaOS là giao diện quản trị máy chủ Linux chạy trong trình duyệt, mang dáng một
desktop GNOME (thanh trên cùng, chế độ Hoạt động, dock, cửa sổ kéo thả). Người
quản trị cài một gói lên máy chủ, mở `https://<máy>:9443`, đăng nhập bằng tài
khoản Linux + mã TOTP, rồi dùng terminal, quản lý file, giám sát hệ thống và
quản lý dịch vụ/log — mọi thao tác chạy **đúng quyền của user Linux đó**.

### Tiêu chí hoàn thành MVP

1. Cài bằng một gói `.deb` trên Ubuntu 22.04+/Debian 12+ có systemd; dịch vụ
   `metaos.service` tự chạy, lắng nghe cổng 9443 với HTTPS (tự sinh chứng chỉ
   tự ký nếu chưa có).
2. Đăng nhập bằng user/mật khẩu Linux (PAM) + TOTP; lần đầu bắt buộc đăng ký TOTP.
3. Sau đăng nhập, mọi thao tác chạy trong tiến trình mang uid/gid của user đó;
   thao tác vượt quyền bị hệ điều hành từ chối và lỗi hiện lên màn hình.
4. Bốn ứng dụng Terminal, Quản lý file, Giám sát hệ thống, Dịch vụ & log hoạt
   động đầy đủ như mô tả ở mục 5.
5. Kiểm thử Go, Vitest, tích hợp trong container và Playwright đều qua.

### Ngoài phạm vi MVP

Quản lý Docker, điều khiển nhiều máy, đa ngôn ngữ, chạy ứng dụng đồ hoạ Linux
(VNC/Wayland), phân phối ngoài `.deb` (rpm, Arch…). Kiến trúc phải **không chặn**
việc thêm nhiều máy về sau (xem mục 3.4).

## 2. Quyết định đã chốt

| Chủ đề | Lựa chọn | Lý do |
|---|---|---|
| Mục đích | Quản trị máy chủ | Người dùng chọn |
| Quy mô | Một máy trước, nhiều máy sau | Giao thức kênh là "hợp đồng" để bridge máy khác nối vào |
| Backend | Go | Một file chạy, nhẹ, có sẵn PTY / D-Bus / WebSocket |
| Frontend | React + TypeScript + Vite | Hệ sinh thái lớn; xterm.js, Monaco có sẵn |
| Đăng nhập | PAM + TOTP | Linux giữ quyền; 2FA khi mở ra Internet |
| Kiến trúc | Tách tiến trình theo phiên (kiểu Cockpit) | Ranh giới quyền do hệ điều hành đảm bảo, không nằm trong code của MetaOS |

## 3. Kiến trúc

```
Trình duyệt (React)
   │  HTTPS + 1 WebSocket (/ws)
   ▼
metaos-ws        user hệ thống "metaos", không có quyền root
   │  - phục vụ giao diện nhúng (go:embed)
   │  - đăng nhập: gọi metaos-auth, kiểm TOTP, cấp cookie phiên
   │  - chuyển tiếp frame giữa trình duyệt và bridge
   │  stdin/stdout của tiến trình con (frame có độ dài ở đầu)
   ▼
metaos-bridge    chạy bằng uid/gid của user đã đăng nhập, mỗi phiên một tiến trình
   ├─ pty       bash qua PTY
   ├─ fs        liệt kê / đọc / ghi / đổi tên / xoá / chmod / chown / nén
   ├─ metrics   /proc, đẩy mỗi giây
   ├─ proc      danh sách tiến trình, gửi tín hiệu, renice
   ├─ systemd   D-Bus org.freedesktop.systemd1
   └─ journal   journalctl --follow -o json
```

### 3.1 Ba file chạy

- **metaos-ws**: chạy dưới user `metaos` (không có shell, không có quyền root).
  Lo HTTPS, WebSocket, phiên, chặn dò mật khẩu, TOTP, phục vụ giao diện.
- **metaos-auth**: file nhỏ có setuid root, được `metaos-ws` gọi. Chỉ làm đúng
  hai việc: (1) kiểm mật khẩu qua PAM (`pam_authenticate` + `pam_acct_mgmt`,
  dịch vụ PAM `metaos`); (2) nếu hợp lệ thì `initgroups` + `setgid` + `setuid`
  sang user rồi `exec metaos-bridge`. Có thêm chế độ `--check-only` chỉ làm
  bước (1), dùng cho khoá màn hình. Mật khẩu nhận qua stdin, không qua đối số
  dòng lệnh. Chỉ user `metaos` được gọi (kiểm uid người gọi). Mục tiêu dưới
  300 dòng để soát được bằng mắt.
- **metaos-bridge**: không có quyền gì đặc biệt; nói giao thức kênh qua
  stdin/stdout với `metaos-ws`.

### 3.2 Giao thức kênh

Một WebSocket cho mỗi phiên trình duyệt, chia thành nhiều kênh.

- **Frame văn bản** (JSON): `{"ch": "<id>", "type": "<loại>", "data": {...}}`.
  - Điều khiển (`ch` rỗng): `open` `{ch, kind, params}`, `ready`, `close`
    `{ch, reason}`, `error` `{ch, code, message}`, `ping`/`pong`.
  - Dữ liệu: `type` do từng loại kênh định nghĩa (vd. `fs` có `list`, `stat`,
    `rename`…; mỗi yêu cầu mang `id` để ghép với phản hồi).
- **Frame nhị phân**: `[1 byte độ dài id kênh][id kênh][dữ liệu]` — dùng cho
  luồng PTY và khúc file tải lên/xuống.
- **ws ↔ bridge**: mỗi frame = `[4 byte độ dài big-endian][1 byte loại: 0=text,
  1=binary][dữ liệu]`.
- `metaos-ws` **không hiểu** nội dung kênh; nó chỉ chuyển tiếp. Nhờ vậy về sau
  bridge ở máy khác (qua SSH hoặc kết nối ngược) nói đúng giao thức này, và giao
  diện chỉ cần thêm trường `host` khi `open`.
- Mã lỗi chuẩn: `access-denied`, `not-found`, `invalid-params`, `unsupported`,
  `internal`, `sudo-required`, `sudo-failed`. `message` là thông điệp gốc của
  hệ thống (vd. `open /etc/shadow: permission denied`).
- Giới hạn: frame văn bản ≤ 1 MiB; khúc tải lên 256 KiB; mỗi phiên ≤ 64 kênh mở
  cùng lúc.

### 3.3 Phiên, đăng nhập, bảo mật

- Luồng đăng nhập: `POST /api/login {user, password}` → metaos-auth → PAM hợp lệ
  → nếu user chưa có TOTP thì trả `totp-enroll` kèm mã QR (bí mật mới, chưa lưu)
  → người dùng nhập mã → lưu bí mật; nếu đã có thì trả `totp-required` → nhập mã.
  Chỉ sau khi TOTP đúng mới khởi động bridge và cấp cookie.
  - Ghi chú thực hiện: bridge được spawn ngay khi PAM hợp lệ nhưng bị giữ ở trạng
    thái chờ; nếu TOTP sai quá 3 lần hoặc quá 2 phút thì ws giết bridge.
- Bí mật TOTP: `/var/lib/metaos/totp/<user>`, quyền 0600, chủ `metaos`. RFC 6238,
  SHA1, 6 số, bước 30 giây, chấp nhận lệch ±1 bước, chặn dùng lại cùng mã.
- Cookie phiên: ngẫu nhiên 256 bit, `HttpOnly; Secure; SameSite=Strict`. Phiên hết
  hạn sau 12 giờ, hoặc 30 phút không hoạt động. Đăng xuất / hết hạn ⇒ dừng hết:
  giết bridge và mọi lệnh đang chạy, giống thoát SSH. Lệnh cố ý tách riêng
  (`nohup`, `setsid`, tmux) được sống; không dùng cgroup/scope để quét sạch.
  Hệ quả: chúng vẫn thuộc cgroup của `metaos.service` nên chết khi restart/dừng
  dịch vụ hoặc nâng cấp gói — M5 quyết có tách scope riêng hay không.
- Một WebSocket cho mỗi phiên: tab/máy thứ hai mở cùng phiên thì **giành phiên** —
  tab cũ báo "Đã mở ở nơi khác", tab mới tiếp quản các terminal đang chạy.
- Chặn CSRF: `POST` phải kèm header `X-MetaOS-CSRF` trùng giá trị trong phiên;
  WebSocket kiểm `Origin` trùng host.
- Chặn dò mật khẩu: tối đa 5 lần sai / 15 phút cho mỗi IP và cho mỗi user; vượt thì
  khoá 15 phút. Mọi lần đăng nhập (thành công/thất bại) ghi vào journal với
  `SYSLOG_IDENTIFIER=metaos`.
- Không cho `root` đăng nhập trực tiếp (có thể bật trong `/etc/metaos/metaos.conf`).
- Việc cần root: bridge chạy `sudo -S -p '' -- <lệnh>` cho từng thao tác; giao diện
  hỏi mật khẩu sudo mỗi lần (MVP không giữ quyền sudo lâu dài). User không có
  trong sudoers ⇒ lỗi `sudo-failed` với thông điệp gốc của sudo.
- HTTPS: chứng chỉ ở `/etc/metaos/tls/`. Nếu thiếu thì tự sinh chứng chỉ tự ký
  (ECDSA P-256, 10 năm) lúc khởi động. Có tuỳ chọn chạy HTTP sau reverse proxy
  (chỉ khi lắng nghe `127.0.0.1`).
- Header: `Content-Security-Policy` chặt (không inline script), `X-Frame-Options:
  DENY`, `Referrer-Policy: no-referrer`.

### 3.4 Đường mở rộng sang nhiều máy (sau MVP, chỉ để không chặn)

- `open` nhận thêm `host`; ws giữ một bridge cho mỗi (phiên, host).
- Bridge ở máy xa: ws chạy `ssh user@host metaos-bridge` hoặc agent chủ động nối
  ngược về. Giao thức không đổi.

## 4. Giao diện desktop

- **Thanh trên cùng**: nút Hoạt động (trái), đồng hồ (giữa), tên máy + chỉ báo
  CPU/RAM thu nhỏ (từ M3) + menu user (khoá màn hình — từ M5, đăng xuất) (phải).
- **Chế độ Hoạt động** (phím Super / nút / góc trên-trái — góc nóng từ M2): lưới cửa sổ thu nhỏ,
  ô tìm ứng dụng, dock.
- **Dock**: hiện ở chế độ Hoạt động; tuỳ chọn luôn hiện.
- **Trình quản lý cửa sổ** (tự viết): kéo, đổi cỡ 8 hướng, thu nhỏ/phóng to/đóng,
  bấm đúp thanh tiêu đề để phóng to, kéo vào cạnh để chia đôi, thứ tự z. Phím
  tắt: Super+←/→ (chia đôi), Super+↑ (phóng to), Alt+Tab, Ctrl+Alt+T (terminal).
  Phím tắt bị trình duyệt giữ thì có phương án thay trong cài đặt.
- **Lưu trạng thái**: vị trí/kích thước/danh sách cửa sổ, giao diện sáng/tối lưu
  ở `~/.config/metaos/desktop.json` qua kênh `fs`; đăng nhập lại thì phục hồi.
- **Giao diện**: sáng/tối theo hệ điều hành, đổi tay được; phông Inter; màu định
  nghĩa bằng token CSS.
- **Khoá màn hình** (M5): phủ toàn màn, yêu cầu mật khẩu Linux (qua PAM) để mở; bridge
  và terminal vẫn chạy.

## 5. Bốn ứng dụng

### 5.1 Terminal
- xterm.js + addon fit/web-links/search; nhiều tab trong một cửa sổ.
- Kênh `pty` với tham số `{cols, rows, shell?, cwd?}`; đổi cỡ gửi `resize`.
- Rớt WebSocket: bridge giữ PTY 60 giây; client nối lại và gắn vào kênh cũ theo
  id (bộ đệm 256 KiB đầu ra gần nhất được phát lại).

### 5.2 Quản lý file
- Breadcrumb, xem danh sách/lưới, sắp xếp theo tên/cỡ/ngày, hiện file ẩn.
- Thao tác: mở, tạo thư mục, đổi tên, xoá (hỏi xác nhận), sao chép/di chuyển,
  chmod/chown (hộp thoại quyền), nén/giải nén tar.gz và zip.
- Tải lên: kéo thả, chia khúc 256 KiB, thanh tiến độ, huỷ được. Tải xuống: luồng
  qua kênh; thư mục thì nén tar.gz khi tải.
- Sửa file: Monaco cho file văn bản ≤ 5 MiB; cảnh báo khi file bị sửa từ nơi khác
  (so mtime trước khi lưu). Xem ảnh trực tiếp.
- Thao tác cần root (file của user khác): nút "Mở với sudo".

### 5.3 Giám sát hệ thống
- Biểu đồ 60 giây gần nhất: CPU tổng + từng nhân, RAM/swap, IO đĩa, mạng theo
  giao diện. Dung lượng từng phân vùng.
- Bảng tiến trình: PID, user, CPU%, RAM, lệnh; sắp xếp, lọc; gửi tín hiệu
  (TERM/KILL), renice. Tiến trình của user khác ⇒ qua sudo.

### 5.4 Dịch vụ & log
- Danh sách unit systemd (service, timer, socket), lọc theo trạng thái/tên; chi
  tiết unit; start/stop/restart/reload/enable/disable qua sudo.
- Log: journal theo thời gian thực, lọc theo unit, mức độ, khoảng thời gian; bộ
  lọc nằm trong URL (`replaceState`, chỉ ghi tham số khác mặc định, đọc vào qua
  danh sách trắng) để chia sẻ link.

## 6. Hành vi chung của giao diện

- **Chỉ báo chờ**: mỗi yêu cầu xuống bridge có cờ chờ riêng + vòng quay; tải lại
  một phần thì phủ mờ nội dung cũ, không xoá trắng; chặn thao tác tốn kém chồng
  nhau; tôn trọng `prefers-reduced-motion` bằng nhịp mờ tỏ thay vì bỏ tín hiệu.
- **Lỗi lên màn hình**: mỗi cửa sổ có băng đỏ (mất dữ liệu, dọn số cũ) và băng hổ
  phách (không chặn). Hiện nguyên văn thông điệp và mã lỗi. Bắt thêm
  `window.error` và `unhandledrejection` ở mức desktop. Gộp trùng, tối đa 5 dòng.
  Vẫn ghi console song song.
- **Mất kết nối**: biểu tượng trạng thái trên thanh trên cùng; tự nối lại với độ
  trễ tăng dần (1, 2, 4… tối đa 30 giây); phiên hết hạn ⇒ về màn đăng nhập.

## 7. Cấu trúc repo

```
MetaOS/
├─ cmd/metaos-ws/        cmd/metaos-bridge/     cmd/metaos-auth/
├─ internal/protocol/    # mã hoá/giải mã frame, định nghĩa điều khiển
├─ internal/channels/    # pty, fs, metrics, proc, systemd, journal
├─ internal/auth/        # PAM (cgo), TOTP, phiên, chặn dò mật khẩu
├─ internal/server/      # HTTP, WebSocket, chuyển tiếp
├─ web/
│  ├─ src/shell/         # thanh trên, Hoạt động, dock, trình quản lý cửa sổ
│  ├─ src/apps/          # terminal, files, monitor, services
│  └─ src/lib/channel.ts # client giao thức kênh
├─ packaging/deb/        # control, postinst, metaos.service, pam.d/metaos
├─ test/integration/     # Dockerfile có systemd + kịch bản
└─ docs/superpowers/specs/
```

## 8. Môi trường phát triển

Máy phát triển chạy Windows 11: backend build/chạy trong WSL2 Ubuntu hoặc
container Docker có systemd. Giao diện chạy `vite dev` với proxy `/api` và `/ws`
sang backend. `make dev` dựng cả hai.

## 9. Kiểm thử

- **Go unit**: protocol (mã hoá/giải mã, frame hỏng, frame quá cỡ); từng kênh (fs
  trên thư mục tạm, metrics trên `/proc` giả, TOTP theo vector RFC 6238, chặn dò
  mật khẩu theo đồng hồ giả). TDD cho `protocol` và `auth`.
- **Tích hợp** (container systemd có user `alice` thường và `bob` có sudo): đăng
  nhập PAM → TOTP → bridge chạy đúng uid; `alice` đọc `/etc/shadow` bị từ chối;
  `bob` restart service qua sudo thành công; phiên hết hạn giết bridge.
- **Web**: Vitest cho trình quản lý cửa sổ (kéo, chia đôi, z-order) và client kênh
  (mở/đóng/lỗi/nối lại). Playwright: đăng nhập + TOTP, mở terminal gõ `whoami`,
  tải file lên và tải xuống, restart một service.
- **Bảo mật**: rà riêng `metaos-auth`, phiên, CSRF, kiểm `Origin` trước khi phát
  hành bản MVP.

## 10. Các mốc

| Mốc | Nội dung | Dùng được gì khi xong |
|---|---|---|
| M1 | Protocol, ws, auth PAM (chưa TOTP), bridge, khung desktop + trình quản lý cửa sổ (chưa có góc nóng, khoá màn hình, chỉ báo CPU/RAM), Terminal | Đăng nhập và dùng terminal trong desktop web |
| M2 | Quản lý file + Monaco; lưu trạng thái desktop; cài đặt phím tắt; góc nóng | Quản lý file |
| M3 | Giám sát hệ thống + tiến trình; chỉ báo CPU/RAM trên thanh trên cùng | Theo dõi máy |
| M4 | Dịch vụ & log, luồng sudo | Quản trị dịch vụ |
| M5 | TOTP, chặn dò mật khẩu, HTTPS tự ký, khoá màn hình, gói `.deb`, rà bảo mật | Được phép mở ra Internet |

**Không mở cổng ra Internet trước khi xong M5.**

## 11. Rủi ro

- **metaos-auth có setuid root**: lỗi ở đây là mất máy. Giữ nó nhỏ, không phân
  tích dữ liệu phức tạp, kiểm uid người gọi, rà bảo mật riêng.
- **PAM cần cgo**: build phải có `libpam0g-dev`; build trong container Debian để
  tránh lệch glibc.
- **Phím tắt bị trình duyệt giữ** (Super, Ctrl+W, Alt+Tab trên một số hệ): có
  phương án thay và ghi rõ trong trợ giúp.
- **Tiến trình tách riêng chết khi restart dịch vụ** (giới hạn đã biết của M1):
  `nohup`/`setsid`/tmux sống qua đăng xuất nhưng vẫn thuộc cgroup của
  `metaos.service` (`KillMode=control-group`), nên chết khi restart/dừng dịch vụ
  hoặc nâng cấp gói. M5 xem xét chạy bridge trong scope riêng theo user
  (`systemd-run` / logind). M1 không đổi `KillMode`.
- **journalctl theo thời gian thực** có thể nặng trên máy nhiều log: giới hạn tốc
  độ đẩy và số dòng giữ trên trình duyệt (mặc định 5.000).
