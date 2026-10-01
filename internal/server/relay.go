package server

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/ali33/MetaOS/internal/protocol"
)

const (
	CloseReplaced         websocket.StatusCode = 4001
	CloseSessionEnded     websocket.StatusCode = 4401
	defaultPingEvery                           = 30 * time.Second
	defaultHeartbeatEvery                      = 15 * time.Second
	writeTimeout                               = 5 * time.Second
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
			s.closing.Add(1)
			go func() {
				defer s.closing.Done()
				c.Close(CloseSessionEnded, reason)
			}()
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

// wsOnlyControl: frame text ch rỗng có type chỉ dành cho ws↔bridge.
func wsOnlyControl(data []byte) (string, bool) {
	m, err := protocol.DecodeMessage(data)
	if err != nil || m.Ch != "" {
		return "", false
	}
	switch m.Type {
	case protocol.TypeDetached, protocol.TypeAttached, protocol.TypeHello:
		return m.Type, true
	}
	return "", false
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
	sess.attachMu.Lock()
	s.mu.Lock()
	old := s.conns[sess.ID]
	s.conns[sess.ID] = c
	s.mu.Unlock()
	// Phiên có thể kết thúc giữa s.session(r) và lúc đăng ký c. Tuyến tính hoá:
	// End đặt ended=true (dưới sess.mu) TRƯỚC khi chạy móc OnEnd, còn móc lấy c
	// khỏi s.conns dưới s.mu. Ta đăng ký c dưới s.mu rồi mới đọc ended, nên chỉ
	// có hai thứ tự: (a) móc chạy sau khi c đã đăng ký ⇒ móc đóng c; (b) móc đã
	// chạy trước ⇒ ta chắc chắn thấy ended=true ở đây và tự đóng c (và cái cũ ta
	// vừa thay chỗ, vì móc không còn thấy nó). Hai bên cùng đóng thì vô hại.
	// Không có test tất định cho khe này (cần chèn móc vào giữa hàm).
	if ended, reason := sess.endState(); ended {
		s.mu.Lock()
		if s.conns[sess.ID] == c {
			delete(s.conns, sess.ID)
		}
		s.mu.Unlock()
		sess.attachMu.Unlock()
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
	sess.attachMu.Unlock()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		t := time.NewTicker(s.pingEvery)
		defer t.Stop()
		hb := time.NewTicker(s.heartbeatEvery)
		defer hb.Stop()
		heartbeat := control(protocol.TypePing).Data
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
				if s.onPing != nil {
					s.onPing()
				}
			case <-hb.C:
				// Nhịp tim cho JS (PH-001). c.Write tự khoá nên an toàn song song
				// với pump; cùng hạn ghi và cùng cách xử lý lỗi như pump.
				wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
				err := c.Write(wctx, websocket.MessageText, heartbeat)
				wcancel()
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
		if !s.store.TouchLive(sess) {
			// Frame tới sau hạn không hoạt động không được hồi sinh phiên. End
			// chạy móc OnEnd: gỡ c và đóng 4401 "expired" — không CloseNow ở đây
			// kẻo cắt ngang frame đóng.
			s.store.End(sess.ID, "expired")
			return
		}
		kind := protocol.KindText
		if typ == websocket.MessageBinary {
			kind = protocol.KindBinary
		} else if t, ok := wsOnlyControl(data); ok {
			// Chỉ ws được gửi các điều khiển này xuống bridge; trình duyệt gửi là lỗi client.
			s.log.Printf("ws user=%s: bỏ frame điều khiển %q từ trình duyệt", sess.User, t)
			continue
		}
		if err := sess.Bridge.Send(protocol.Frame{Kind: kind, Data: data}); err != nil {
			break
		}
	}
	sess.attachMu.Lock()
	s.mu.Lock()
	isCurrent := s.conns[sess.ID] == c
	if isCurrent {
		delete(s.conns, sess.ID)
	}
	s.mu.Unlock()
	if isCurrent {
		_ = sess.Bridge.Send(control(protocol.TypeDetached))
	}
	sess.attachMu.Unlock()
	c.CloseNow()
}

// Shutdown kết thúc mọi phiên (WebSocket nhận 4401 "shutdown") và chờ các
// lần đóng WebSocket xong, tối đa tới hạn của ctx.
func (s *Server) Shutdown(ctx context.Context) {
	s.store.EndAll("shutdown")
	done := make(chan struct{})
	go func() {
		s.closing.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
