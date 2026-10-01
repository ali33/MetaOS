package authx

import (
	"errors"
	"testing"
)

func TestCheckCaller(t *testing.T) {
	lookup := func(name string) (int, error) {
		if name != CallerUser {
			t.Fatalf("tra sai tên %q", name)
		}
		return 998, nil
	}
	if err := CheckCaller(998, lookup); err != nil {
		t.Fatalf("metaos phải được gọi: %v", err)
	}
	for _, uid := range []int{0, 1000, 997} {
		if err := CheckCaller(uid, lookup); !errors.Is(err, ErrCallerNotAllowed) {
			t.Errorf("uid %d: phải bị chặn, nhận %v", uid, err)
		}
	}
	noUser := func(string) (int, error) { return 0, errors.New("unknown user metaos") }
	if err := CheckCaller(0, noUser); !errors.Is(err, ErrCallerNotAllowed) {
		t.Errorf("không có user metaos thì phải chặn tất cả, kể cả uid 0: %v", err)
	}
}
