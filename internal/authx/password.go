// Package authx chứa phần logic thuần của metaos-auth (binary setuid root).
// Giữ nhỏ: mọi dòng ở đây được rà bảo mật bằng mắt.
package authx

import (
	"errors"
	"io"
)

const MaxPassword = 512

var (
	ErrPasswordInvalid = errors.New("password: empty, unterminated or contains NUL")
	ErrPasswordTooLong = errors.New("password: too long")
)

// ReadPassword đọc từng byte tới '\n' đầu tiên. Không bao giờ đọc quá '\n',
// vì phần sau của stdin là luồng frame dành cho metaos-bridge.
func ReadPassword(r io.Reader) ([]byte, error) {
	buf := make([]byte, 0, MaxPassword) // cấp đủ một lần để append không để lại bản sao
	var b [1]byte
	for {
		n, err := r.Read(b[:])
		if n == 1 {
			switch {
			case b[0] == '\n':
				if len(buf) == 0 {
					return nil, ErrPasswordInvalid
				}
				return buf, nil
			case b[0] == 0:
				Wipe(buf)
				return nil, ErrPasswordInvalid
			case len(buf) == MaxPassword:
				Wipe(buf)
				return nil, ErrPasswordTooLong
			}
			buf = append(buf, b[0])
			continue
		}
		if err != nil {
			Wipe(buf)
			if err == io.EOF {
				return nil, ErrPasswordInvalid
			}
			return nil, err
		}
	}
}

func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
