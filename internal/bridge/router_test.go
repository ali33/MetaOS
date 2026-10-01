package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ali33/MetaOS/internal/channels"
	"github.com/ali33/MetaOS/internal/protocol"
)

// echoChan: kênh giả trả lại nguyên frame nhị phân; không gắn lại được trừ khi reattach=true.
type echoChan struct {
	a          channels.OpenArgs
	reattach   bool
	mu         sync.Mutex
	closed     string
	detached   bool
	reattached bool
}

func (e *echoChan) HandleText(m protocol.Message) {
	_ = e.a.Out.SendText(protocol.Message{Ch: e.a.ID, Type: "echo", Data: m.Data})
}
func (e *echoChan) HandleBinary(p []byte)    { _ = e.a.Out.SendBinary(e.a.ID, p) }
func (e *echoChan) Reattachable() bool       { return e.reattach }
func (e *echoChan) Detach()                  { e.mu.Lock(); e.detached = true; e.mu.Unlock() }
func (e *echoChan) Reattach(json.RawMessage) { e.mu.Lock(); e.reattached = true; e.mu.Unlock() }
func (e *echoChan) Close(reason string) {
	e.mu.Lock()
	first := e.closed == ""
	e.closed = reason
	e.mu.Unlock()
	if first {
		_ = e.a.Out.SendText(protocol.Control(protocol.TypeClose, protocol.CloseData{Ch: e.a.ID, Reason: reason}))
		e.a.Done()
	}
}

type fragileChan struct{ echoChan }

func (f *fragileChan) Reattach(json.RawMessage) { f.Close("exit") }

type harness struct {
	t     *testing.T
	in    *io.PipeWriter
	out   *io.PipeReader
	chans map[string]*echoChan
	mu    sync.Mutex
	runE  chan error
}

func newHarness(t *testing.T) *harness {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h := &harness{t: t, in: inW, out: outR, chans: map[string]*echoChan{}, runE: make(chan error, 1)}
	kinds := map[string]channels.Factory{
		"echo": func(a channels.OpenArgs) (channels.Channel, error) {
			var p struct {
				Reattachable bool `json:"reattachable"`
			}
			_ = json.Unmarshal(a.Params, &p)
			c := &echoChan{a: a, reattach: p.Reattachable}
			h.mu.Lock()
			h.chans[a.ID] = c
			h.mu.Unlock()
			return c, nil
		},
		// dies: kênh kết thúc (gọi Done) ngay trong factory, như shell thoát tức thì.
		"dies": func(a channels.OpenArgs) (channels.Channel, error) {
			c := &echoChan{a: a, reattach: true}
			c.Close("exit")
			return c, nil
		},
		// fragile: kênh kết thúc đúng lúc đang được gắn lại.
		"fragile": func(a channels.OpenArgs) (channels.Channel, error) {
			return &fragileChan{echoChan{a: a, reattach: true}}, nil
		},
		"broken": func(a channels.OpenArgs) (channels.Channel, error) {
			return nil, &channels.Error{Code: protocol.CodeAccessDenied, Err: errors.New("open /x: permission denied")}
		},
	}
	r := NewRouter(inR, NewSender(outW), kinds, io.Discard)
	go func() { h.runE <- r.Run(); outW.Close() }()
	return h
}

func (h *harness) sendText(s string) {
	h.t.Helper()
	if err := protocol.WritePipeFrame(h.in, protocol.Frame{Kind: protocol.KindText, Data: []byte(s)}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) sendBinary(ch, p string) {
	b, _ := protocol.EncodeBinary(ch, []byte(p))
	_ = protocol.WritePipeFrame(h.in, protocol.Frame{Kind: protocol.KindBinary, Data: b})
}

func (h *harness) next() protocol.Frame {
	h.t.Helper()
	type res struct {
		f   protocol.Frame
		err error
	}
	c := make(chan res, 1)
	go func() { f, err := protocol.ReadPipeFrame(h.out); c <- res{f, err} }()
	select {
	case r := <-c:
		if r.err != nil {
			h.t.Fatalf("đọc frame: %v", r.err)
		}
		return r.f
	case <-time.After(3 * time.Second):
		h.t.Fatal("hết giờ chờ frame")
	}
	return protocol.Frame{}
}

func (h *harness) nextMsg() protocol.Message {
	h.t.Helper()
	f := h.next()
	m, err := protocol.DecodeMessage(f.Data)
	if err != nil {
		h.t.Fatalf("frame không phải message: %q", f.Data)
	}
	return m
}

func (h *harness) expectError(code string) {
	h.t.Helper()
	m := h.nextMsg()
	var e protocol.ErrorData
	_ = json.Unmarshal(m.Data, &e)
	if m.Type != protocol.TypeError || e.Code != code {
		h.t.Fatalf("muốn error %s, nhận %s %s", code, m.Type, m.Data)
	}
}

func (h *harness) open(id, kind, params string) protocol.Message {
	h.sendText(fmt.Sprintf(`{"ch":"","type":"open","data":{"ch":%q,"kind":%q,"params":%s}}`, id, kind, params))
	return h.nextMsg()
}

func TestRouterOpenReady(t *testing.T) {
	h := newHarness(t)
	if m := h.open("a", "echo", `{}`); m.Type != protocol.TypeReady || string(m.Data) != `{"ch":"a"}` {
		t.Fatalf("got %s %s", m.Type, m.Data)
	}
}

func TestRouterOpenErrors(t *testing.T) {
	h := newHarness(t)
	h.open("a", "echo", `{}`)
	h.sendText(`{"ch":"","type":"open","data":{"ch":"a","kind":"echo"}}`)
	h.expectError(protocol.CodeInvalidParams) // id trùng
	h.sendText(`{"ch":"","type":"open","data":{"ch":"b","kind":"nope"}}`)
	h.expectError(protocol.CodeUnsupported)
	h.sendText(`{"ch":"","type":"open","data":{"ch":"c","kind":"broken"}}`)
	h.expectError(protocol.CodeAccessDenied)
	h.sendText(`{"ch":"","type":"open","data":{"ch":"x y","kind":"echo"}}`)
	h.expectError(protocol.CodeInvalidParams)
}

func TestRouterChannelLimit(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < protocol.MaxChannels; i++ {
		if m := h.open(fmt.Sprintf("c%d", i), "echo", `{}`); m.Type != protocol.TypeReady {
			t.Fatalf("kênh %d: %s %s", i, m.Type, m.Data)
		}
	}
	h.sendText(`{"ch":"","type":"open","data":{"ch":"over","kind":"echo"}}`)
	h.expectError(protocol.CodeInvalidParams)
	h.sendText(`{"ch":"","type":"close","data":{"ch":"c0"}}`)
	if m := h.nextMsg(); m.Type != protocol.TypeClose {
		t.Fatalf("got %s", m.Type)
	}
	if m := h.open("over", "echo", `{}`); m.Type != protocol.TypeReady {
		t.Fatalf("đóng một kênh rồi phải mở được: %s", m.Data)
	}
}

func TestRouterRoutesDataAndBinary(t *testing.T) {
	h := newHarness(t)
	h.open("a", "echo", `{}`)
	h.sendBinary("a", "gõ phím")
	f := h.next()
	ch, p, _ := protocol.DecodeBinary(f.Data)
	if f.Kind != protocol.KindBinary || ch != "a" || string(p) != "gõ phím" {
		t.Fatalf("got %v %q %q", f.Kind, ch, p)
	}
	h.sendText(`{"ch":"a","type":"x","data":{"k":1}}`)
	if m := h.nextMsg(); m.Type != "echo" || string(m.Data) != `{"k":1}` {
		t.Fatalf("got %s %s", m.Type, m.Data)
	}
	h.sendText(`{"ch":"zz","type":"x"}`)
	h.expectError(protocol.CodeNotFound)
}

func TestRouterPingAndBadJSONKeepsRunning(t *testing.T) {
	h := newHarness(t)
	h.sendText(`{rác`)
	h.expectError(protocol.CodeInvalidParams)
	h.sendText(`{"ch":"","type":"ping","data":{"n":7}}`)
	if m := h.nextMsg(); m.Type != protocol.TypePong || string(m.Data) != `{"n":7}` {
		t.Fatalf("got %s %s", m.Type, m.Data)
	}
}

func TestRouterDetachAndReattach(t *testing.T) {
	h := newHarness(t)
	h.open("keep", "echo", `{"reattachable":true}`)
	h.open("drop", "echo", `{}`)
	h.sendText(`{"ch":"","type":"detached"}`)
	if m := h.nextMsg(); m.Type != protocol.TypeClose || string(m.Data) != `{"ch":"drop","reason":"detached"}` {
		t.Fatalf("got %s %s", m.Type, m.Data)
	}
	if m := h.open("keep", "echo", `{"reattach":true}`); m.Type != protocol.TypeReady {
		t.Fatalf("gắn lại: %s %s", m.Type, m.Data)
	}
	h.mu.Lock()
	k := h.chans["keep"]
	h.mu.Unlock()
	if !k.detached || !k.reattached {
		t.Fatalf("detached=%v reattached=%v", k.detached, k.reattached)
	}
	h.sendText(`{"ch":"","type":"open","data":{"ch":"gone","kind":"echo","params":{"reattach":true}}}`)
	h.expectError(protocol.CodeNotFound)
}

func TestRouterAttachedListsReattachableChannels(t *testing.T) { // D17
	h := newHarness(t)
	h.open("b", "echo", `{"reattachable":true}`)
	h.open("a", "echo", `{"reattachable":true}`)
	h.open("x", "echo", `{}`)
	h.sendText(`{"ch":"","type":"attached"}`)
	if m := h.nextMsg(); m.Type != protocol.TypeChannels || string(m.Data) != `[{"ch":"a","kind":"echo"},{"ch":"b","kind":"echo"}]` {
		t.Fatalf("got %s %s", m.Type, m.Data)
	}
}

func TestRouterExitsOnStdinEOF(t *testing.T) { // Review Focus #1
	h := newHarness(t)
	h.open("a", "echo", `{}`)
	h.open("b", "echo", `{}`)
	go io.Copy(io.Discard, h.out)
	h.in.Close()
	select {
	case err := <-h.runE:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run không trả về sau EOF")
	}
	for id, c := range h.chans {
		if c.closed != "shutdown" {
			t.Errorf("kênh %s: closed=%q", id, c.closed)
		}
	}
}

func TestRouterPipeErrorStops(t *testing.T) {
	h := newHarness(t)
	go io.Copy(io.Discard, h.out)
	// Chỉ ghi đúng 5 byte header: io.Pipe.Write chặn tới khi mọi byte được đọc,
	// mà router dừng ngay sau header nên byte thứ 6 sẽ không bao giờ được đọc.
	_, _ = h.in.Write([]byte{0, 0, 0, 1, 9}) // loại frame 9
	select {
	case err := <-h.runE:
		if !errors.Is(err, protocol.ErrBadKind) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run không dừng khi pipe hỏng")
	}
}

// Kênh đã gọi Done thì không bao giờ được báo ready, và id của nó được giải phóng.
func TestRouterNoReadyForFinishedChannel(t *testing.T) {
	h := newHarness(t)
	if m := h.open("d", "dies", `{}`); m.Type != protocol.TypeClose {
		t.Fatalf("muốn close, nhận %s %s", m.Type, m.Data)
	}
	h.sendText(`{"ch":"","type":"open","data":{"ch":"d","kind":"dies","params":{"reattach":true}}}`)
	h.expectError(protocol.CodeNotFound) // không có ready lọt vào giữa
	if m := h.open("d", "echo", `{"reattachable":true}`); m.Type != protocol.TypeReady {
		t.Fatalf("id của kênh đã chết phải dùng lại được: %s %s", m.Type, m.Data)
	}

	h.open("f", "fragile", `{}`)
	h.sendText(`{"ch":"","type":"detached"}`)
	h.sendText(`{"ch":"","type":"open","data":{"ch":"f","kind":"fragile","params":{"reattach":true}}}`)
	if m := h.nextMsg(); m.Type != protocol.TypeClose || string(m.Data) != `{"ch":"f","reason":"exit"}` {
		t.Fatalf("got %s %s", m.Type, m.Data)
	}
	h.expectError(protocol.CodeNotFound)
	h.sendText(`{"ch":"","type":"ping"}`)
	if m := h.nextMsg(); m.Type != protocol.TypePong {
		t.Fatalf("sau kênh chết phải là pong, nhận %s %s", m.Type, m.Data)
	}
}
