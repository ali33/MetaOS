package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/ali33/MetaOS/internal/authx"
	"github.com/ali33/MetaOS/internal/protocol"
)

var (
	ErrAuthFailed      = errors.New("authentication failed")
	ErrPasswordExpired = errors.New("password expired")
	ErrLaunch          = errors.New("bridge launch failed")
)

var StopWait = 5 * time.Second

type Launcher interface {
	Launch(ctx context.Context, user string, password []byte) (BridgeConn, protocol.HelloData, error)
}

type AuthLauncher struct {
	Command      func(user string) *exec.Cmd
	HelloTimeout time.Duration
	Log          *log.Logger
}

func NewAuthLauncher(cfg Config, logw io.Writer) *AuthLauncher {
	return &AuthLauncher{
		Command:      func(user string) *exec.Cmd { return exec.Command(cfg.AuthPath, user) },
		HelloTimeout: cfg.HelloTimeout,
		Log:          log.New(logw, "metaos-ws: ", 0),
	}
}

type procBridge struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	wmu      sync.Mutex
	frames   chan protocol.Frame
	done     chan struct{}
	exitErr  error
	stopOnce sync.Once
	stopped  chan struct{} // đóng khi dừng: vòng đọc bỏ frame thay vì chờ người đọc Frames()
	sigOnce  sync.Once
	log      *log.Logger
}

// signalStop đóng stdin (bridge gặp EOF sẽ tự thoát) và báo vòng đọc bỏ các
// frame còn lại, để nó đọc cạn tới EOF rồi Wait và đóng done.
func (b *procBridge) signalStop() {
	b.sigOnce.Do(func() {
		close(b.stopped)
		b.stdin.Close()
	})
}

func (b *procBridge) Send(f protocol.Frame) error {
	b.wmu.Lock()
	defer b.wmu.Unlock()
	return protocol.WritePipeFrame(b.stdin, f)
}
func (b *procBridge) Frames() <-chan protocol.Frame { return b.frames }
func (b *procBridge) Done() <-chan struct{}         { return b.done }

// Stop: ws (uid metaos) không gửi được tín hiệu cho bridge (uid của user), nên
// chỉ có thể đóng stdin và chờ bridge tự thoát. KHÔNG lấy wmu: nếu một Send
// đang chặn (bridge ngừng đọc stdin) mà Stop chờ khoá thì đăng xuất treo.
// Đóng pipe đang được ghi dở là an toàn — Write đang chặn sẽ trả lỗi.
func (b *procBridge) Stop() {
	b.stopOnce.Do(func() {
		b.signalStop()
		select {
		case <-b.done:
		case <-time.After(StopWait):
			b.log.Printf("bridge pid %d chưa thoát sau %s kể từ khi đóng stdin", b.cmd.Process.Pid, StopWait)
		}
	})
}

func (l *AuthLauncher) Launch(ctx context.Context, user string, password []byte) (BridgeConn, protocol.HelloData, error) {
	var hello protocol.HelloData
	if len(password) > authx.MaxPassword {
		return nil, hello, ErrAuthFailed // như sai mật khẩu; không khởi chạy metaos-auth
	}
	cmd := l.Command(user)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, hello, fmt.Errorf("%w: %v", ErrLaunch, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, hello, fmt.Errorf("%w: %v", ErrLaunch, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, hello, fmt.Errorf("%w: %v", ErrLaunch, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, hello, fmt.Errorf("%w: %v", ErrLaunch, err)
	}
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			l.Log.Printf("auth[%s]: %s", user, sc.Text())
		}
	}()
	timer := time.NewTimer(l.HelloTimeout) // tính cả thời gian ghi mật khẩu
	defer timer.Stop()
	b := &procBridge{cmd: cmd, stdin: stdin, frames: make(chan protocol.Frame, 64), done: make(chan struct{}), stopped: make(chan struct{}), log: l.Log}
	first := make(chan protocol.Frame, 1)
	go func() {
		defer close(b.frames)
		gotFirst := false
		for {
			f, err := protocol.ReadPipeFrame(stdout)
			if err != nil {
				break
			}
			if !gotFirst {
				gotFirst = true
				first <- f
				continue
			}
			select {
			case b.frames <- f:
			case <-b.stopped: // đã dừng, không còn ai đọc: bỏ frame, tiếp tục đọc cạn
			}
		}
		<-stderrDone // Wait đóng các pipe; đọc hết stderr trước để không mất dòng log cuối
		b.exitErr = cmd.Wait()
		close(b.done)
	}()
	buf := append(append([]byte(nil), password...), '\n')
	_, werr := stdin.Write(buf)
	for i := range buf {
		buf[i] = 0
	}
	if werr != nil {
		l.Log.Printf("ghi mật khẩu cho %s: %v", user, werr)
	}
	fail := func(e error) (BridgeConn, protocol.HelloData, error) {
		b.signalStop()
		_ = cmd.Process.Kill() // còn là metaos-auth (ruid = metaos) nên kill được; đã là bridge thì EPERM, đóng stdin là đủ
		return nil, protocol.HelloData{}, e
	}
	select {
	case f := <-first:
		m, err := protocol.DecodeMessage(f.Data)
		if f.Kind != protocol.KindText || err != nil || m.Type != protocol.TypeHello {
			return fail(fmt.Errorf("%w: first frame is not hello", ErrLaunch))
		}
		if err := json.Unmarshal(m.Data, &hello); err != nil || hello.User != user {
			return fail(fmt.Errorf("%w: hello user %q != %q", ErrLaunch, hello.User, user))
		}
		return b, hello, nil
	case <-b.done:
		var ee *exec.ExitError
		if errors.As(b.exitErr, &ee) {
			switch ee.ExitCode() {
			case 1:
				return nil, hello, ErrAuthFailed
			case 4:
				return nil, hello, ErrPasswordExpired
			}
		}
		return nil, hello, fmt.Errorf("%w: auth exited: %v", ErrLaunch, b.exitErr)
	case <-timer.C:
		return fail(fmt.Errorf("%w: no hello within %s", ErrLaunch, l.HelloTimeout))
	case <-ctx.Done():
		return fail(fmt.Errorf("%w: %v", ErrLaunch, ctx.Err()))
	}
}
