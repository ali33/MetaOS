package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ali33/MetaOS/internal/protocol"
)

type liveEnv struct {
	*testEnv
	ts *httptest.Server
}

func newLive(t *testing.T) *liveEnv {
	e := newTestServer(t)
	ts := httptest.NewServer(e.h)
	t.Cleanup(ts.Close)
	return &liveEnv{e, ts}
}

func (l *liveEnv) loginLive(t *testing.T) (cookie, csrf string) {
	req, _ := http.NewRequest("POST", l.ts.URL+"/api/login", strings.NewReader(`{"user":"alice","password":"Mật khẩu 1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("login: %v %v", err, resp)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == CookieName {
			cookie = c.Name + "=" + c.Value
		}
	}
	// csrf lấy từ thân trả lời đăng nhập, không đọc l.store.m khi không giữ khoá (race).
	var body struct {
		CSRF string `json:"csrf"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.CSRF == "" {
		t.Fatalf("login body: %v %q", err, body.CSRF)
	}
	return cookie, body.CSRF
}

func (l *liveEnv) dial(t *testing.T, cookie, origin string) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	h := http.Header{}
	if cookie != "" {
		h.Set("Cookie", cookie)
	}
	h.Set("Origin", origin)
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(l.ts.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: h})
}

func readCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 150; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("hết giờ chờ: %s", what)
}

func TestWSRequiresSession(t *testing.T) {
	l := newLive(t)
	_, resp, err := l.dial(t, "", l.ts.URL)
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatalf("got %v %v", err, resp)
	}
}

func TestWSRejectsCrossOrigin(t *testing.T) {
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	_, resp, err := l.dial(t, cookie, "https://evil.example")
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("got %v %v", err, resp)
	}
}

func TestWSRelaysTextAndBinary(t *testing.T) {
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	c, _, err := l.dial(t, cookie, l.ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	ctx, cancel := readCtx()
	defer cancel()
	_ = c.Write(ctx, websocket.MessageBinary, []byte{1, 'a', 'x'})
	typ, data, err := c.Read(ctx)
	if err != nil || typ != websocket.MessageBinary || string(data) != "\x01ax" {
		t.Fatalf("binary: %v %v %q", err, typ, data)
	}
	_ = c.Write(ctx, websocket.MessageText, []byte(`{"ch":"a","type":"resize"}`))
	typ, data, err = c.Read(ctx)
	if err != nil || typ != websocket.MessageText || string(data) != `{"ch":"a","type":"resize"}` {
		t.Fatalf("text: %v %v %q", err, typ, data)
	}
}

func TestWSDetachAttach(t *testing.T) {
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	c, _, _ := l.dial(t, cookie, l.ts.URL)
	b := l.l.lastBridge()
	waitUntil(t, "attached", func() bool { return strings.Join(b.controls(), ",") == "attached" })
	c.Close(websocket.StatusNormalClosure, "")
	waitUntil(t, "detached", func() bool { return strings.Join(b.controls(), ",") == "attached,detached" })
	c2, _, err := l.dial(t, cookie, l.ts.URL)
	if err != nil {
		t.Fatalf("nối lại: %v", err)
	}
	defer c2.CloseNow()
	waitUntil(t, "attached lần 2", func() bool { return strings.Join(b.controls(), ",") == "attached,detached,attached" })
	if b.isStopped() {
		t.Fatal("rớt WebSocket không được giết bridge")
	}
}

func TestSecondSocketReplacesFirst(t *testing.T) { // Review Focus #4
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	c1, _, _ := l.dial(t, cookie, l.ts.URL)
	c2, _, err := l.dial(t, cookie, l.ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.CloseNow()
	ctx, cancel := readCtx()
	defer cancel()
	_, _, err = c1.Read(ctx)
	if websocket.CloseStatus(err) != CloseReplaced {
		t.Fatalf("c1 phải bị đóng 4001, nhận %v", err)
	}
	_ = c2.Write(ctx, websocket.MessageBinary, []byte{1, 'a', 'y'})
	if _, data, err := c2.Read(ctx); err != nil || string(data) != "\x01ay" {
		t.Fatalf("c2 phải chạy: %v %q", err, data)
	}
	b := l.l.lastBridge()
	waitUntil(t, "detached giữa hai attached", func() bool {
		return strings.Join(b.controls(), ",") == "attached,detached,attached"
	})
}

func TestLogoutClosesSocket(t *testing.T) {
	l := newLive(t)
	cookie, csrf := l.loginLive(t)
	c, _, _ := l.dial(t, cookie, l.ts.URL)
	req, _ := http.NewRequest("POST", l.ts.URL+"/api/logout", nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set(CSRFHeader, csrf)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 204 {
		t.Fatalf("logout %v %v", err, resp)
	}
	ctx, cancel := readCtx()
	defer cancel()
	_, _, err := c.Read(ctx)
	var ce websocket.CloseError
	if !errors.As(err, &ce) || ce.Code != CloseSessionEnded || ce.Reason != "logout" {
		t.Fatalf("muốn 4401 logout, nhận %v", err)
	}
}

func TestBridgeExitClosesSocket(t *testing.T) {
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	c, _, _ := l.dial(t, cookie, l.ts.URL)
	l.l.lastBridge().Stop()
	ctx, cancel := readCtx()
	defer cancel()
	if _, _, err := c.Read(ctx); websocket.CloseStatus(err) != CloseSessionEnded {
		t.Fatalf("got %v", err)
	}
}

func TestWSOversizeFrameKeepsSession(t *testing.T) {
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	c, _, _ := l.dial(t, cookie, l.ts.URL)
	ctx, cancel := readCtx()
	defer cancel()
	_ = c.Write(ctx, websocket.MessageText, make([]byte, protocol.MaxFrame+1))
	if _, _, err := c.Read(ctx); websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("muốn 1009, nhận %v", err)
	}
	if l.store.Len() != 1 {
		t.Fatal("frame quá cỡ không được làm mất phiên")
	}
}

func TestWSActivityTouchesSession(t *testing.T) {
	l := newLive(t)
	cookie, _ := l.loginLive(t)
	c, _, _ := l.dial(t, cookie, l.ts.URL)
	defer c.CloseNow()
	l.clock.Advance(20 * time.Minute)
	ctx, cancel := readCtx()
	defer cancel()
	_ = c.Write(ctx, websocket.MessageBinary, []byte{1, 'a', 'z'})
	_, _, _ = c.Read(ctx)
	l.clock.Advance(20 * time.Minute)
	req, _ := http.NewRequest("GET", l.ts.URL+"/api/session", nil)
	req.Header.Set("Cookie", cookie)
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 200 {
		t.Fatalf("40 phút sau đăng nhập, 20 phút sau lần gõ cuối: phải còn phiên, nhận %d", resp.StatusCode)
	}
}
