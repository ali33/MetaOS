package authx

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

// Chạy lại chính binary test ở tiến trình con có stdout là ống, để việc đổi
// fd 1 không làm hỏng output của go test.
func TestStdoutGuard(t *testing.T) {
	if os.Getenv("METAOS_FDGUARD_CHILD") == "1" {
		restore, err := GuardFD(1)
		if err != nil {
			os.Stderr.WriteString(err.Error())
			os.Exit(3)
		}
		os.Stdout.WriteString("PAM-RÁC")
		if err := restore(); err != nil {
			os.Exit(4)
		}
		os.Stdout.WriteString("OK")
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStdoutGuard$")
	cmd.Env = append(os.Environ(), "METAOS_FDGUARD_CHILD=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("tiến trình con lỗi: %v", err)
	}
	if string(out) != "OK" {
		t.Fatalf("stdout bị bẩn: %q", out)
	}
}

func TestGuardFDSavedIsCloexecAndRestoreIdempotent(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	target, err := syscall.Dup(int(w.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(target)

	before := openFDs(t)
	restore, err := GuardFD(target)
	if err != nil {
		t.Fatal(err)
	}
	// fd mới xuất hiện sau GuardFD chính là bản giữ lại; nó phải có CLOEXEC.
	var saved int = -1
	for fd := range openFDs(t) {
		// bỏ fd tạm của chính việc đọc /proc/self/fd (đã đóng khi ReadDir xong)
		if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0); e == 0 && !before[fd] {
			saved = fd
		}
	}
	if saved < 0 {
		t.Fatal("không tìm thấy fd giữ lại")
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(saved), syscall.F_GETFD, 0)
	if errno != 0 || flags&syscall.FD_CLOEXEC == 0 {
		t.Fatalf("fd giữ lại thiếu FD_CLOEXEC: flags=%d errno=%v", flags, errno)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	// fd bị đóng rồi có thể được tái dùng; restore lần hai không được đụng vào.
	reused, err := syscall.Dup(target)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(reused)
	if err := restore(); err != nil {
		t.Fatalf("restore lần hai phải là no-op: %v", err)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(reused), syscall.F_GETFD, 0); errno != 0 {
		t.Fatalf("restore lần hai đã đóng nhầm fd khác: %v", errno)
	}
}

func openFDs(t *testing.T) map[int]bool {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	m := map[int]bool{}
	for _, e := range ents {
		n, _ := strconv.Atoi(e.Name())
		// ReadDir đã đóng fd thư mục của nó; chỉ giữ fd còn sống
		if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(n), syscall.F_GETFD, 0); errno == 0 {
			m[n] = true
		}
	}
	return m
}
