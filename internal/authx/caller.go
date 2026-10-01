package authx

import "errors"

const CallerUser = "metaos"

var ErrCallerNotAllowed = errors.New("caller is not the metaos user")

// CheckCaller chỉ cho đúng uid thật của user "metaos" gọi. Không có user đó
// thì chặn tất cả — kể cả root, vì root không cần binary này.
func CheckCaller(uid int, lookupUID func(name string) (int, error)) error {
	want, err := lookupUID(CallerUser)
	if err != nil || uid != want {
		return ErrCallerNotAllowed
	}
	return nil
}
