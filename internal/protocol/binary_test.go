package protocol

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestBinaryRoundTrip(t *testing.T) {
	b, err := EncodeBinary("t1", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, append([]byte{2, 't', '1'}, "hello"...)) {
		t.Fatalf("layout % x", b)
	}
	ch, p, err := DecodeBinary(b)
	if err != nil || ch != "t1" || string(p) != "hello" {
		t.Fatalf("got %q %q %v", ch, p, err)
	}
}

func TestBinaryEmptyPayloadAllowed(t *testing.T) {
	b, _ := EncodeBinary("a", nil)
	if ch, p, err := DecodeBinary(b); err != nil || ch != "a" || len(p) != 0 {
		t.Fatalf("got %q %q %v", ch, p, err)
	}
}

func TestBinaryMalformed(t *testing.T) {
	cases := map[string][]byte{
		"rỗng":        {},
		"id dài 0":    {0, 'x'},
		"id cụt":      {5, 'a', 'b'},
		"id ký tự lạ": {2, 'a', '/'},
	}
	for name, b := range cases {
		if _, _, err := DecodeBinary(b); err == nil {
			t.Errorf("%s: phải lỗi", name)
		}
	}
}

func TestValidateChannelID(t *testing.T) {
	ok := []string{"a", "t-1", "pty_2.x", strings.Repeat("a", 64)}
	bad := []string{"", strings.Repeat("a", 65), "a b", "ä", "a/b"}
	for _, id := range ok {
		if err := ValidateChannelID(id); err != nil {
			t.Errorf("%q: %v", id, err)
		}
	}
	for _, id := range bad {
		if err := ValidateChannelID(id); !errors.Is(err, ErrBadChannelID) {
			t.Errorf("%q: phải ErrBadChannelID, nhận %v", id, err)
		}
	}
	if _, err := EncodeBinary("a b", nil); !errors.Is(err, ErrBadChannelID) {
		t.Errorf("EncodeBinary phải kiểm id")
	}
}
