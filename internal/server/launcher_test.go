package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ali33/MetaOS/internal/authx"
	"github.com/ali33/MetaOS/internal/protocol"
)

// TestHelperAuth không phải test thật: khi METAOS_FAKE_AUTH có giá trị, nó đóng
// vai metaos-auth + metaos-bridge theo kịch bản.
func TestHelperAuth(t *testing.T) {
	mode := os.Getenv("METAOS_FAKE_AUTH")
	if mode == "" {
		return
	}
	in := bufio.NewReader(os.Stdin)
	pw, _ := in.ReadString('\n')
	hello := func(user string) {
		b, _ := protocol.Control(protocol.TypeHello, protocol.HelloData{User: user, UID: 1001, Hostname: "srv1"}).Encode()
		_ = protocol.WritePipeFrame(os.Stdout, protocol.Frame{Kind: protocol.KindText, Data: b})
	}
	switch mode {
	case "ok":
		if pw != "Mật khẩu 1\n" {
			os.Exit(1)
		}
		hello(os.Getenv("METAOS_FAKE_USER"))
		for { // dội mọi frame lên tới khi stdin EOF
			f, err := protocol.ReadPipeFrame(in)
			if err != nil {
				os.Exit(0)
			}
			_ = protocol.WritePipeFrame(os.Stdout, f)
		}
	case "fail":
		fmt.Fprintln(os.Stderr, "authentication failed")
		os.Exit(1)
	case "expired":
		os.Exit(4)
	case "garbage":
		os.Stdout.WriteString("Welcome to Debian!\n")
		time.Sleep(time.Hour)
	case "hang":
		time.Sleep(time.Hour)
	case "wronguser":
		hello("mallory")
		time.Sleep(time.Hour)
	case "stuck": // chào xong rồi không bao giờ đọc stdin nữa
		hello(os.Getenv("METAOS_FAKE_USER"))
		time.Sleep(time.Hour)
	case "flood": // như bridge lúc đăng xuất: stdin EOF ⇒ xả một loạt frame rồi thoát
		hello(os.Getenv("METAOS_FAKE_USER"))
		_, _ = io.Copy(io.Discard, in)
		for i := 0; i < 200; i++ {
			_ = protocol.WritePipeFrame(os.Stdout, protocol.Frame{Kind: protocol.KindBinary, Data: []byte{1, 'a', byte(i)}})
		}
		os.Exit(0)
	case "exit2":
		os.Exit(2)
	case "exit3":
		os.Exit(3)
	}
	os.Exit(0)
}

func fakeLauncher(mode string) *AuthLauncher {
	l := NewAuthLauncher(DefaultConfig(), io.Discard)
	l.HelloTimeout = 2 * time.Second
	l.Command = func(user string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperAuth$")
		cmd.Env = append(os.Environ(), "METAOS_FAKE_AUTH="+mode, "METAOS_FAKE_USER="+user)
		return cmd
	}
	return l
}

func TestLaunchOK(t *testing.T) {
	b, hello, err := fakeLauncher("ok").Launch(context.Background(), "alice", []byte("Mật khẩu 1"))
	if err != nil {
		t.Fatal(err)
	}
	if hello.User != "alice" || hello.Hostname != "srv1" {
		t.Fatalf("hello %+v", hello)
	}
	_ = b.Send(protocol.Frame{Kind: protocol.KindBinary, Data: []byte{1, 'a', 'x'}})
	select {
	case f := <-b.Frames():
		if f.Kind != protocol.KindBinary || string(f.Data) != "\x01ax" {
			t.Fatalf("got %+v", f)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("không nhận frame dội lại")
	}
	b.Stop()
	select {
	case <-b.Done():
	case <-time.After(StopWait + time.Second):
		t.Fatal("Stop không làm tiến trình thoát")
	}
	if _, open := <-b.Frames(); open {
		t.Fatal("Frames phải đóng sau khi bridge thoát")
	}
}

func TestLaunchErrors(t *testing.T) {
	cases := map[string]error{
		"fail":      ErrAuthFailed,
		"expired":   ErrPasswordExpired,
		"garbage":   ErrLaunch, // Review Focus #3: rác trên stdout không được làm treo
		"hang":      ErrLaunch,
		"wronguser": ErrLaunch,
		"exit2":     ErrLaunch, // người gọi / đối số không hợp lệ
		"exit3":     ErrLaunch, // lỗi nội bộ của metaos-auth
	}
	for mode, want := range cases {
		start := time.Now()
		_, _, err := fakeLauncher(mode).Launch(context.Background(), "alice", []byte("Mật khẩu 1"))
		if !errors.Is(err, want) {
			t.Errorf("%s: got %v want %v", mode, err, want)
		}
		if time.Since(start) > 4*time.Second {
			t.Errorf("%s: mất %s, quá lâu", mode, time.Since(start))
		}
	}
}

// Review Focus #5: bridge ngừng đọc stdin thì Send chặn, nhưng Stop (đăng xuất)
// vẫn phải trả về trong StopWait chứ không chờ khoá ghi.
func TestStopDoesNotWaitForBlockedSend(t *testing.T) {
	old := StopWait
	StopWait = 300 * time.Millisecond
	t.Cleanup(func() { StopWait = old })
	b, _, err := fakeLauncher("stuck").Launch(context.Background(), "alice", []byte("Mật khẩu 1"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.(*procBridge).cmd.Process.Kill() })
	sendErr := make(chan error, 1)
	go func() {
		chunk := make([]byte, protocol.MaxFrame)
		var err error
		for err == nil {
			err = b.Send(protocol.Frame{Kind: protocol.KindBinary, Data: chunk})
		}
		sendErr <- err
	}()
	time.Sleep(300 * time.Millisecond) // đủ để pipe đầy và Send chặn
	select {
	case err := <-sendErr:
		t.Fatalf("Send phải đang chặn, nhưng đã trả %v", err)
	default:
	}
	stopped := make(chan struct{})
	go func() { b.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop treo khi Send đang chặn")
	}
	select {
	case err := <-sendErr:
		if err == nil {
			t.Fatal("Send đang chặn phải trả lỗi sau Stop")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Send vẫn chặn sau Stop")
	}
}

// Lúc đăng xuất không còn ai đọc Frames(), trong khi bridge xả frame đóng kênh
// và đầu ra của shell bị SIGHUP. Vòng đọc phải đọc cạn tới EOF để Wait và đóng
// Done — nếu không tiến trình thành zombie và phiên không bao giờ kết thúc.
func TestStopDrainsUnreadFrames(t *testing.T) {
	b, _, err := fakeLauncher("flood").Launch(context.Background(), "alice", []byte("Mật khẩu 1"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.(*procBridge).cmd.Process.Kill() })
	b.Stop() // không đọc Frames()
	select {
	case <-b.Done():
	case <-time.After(StopWait):
		t.Fatal("Done không đóng khi còn frame chưa ai đọc")
	}
}

func TestLaunchContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := fakeLauncher("hang").Launch(ctx, "alice", []byte("Mật khẩu 1"))
	if !errors.Is(err, ErrLaunch) {
		t.Fatalf("got %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("huỷ ctx mà mất %s", d)
	}
}

// Launch tự phòng thủ: mật khẩu quá dài bị từ chối như sai mật khẩu mà không
// khởi chạy metaos-auth.
func TestLaunchRejectsLongPassword(t *testing.T) {
	l := fakeLauncher("ok")
	spawned := 0
	cmdf := l.Command
	l.Command = func(user string) *exec.Cmd { spawned++; return cmdf(user) }
	_, _, err := l.Launch(context.Background(), "alice", make([]byte, authx.MaxPassword+1))
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
	if spawned != 0 {
		t.Fatalf("metaos-auth bị gọi %d lần", spawned)
	}
}

func TestLaunchWrongPasswordIsAuthFailed(t *testing.T) {
	_, _, err := fakeLauncher("ok").Launch(context.Background(), "alice", []byte("sai"))
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}
