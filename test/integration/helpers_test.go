//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ali33/MetaOS/internal/protocol"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	baseURL  = env("METAOS_IT_URL", "https://127.0.0.1:9443")
	tlsSkip  = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	insecure = &http.Client{Timeout: 30 * time.Second, Transport: tlsSkip}
	// coder/websocket từ chối HTTPClient có Timeout > 0 (hạn chót đi qua context).
	wsHTTP = &http.Client{Transport: tlsSkip}
)

// dexec chạy lệnh ngay trong container metaos-it (test chạy ở đó dưới root).
func dexec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

type sess struct{ cookie, csrf string }

func login(t *testing.T, user, pw string) (sess, int) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"user": user, "password": pw})
	req, _ := http.NewRequest("POST", baseURL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := insecure.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s sess
	for _, c := range resp.Cookies() {
		if c.Name == "metaos_session" {
			s.cookie = c.Name + "=" + c.Value
		}
	}
	var b struct {
		CSRF string `json:"csrf"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&b)
	s.csrf = b.CSRF
	return s, resp.StatusCode
}

func mustLogin(t *testing.T, user, pw string) sess {
	s, code := login(t, user, pw)
	if code != 200 {
		t.Fatalf("login %s: %d", user, code)
	}
	return s
}

func logout(t *testing.T, s sess) int {
	req, _ := http.NewRequest("POST", baseURL+"/api/logout", nil)
	req.Header.Set("Cookie", s.cookie)
	req.Header.Set("X-MetaOS-CSRF", s.csrf)
	resp, err := insecure.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

type term struct {
	t   *testing.T
	c   *websocket.Conn
	id  string
	out strings.Builder
}

func dialWS(t *testing.T, s sess) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h := http.Header{}
	h.Set("Cookie", s.cookie)
	h.Set("Origin", baseURL)
	c, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(baseURL, "https")+"/ws",
		&websocket.DialOptions{HTTPHeader: h, HTTPClient: wsHTTP})
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	c.SetReadLimit(protocol.MaxFrame)
	return c
}

// openPty mở (hoặc gắn lại) kênh pty và chờ ready.
func openPty(t *testing.T, c *websocket.Conn, id string, reattach bool) *term {
	t.Helper()
	params := `{"cols":120,"rows":30}`
	if reattach {
		params = `{"cols":120,"rows":30,"reattach":true}`
	}
	msg := fmt.Sprintf(`{"ch":"","type":"open","data":{"ch":%q,"kind":"pty","params":%s}}`, id, params)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatal(err)
	}
	tm := &term{t: t, c: c, id: id}
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("chờ ready: %v", err)
		}
		if typ == websocket.MessageBinary {
			_, p, _ := protocol.DecodeBinary(data)
			tm.out.Write(p)
			continue
		}
		m, _ := protocol.DecodeMessage(data)
		if m.Type == protocol.TypeReady {
			return tm
		}
		if m.Type == protocol.TypeError {
			t.Fatalf("mở pty lỗi: %s", m.Data)
		}
	}
}

func (tm *term) send(s string) {
	b, _ := protocol.EncodeBinary(tm.id, []byte(s))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tm.c.Write(ctx, websocket.MessageBinary, b); err != nil {
		tm.t.Fatal(err)
	}
}

// expect đọc tới khi đầu ra chứa want, tối đa 10 giây.
func (tm *term) expect(want string) string {
	tm.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for !strings.Contains(tm.out.String(), want) {
		typ, data, err := tm.c.Read(ctx)
		if err != nil {
			tm.t.Fatalf("chờ %q: %v\nđầu ra đến giờ:\n%s", want, err, tm.out.String())
		}
		if typ == websocket.MessageBinary {
			_, p, _ := protocol.DecodeBinary(data)
			tm.out.Write(p)
		}
	}
	return tm.out.String()
}

// waitProcs chờ tới khi danh sách tiến trình của user (pgrep -a) thoả ok.
func waitProcs(t *testing.T, user string, within time.Duration, what string, ok func(list string) bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	var out string
	for time.Now().Before(deadline) {
		out, _ = dexec(t, "pgrep", "-u", user, "-a")
		if ok(out) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s: sau %s tiến trình của %s là:\n%s", what, within, user, out)
}

// waitNoProcs chờ tới khi user không còn tiến trình nào trong container.
func waitNoProcs(t *testing.T, user string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	var out string
	for time.Now().Before(deadline) {
		var err error
		out, err = dexec(t, "pgrep", "-u", user, "-a")
		if err != nil { // pgrep thoát 1 khi không có tiến trình nào
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("user %s vẫn còn tiến trình sau %s:\n%s", user, within, out)
}

func waitReady(t *testing.T) {
	t.Helper()
	for i := 0; i < 60; i++ {
		if resp, err := insecure.Get(baseURL + "/api/session"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == 401 {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("metaos không sẵn sàng")
}
