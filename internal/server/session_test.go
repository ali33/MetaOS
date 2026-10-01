package server

import (
	"testing"
	"time"
)

func newTestStore() (*Store, *fakeClock) {
	c := newFakeClock()
	return NewStore(c, 12*time.Hour, 30*time.Minute), c
}

func TestSessionCreateGet(t *testing.T) {
	st, _ := newTestStore()
	b := newFakeBridge()
	s, err := st.Create("alice", "srv1", b)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ID) != 43 || len(s.CSRF) != 43 || s.ID == s.CSRF {
		t.Fatalf("id/csrf phải là 256 bit base64url khác nhau: %q %q", s.ID, s.CSRF)
	}
	got, ok := st.Get(s.ID)
	if !ok || got != s || got.User != "alice" || got.Hostname != "srv1" {
		t.Fatalf("Get: %v %v", got, ok)
	}
	if _, ok := st.Get("khong-co"); ok {
		t.Fatal("id lạ phải trả false")
	}
}

func TestSessionIDsUnique(t *testing.T) {
	st, _ := newTestStore()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		s, _ := st.Create("u", "h", newFakeBridge())
		if seen[s.ID] {
			t.Fatal("trùng id")
		}
		seen[s.ID] = true
	}
}

func TestSessionIdleExpiry(t *testing.T) {
	st, clk := newTestStore()
	b := newFakeBridge()
	s, _ := st.Create("alice", "h", b)
	var reason string
	s.OnEnd(func(r string) { reason = r })
	clk.Advance(29 * time.Minute)
	if _, ok := st.Get(s.ID); !ok {
		t.Fatal("29 phút chưa được hết hạn")
	}
	clk.Advance(29 * time.Minute) // Get ở trên không tính là hoạt động
	if _, ok := st.Get(s.ID); ok {
		t.Fatal("58 phút không Touch phải hết hạn")
	}
	if !b.isStopped() || reason != "expired" || st.Len() != 0 {
		t.Fatalf("stopped=%v reason=%q len=%d", b.isStopped(), reason, st.Len())
	}
}

func TestSessionTouchExtendsButNotPastMax(t *testing.T) {
	st, clk := newTestStore()
	s, _ := st.Create("alice", "h", newFakeBridge())
	for i := 0; i < 71; i++ { // 71 × 10 phút = 11 giờ 50 phút
		clk.Advance(10 * time.Minute)
		st.Touch(s)
		if _, ok := st.Get(s.ID); !ok {
			t.Fatalf("hết hạn sớm ở vòng %d", i)
		}
	}
	clk.Advance(11 * time.Minute) // 12 giờ 01 phút
	st.Touch(s)
	if _, ok := st.Get(s.ID); ok {
		t.Fatal("quá 12 giờ phải hết hạn dù vẫn hoạt động")
	}
}

func TestSessionEndLogout(t *testing.T) {
	st, _ := newTestStore()
	b := newFakeBridge()
	s, _ := st.Create("alice", "h", b)
	var reasons []string
	s.OnEnd(func(r string) { reasons = append(reasons, r) })
	st.End(s.ID, "logout")
	st.End(s.ID, "logout") // lần hai vô hại
	if !b.isStopped() || len(reasons) != 1 || reasons[0] != "logout" {
		t.Fatalf("stopped=%v reasons=%v", b.isStopped(), reasons)
	}
}

func TestSessionReap(t *testing.T) {
	st, clk := newTestStore()
	a, _ := st.Create("a", "h", newFakeBridge())
	clk.Advance(20 * time.Minute)
	b, _ := st.Create("b", "h", newFakeBridge())
	clk.Advance(15 * time.Minute)
	st.Reap()
	if st.Len() != 1 {
		t.Fatalf("len=%d", st.Len())
	}
	if _, ok := st.Get(a.ID); ok {
		t.Fatal("a phải bị dọn")
	}
	if _, ok := st.Get(b.ID); !ok {
		t.Fatal("b còn hạn")
	}
}

func TestSessionBridgeExitEndsSession(t *testing.T) {
	st, _ := newTestStore()
	b := newFakeBridge()
	s, _ := st.Create("alice", "h", b)
	ended := make(chan string, 1)
	s.OnEnd(func(r string) { ended <- r })
	b.Stop() // giả lập bridge tự chết
	select {
	case r := <-ended:
		if r != "bridge-exit" {
			t.Fatalf("reason=%q", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("phiên không kết thúc khi bridge chết")
	}
}

func TestSessionEndAll(t *testing.T) {
	st, _ := newTestStore()
	a, b := newFakeBridge(), newFakeBridge()
	st.Create("a", "h", a)
	st.Create("b", "h", b)
	st.EndAll("shutdown")
	if st.Len() != 0 || !a.isStopped() || !b.isStopped() {
		t.Fatal("EndAll phải dừng mọi bridge")
	}
}

func TestSessionCSRF(t *testing.T) {
	st, _ := newTestStore()
	s, _ := st.Create("alice", "h", newFakeBridge())
	if !s.CheckCSRF(s.CSRF) || s.CheckCSRF("") || s.CheckCSRF(s.CSRF+"x") || s.CheckCSRF(s.ID) {
		t.Fatal("CheckCSRF sai")
	}
}

func TestSessionOnEndAfterEnd(t *testing.T) {
	st, _ := newTestStore()
	s, _ := st.Create("alice", "h", newFakeBridge())
	st.End(s.ID, "logout")
	var reasons []string
	s.OnEnd(func(r string) { reasons = append(reasons, r) })
	if len(reasons) != 1 || reasons[0] != "logout" {
		t.Fatalf("reasons=%v", reasons)
	}
}

func TestSessionAbsoluteMaxWithoutTouch(t *testing.T) {
	c := newFakeClock()
	st := NewStore(c, time.Hour, 2*time.Hour) // idle > max: chỉ max có thể cắt
	s, _ := st.Create("alice", "h", newFakeBridge())
	c.Advance(time.Hour - time.Second)
	if _, ok := st.Get(s.ID); !ok {
		t.Fatal("chưa tới max phải còn")
	}
	c.Advance(time.Second)
	if _, ok := st.Get(s.ID); ok {
		t.Fatal("đúng max phải hết hạn")
	}
}

func TestSessionIdleBoundary(t *testing.T) {
	st, clk := newTestStore()
	s, _ := st.Create("alice", "h", newFakeBridge())
	clk.Advance(30*time.Minute - time.Second)
	if _, ok := st.Get(s.ID); !ok {
		t.Fatal("29m59s phải còn")
	}
	clk.Advance(time.Second)
	if _, ok := st.Get(s.ID); ok {
		t.Fatal("đúng 30m phải hết hạn")
	}
}
