package authx

import (
	"os"
	"os/exec"
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
