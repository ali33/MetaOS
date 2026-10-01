package pty

import "bytes"

// Ring giữ max byte đầu ra gần nhất để phát lại khi client gắn lại.
// Không an toàn đồng thời; kênh pty khoá bên ngoài.
type Ring struct {
	max     int
	data    []byte
	dropped bool
	total   uint64 // tổng số byte đã ghi từ đầu kênh
}

func NewRing(max int) *Ring { return &Ring{max: max, data: make([]byte, 0, max)} }

func (r *Ring) Write(p []byte) {
	r.total += uint64(len(p))
	r.data = append(r.data, p...)
	if over := len(r.data) - r.max; over > 0 {
		copy(r.data, r.data[over:])
		r.data = r.data[:r.max]
		r.dropped = true
	}
}

// Snapshot trả bản sao. Nếu đã phải bỏ byte đầu, cắt tới sau '\n' đầu tiên để
// không phát lại nửa chuỗi escape; không có '\n' thì bỏ byte tiếp nối UTF-8.
func (r *Ring) Snapshot() []byte {
	d := r.data
	if r.dropped {
		if i := bytes.IndexByte(d, '\n'); i >= 0 {
			d = d[i+1:]
		} else {
			for len(d) > 0 && d[0]&0xC0 == 0x80 {
				d = d[1:]
			}
		}
	}
	return append([]byte(nil), d...)
}

// From trả đầu ra kể từ vị trí off (tính trên toàn luồng). Còn trong bộ đệm ⇒
// reset=false, start=off. Không có off hoặc off đã trôi khỏi bộ đệm ⇒ bản chụp
// như Snapshot, reset=true, start = vị trí của byte đầu bản chụp.
func (r *Ring) From(off *uint64) (data []byte, start uint64, reset bool) {
	first := r.total - uint64(len(r.data))
	if off != nil && *off >= first && *off <= r.total {
		return append([]byte(nil), r.data[*off-first:]...), *off, false
	}
	snap := r.Snapshot()
	return snap, r.total - uint64(len(snap)), true
}
