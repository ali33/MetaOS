package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ali33/MetaOS/internal/protocol"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 30, 8, 0, 0, 0, time.Local)}
}
func (c *fakeClock) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

// fakeBridge: frame gửi xuống được ghi lại; frame nhị phân và text có ch khác
// rỗng được dội ngược lên như bridge thật trả lời.
type fakeBridge struct {
	mu       sync.Mutex
	sent     []protocol.Frame
	frames   chan protocol.Frame
	done     chan struct{}
	stopped  bool
	stopOnce sync.Once
}

func newFakeBridge() *fakeBridge {
	return &fakeBridge{frames: make(chan protocol.Frame, 64), done: make(chan struct{})}
}

// Send giữ khoá cả lúc đẩy vào frames để không bao giờ đẩy vào kênh đã đóng
// (bộ đệm 64 là đủ cho test).
func (b *fakeBridge) Send(f protocol.Frame) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return io.ErrClosedPipe
	}
	b.sent = append(b.sent, f)
	if f.Kind == protocol.KindBinary {
		b.frames <- f
	} else if m, err := protocol.DecodeMessage(f.Data); err == nil && m.Ch != "" {
		b.frames <- f
	}
	return nil
}
func (b *fakeBridge) Frames() <-chan protocol.Frame { return b.frames }
func (b *fakeBridge) Done() <-chan struct{}         { return b.done }
func (b *fakeBridge) Stop() {
	b.stopOnce.Do(func() {
		b.mu.Lock()
		b.stopped = true
		close(b.frames)
		b.mu.Unlock()
		close(b.done)
	})
}
func (b *fakeBridge) isStopped() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.stopped }

// controls trả các type điều khiển (ch rỗng) đã gửi xuống, theo thứ tự.
func (b *fakeBridge) controls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, f := range b.sent {
		if f.Kind != protocol.KindText {
			continue
		}
		if m, err := protocol.DecodeMessage(f.Data); err == nil && m.Ch == "" {
			out = append(out, m.Type)
		}
	}
	return out
}

type stubLauncher struct {
	mu     sync.Mutex
	calls  int
	err    error
	bridge *fakeBridge

	rootUID  bool   // trả UID 0 dù tên không phải root
	onLaunch func() // chạy giữa Launch (giả lập client ngắt kết nối)
}

func (l *stubLauncher) Launch(ctx context.Context, user string, pw []byte) (BridgeConn, protocol.HelloData, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.err != nil {
		return nil, protocol.HelloData{}, l.err
	}
	if string(pw) != "Mật khẩu 1" {
		return nil, protocol.HelloData{}, ErrAuthFailed
	}
	if l.onLaunch != nil {
		l.onLaunch()
	}
	l.bridge = newFakeBridge()
	uid := 1000
	if l.rootUID || user == "root" {
		uid = 0
	}
	return l.bridge, protocol.HelloData{User: user, UID: uid, Hostname: "srv1"}, nil
}

func (l *stubLauncher) lastBridge() *fakeBridge { l.mu.Lock(); defer l.mu.Unlock(); return l.bridge }
func (l *stubLauncher) callCount() int          { l.mu.Lock(); defer l.mu.Unlock(); return l.calls }

type testEnv struct {
	srv   *Server
	h     http.Handler
	clock *fakeClock
	store *Store
	l     *stubLauncher
}

func newTestServer(t *testing.T, mut ...func(*Config)) *testEnv {
	t.Helper()
	cfg := DefaultConfig()
	for _, m := range mut {
		m(&cfg)
	}
	clk := newFakeClock()
	st := NewStore(clk, cfg.SessionMax, cfg.SessionIdle)
	l := &stubLauncher{}
	static := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html>INDEX")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	srv := New(cfg, st, l, static, io.Discard)
	return &testEnv{srv: srv, h: srv.Handler(), clock: clk, store: st, l: l}
}

func (e *testEnv) do(method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://srv1:9443"+path, strings.NewReader(body))
	r.Host = "srv1:9443"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *testEnv) login(t *testing.T, user, pw string) (cookie, csrf string) {
	t.Helper()
	w := e.do("POST", "/api/login", `{"user":"`+user+`","password":"`+pw+`"}`, nil)
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == CookieName {
			cookie = c.Name + "=" + c.Value
		}
	}
	var body struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return cookie, body.CSRF
}
