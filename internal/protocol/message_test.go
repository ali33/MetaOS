package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestDecodeMessage(t *testing.T) {
	m, err := DecodeMessage([]byte(`{"ch":"t1","type":"resize","data":{"cols":80,"rows":24}}`))
	if err != nil || m.Ch != "t1" || m.Type != "resize" || string(m.Data) != `{"cols":80,"rows":24}` {
		t.Fatalf("got %+v %v", m, err)
	}
}

func TestDecodeMessageRejects(t *testing.T) {
	cases := map[string]string{
		"không phải JSON": `{`,
		"thiếu type":      `{"ch":""}`,
		"ch sai":          `{"ch":"a b","type":"x"}`,
		"mảng":            `[]`,
	}
	for name, s := range cases {
		if _, err := DecodeMessage([]byte(s)); !errors.Is(err, ErrBadMessage) {
			t.Errorf("%s: phải ErrBadMessage, nhận %v", name, err)
		}
	}
	big := `{"ch":"","type":"x","data":"` + strings.Repeat("a", MaxFrame) + `"}`
	if _, err := DecodeMessage([]byte(big)); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("quá cỡ: nhận %v", err)
	}
}

func TestControlEncode(t *testing.T) {
	b, err := Control(TypeError, ErrorData{Ch: "t1", Code: CodeNotFound, Message: "x"}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ch":"","type":"error","data":{"ch":"t1","code":"not-found","message":"x"}}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	var o OpenData
	_ = json.Unmarshal([]byte(`{"ch":"t1","kind":"pty","params":{"cols":80}}`), &o)
	if o.Ch != "t1" || o.Kind != "pty" || string(o.Params) != `{"cols":80}` {
		t.Fatalf("OpenData %+v", o)
	}
}

func TestCodeFromErr(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("open x: %w", fs.ErrPermission), CodeAccessDenied},
		{fmt.Errorf("open x: %w", fs.ErrNotExist), CodeNotFound},
		{errors.New("khác"), CodeInternal},
	}
	for _, c := range cases {
		if got := CodeFromErr(c.err); got != c.want {
			t.Errorf("%v: got %s want %s", c.err, got, c.want)
		}
	}
}
