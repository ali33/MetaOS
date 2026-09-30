// Package pty là kênh terminal: một shell đăng nhập trên PTY.
package pty

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	cpty "github.com/creack/pty"

	"github.com/ali33/MetaOS/internal/channels"
	"github.com/ali33/MetaOS/internal/protocol"
)

const (
	RingSize   = 256 << 10
	inputQueue = 64 // số khúc phím gõ được xếp hàng chờ ghi vào PTY
)

var (
	DetachTimeout = 60 * time.Second
	KillGrace     = 2 * time.Second
	ExitDrain     = 200 * time.Millisecond
	shellsFile    = "/etc/shells"
)

type Params struct {
	Cols     int     `json:"cols"`
	Rows     int     `json:"rows"`
	Shell    string  `json:"shell,omitempty"`
	Cwd      string  `json:"cwd,omitempty"`
	Reattach bool    `json:"reattach,omitempty"`
	Offset   *uint64 `json:"offset,omitempty"`
}

type ReplayData struct {
	Reset  bool   `json:"reset"`
	Offset uint64 `json:"offset"`
}

type resizeData struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

type channel struct {
	id   string
	out  channels.Sender
	done func()
	cmd  *exec.Cmd
	tty  *os.File
	in   chan []byte   // hàng đợi phím gõ; writer() rút ra
	quit chan struct{} // đóng khi kênh kết thúc

	dropping atomic.Bool // đang bỏ phím vì hàng đợi đầy (chỉ báo một lần mỗi đợt)

	mu          sync.Mutex
	ring        *Ring
	attached    bool
	detachTimer *time.Timer
	closeReason string
	closeOnce   sync.Once
	killed      chan struct{} // đóng sau khi killSession của Close đã chạy
}

func bad(format string, a ...any) error {
	return &channels.Error{Code: protocol.CodeInvalidParams, Err: fmt.Errorf(format, a...)}
}

func validSize(c, r int) bool { return c >= 1 && c <= 1000 && r >= 1 && r <= 1000 }

func allowedShell(path string) bool {
	f, err := os.Open(shellsFile)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == path {
			return true
		}
	}
	return false
}

func New(a channels.OpenArgs) (channels.Channel, error) {
	var p Params
	if err := json.Unmarshal(a.Params, &p); err != nil {
		return nil, bad("pty params: %v", err)
	}
	if !validSize(p.Cols, p.Rows) {
		return nil, bad("pty size out of range: %dx%d (1..1000)", p.Cols, p.Rows)
	}
	if p.Shell == "" {
		p.Shell = os.Getenv("SHELL")
	}
	if p.Shell == "" {
		p.Shell = "/bin/bash"
	}
	if !allowedShell(p.Shell) {
		return nil, bad("shell %s is not listed in %s", p.Shell, shellsFile)
	}
	if p.Cwd == "" {
		p.Cwd = os.Getenv("HOME")
	}
	st, err := os.Stat(p.Cwd)
	if err != nil {
		return nil, &channels.Error{Code: protocol.CodeFromErr(err), Err: err}
	}
	if !st.IsDir() {
		return nil, bad("cwd %s is not a directory", p.Cwd)
	}
	cmd := exec.Command(p.Shell, "-l")
	cmd.Dir = p.Cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	tty, err := cpty.StartWithSize(cmd, &cpty.Winsize{Cols: uint16(p.Cols), Rows: uint16(p.Rows)})
	if err != nil {
		return nil, &channels.Error{Code: protocol.CodeFromErr(err), Err: err}
	}
	c := &channel{id: a.ID, out: a.Out, done: a.Done, cmd: cmd, tty: tty, ring: NewRing(RingSize), attached: true,
		in: make(chan []byte, inputQueue), quit: make(chan struct{}), killed: make(chan struct{})}
	pumpDone := make(chan struct{})
	go c.writer()
	go func() { c.pump(); close(pumpDone) }()
	go c.finish(pumpDone)
	return c, nil
}

// writer ghi phím gõ vào PTY trên goroutine riêng: PTY đầy chỉ làm chặn
// goroutine này, không chặn router.
func (c *channel) writer() {
	for {
		select {
		case p := <-c.in:
			if _, err := c.tty.Write(p); err != nil {
				return
			}
			if len(c.in) == 0 {
				c.dropping.Store(false) // hàng đợi đã rút cạn: đợt tràn sau sẽ báo lại một lần
			}
		case <-c.quit:
			return
		}
	}
}

// pump chỉ chuyển đầu ra; việc kết thúc kênh do finish() làm.
func (c *channel) pump() {
	buf := make([]byte, 32<<10)
	for {
		n, err := c.tty.Read(buf)
		if n > 0 {
			c.mu.Lock()
			c.ring.Write(buf[:n])
			if c.attached {
				_ = c.out.SendBinary(c.id, buf[:n])
			}
			c.mu.Unlock()
		}
		if err != nil { // EIO khi mọi đầu slave đã đóng, hoặc master bị đóng
			return
		}
	}
}

// finish là nơi DUY NHẤT kết thúc kênh, và chỉ dựa vào việc shell thoát —
// không chờ pump. Shell thoát nhưng job nền (nohup) vẫn có thể giữ đầu slave
// nên Read trên master không bao giờ trả EIO; nếu Close() lại không gỡ được
// Read đang chặn (master ở chế độ blocking) thì chờ pump sẽ treo mãi. Vì vậy:
// đợi pump tối đa ExitDrain để lấy nốt đầu ra, đóng master, rồi gửi close.
func (c *channel) finish(pumpDone <-chan struct{}) {
	werr := c.cmd.Wait()
	select {
	case <-pumpDone:
	case <-time.After(ExitDrain):
	}
	close(c.quit)
	c.tty.Close()
	select { // Close gỡ được Read thì pump dừng ngay; không thì thôi, không chờ
	case <-pumpDone:
	case <-time.After(ExitDrain):
	}
	c.mu.Lock()
	c.attached = false // pump còn sót (nếu có) không được gửi sau close
	reason := c.closeReason
	if c.detachTimer != nil {
		c.detachTimer.Stop()
	}
	c.mu.Unlock()
	if reason != "" {
		// Bị đóng có lý do (Close): bước SIGKILL là một phần của việc đóng. Router
		// thoát ngay sau Done, nên phải chờ killSession chạy xong rồi mới báo.
		<-c.killed
	}
	cd := protocol.CloseData{Ch: c.id, Reason: reason}
	if reason == "" {
		cd.Reason = "exit"
		code := 0
		var ee *exec.ExitError
		if errors.As(werr, &ee) {
			code = ee.ExitCode()
		}
		cd.ExitCode = &code
	}
	_ = c.out.SendText(protocol.Control(protocol.TypeClose, cd))
	c.done()
}

func (c *channel) HandleText(m protocol.Message) {
	if m.Type != "resize" {
		c.sendErr(protocol.CodeUnsupported, fmt.Sprintf("pty: unknown message type %q", m.Type))
		return
	}
	var r resizeData
	if err := json.Unmarshal(m.Data, &r); err != nil || !validSize(r.Cols, r.Rows) {
		c.sendErr(protocol.CodeInvalidParams, fmt.Sprintf("pty: bad resize %s", m.Data))
		return
	}
	if err := cpty.Setsize(c.tty, &cpty.Winsize{Cols: uint16(r.Cols), Rows: uint16(r.Rows)}); err != nil {
		c.sendErr(protocol.CodeFromErr(err), err.Error())
	}
}

func (c *channel) HandleBinary(p []byte) {
	select {
	case c.in <- append([]byte(nil), p...):
	case <-c.quit:
	default:
		if !c.dropping.Swap(true) {
			c.sendErr(protocol.CodeInternal, "pty: input queue full (the program is not reading its input); keystrokes dropped")
		}
	}
}

func (c *channel) sendErr(code, msg string) {
	_ = c.out.SendText(protocol.Control(protocol.TypeError, protocol.ErrorData{Ch: c.id, Code: code, Message: msg}))
}

func (c *channel) Reattachable() bool { return true }

func (c *channel) Detach() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attached = false
	if c.detachTimer != nil {
		c.detachTimer.Stop()
	}
	c.detachTimer = time.AfterFunc(DetachTimeout, func() { c.Close("detach-timeout") })
}

// Reattach gửi `replay` rồi phần đầu ra client còn thiếu (theo params.offset),
// hoặc bản chụp kèm reset=true nếu offset đã trôi khỏi bộ đệm.
func (c *channel) Reattach(params json.RawMessage) {
	var p Params
	_ = json.Unmarshal(params, &p)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.detachTimer != nil {
		c.detachTimer.Stop()
		c.detachTimer = nil
	}
	data, start, reset := c.ring.From(p.Offset)
	rd, _ := json.Marshal(ReplayData{Reset: reset, Offset: start})
	_ = c.out.SendText(protocol.Message{Ch: c.id, Type: "replay", Data: rd})
	if len(data) > 0 {
		_ = c.out.SendBinary(c.id, data)
	}
	c.attached = true
}

// Close: SIGHUP cho cả session của shell; sau KillGrace thì SIGKILL những gì
// còn lại trừ tiến trình cố ý bỏ qua SIGHUP (Q1, Q3 — giống thoát SSH: nohup và
// tmux/setsid được sống). finish() chỉ gửi control close và gọi Done sau khi
// shell đã thoát VÀ killSession đã chạy — Done không đến trước bước SIGKILL.
func (c *channel) Close(reason string) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closeReason = reason
		c.mu.Unlock()
		sid := c.cmd.Process.Pid
		signalSession(sid, syscall.SIGHUP)
		time.AfterFunc(KillGrace, func() { killSession(sid); close(c.killed) })
	})
}
