package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestPipeRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := []Frame{{KindText, []byte(`{"ch":"","type":"ping"}`)}, {KindBinary, []byte{0, 1, 2}}, {KindText, []byte{}}}
	for _, f := range in {
		if err := WritePipeFrame(&buf, f); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range in {
		got, err := ReadPipeFrame(&buf)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if got.Kind != want.Kind || !bytes.Equal(got.Data, want.Data) {
			t.Fatalf("frame %d: got %+v want %+v", i, got, want)
		}
	}
	if _, err := ReadPipeFrame(&buf); err != io.EOF {
		t.Fatalf("hết luồng phải trả io.EOF, nhận %v", err)
	}
}

func TestPipeHeaderLayout(t *testing.T) {
	var buf bytes.Buffer
	_ = WritePipeFrame(&buf, Frame{KindBinary, []byte("abc")})
	want := []byte{0, 0, 0, 3, 1, 'a', 'b', 'c'}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("got % x want % x", buf.Bytes(), want)
	}
}

func TestPipeRejectsOversizeWithoutAllocating(t *testing.T) {
	hdr := make([]byte, 5)
	binary.BigEndian.PutUint32(hdr, 0xFFFFFFFF)
	if _, err := ReadPipeFrame(bytes.NewReader(hdr)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("got %v", err)
	}
	if err := WritePipeFrame(io.Discard, Frame{KindText, make([]byte, MaxFrame+1)}); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("write got %v", err)
	}
}

func TestPipeBadKind(t *testing.T) {
	if _, err := ReadPipeFrame(bytes.NewReader([]byte{0, 0, 0, 0, 7})); !errors.Is(err, ErrBadKind) {
		t.Fatalf("got %v", err)
	}
	if err := WritePipeFrame(io.Discard, Frame{Kind: 9}); !errors.Is(err, ErrBadKind) {
		t.Fatalf("write got %v", err)
	}
}

func TestPipeTruncated(t *testing.T) {
	for _, b := range [][]byte{{0, 0}, {0, 0, 0, 5, 0, 'a'}} {
		if _, err := ReadPipeFrame(bytes.NewReader(b)); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("% x: got %v", b, err)
		}
	}
}
