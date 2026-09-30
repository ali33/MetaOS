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
	go func() {
		chunk := make([]byte, protocol.MaxFrame)
		for b.Send(protocol.Frame{Kind: protocol.KindBinary, Data: chunk}) == nil {
		}
	}()
	time.Sleep(300 * time.Millisecond) // đủ để pipe đầy và Send chặn
	stopped := make(chan struct{})
	go func() { b.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop treo khi Send đang chặn")
	}
}

func TestLaunchWrongPasswordIsAuthFailed(t *testing.T) {
	_, _, err := fakeLauncher("ok").Launch(context.Background(), "alice", []byte("sai"))
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}
