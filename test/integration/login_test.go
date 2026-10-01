//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"
)

func TestLoginWrongPasswordIs401AndJournaled(t *testing.T) {
	if _, code := login(t, "alice", "sai-mat-khau"); code != 401 {
		t.Fatalf("got %d", code)
	}
	out, _ := dexec(t, "journalctl", "SYSLOG_IDENTIFIER=metaos", "--since", "-2min", "--no-pager", "-o", "cat")
	if !strings.Contains(out, `login failed user="alice"`) {
		t.Fatalf("journal thiếu dòng đăng nhập hỏng:\n%s", out)
	}
}

func TestRootLoginDisabled(t *testing.T) {
	if _, code := login(t, "root", "bất-kỳ"); code != 403 {
		t.Fatalf("got %d", code)
	}
}

func TestBridgeRunsAsUser(t *testing.T) {
	for user, pw := range map[string]string{"alice": "alice-pass-1", "bob": "bob-pass-1"} {
		t.Run(user, func(t *testing.T) {
			s := mustLogin(t, user, pw)
			defer logout(t, s)
			uid, _ := dexec(t, "id", "-u", user)
			c := dialWS(t, s)
			defer c.CloseNow()
			tm := openPty(t, c, "t1", false)
			tm.send("echo UID=$(id -u) NAME=$(id -un) HOME=$HOME\n")
			tm.expect("UID=" + uid + " NAME=" + user + " HOME=/home/" + user)
			ps, _ := dexec(t, "ps", "-o", "user=", "-C", "metaos-bridge")
			if !strings.Contains(ps, user) {
				t.Fatalf("không có metaos-bridge chạy bằng %s:\n%s", user, ps)
			}
		})
	}
}

func TestAliceCannotReadShadow(t *testing.T) {
	s := mustLogin(t, "alice", "alice-pass-1")
	defer logout(t, s)
	c := dialWS(t, s)
	defer c.CloseNow()
	tm := openPty(t, c, "t1", false)
	// PTY vọng lại dòng lệnh đã gõ: dấu hiệu phải chỉ xuất hiện SAU khi shell mở
	// rộng biểu thức, nên dùng $((…)) thay cho chuỗi cố định.
	tm.send("cat /etc/shadow; echo RC-$((0+$?))-END\n")
	out := tm.expect("RC-1-END")
	if !strings.Contains(out, "Permission denied") {
		t.Fatalf("đầu ra:\n%s", out)
	}
}

func TestLogoutKillsBridge(t *testing.T) { // Review Focus #1
	s := mustLogin(t, "alice", "alice-pass-1")
	c := dialWS(t, s)
	defer c.CloseNow()
	tm := openPty(t, c, "t1", false)
	t.Cleanup(func() { _, _ = dexec(t, "pkill", "-KILL", "-u", "alice") })
	tm.send("sleep 1000 & nohup sleep 1001 >/dev/null 2>&1 & setsid sleep 1002 </dev/null >/dev/null 2>&1 & echo STARTED-$((1+1))\n")
	tm.expect("STARTED-2")
	waitProcs(t, "alice", 5*time.Second, "đủ 3 tiến trình sleep trước khi đăng xuất", func(list string) bool {
		return strings.Contains(list, "sleep 1000") && strings.Contains(list, "sleep 1001") && strings.Contains(list, "sleep 1002")
	})
	if code := logout(t, s); code != 204 {
		t.Fatalf("logout %d", code)
	}
	// Q1/Q3: shell, bridge và job thường chết; nohup và setsid được sống (giống SSH).
	waitProcs(t, "alice", 10*time.Second, "chỉ còn nohup + setsid", func(list string) bool {
		return !strings.Contains(list, "metaos-bridge") && !strings.Contains(list, "bash") &&
			!strings.Contains(list, "sleep 1000") &&
			strings.Contains(list, "sleep 1001") && strings.Contains(list, "sleep 1002")
	})
}

func TestReconnectReplaysOutput(t *testing.T) {
	s := mustLogin(t, "alice", "alice-pass-1")
	defer logout(t, s)
	c := dialWS(t, s)
	tm := openPty(t, c, "t1", false)
	tm.send("echo MARK-$((6*7))\n")
	tm.expect("MARK-42")
	c.CloseNow() // giả lập rớt mạng
	time.Sleep(time.Second)
	c2 := dialWS(t, s)
	defer c2.CloseNow()
	tm2 := openPty(t, c2, "t1", true)
	tm2.expect("MARK-42") // có trong bộ đệm phát lại
	tm2.send("echo SAU-$((1+1))\n")
	tm2.expect("SAU-2")
}
