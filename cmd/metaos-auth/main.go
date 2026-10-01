// metaos-auth: setuid root. Kiểm mật khẩu qua PAM rồi hạ quyền và exec
// metaos-bridge. Chỉ user "metaos" được gọi. Xem docs/superpowers/specs mục 3.1.
package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/ali33/MetaOS/internal/authx"
)

// Hằng số biên dịch: người gọi không được chọn file để exec.
const bridgePath = "/usr/lib/metaos/metaos-bridge"

const exitOK, exitAuth, exitUsage, exitInternal, exitExpired = 0, 1, 2, 3, 4

func main() { os.Exit(run()) }

func fail(code int, format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "metaos-auth: "+format+"\n", a...)
	return code
}

func lookupUID(name string) (int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

func run() int {
	args, err := authx.ParseArgs(os.Args[1:])
	if err != nil {
		return fail(exitUsage, "%v", err)
	}
	// uid THẬT (getuid), không phải euid — euid luôn là 0 dưới setuid.
	if err := authx.CheckCaller(os.Getuid(), lookupUID); err != nil {
		return fail(exitUsage, "%v (uid %d)", err, os.Getuid())
	}
	os.Clearenv() // môi trường của người gọi không được chạm tới PAM lẫn bridge
	// fd 1 trỏ /dev/null từ trước pam_start tới ngay trước exec; các nhánh lỗi
	// thoát khi fd 1 vẫn là /dev/null.
	restore, err := authx.GuardFD(1)
	if err != nil {
		return fail(exitInternal, "guard stdout: %v", err)
	}
	pw, err := authx.ReadPassword(os.Stdin)
	if err == authx.ErrPasswordInvalid || err == authx.ErrPasswordTooLong {
		return fail(exitAuth, "%v", err)
	} else if err != nil { // stdin hỏng là lỗi hạ tầng, không phải sai mật khẩu
		return fail(exitInternal, "read password: %v", err)
	}
	code, rc := pamCheck(args.User, pw)
	authx.Wipe(pw)
	if code != exitOK {
		return fail(code, "pam check failed for %s (pam %d)", args.User, rc)
	}
	if args.CheckOnly {
		return exitOK
	}
	u, err := user.Lookup(args.User)
	if err != nil {
		return fail(exitInternal, "lookup %s: %v", args.User, err)
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return fail(exitInternal, "bad uid/gid for %s", args.User)
	}
	shell, err := initgroups(args.User, gid)
	if err != nil {
		return fail(exitInternal, "%v", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fail(exitInternal, "setgid: %v", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fail(exitInternal, "setuid: %v", err)
	}
	// Phải không lấy lại được root (lấy lại được nghĩa là hạ quyền hỏng) và mọi id phải khớp.
	if (uid != 0 && syscall.Setuid(0) == nil) || os.Getuid() != uid || os.Geteuid() != uid || os.Getgid() != gid || os.Getegid() != gid {
		return fail(exitInternal, "privilege drop failed")
	}
	if err := os.Chdir(u.HomeDir); err != nil {
		_ = os.Chdir("/")
	}
	syscall.Umask(0o022)
	if err := closeExtraFDs(); err != nil {
		return fail(exitInternal, "close fds: %v", err)
	}
	env := authx.BridgeEnv(authx.UserInfo{Name: args.User, UID: uid, GID: gid, Home: u.HomeDir, Shell: shell})
	flushStdio() // fd 1 vẫn là /dev/null: bộ đệm printf của PAM/NSS đổ vào đó
	if err := restore(); err != nil {
		return fail(exitInternal, "restore stdout: %v", err)
	}
	return fail(exitInternal, "exec %s: %v", bridgePath, syscall.Exec(bridgePath, []string{"metaos-bridge"}, env))
}

// closeExtraFDs đánh dấu close-on-exec mọi fd > 2 — kể cả fd do module PAM mở —
// để bridge chỉ thừa hưởng stdin/stdout/stderr. Không đọc được danh sách fd thì
// dừng (fail closed) thay vì exec với fd lạ.
func closeExtraFDs() error {
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return err
	}
	for _, e := range ents {
		if fd, err := strconv.Atoi(e.Name()); err == nil && fd > 2 {
			syscall.CloseOnExec(fd)
		}
	}
	return nil
}
