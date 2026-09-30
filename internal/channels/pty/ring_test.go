package pty

import (
	"bytes"
	"testing"
)

func TestRingNoDropReturnsAll(t *testing.T) {
	r := NewRing(16)
	r.Write([]byte("ab"))
	r.Write([]byte("cd"))
	if got := r.Snapshot(); string(got) != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestRingKeepsTail(t *testing.T) {
	r := NewRing(8)
	r.Write([]byte("0123456789\nabcdef"))
	got := r.Snapshot()
	if len(got) > 8 || !bytes.HasSuffix(got, []byte("abcdef")) {
		t.Fatalf("got %q", got)
	}
}

func TestRingSnapshotStartsAtLineBoundary(t *testing.T) {
	r := NewRing(10)
	r.Write([]byte("\x1b[31mĐỏ rất dài\nsau"))
	if got := r.Snapshot(); string(got) != "sau" {
		t.Fatalf("đã cắt thì phải bắt đầu sau \\n đầu tiên: %q", got)
	}
}

func TestRingUTF8BoundaryWithoutNewline(t *testing.T) {
	r := NewRing(5)
	r.Write([]byte("xxxxxĐĐĐ")) // Đ = c4 90; giữ 5 byte cuối = 90 c4 90 c4 90
	got := r.Snapshot()
	if len(got) == 0 || got[0]&0xC0 == 0x80 {
		t.Fatalf("không được bắt đầu bằng byte tiếp nối UTF-8: % x", got)
	}
	if string(got) != "ĐĐ" {
		t.Fatalf("got %q", got)
	}
}

func TestRingFrom(t *testing.T) {
	r := NewRing(4)
	r.Write([]byte("abcdef")) // giữ "cdef", vị trí 2..6
	u := func(v uint64) *uint64 { return &v }
	cases := []struct {
		off   *uint64
		data  string
		start uint64
		reset bool
	}{
		{u(4), "ef", 4, false},
		{u(6), "", 6, false},
		{u(1), "cdef", 2, true}, // đã trôi khỏi bộ đệm
		{u(7), "cdef", 2, true}, // lớn hơn tổng: client lạ, gửi bản chụp
		{nil, "cdef", 2, true},
	}
	for _, c := range cases {
		d, st, rs := r.From(c.off)
		if string(d) != c.data || st != c.start || rs != c.reset {
			t.Errorf("From(%v) = %q %d %v", c.off, d, st, rs)
		}
	}
}

func TestRingSnapshotIsCopy(t *testing.T) {
	r := NewRing(8)
	r.Write([]byte("abc"))
	s := r.Snapshot()
	s[0] = 'X'
	if string(r.Snapshot()) != "abc" {
		t.Fatal("Snapshot phải trả bản sao")
	}
}
