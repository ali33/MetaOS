package authx

import (
	"errors"
	"testing"
)

func TestParseArgs(t *testing.T) {
	a, err := ParseArgs([]string{"alice"})
	if err != nil || a.User != "alice" || a.CheckOnly {
		t.Fatalf("got %+v %v", a, err)
	}
	a, err = ParseArgs([]string{"--check-only", "bob.x"})
	if err != nil || a.User != "bob.x" || !a.CheckOnly {
		t.Fatalf("got %+v %v", a, err)
	}
	bad := [][]string{
		{}, {"--check-only"}, {"a", "b"}, {"--other", "a"}, {"-alice"},
		{"a/b"}, {"a b"}, {""}, {"abcdefghijklmnopqrstuvwxyz0123456"}, // 33 ký tự
	}
	for _, argv := range bad {
		if _, err := ParseArgs(argv); !errors.Is(err, ErrUsage) {
			t.Errorf("%q: phải ErrUsage, nhận %v", argv, err)
		}
	}
}
