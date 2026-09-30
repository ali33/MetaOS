package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoginSuccessSetsStrictCookie(t *testing.T) {
	e := newTestServer(t)
	w := e.do("POST", "/api/login", `{"user":"alice","password":"Mật khẩu 1"}`, map[string]string{"Origin": "https://srv1:9443"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var c *http.Cookie
	for _, x := range w.Result().Cookies() {
		if x.Name == CookieName {
			c = x
		}
	}
	if c == nil || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || len(c.Value) != 43 {
		t.Fatalf("cookie sai: %+v", c)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["user"] != "alice" || body["hostname"] != "srv1" || len(body["csrf"]) != 43 {
		t.Fatalf("body %v", body)
	}
	if e.store.Len() != 1 {
		t.Fatal("chưa tạo phiên")
	}
}

func TestLoginFailures(t *testing.T) {
	cases := []struct {
		name, body string
		hdr        map[string]string
		code       int
		errCode    string
		launches   int
	}{
		{"sai mật khẩu", `{"user":"alice","password":"sai"}`, nil, 401, "auth-failed", 1},
		{"root bị tắt", `{"user":"root","password":"Mật khẩu 1"}`, nil, 403, "root-disabled", 0},
		{"tên user lạ", `{"user":"-rf","password":"x"}`, nil, 401, "auth-failed", 0},
		{"mật khẩu có \n", `{"user":"alice","password":"a\nb"}`, nil, 401, "auth-failed", 0},
		{"mật khẩu có NUL", `{"user":"alice","password":"a\u0000b"}`, nil, 401, "auth-failed", 0},
		{"mật khẩu 513 byte", `{"user":"alice","password":"` + strings.Repeat("a", 513) + `"}`, nil, 401, "auth-failed", 0},
		{"mật khẩu rỗng", `{"user":"alice","password":""}`, nil, 401, "auth-failed", 0},
		{"khác origin", `{"user":"alice","password":"Mật khẩu 1"}`, map[string]string{"Origin": "https://evil.example"}, 403, "bad-origin", 0},
		{"JSON hỏng", `{`, nil, 400, "invalid-request", 0},
	}
	for _, c := range cases {
		e := newTestServer(t)
		w := e.do("POST", "/api/login", c.body, c.hdr)
		var body map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != c.code || body["error"] != c.errCode || e.l.callCount() != c.launches {
			t.Errorf("%s: %d %v launches=%d", c.name, w.Code, body, e.l.callCount())
		}
		if len(w.Result().Cookies()) != 0 {
			t.Errorf("%s: không được đặt cookie", c.name)
		}
	}
}

func TestLoginRequiresJSONContentType(t *testing.T) {
	e := newTestServer(t)
	r := e.do("POST", "/api/login", "", nil)
	if r.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("got %d", r.Code)
	}
}

func TestLoginErrorsFromLauncher(t *testing.T) {
	for err, want := range map[error]string{ErrPasswordExpired: "password-expired", errors.New("boom"): "internal"} {
		e := newTestServer(t)
		e.l.err = err
		w := e.do("POST", "/api/login", `{"user":"alice","password":"Mật khẩu 1"}`, nil)
		if !strings.Contains(w.Body.String(), `"`+want+`"`) || strings.Contains(w.Body.String(), "boom") {
			t.Errorf("%v: %d %s", err, w.Code, w.Body)
		}
	}
}

func TestLoginRootAllowedWithFlag(t *testing.T) {
	e := newTestServer(t, func(c *Config) { c.AllowRoot = true })
	if w := e.do("POST", "/api/login", `{"user":"root","password":"Mật khẩu 1"}`, nil); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
}

func TestSessionEndpoint(t *testing.T) {
	e := newTestServer(t)
	if w := e.do("GET", "/api/session", "", nil); w.Code != 401 {
		t.Fatalf("không cookie: %d", w.Code)
	}
	cookie, csrf := e.login(t, "alice", "Mật khẩu 1")
	w := e.do("GET", "/api/session", "", map[string]string{"Cookie": cookie})
	if w.Code != 200 || !strings.Contains(w.Body.String(), csrf) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	e.clock.Advance(31 * time.Minute)
	if w := e.do("GET", "/api/session", "", map[string]string{"Cookie": cookie}); w.Code != 401 {
		t.Fatalf("hết hạn mà vẫn %d", w.Code)
	}
}

func TestLogoutRequiresCSRF(t *testing.T) {
	e := newTestServer(t)
	cookie, csrf := e.login(t, "alice", "Mật khẩu 1")
	if w := e.do("POST", "/api/logout", "", map[string]string{"Cookie": cookie}); w.Code != 403 {
		t.Fatalf("thiếu CSRF: %d", w.Code)
	}
	if w := e.do("POST", "/api/logout", "", map[string]string{"Cookie": cookie, CSRFHeader: "sai"}); w.Code != 403 {
		t.Fatalf("CSRF sai: %d", w.Code)
	}
	w := e.do("POST", "/api/logout", "", map[string]string{"Cookie": cookie, CSRFHeader: csrf})
	if w.Code != 204 || !e.l.lastBridge().isStopped() || e.store.Len() != 0 {
		t.Fatalf("logout %d stopped=%v", w.Code, e.l.lastBridge().isStopped())
	}
	if c := w.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatalf("phải xoá cookie: %+v", c)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newTestServer(t)
	for _, p := range []string{"/", "/api/session"} {
		w := e.do("GET", p, "", nil)
		h := w.Header()
		if !strings.Contains(h.Get("Content-Security-Policy"), "script-src 'self'") ||
			h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "no-referrer" ||
			h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: header thiếu %v", p, h)
		}
	}
	if w := e.do("GET", "/api/session", "", nil); w.Header().Get("Cache-Control") != "no-store" {
		t.Error("/api phải no-store")
	}
}

func TestStaticAndSPAFallback(t *testing.T) {
	e := newTestServer(t)
	if w := e.do("GET", "/assets/app.js", "", nil); w.Code != 200 || w.Body.String() != "console.log(1)" {
		t.Fatalf("asset %d", w.Code)
	}
	if w := e.do("GET", "/apps/terminal", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "INDEX") {
		t.Fatalf("fallback %d %s", w.Code, w.Body)
	}
	if w := e.do("GET", "/assets/khong-co.js", "", nil); w.Code != 404 {
		t.Fatalf("file có đuôi mà thiếu phải 404: %d", w.Code)
	}
}
