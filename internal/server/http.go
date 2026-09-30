package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/ali33/MetaOS/internal/authx"
)

const (
	CookieName = "metaos_session"
	CSRFHeader = "X-MetaOS-CSRF"
	csp        = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
		"font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
)

type Server struct {
	cfg      Config
	store    *Store
	launcher Launcher
	static   fs.FS
	log      *log.Logger

	mu    sync.Mutex
	conns map[string]*websocket.Conn // Task 10: WebSocket hiện tại của mỗi phiên
}

func New(cfg Config, store *Store, l Launcher, static fs.FS, logw io.Writer) *Server {
	return &Server{cfg: cfg, store: store, launcher: l, static: static,
		log: log.New(logw, "metaos-ws: ", 0), conns: map[string]*websocket.Conn{}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { apiErr(w, http.StatusNotFound, "not-found") })
	mux.HandleFunc("/api/login", s.login)
	mux.HandleFunc("/api/session", s.sessionInfo)
	mux.HandleFunc("/api/logout", s.logout)
	mux.HandleFunc("/ws", s.ws)
	mux.Handle("/", s.staticHandler())
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, code int, e string) { writeJSON(w, code, map[string]string{"error": e}) }

// sameOrigin: không có Origin (client không phải trình duyệt) thì cho qua;
// có thì host phải trùng Host của request.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) session(r *http.Request) (*Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, false
	}
	return s.store.Get(c.Value)
}

type sessionBody struct {
	User     string `json:"user"`
	CSRF     string `json:"csrf"`
	Hostname string `json:"hostname"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErr(w, http.StatusMethodNotAllowed, "invalid-request")
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		apiErr(w, http.StatusUnsupportedMediaType, "invalid-request")
		return
	}
	if !sameOrigin(r) {
		apiErr(w, http.StatusForbidden, "bad-origin")
		return
	}
	var req struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid-request")
		return
	}
	ip := clientIP(r)
	failed := func(code int, e, reason string) {
		s.log.Printf("login failed user=%q ip=%s reason=%q", req.User, ip, reason)
		apiErr(w, code, e)
	}
	if _, err := authx.ParseArgs([]string{req.User}); err != nil {
		failed(http.StatusUnauthorized, "auth-failed", "bad-username")
		return
	}
	pw := req.Password
	if pw == "" || len(pw) > authx.MaxPassword || strings.ContainsAny(pw, "\n\x00") {
		failed(http.StatusUnauthorized, "auth-failed", "bad-password-format")
		return
	}
	if req.User == "root" && !s.cfg.AllowRoot {
		failed(http.StatusForbidden, "root-disabled", "root-disabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.HelloTimeout+5*time.Second)
	defer cancel()
	b, hello, err := s.launcher.Launch(ctx, req.User, []byte(pw))
	switch {
	case errors.Is(err, ErrAuthFailed):
		failed(http.StatusUnauthorized, "auth-failed", "pam")
		return
	case errors.Is(err, ErrPasswordExpired):
		failed(http.StatusUnauthorized, "password-expired", "password-expired")
		return
	case err != nil:
		failed(http.StatusInternalServerError, "internal", err.Error())
		return
	}
	// Chính sách root theo uid thật, không chỉ theo tên: "toor" cũng là uid 0.
	if hello.UID == 0 && !s.cfg.AllowRoot {
		b.Stop()
		failed(http.StatusForbidden, "root-disabled", "root-disabled")
		return
	}
	if c, err := r.Cookie(CookieName); err == nil {
		s.store.End(c.Value, "replaced") // không để bridge của phiên cũ mồ côi
	}
	sess, err := s.store.Create(req.User, hello.Hostname, b)
	if err != nil {
		b.Stop()
		failed(http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if r.Context().Err() != nil { // client đã ngắt trong lúc chờ Launch
		s.store.End(sess.ID, "client-gone")
		return
	}
	s.log.Printf("login ok user=%s ip=%s", req.User, ip)
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: sess.ID, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, sessionBody{sess.User, sess.CSRF, sess.Hostname})
}

func (s *Server) sessionInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "invalid-request")
		return
	}
	sess, ok := s.session(r)
	if !ok {
		apiErr(w, http.StatusUnauthorized, "no-session")
		return
	}
	s.store.Touch(sess)
	writeJSON(w, http.StatusOK, sessionBody{sess.User, sess.CSRF, sess.Hostname})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErr(w, http.StatusMethodNotAllowed, "invalid-request")
		return
	}
	sess, ok := s.session(r)
	if !ok {
		apiErr(w, http.StatusUnauthorized, "no-session")
		return
	}
	if !sess.CheckCSRF(r.Header.Get(CSRFHeader)) {
		apiErr(w, http.StatusForbidden, "csrf")
		return
	}
	s.store.End(sess.ID, "logout")
	s.log.Printf("logout user=%s ip=%s", sess.User, clientIP(r))
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			fi, err := fs.Stat(s.static, p)
			switch {
			case err == nil && fi.IsDir():
				http.NotFound(w, r) // không liệt kê thư mục
				return
			case err != nil && !strings.Contains(path.Base(p), "."):
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) RunReaper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.store.Reap()
		}
	}
}

// ws được thay bằng bản thật ở Task 10 (relay.go).
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	apiErr(w, http.StatusNotImplemented, "unsupported")
}
