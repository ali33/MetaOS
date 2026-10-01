package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/ali33/MetaOS/internal/channels"
	"github.com/ali33/MetaOS/internal/protocol"
)

var ShutdownWait = 3 * time.Second

// entry là một kênh đã mở. done được đặt (dưới r.mu) khi kênh gọi Done; từ lúc
// đó kênh không còn trong map và không bao giờ được báo ready nữa.
type entry struct {
	c    channels.Channel
	kind string
	done bool
}

// Router đọc frame từ ws và định tuyến tới kênh. Mọi lời gọi Detach, Reattach
// và Close theo lệnh điều khiển đều chạy trên goroutine của Run (kênh pty không
// tự tuần tự hoá Detach với Reattach). Chỉ Done của kênh chạy ở goroutine khác.
type Router struct {
	in    io.Reader
	out   *Sender
	kinds map[string]channels.Factory
	log   *log.Logger

	mu    sync.Mutex
	chans map[string]*entry
	wg    sync.WaitGroup
}

func NewRouter(in io.Reader, out *Sender, kinds map[string]channels.Factory, logw io.Writer) *Router {
	return &Router{in: in, out: out, kinds: kinds, log: log.New(logw, "metaos-bridge: ", 0), chans: map[string]*entry{}}
}

// Run trả nil khi stdin EOF (sau khi đã đóng hết kênh); trả lỗi khi luồng frame
// hỏng ở cấp pipe (frame quá lớn, sai loại, cụt).
func (r *Router) Run() error {
	for {
		f, err := protocol.ReadPipeFrame(r.in)
		if err != nil {
			r.shutdown()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if f.Kind == protocol.KindBinary {
			r.binary(f.Data)
		} else {
			r.text(f.Data)
		}
	}
}

func (r *Router) sendErr(ch, code, msg string) {
	_ = r.out.SendText(protocol.Control(protocol.TypeError, protocol.ErrorData{Ch: ch, Code: code, Message: msg}))
}

func (r *Router) get(id string) *entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.chans[id]
}

// live chụp danh sách kênh đang sống, để gọi vào kênh mà không giữ r.mu.
func (r *Router) live() []*entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]*entry, 0, len(r.chans))
	for _, e := range r.chans {
		list = append(list, e)
	}
	return list
}

func (r *Router) binary(b []byte) {
	ch, p, err := protocol.DecodeBinary(b)
	if err != nil {
		r.log.Printf("bad binary frame: %v", err)
		return
	}
	if e := r.get(ch); e != nil {
		e.c.HandleBinary(p)
	} else {
		r.log.Printf("binary frame for unknown channel %s dropped", ch)
	}
}

func (r *Router) text(b []byte) {
	m, err := protocol.DecodeMessage(b)
	if err != nil {
		r.sendErr("", protocol.CodeInvalidParams, err.Error())
		return
	}
	if m.Ch != "" {
		if e := r.get(m.Ch); e != nil {
			e.c.HandleText(m)
		} else {
			r.sendErr(m.Ch, protocol.CodeNotFound, "channel "+m.Ch+" is not open")
		}
		return
	}
	switch m.Type {
	case protocol.TypeOpen:
		r.open(m.Data)
	case protocol.TypeClose:
		var d protocol.CloseData
		_ = json.Unmarshal(m.Data, &d)
		if e := r.get(d.Ch); e != nil {
			e.c.Close("client")
		}
	case protocol.TypePing:
		_ = r.out.SendText(protocol.Message{Type: protocol.TypePong, Data: m.Data})
	case protocol.TypeDetached:
		for _, e := range r.live() {
			if e.c.Reattachable() {
				e.c.Detach()
			} else {
				e.c.Close("detached")
			}
		}
	case protocol.TypeAttached:
		// D17: cho WebSocket mới biết những kênh nó có thể tiếp quản.
		r.mu.Lock()
		list := []protocol.ChannelInfo{}
		for id, e := range r.chans {
			if e.c.Reattachable() {
				list = append(list, protocol.ChannelInfo{Ch: id, Kind: e.kind})
			}
		}
		r.mu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i].Ch < list[j].Ch })
		_ = r.out.SendText(protocol.Control(protocol.TypeChannels, list))
	default:
		r.sendErr("", protocol.CodeUnsupported, "unknown control type: "+m.Type)
	}
}

func (r *Router) open(raw json.RawMessage) {
	var d protocol.OpenData
	if err := json.Unmarshal(raw, &d); err != nil {
		r.sendErr("", protocol.CodeInvalidParams, err.Error())
		return
	}
	if err := protocol.ValidateChannelID(d.Ch); err != nil {
		r.sendErr("", protocol.CodeInvalidParams, fmt.Sprintf("bad channel id %q", d.Ch))
		return
	}
	var ra struct {
		Reattach bool `json:"reattach"`
	}
	_ = json.Unmarshal(d.Params, &ra)
	if ra.Reattach {
		r.reattach(d)
		return
	}
	factory, ok := r.kinds[d.Kind]
	if !ok {
		r.sendErr(d.Ch, protocol.CodeUnsupported, "unknown channel kind: "+d.Kind)
		return
	}
	r.mu.Lock()
	switch {
	case r.chans[d.Ch] != nil:
		r.mu.Unlock()
		r.sendErr(d.Ch, protocol.CodeInvalidParams, "channel id in use: "+d.Ch)
		return
	case len(r.chans) >= protocol.MaxChannels:
		r.mu.Unlock()
		r.sendErr(d.Ch, protocol.CodeInvalidParams, fmt.Sprintf("too many channels (max %d)", protocol.MaxChannels))
		return
	}
	r.mu.Unlock()
	id := d.Ch
	e := &entry{kind: d.Kind}
	r.wg.Add(1)
	var once sync.Once
	done := func() {
		once.Do(func() {
			r.mu.Lock()
			e.done = true
			if r.chans[id] == e {
				delete(r.chans, id)
			}
			r.mu.Unlock()
			r.wg.Done()
		})
	}
	c, err := factory(channels.OpenArgs{ID: id, Params: d.Params, Out: r.out, Done: done})
	if err != nil {
		r.wg.Done()
		code := protocol.CodeFromErr(err)
		var ce *channels.Error
		if errors.As(err, &ce) {
			code = ce.Code
		}
		r.sendErr(id, code, err.Error())
		return
	}
	r.mu.Lock()
	e.c = c
	finished := e.done // kênh đã kết thúc ngay trong factory (shell thoát tức thì)
	if !finished {
		r.chans[id] = e
	}
	r.mu.Unlock()
	if finished {
		// Kênh đã tự gửi close; báo ready lúc này là nói dối.
		return
	}
	_ = r.out.SendText(protocol.Control(protocol.TypeReady, protocol.ReadyData{Ch: id}))
}

// reattach gắn lại kênh cùng id. Kênh có thể kết thúc bất cứ lúc nào (Done ở
// goroutine khác); Reattach trên kênh đã kết thúc thì im lặng, nên phải kiểm
// lại dưới khoá sau khi gọi và không báo ready cho kênh đã gọi Done.
func (r *Router) reattach(d protocol.OpenData) {
	e := r.get(d.Ch)
	if e == nil || !e.c.Reattachable() {
		r.sendErr(d.Ch, protocol.CodeNotFound, "channel "+d.Ch+" is gone")
		return
	}
	e.c.Reattach(d.Params)
	r.mu.Lock()
	gone := e.done
	r.mu.Unlock()
	if gone {
		r.sendErr(d.Ch, protocol.CodeNotFound, "channel "+d.Ch+" is gone")
		return
	}
	_ = r.out.SendText(protocol.Control(protocol.TypeReady, protocol.ReadyData{Ch: d.Ch}))
}

// shutdown gọi Close trên mọi kênh (không chặn), rồi mới chờ tối đa
// ShutdownWait cho mọi Done.
func (r *Router) shutdown() {
	for _, e := range r.live() {
		e.c.Close("shutdown")
	}
	waited := make(chan struct{})
	go func() { r.wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(ShutdownWait):
		r.log.Printf("shutdown: channels still closing after %s", ShutdownWait)
	}
}
