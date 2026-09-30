package authx

import (
	"os"
	"sync"
	"syscall"
)

// GuardFD trỏ fd sang /dev/null và trả hàm trả fd về chỗ cũ. Dùng quanh PAM
// để module nào ghi thẳng ra stdout cũng không chen vào luồng frame.
func GuardFD(fd int) (func() error, error) {
	saved, err := syscall.Dup(fd)
	if err != nil {
		return nil, err
	}
	// Không để bản giữ stdout lọt sang tiến trình PAM fork/exec (rò đầu ghi ống frame).
	syscall.CloseOnExec(saved)
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
	var once sync.Once
	return func() error {
		var err error
		once.Do(func() {
			defer syscall.Close(saved)
			err = syscall.Dup3(saved, fd, 0)
		})
		return err
	}, nil
}
