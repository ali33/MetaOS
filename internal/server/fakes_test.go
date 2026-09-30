package server

import (
	"io"
	"sync"
	"time"

	"github.com/ali33/MetaOS/internal/protocol"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 9, 30, 8, 0, 0, 0, time.Local)} }
func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
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
