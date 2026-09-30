// metaos-bridge: chạy bằng uid của user đã đăng nhập, nói giao thức kênh qua
// stdin/stdout với metaos-ws. Không có quyền đặc biệt.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"

	"github.com/ali33/MetaOS/internal/bridge"
	"github.com/ali33/MetaOS/internal/channels"
	"github.com/ali33/MetaOS/internal/channels/pty"
	"github.com/ali33/MetaOS/internal/protocol"
)

var version = "dev" // -ldflags "-X main.version=…"

func main() {
	// Ghi vào stdout khi ws đã chết sẽ gây SIGPIPE; mặc định Go thoát ngay và
	// bỏ lại các shell không ai dọn. Dùng Notify (không phải Ignore): trạng thái
	// SIG_IGN được giữ qua exec nên Ignore sẽ lây sang mọi lệnh người dùng chạy
	// (`yes | head` báo Broken pipe, job nền bỏ qua SIGHUP). Với Notify, lệnh
	// write trả EPIPE và các tiến trình con vẫn nhận xử lý tín hiệu mặc định.
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	go func() {
		for range sigpipe {
		}
	}()
	out := bridge.NewSender(os.Stdout)
	u, err := user.Current()
	if err != nil {
		fmt.Fprintf(os.Stderr, "metaos-bridge: %v\n", err)
		os.Exit(1)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	host, _ := os.Hostname()
	if err := out.SendText(protocol.Control(protocol.TypeHello, protocol.HelloData{
		User: u.Username, UID: uid, GID: gid, Home: u.HomeDir, Hostname: host, Version: version,
	})); err != nil {
		os.Exit(1)
	}
	kinds := map[string]channels.Factory{"pty": pty.New}
	if err := bridge.NewRouter(os.Stdin, out, kinds, os.Stderr).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "metaos-bridge: %v\n", err)
		os.Exit(1)
	}
}
