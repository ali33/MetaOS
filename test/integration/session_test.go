//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestIdleExpiryKillsBridge đổi session-idle của dịch vụ thành 6 giây, rồi trả lại.
func TestIdleExpiryKillsBridge(t *testing.T) {
	setOpts := func(opts string) {
		cmd := "rm -f /etc/default/metaos"
		if opts != "" {
			cmd = "echo 'METAOS_OPTS=" + opts + "' > /etc/default/metaos"
		}
		if out, err := dexec(t, "sh", "-c", cmd+" && systemctl restart metaos"); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		waitReady(t)
	}
	setOpts("--session-idle=6s --session-max=1h")
	t.Cleanup(func() { setOpts("") })

	s := mustLogin(t, "alice", "alice-pass-1")
	c := dialWS(t, s)
	defer c.CloseNow()
	tm := openPty(t, c, "t1", false)
	tm.send("sleep 1000 & echo STARTED-$((1+1))\n")
	tm.expect("STARTED-2")
	waitProcs(t, "alice", 5*time.Second, "sleep 1000 đã chạy", func(list string) bool {
		return strings.Contains(list, "sleep 1000")
	})
	// Không gửi gì nữa. Ping WebSocket của ws không tính là hoạt động.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var err error
	for err == nil {
		_, _, err = c.Read(ctx)
	}
	var ce websocket.CloseError
	if !errors.As(err, &ce) || ce.Code != 4401 || ce.Reason != "expired" {
		t.Fatalf("muốn đóng 4401 expired, nhận %v", err)
	}
	waitNoProcs(t, "alice", 10*time.Second)
}
