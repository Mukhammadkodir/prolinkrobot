package app

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/sulton0011/errs"
)

func AcquireRunLock() (func(), error) {
	lockPath := filepath.Join(os.TempDir(), "prolinkrobot.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errs.Wrap(&err, "os.OpenFile")
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errs.New("another bot instance is already running")
	}

	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
