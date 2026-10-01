// metaos-ws: chạy dưới user "metaos", không root. Xem spec mục 3.1.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ali33/MetaOS/internal/server"
	"github.com/ali33/MetaOS/web"
)

func main() {
	cfg := server.DefaultConfig()
	flag.StringVar(&cfg.Listen, "listen", cfg.Listen, "địa chỉ lắng nghe")
	flag.StringVar(&cfg.TLSCert, "tls-cert", "", "chứng chỉ PEM")
	flag.StringVar(&cfg.TLSKey, "tls-key", "", "khoá PEM")
	flag.StringVar(&cfg.AuthPath, "auth-path", cfg.AuthPath, "đường dẫn metaos-auth")
	flag.BoolVar(&cfg.AllowRoot, "allow-root", false, "cho root đăng nhập trực tiếp")
	flag.DurationVar(&cfg.SessionMax, "session-max", cfg.SessionMax, "thời hạn tối đa của phiên")
	flag.DurationVar(&cfg.SessionIdle, "session-idle", cfg.SessionIdle, "hết hạn khi không hoạt động")
	flag.DurationVar(&cfg.HelloTimeout, "hello-timeout", cfg.HelloTimeout, "chờ bridge khởi động")
	flag.Parse()
	logger := log.New(os.Stderr, "metaos-ws: ", 0)
	if err := cfg.Validate(); err != nil {
		logger.Fatal(err)
	}
	store := server.NewStore(server.RealClock{}, cfg.SessionMax, cfg.SessionIdle)
	srv := server.New(cfg, store, server.NewAuthLauncher(cfg, os.Stderr), web.Dist, os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go srv.RunReaper(ctx, min(cfg.SessionIdle/2, time.Minute))
	hs := &http.Server{
		Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second,
		// Tắt HTTP/2: WebSocket qua HTTP/2 (RFC 8441) không được thư viện hỗ trợ;
		// ép HTTP/1.1 để trình duyệt luôn nâng cấp bằng Upgrade.
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}
	// Tắt: đóng mọi WebSocket 4401 "shutdown" và CHỜ frame đóng đi hết rồi mới
	// thoát; tổng cộng tối đa 10 giây, dưới TimeoutStopSec mặc định (90 s) của systemd.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Song song: hs.Shutdown đóng listener ngay nhưng có thể chờ một request
		// đăng nhập đang dở; không để nó ăn hết hạn chót của việc đóng WebSocket.
		httpDone := make(chan struct{})
		go func() {
			defer close(httpDone)
			_ = hs.Shutdown(sctx)
		}()
		srv.Shutdown(sctx)
		<-httpDone
	}()
	logger.Printf("listening on %s (tls=%v)", cfg.Listen, cfg.TLS())
	var err error
	if cfg.TLS() {
		err = hs.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		err = hs.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
	<-stopped
}
