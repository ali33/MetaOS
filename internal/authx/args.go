package authx

import "errors"

var ErrUsage = errors.New("usage: metaos-auth [--check-only] <user>")

type Args struct {
	CheckOnly bool
	User      string
}

func ParseArgs(argv []string) (Args, error) {
	var a Args
	if len(argv) == 2 && argv[0] == "--check-only" {
		a.CheckOnly, argv = true, argv[1:]
	}
	if len(argv) != 1 || !validUser(argv[0]) {
		return Args{}, ErrUsage
	}
	a.User = argv[0]
	return a, nil
}

// validUser: 1..32 ký tự [A-Za-z0-9_.-], không bắt đầu bằng '-' hay '.'.
func validUser(s string) bool {
	if len(s) < 1 || len(s) > 32 || s[0] == '-' || s[0] == '.' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}
