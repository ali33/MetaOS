package authx

import (
	"os"
	"syscall"
)

// GuardFD trỏ fd sang /dev/null và trả hàm trả fd về chỗ cũ. Dùng quanh PAM
// để module nào ghi thẳng ra stdout cũng không chen vào luồng frame.
func GuardFD(fd int) (func() error, error) {
	saved, err := syscall.Dup(fd)
	if err != nil {
		return nil, err
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		syscall.Close(saved)
		return nil, err
	}
	defer null.Close()
	if err := syscall.Dup3(int(null.Fd()), fd, 0); err != nil {
		syscall.Close(saved)
		return nil, err
	}
	return func() error {
		defer syscall.Close(saved)
		return syscall.Dup3(saved, fd, 0)
	}, nil
}
