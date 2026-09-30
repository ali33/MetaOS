package authx

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// oneByteReader bắt ReadPassword phải chịu được Read trả từng mẩu nhỏ.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) { return o.r.Read(p[:1]) }

func TestReadPasswordStopsAtNewline(t *testing.T) {
	r := bytes.NewReader([]byte("mật khẩu có dấu cách\nFRAME"))
	pw, err := ReadPassword(r)
	if err != nil || string(pw) != "mật khẩu có dấu cách" {
		t.Fatalf("got %q %v", pw, err)
	}
	rest, _ := io.ReadAll(r)
	if string(rest) != "FRAME" {
		t.Fatalf("đã nuốt lố byte của luồng sau: còn lại %q", rest)
	}
}

func TestReadPasswordSmallReads(t *testing.T) {
	pw, err := ReadPassword(oneByteReader{strings.NewReader("abc\n")})
	if err != nil || string(pw) != "abc" {
		t.Fatalf("got %q %v", pw, err)
	}
}

func TestReadPasswordRejects(t *testing.T) {
	cases := map[string]struct {
		in   string
		want error
	}{
		"rỗng":        {"\n", ErrPasswordInvalid},
		"không có \n": {"abc", ErrPasswordInvalid},
		"có NUL":      {"a\x00b\n", ErrPasswordInvalid},
		"dài 513":     {strings.Repeat("a", MaxPassword+1) + "\n", ErrPasswordTooLong},
		"dài 10 KB":   {strings.Repeat("a", 10<<10) + "\n", ErrPasswordTooLong},
	}
	for name, c := range cases {
		if _, err := ReadPassword(strings.NewReader(c.in)); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", name, err, c.want)
		}
	}
	if pw, err := ReadPassword(strings.NewReader(strings.Repeat("a", MaxPassword) + "\n")); err != nil || len(pw) != MaxPassword {
		t.Errorf("đúng 512 byte phải qua: %d %v", len(pw), err)
	}
}

func TestWipe(t *testing.T) {
	b := []byte("secret")
	Wipe(b)
	if !bytes.Equal(b, make([]byte, 6)) {
		t.Fatalf("chưa xoá: %q", b)
	}
}
