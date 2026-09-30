# MetaOS — luật riêng của dự án

MetaOS: desktop web kiểu GNOME để quản trị máy chủ Linux. Backend Go (metaos-ws không-root,
metaos-auth setuid PAM, metaos-bridge chạy bằng uid người đăng nhập), frontend React + TS + Vite
nhúng vào Go, một WebSocket nhiều kênh, đăng nhập PAM + TOTP.

## Sheet quản lý dự án — nguồn sự thật về tiến độ

Sheet **"[X] MetaOS"** là file quản lý dự án: toàn bộ hạng mục, việc đã làm, việc dự kiến,
trạng thái, người phụ trách, commit.
https://docs.google.com/spreadsheets/d/1pIaFPXpWtPoOLYjyaM_I-ZIf8DzfQjd0aURTUt8FG7I/edit?gid=0#gid=0
(file id `1pIaFPXpWtPoOLYjyaM_I-ZIf8DzfQjd0aURTUt8FG7I`, tab `Sheet1`)

### Cấu trúc (không đổi thứ tự cột)

| Cột | Tên | Giá trị |
|---|---|---|
| A | Mã | `M0-xx` thiết kế/quản lý · `M1-xx`…`M5-xx` task theo mốc (xx = số Task trong kế hoạch) · `M2`…`M5` dòng tổng của mốc chưa lập kế hoạch · `X1`… sau MVP |
| B | Mốc | `M1 Nền + Terminal`, `M2 Quản lý file`, `M3 Giám sát`, `M4 Dịch vụ & log`, `M5 Ra Internet`, `Sau MVP` |
| C | Hạng mục | tên ngắn, đọc được không cần mở tài liệu |
| D | Loại | Tài liệu · Rà soát · Quản lý · Quyết định · Phát triển · Mốc |
| E | Phụ thuộc | mã các dòng khác, cách nhau bởi dấu phẩy |
| F | Trạng thái | **Chưa lập kế hoạch · Chưa làm · Đang làm · Chờ duyệt · Bị chặn · Xong · Tồn đọng · Huỷ** |
| G | Ưu tiên | Cao · Trung bình · Thấp |
| H | Phụ trách | `Claude` hoặc `Chủ dự án` (quyết định/duyệt) |
| I, J | Bắt đầu, Hoàn thành | `YYYY-MM-DD` |
| K | Commit | hash ngắn, nhiều hash cách nhau bởi dấu cách |
| L | Tài liệu | đường dẫn trong repo, kèm `— Task N` hoặc `§mục` |
| M | Ghi chú | lý do bị chặn, quyết định, việc dời mốc… |

### Khi nào cập nhật

- **Đầu phiên:** đọc sheet để biết việc đang dở và việc kế tiếp (task `Chưa làm` có mọi phụ thuộc đã `Xong`).
- **Bắt đầu một task** ⇒ `Đang làm` + ngày bắt đầu.
- **Xong một task** (test xanh, đã commit) ⇒ `Xong` + ngày hoàn thành + hash commit.
- **Gặp lỗi chặn / chờ người dùng** ⇒ `Bị chặn` hoặc `Chờ duyệt`, ghi lý do ở cột M.
- **Lập kế hoạch mốc mới** ⇒ thay dòng tổng `Mx` bằng các dòng `Mx-01…`; **đổi kế hoạch/quyết định** ⇒ sửa
  dòng liên quan và ghi lý do ở cột M. Dòng bỏ thì đặt `Huỷ`, **không xoá** (giữ dấu vết).
- Sheet phải khớp với kế hoạch (`docs/superpowers/plans/`) và spec (`docs/superpowers/specs/`).

### Cách đọc / ghi

- **Đọc:** Google Drive connector `read_file_content` với file id trên (nhanh, không cần trình duyệt).
- **Ghi:** Drive connector **không ghi được ô**. Người dùng đã chọn ghi qua **Chrome** (claude-in-chrome):
  mở sheet, bấm chọn ô đích (vd. ô cột F của dòng task), rồi dán bằng JavaScript:
  ```js
  const dt = new DataTransfer();
  dt.setData('text/plain', 'Xong\t\t\t2026-10-01');   // Tab = sang ô phải, \n = xuống dòng
  document.activeElement.dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true}));
  ```
  Ghi xong **đọc lại bằng Drive connector** để xác nhận đã lưu.
- Không đọc được hoặc không ghi được (thiếu quyền, Chrome không phản hồi, lỗi kết nối) ⇒ báo ngay cho
  người dùng, không bỏ qua im lặng.

## Tài liệu chính

- Spec: `docs/superpowers/specs/2026-09-30-metaos-web-desktop-design.md`
- Kế hoạch M1: `docs/superpowers/plans/2026-09-30-metaos-m1.md`

## Luật an toàn

- **Không mở cổng MetaOS ra Internet trước khi xong M5** (TOTP, chặn dò mật khẩu, rà bảo mật).
- Mọi lệnh Go build/test chạy trong Docker; máy phát triển là Windows 11.
