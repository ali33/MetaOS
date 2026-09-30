package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"

	"github.com/ali33/MetaOS/internal/protocol"
)

type BridgeConn interface {
	Send(f protocol.Frame) error
	Frames() <-chan protocol.Frame
	Done() <-chan struct{}
	Stop()
}

type Clock interface{ Now() time.Time }

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type Session struct {
	ID, CSRF, User, Hostname string
	Bridge                   BridgeConn

	mu         sync.Mutex
	created    time.Time
	lastActive time.Time
	onEnd      []func(string)
	ended      bool
	endReason  string
}

func (s *Session) CheckCSRF(token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.CSRF)) == 1
}

func (s *Session) OnEnd(f func(reason string)) {
	s.mu.Lock()
	if s.ended {
		reason := s.endReason
		s.mu.Unlock()
		f(reason)
		return
	}
	s.onEnd = append(s.onEnd, f)
	s.mu.Unlock()
}

type Store struct {
	clock     Clock
	max, idle time.Duration
	mu        sync.Mutex
	m         map[string]*Session
}

func NewStore(c Clock, max, idle time.Duration) *Store {
	return &Store{clock: c, max: max, idle: idle, m: map[string]*Session{}}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (st *Store) Create(user, hostname string, b BridgeConn) (*Session, error) {
	id, err := randomToken()
	if err != nil {
		return nil, err
	}
	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := st.clock.Now()
	s := &Session{ID: id, CSRF: csrf, User: user, Hostname: hostname, Bridge: b, created: now, lastActive: now}
	st.mu.Lock()
	st.m[id] = s
	st.mu.Unlock()
	go func() {
		<-b.Done()
		st.End(id, "bridge-exit")
	}()
	return s, nil
}

func (st *Store) expired(s *Session, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Sub(s.created) >= st.max || now.Sub(s.lastActive) >= st.idle
}

func (st *Store) Get(id string) (*Session, bool) {
	st.mu.Lock()
	s := st.m[id]
	st.mu.Unlock()
	if s == nil {
		return nil, false
	}
	if st.expired(s, st.clock.Now()) {
		st.End(id, "expired")
		return nil, false
	}
	return s, true
}

func (st *Store) Touch(s *Session) {
	s.mu.Lock()
	s.lastActive = st.clock.Now()
	s.mu.Unlock()
}

func (st *Store) End(id, reason string) {
	st.mu.Lock()
	s := st.m[id]
	delete(st.m, id)
	st.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	hooks := s.onEnd
	s.onEnd = nil
	s.ended, s.endReason = true, reason
	s.mu.Unlock()
	for _, f := range hooks {
		f(reason)
	}
	s.Bridge.Stop()
}

func (st *Store) Reap() {
	now := st.clock.Now()
	st.mu.Lock()
	var dead []string
	for id, s := range st.m {
		if st.expired(s, now) {
			dead = append(dead, id)
		}
	}
	st.mu.Unlock()
	for _, id := range dead {
		st.End(id, "expired")
	}
}

func (st *Store) Len() int { st.mu.Lock(); defer st.mu.Unlock(); return len(st.m) }

func (st *Store) EndAll(reason string) {
	st.mu.Lock()
	ids := make([]string, 0, len(st.m))
	for id := range st.m {
		ids = append(ids, id)
	}
	st.mu.Unlock()
	for _, id := range ids {
		st.End(id, reason)
	}
}
