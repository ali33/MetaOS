package pty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ali33/MetaOS/internal/channels"
	"github.com/ali33/MetaOS/internal/protocol"
)

type fakeSender struct {
	mu   sync.Mutex
	out  bytes.Buffer
	msgs []protocol.Message
}

func (f *fakeSender) SendText(m protocol.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, m)
	return nil
}
func (f *fakeSender) SendBinary(ch string, p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out.Write(p)
	return nil
}
func (f *fakeSender) output() string { f.mu.Lock(); defer f.mu.Unlock(); return f.out.String() }
func (f *fakeSender) reset()         { f.mu.Lock(); defer f.mu.Unlock(); f.out.Reset() }
func (f *fakeSender) findMsg(typ string) (protocol.Message, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.msgs) - 1; i >= 0; i-- {
		if f.msgs[i].Type == typ {
			return f.msgs[i], true
		}
	}
	return protocol.Message{}, false
}
func (f *fakeSender) closeMsg() (protocol.CloseData, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.msgs {
		if m.Type == protocol.TypeClose {
			var c protocol.CloseData
			_ = json.Unmarshal(m.Data, &c)
			return c, true
		}
	}
	return protocol.CloseData{}, false
}

func syscallKill(pid int) error { p, _ := os.FindProcess(pid); return p.Kill() }

// alive: tiến trình còn chạy thật; zombie (trạng thái Z) coi như đã chết.
func alive(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	st := string(b)
	i := strings.LastIndexByte(st, ')')
	return i >= 0 && len(st) > i+2 && st[i+2] != 'Z'
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("hết giờ chờ: %s", what)
}

func open(t *testing.T, params string) (channels.Channel, *fakeSender, chan struct{}) {
	t.Helper()
	s := &fakeSender{}
	done := make(chan struct{})
	c, err := New(channels.OpenArgs{ID: "t1", Params: json.RawMessage(params), Out: s, Done: func() { close(done) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close("test-cleanup") })
	return c, s, done
}

func TestPtyEcho(t *testing.T) {
	c, s, _ := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("echo hi-$((1+1))\n"))
	waitFor(t, "hi-2", func() bool { return bytes.Contains([]byte(s.output()), []byte("hi-2")) })
}

func TestPtyResize(t *testing.T) {
	c, s, _ := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleText(protocol.Message{Ch: "t1", Type: "resize", Data: json.RawMessage(`{"cols":100,"rows":40}`)})
	c.HandleBinary([]byte("stty size\n"))
	waitFor(t, "40 100", func() bool { return bytes.Contains([]byte(s.output()), []byte("40 100")) })
}

func TestPtyExitSendsCloseWithCode(t *testing.T) {
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("exit 3\n"))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Done chưa được gọi")
	}
	cd, ok := s.closeMsg()
	if !ok || cd.Reason != "exit" || cd.ExitCode == nil || *cd.ExitCode != 3 {
		t.Fatalf("close = %+v ok=%v", cd, ok)
	}
}

func TestPtyInvalidParams(t *testing.T) {
	cases := map[string]string{
		`{"cols":0,"rows":24}`:                            protocol.CodeInvalidParams,
		`{"cols":80,"rows":1001}`:                         protocol.CodeInvalidParams,
		`{"cols":80,"rows":24,"shell":"/usr/bin/env"}`:    protocol.CodeInvalidParams,
		`{"cols":80,"rows":24,"cwd":"/khong-co-thu-muc"}`: protocol.CodeNotFound,
		`{"cols":80,"rows":24,"cwd":"/etc/hostname"}`:     protocol.CodeInvalidParams,
		`không phải json`:                                 protocol.CodeInvalidParams,
	}
	for params, code := range cases {
		_, err := New(channels.OpenArgs{ID: "x", Params: json.RawMessage(params), Out: &fakeSender{}, Done: func() {}})
		var ce *channels.Error
		if !errors.As(err, &ce) || ce.Code != code {
			t.Errorf("%s: got %v, want code %s", params, err, code)
		}
	}
}

func TestPtyCloseKillsProcessGroup(t *testing.T) { // Review Focus #1
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("sleep 300 & echo PID=$!\n"))
	re := regexp.MustCompile(`PID=(\d+)`)
	waitFor(t, "PID", func() bool { return re.MatchString(s.output()) })
	pid, _ := strconv.Atoi(re.FindStringSubmatch(s.output())[1])
	c.Close("client")
	<-done
	waitFor(t, "job nền chết", func() bool { return !alive(pid) })
	if cd, _ := s.closeMsg(); cd.Reason != "client" {
		t.Fatalf("reason = %q", cd.Reason)
	}
}

// Q3: lệnh cố ý tách riêng được sống sau khi kênh đóng (đăng xuất / hết hạn).
func TestPtyCloseSparesDetached(t *testing.T) {
	old := KillGrace
	KillGrace = 300 * time.Millisecond
	t.Cleanup(func() { KillGrace = old })
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	// setsid fork khi đang là trưởng nhóm (job nền của bash luôn là trưởng nhóm),
	// nên $! là pid của tiến trình cha đã thoát. Để tiến trình mới tự in pid của nó.
	c.HandleBinary([]byte("nohup sleep 301 >/dev/null 2>&1 & echo NPID=$!; setsid sh -c 'echo SPID=$$; exec sleep 302' </dev/null 2>/dev/null &\n"))
	re := regexp.MustCompile(`NPID=(\d+)[\s\S]*SPID=(\d+)\r?\n`)
	waitFor(t, "pid", func() bool { return re.MatchString(s.output()) })
	m := re.FindStringSubmatch(s.output())
	npid, _ := strconv.Atoi(m[1])
	spid, _ := strconv.Atoi(m[2])
	t.Cleanup(func() { _ = syscallKill(npid); _ = syscallKill(spid) })
	c.Close("client")
	<-done
	time.Sleep(KillGrace + 500*time.Millisecond)
	if !alive(npid) || !alive(spid) {
		t.Fatalf("nohup còn=%v setsid còn=%v — cả hai phải sống", alive(npid), alive(spid))
	}
}

// D6/Q1: tiến trình BẮT (không bỏ qua) SIGHUP vẫn phải chết khi kênh bị đóng, và
// phải chết TRƯỚC khi Done được gọi — router thoát ngay sau Done, nên SIGKILL hẹn
// giờ sau Done sẽ không bao giờ chạy.
func TestPtyCloseKillsHUPCatcher(t *testing.T) {
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte(`bash -c 'trap ":" HUP; echo CPID=$$; while :; do sleep .1; done' &` + "\n"))
	re := regexp.MustCompile(`CPID=(\d+)\r?\n`)
	waitFor(t, "CPID", func() bool { return re.MatchString(s.output()) })
	pid, _ := strconv.Atoi(re.FindStringSubmatch(s.output())[1])
	t.Cleanup(func() { _ = syscallKill(pid) })
	if !alive(pid) {
		t.Fatalf("tiến trình bắt HUP %d chưa chạy", pid)
	}
	c.Close("client")
	select {
	case <-done:
	case <-time.After(KillGrace + 5*time.Second):
		t.Fatal("Done chưa được gọi")
	}
	// SIGKILL đã gửi trước Done; chỉ chừa chút thời gian để nhân kết liễu tiến trình.
	deadline := time.Now().Add(200 * time.Millisecond)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if alive(pid) {
		t.Fatalf("tiến trình bắt SIGHUP %d vẫn sống sau khi Done được gọi", pid)
	}
	if cd, _ := s.closeMsg(); cd.Reason != "client" {
		t.Fatalf("reason = %q", cd.Reason)
	}
}

func TestPtyDetachReattachReplays(t *testing.T) {
	c, s, _ := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("echo MARK-$((6*7))\n"))
	waitFor(t, "MARK-42", func() bool { return bytes.Contains([]byte(s.output()), []byte("MARK-42")) })
	c.Detach()
	s.reset()
	c.Reattach(nil) // không có offset ⇒ bản chụp đầy đủ, reset=true
	if !bytes.Contains([]byte(s.output()), []byte("MARK-42")) {
		t.Fatalf("không phát lại bộ đệm: %q", s.output())
	}
	m, ok := s.findMsg("replay")
	var rd ReplayData
	_ = json.Unmarshal(m.Data, &rd)
	if !ok || !rd.Reset {
		t.Fatalf("muốn replay reset=true, nhận %s", m.Data)
	}
}

func TestPtyReattachSendsOnlyMissed(t *testing.T) {
	c, s, _ := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("echo ONE-$((1+0))\n"))
	waitFor(t, "ONE-1", func() bool { return strings.Contains(s.output(), "ONE-1") })
	time.Sleep(300 * time.Millisecond) // để dấu nhắc in xong
	got := uint64(len(s.output()))
	c.Detach()
	c.HandleBinary([]byte("echo TWO-$((1+1))\n")) // chạy lúc đang rớt: chỉ vào bộ đệm
	time.Sleep(500 * time.Millisecond)
	s.reset()
	c.Reattach(json.RawMessage(fmt.Sprintf(`{"reattach":true,"offset":%d}`, got)))
	out := s.output()
	if strings.Contains(out, "ONE-1") || !strings.Contains(out, "TWO-2") {
		t.Fatalf("chỉ được gửi phần còn thiếu: %q", out)
	}
	m, _ := s.findMsg("replay")
	var rd ReplayData
	_ = json.Unmarshal(m.Data, &rd)
	if rd.Reset || rd.Offset != got {
		t.Fatalf("replay = %+v, muốn reset=false offset=%d", rd, got)
	}
}

func TestPtyInputDoesNotBlock(t *testing.T) { // Review Focus #5
	c, s, _ := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("sleep 30\n")) // chương trình tiền cảnh không đọc stdin
	time.Sleep(300 * time.Millisecond)
	chunk := bytes.Repeat([]byte("x"), 32<<10)
	start := time.Now()
	for i := 0; i < 200; i++ { // 6,4 MB
		c.HandleBinary(chunk)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("HandleBinary bị chặn %s", d)
	}
	waitFor(t, "báo hàng đợi đầy", func() bool {
		m, ok := s.findMsg(protocol.TypeError)
		return ok && strings.Contains(string(m.Data), "input queue full")
	})
}

func TestPtyExitWithBackgroundJobClosesChannel(t *testing.T) {
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("sleep 100 & exit 0\n")) // job nền vẫn giữ đầu slave của PTY
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shell đã thoát mà kênh không đóng")
	}
	if cd, _ := s.closeMsg(); cd.Reason != "exit" || cd.ExitCode == nil || *cd.ExitCode != 0 {
		t.Fatalf("close = %+v", cd)
	}
}

func TestPtyDetachTimeout(t *testing.T) {
	old := DetachTimeout
	DetachTimeout = 200 * time.Millisecond
	t.Cleanup(func() { DetachTimeout = old })
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.Detach()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("kênh không tự đóng sau DetachTimeout")
	}
	if cd, _ := s.closeMsg(); cd.Reason != "detach-timeout" {
		t.Fatalf("reason = %q", cd.Reason)
	}
}

func TestPtyReattachCancelsTimeout(t *testing.T) {
	old := DetachTimeout
	DetachTimeout = 200 * time.Millisecond
	t.Cleanup(func() { DetachTimeout = old })
	c, _, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.Detach()
	c.Reattach(nil)
	select {
	case <-done:
		t.Fatal("gắn lại rồi mà kênh vẫn bị đóng")
	case <-time.After(600 * time.Millisecond):
	}
}

// blockSender: SendBinary chặn cho tới khi release đóng — như ống stdout của
// bridge đầy vì ws đã ngừng đọc (đăng xuất). SendText vẫn đi.
type blockSender struct {
	fakeSender
	entered chan struct{}
	once    sync.Once
	release chan struct{}
}

func (b *blockSender) SendBinary(ch string, p []byte) error {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return b.fakeSender.SendBinary(ch, p)
}

// Review Focus #1: Close không được chờ một khoá đang bị giữ qua lệnh ghi đầu ra.
func TestPtyCloseNotBlockedByStuckOutput(t *testing.T) {
	s := &blockSender{entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	c, err := New(channels.OpenArgs{ID: "t1", Params: json.RawMessage(`{"cols":80,"rows":24,"shell":"/bin/bash"}`), Out: s, Done: func() { close(done) }})
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(s.release) }) }
	t.Cleanup(func() { unblock(); c.Close("test-cleanup") })
	select {
	case <-s.entered: // pump đang kẹt trong SendBinary
	case <-time.After(5 * time.Second):
		t.Fatal("shell không in gì")
	}
	pid := c.(*channel).cmd.Process.Pid
	returned := make(chan struct{})
	go func() { c.Detach(); c.Close("shutdown"); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Detach/Close bị chặn sau lệnh ghi đầu ra đang kẹt")
	}
	waitFor(t, "shell chết dù đầu ra kẹt", func() bool { return !alive(pid) })
	unblock()
	select {
	case <-done:
	case <-time.After(KillGrace + 2*time.Second):
		t.Fatal("Done chưa được gọi sau khi đầu ra thông")
	}
	if cd, _ := s.closeMsg(); cd.Reason != "shutdown" {
		t.Fatalf("reason = %q", cd.Reason)
	}
}

// D6: shell đăng nhập bỏ qua SIGHUP vẫn phải chết — shell không bao giờ là lệnh
// "cố ý tách riêng" (Q3 chỉ tha tiến trình con dùng nohup/setsid).
func TestPtyCloseKillsHUPIgnoringShell(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(home+"/.bash_profile", []byte("trap '' HUP\necho READY-$((2*21))\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	waitFor(t, "READY-42", func() bool { return strings.Contains(s.output(), "READY-42") })
	pid := c.(*channel).cmd.Process.Pid
	if !ignoresHUP(pid) {
		t.Fatalf("shell %d phải đang bỏ qua SIGHUP", pid)
	}
	c.Close("client")
	select {
	case <-done:
	case <-time.After(KillGrace + time.Second):
		t.Fatal("shell bỏ qua SIGHUP không bị giết: Done chưa được gọi")
	}
	if alive(pid) {
		t.Fatalf("shell %d vẫn sống", pid)
	}
}

// Shell đã thoát (đã được thu dọn) ⇒ Close/Detach muộn không được bắn tín hiệu:
// pid có thể đã thuộc về tiến trình khác.
func TestPtyLateCloseDoesNotSignal(t *testing.T) {
	old := DetachTimeout
	DetachTimeout = 50 * time.Millisecond
	t.Cleanup(func() { DetachTimeout = old })
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("sleep 303 & echo JPID=$!; exit 0\n"))
	re := regexp.MustCompile(`JPID=(\d+)`)
	waitFor(t, "JPID", func() bool { return re.MatchString(s.output()) })
	jpid, _ := strconv.Atoi(re.FindStringSubmatch(s.output())[1])
	t.Cleanup(func() { _ = syscallKill(jpid) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Done chưa được gọi")
	}
	if !alive(jpid) {
		t.Fatalf("job nền %d phải còn sống sau khi shell tự thoát", jpid)
	}
	c.Detach()
	c.Close("client")
	time.Sleep(300 * time.Millisecond)
	if !alive(jpid) {
		t.Fatalf("Close/Detach sau khi shell đã thoát vẫn bắn tín hiệu: job %d đã chết", jpid)
	}
	if cd, _ := s.closeMsg(); cd.Reason != "exit" {
		t.Fatalf("reason = %q", cd.Reason)
	}
}

// Phiên đã sạch (không còn tiến trình nào phải giết) thì không phải chờ đủ KillGrace.
func TestPtyCloseFastWhenSessionClean(t *testing.T) {
	c, _, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	c.Close("client")
	select {
	case <-done:
	case <-time.After(KillGrace + 2*time.Second):
		t.Fatal("Done chưa được gọi")
	}
	if d := time.Since(start); d >= KillGrace/2 {
		t.Fatalf("Done sau %s — phiên đã sạch thì không phải chờ KillGrace %s", d, KillGrace)
	}
}

// Gắn lại một kênh đã kết thúc: không được gửi replay sau control close.
func TestPtyReattachAfterFinishSendsNothing(t *testing.T) {
	c, s, done := open(t, `{"cols":80,"rows":24,"shell":"/bin/bash"}`)
	c.HandleBinary([]byte("exit 0\n"))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Done chưa được gọi")
	}
	s.reset()
	c.Reattach(nil)
	if _, ok := s.findMsg("replay"); ok {
		t.Fatal("đã gửi replay sau close")
	}
	if out := s.output(); out != "" {
		t.Fatalf("đã gửi đầu ra sau close: %q", out)
	}
}

// Tiến trình fork trong lúc chờ KillGrace: con mới không có trong lần quét đầu.
// killPasses quét lại (tối đa 3 lượt) tới khi không còn pid nào giết được.
func TestKillPassesRescansForkedChildren(t *testing.T) {
	scans := [][]int{{10, 11}, {10, 12}, {}} // lượt 2: 12 vừa được 11 fork ra; 10 được tha
	var killed []int
	n := 0
	killPasses(func() []int { s := scans[n]; n++; return s }, func(pid int) bool { return pid == 10 },
		func(pid int) { killed = append(killed, pid) })
	if fmt.Sprint(killed) != "[11 12]" || n != 3 {
		t.Fatalf("killed=%v scans=%d", killed, n)
	}
	n, killed = 0, nil
	forever := func() []int { n++; return []int{99} }
	killPasses(forever, func(int) bool { return false }, func(pid int) { killed = append(killed, pid) })
	if n != 3 || len(killed) != 3 {
		t.Fatalf("tối đa 3 lượt: scans=%d killed=%v", n, killed)
	}
}
