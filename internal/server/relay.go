package server

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/ali33/MetaOS/internal/protocol"
)

const (
	CloseReplaced     websocket.StatusCode = 4001
	CloseSessionEnded websocket.StatusCode = 4401
	pingEvery                              = 30 * time.Second
	writeTimeout                           = 10 * time.Second
)

func (s *Server) current(id string) *websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns[id]
}

// startSession đăng ký móc đóng WebSocket một cách đồng bộ (trước khi trình
// duyệt có cookie để mở WebSocket), rồi mới chạy pump — tránh chạy đua với End.
func (s *Server) startSession(sess *Session) {
	sess.OnEnd(func(reason string) {
		s.mu.Lock()
		c := s.conns[sess.ID]
		delete(s.conns, sess.ID)
		s.mu.Unlock()
		if c != nil {
			go c.Close(CloseSessionEnded, reason)
		}
	})
	go s.pump(sess)
}

// pump chuyển frame bridge → WebSocket hiện tại. Không có WebSocket thì bỏ
// frame chứ không chặn: chặn thì bridge treo khi ghi stdout.
func (s *Server) pump(sess *Session) {
	for f := range sess.Bridge.Frames() {
		c := s.current(sess.ID)
		if c == nil {
			continue
		}
		typ := websocket.MessageText
		if f.Kind == protocol.KindBinary {
			typ = websocket.MessageBinary
		}
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		if err := c.Write(ctx, typ, f.Data); err != nil {
			c.CloseNow() // vòng đọc của ws() sẽ thấy và gửi detached
		}
		cancel()
	}
}

func control(typ string) protocol.Frame {
	b, _ := protocol.Message{Type: typ}.Encode()
	return protocol.Frame{Kind: protocol.KindText, Data: b}
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r)
	if !ok {
		apiErr(w, http.StatusUnauthorized, "no-session")
		return
	}
	if r.Header.Get("Origin") == "" || !sameOrigin(r) {
		apiErr(w, http.StatusForbidden, "bad-origin")
		return
	}
	c, err := websocket.Accept(w, r, nil) // nil: thư viện cũng tự kiểm Origin == Host
	if err != nil {
		s.log.Printf("ws accept user=%s: %v", sess.User, err)
		return
	}
	c.SetReadLimit(protocol.MaxFrame)
	s.mu.Lock()
	old := s.conns[sess.ID]
	s.conns[sess.ID] = c
	s.mu.Unlock()
	// Phiên có thể kết thúc giữa s.session(r) và lúc đăng ký c: móc OnEnd đã
	// chạy rồi nên không ai đóng c nữa — tự đóng 4401.
	if ended, reason := sess.endState(); ended {
		s.mu.Lock()
		if s.conns[sess.ID] == c {
			delete(s.conns, sess.ID)
		}
		s.mu.Unlock()
		if old != nil {
			go old.Close(CloseSessionEnded, reason)
		}
		c.Close(CloseSessionEnded, reason)
		return
	}
	if old != nil {
		go old.Close(CloseReplaced, "replaced")
		// Kênh của WebSocket cũ phải vào trạng thái chờ gắn lại (hẹn giờ 60 giây);
		// không gửi detached thì shell của tab cũ sống mãi tới khi đăng xuất.
		_ = sess.Bridge.Send(control(protocol.TypeDetached))
	}
	_ = sess.Bridge.Send(control(protocol.TypeAttached))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
				err := c.Ping(pctx)
				pcancel()
				if err != nil {
					c.CloseNow()
					return
				}
			}
		}
	}()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		s.store.Touch(sess)
		kind := protocol.KindText
		if typ == websocket.MessageBinary {
			kind = protocol.KindBinary
		}
		if err := sess.Bridge.Send(protocol.Frame{Kind: kind, Data: data}); err != nil {
			break
		}
	}
	s.mu.Lock()
	isCurrent := s.conns[sess.ID] == c
	if isCurrent {
		delete(s.conns, sess.ID)
	}
	s.mu.Unlock()
	if isCurrent {
		_ = sess.Bridge.Send(control(protocol.TypeDetached))
	}
	c.CloseNow()
}
