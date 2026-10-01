package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali33/MetaOS/internal/protocol"
)

func TestBridgeBinaryHelloPtyAndEOF(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "metaos-bridge")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin)
	cmd.Env = []string{"HOME=/tmp", "SHELL=/bin/bash", "PATH=/usr/bin:/bin"}
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	read := func() protocol.Frame {
		f, err := protocol.ReadPipeFrame(stdout)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return f
	}
	hello, _ := protocol.DecodeMessage(read().Data)
	var hd protocol.HelloData
	_ = json.Unmarshal(hello.Data, &hd)
	if hello.Type != protocol.TypeHello || hd.User == "" {
		t.Fatalf("frame đầu phải là hello: %+v", hello)
	}
	open := `{"ch":"","type":"open","data":{"ch":"t1","kind":"pty","params":{"cols":80,"rows":24}}}`
	_ = protocol.WritePipeFrame(stdin, protocol.Frame{Kind: protocol.KindText, Data: []byte(open)})
	if m, _ := protocol.DecodeMessage(read().Data); m.Type != protocol.TypeReady {
		t.Fatalf("muốn ready, nhận %s %s", m.Type, m.Data)
	}
	b, _ := protocol.EncodeBinary("t1", []byte("echo BRIDGE-$((40+2))\n"))
	_ = protocol.WritePipeFrame(stdin, protocol.Frame{Kind: protocol.KindBinary, Data: b})
	var got strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(got.String(), "BRIDGE-42") && time.Now().Before(deadline) {
		if f := read(); f.Kind == protocol.KindBinary {
			_, p, _ := protocol.DecodeBinary(f.Data)
			got.Write(p)
		}
	}
	if !strings.Contains(got.String(), "BRIDGE-42") {
		t.Fatalf("không thấy đầu ra: %q", got.String())
	}
	// Lệnh con của shell không được thừa hưởng tín hiệu bị bỏ qua từ bridge.
	b, _ = protocol.EncodeBinary("t1", []byte("grep SigIgn /proc/self/status | sed s/SigIgn/IGN-MASK/\n"))
	_ = protocol.WritePipeFrame(stdin, protocol.Frame{Kind: protocol.KindBinary, Data: b})
	deadline = time.Now().Add(5 * time.Second)
	for !strings.Contains(got.String(), "IGN-MASK:") && time.Now().Before(deadline) {
		if f := read(); f.Kind == protocol.KindBinary {
			_, p, _ := protocol.DecodeBinary(f.Data)
			got.Write(p)
		}
	}
	if !strings.Contains(got.String(), "IGN-MASK:\t0000000000000000") {
		t.Fatalf("tiến trình con bị bỏ qua tín hiệu: %q", got.String())
	}
	stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	go func() {
		for {
			if _, err := protocol.ReadPipeFrame(stdout); err != nil {
				return
			}
		}
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("bridge thoát lỗi: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bridge không thoát sau khi stdin đóng")
	}
}
