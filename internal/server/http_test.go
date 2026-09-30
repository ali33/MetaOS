package server

import (
	"context"
	"net/http/httptest"
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
		{"mật khẩu 10 KB", `{"user":"alice","password":"` + strings.Repeat("a", 10*1024) + `"}`, nil, 400, "invalid-request", 0},
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
	if r.Code != http.StatusUnsupportedMediaType || e.l.callCount() != 0 {
		t.Fatalf("got %d launches=%d", r.Code, e.l.callCount())
	}
}

func TestLoginErrorsFromLauncher(t *testing.T) {
	type want struct {
		code int
		e    string
	}
	for err, want := range map[error]want{ErrPasswordExpired: {401, "password-expired"}, errors.New("boom"): {500, "internal"}} {
		e := newTestServer(t)
		e.l.err = err
		w := e.do("POST", "/api/login", `{"user":"alice","password":"Mật khẩu 1"}`, nil)
		if w.Code != want.code || !strings.Contains(w.Body.String(), `"`+want.e+`"`) || strings.Contains(w.Body.String(), "boom") {
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

func TestLoginRootAliasUID0Rejected(t *testing.T) {
	e := newTestServer(t)
	e.l.rootUID = true
	w := e.do("POST", "/api/login", `{"user":"toor","password":"Mật khẩu 1"}`, nil)
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"root-disabled"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !e.l.lastBridge().isStopped() || e.store.Len() != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatal("bridge uid 0 phải bị dừng, không tạo phiên")
	}
	e = newTestServer(t, func(c *Config) { c.AllowRoot = true })
	e.l.rootUID = true
	if w := e.do("POST", "/api/login", `{"user":"toor","password":"Mật khẩu 1"}`, nil); w.Code != 200 {
		t.Fatalf("có cờ thì phải vào được: %d", w.Code)
	}
}

func TestUnknownAPIPathsAre404(t *testing.T) {
	e := newTestServer(t)
	for _, p := range []string{"/api/foo", "/api/login/", "/api/"} {
		w := e.do("GET", p, "", nil)
		if w.Code != 404 || !strings.Contains(w.Body.String(), `"not-found"`) || strings.Contains(w.Body.String(), "INDEX") {
			t.Errorf("%s: %d %s", p, w.Code, w.Body)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	e := newTestServer(t)
	for _, c := range [][2]string{{"GET", "/api/login"}, {"GET", "/api/logout"}, {"POST", "/api/session"}, {"DELETE", "/api/session"}} {
		if w := e.do(c[0], c[1], "", nil); w.Code != 405 {
			t.Errorf("%s %s: %d", c[0], c[1], w.Code)
		}
	}
}

func TestStaticNoDirectoryListing(t *testing.T) {
	e := newTestServer(t)
	for _, p := range []string{"/assets", "/assets/"} {
		if w := e.do("GET", p, "", nil); w.Code != 404 || strings.Contains(w.Body.String(), "app.js") {
			t.Errorf("%s: %d %s", p, w.Code, w.Body)
		}
	}
}

func TestLoginEndsSessionOfIncomingCookie(t *testing.T) {
	e := newTestServer(t)
	cookie, _ := e.login(t, "alice", "Mật khẩu 1")
	first := e.l.lastBridge()
	w := e.do("POST", "/api/login", `{"user":"alice","password":"Mật khẩu 1"}`, map[string]string{"Cookie": cookie})
	if w.Code != 200 || !first.isStopped() || e.store.Len() != 1 {
		t.Fatalf("%d stopped=%v len=%d", w.Code, first.isStopped(), e.store.Len())
	}
	// đăng nhập hỏng thì không đụng tới phiên đang có
	cookie2 := ""
	for _, c := range w.Result().Cookies() {
		cookie2 = c.Name + "=" + c.Value
	}
	e.do("POST", "/api/login", `{"user":"alice","password":"sai"}`, map[string]string{"Cookie": cookie2})
	if e.store.Len() != 1 || e.l.lastBridge().isStopped() {
		t.Fatal("đăng nhập hỏng không được kết thúc phiên cũ")
	}
}

func TestLoginClientGoneEndsNewSession(t *testing.T) {
	e := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	e.l.onLaunch = cancel
	r := httptest.NewRequest("POST", "https://srv1:9443/api/login", strings.NewReader(`{"user":"alice","password":"Mật khẩu 1"}`)).WithContext(ctx)
	r.Host = "srv1:9443"
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(httptest.NewRecorder(), r)
	if e.store.Len() != 0 || !e.l.lastBridge().isStopped() {
		t.Fatalf("phiên mồ côi: len=%d stopped=%v", e.store.Len(), e.l.lastBridge().isStopped())
	}
}

func TestLogoutWithMatchingOrigin(t *testing.T) {
	e := newTestServer(t)
	cookie, csrf := e.login(t, "alice", "Mật khẩu 1")
	w := e.do("POST", "/api/logout", "", map[string]string{"Cookie": cookie, CSRFHeader: csrf, "Origin": "https://srv1:9443"})
	if w.Code != 204 {
		t.Fatalf("%d", w.Code)
	}
}
