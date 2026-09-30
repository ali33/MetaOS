# MetaOS — luật riêng của dự án

MetaOS: desktop web kiểu GNOME để quản trị máy chủ Linux. Backend Go (metaos-ws không-root,
metaos-auth setuid PAM, metaos-bridge chạy bằng uid người đăng nhập), frontend React + TS + Vite
nhúng vào Go, một WebSocket nhiều kênh, đăng nhập PAM + TOTP.

## Sheet quản lý dự án

Nguồn sự thật về tiến độ và phản hồi của tester. Quy trình đọc/ghi, cấu trúc cột, trạng thái: skill
**`quan-ly-du-an`** (`~/.claude/skills/quan-ly-du-an/`). Đầu phiên đọc sheet để lấy việc kế tiếp và phản hồi
đang mở; sau mỗi lần bắt đầu/xong việc, commit, đổi kế hoạch, bị chặn, xử lý phản hồi ⇒ cập nhật sheet.

- Sheet: **"[X] MetaOS"** — https://docs.google.com/spreadsheets/d/1pIaFPXpWtPoOLYjyaM_I-ZIf8DzfQjd0aURTUt8FG7I/edit?gid=0#gid=0
- File id: `1pIaFPXpWtPoOLYjyaM_I-ZIf8DzfQjd0aURTUt8FG7I`
- Tab `Công việc` (trước đây tên `Sheet1`): gid `0` · mã `M0-xx`…`M5-xx`, dòng tổng `M2`…`M5`, `X` sau MVP, `TD-xx` tồn đọng
- Tab `Phản hồi`: gid `<chưa có — điền sau khi tạo tab>` · mã `PH-001`…
- Chủ dự án (duyệt đề xuất, quyết định): `Chủ dự án` (chưa có họ tên — điền khi người dùng cho biết)
- Tester: chưa có
- Spec: `docs/superpowers/specs/` · kế hoạch: `docs/superpowers/plans/`
- Đọc qua Drive: chưa kiểm với hai tab
- Ghi: qua Chrome theo skill (§4) — chỉ dán khối liền không có ô rỗng (ô rỗng sẽ xoá ô đích). Không đọc/ghi
  được ⇒ báo ngay cho người dùng, không bỏ qua im lặng.

## Tài liệu chính

- Spec: `docs/superpowers/specs/2026-09-30-metaos-web-desktop-design.md`
- Kế hoạch M1: `docs/superpowers/plans/2026-09-30-metaos-m1.md`

## Luật an toàn

- **Không mở cổng MetaOS ra Internet trước khi xong M5** (TOTP, chặn dò mật khẩu, rà bảo mật).
- Mọi lệnh Go build/test chạy trong Docker; máy phát triển là Windows 11.
