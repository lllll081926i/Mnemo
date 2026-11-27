//go:build windows

package dlengine

import (
	"errors"
	"syscall"
)

const (
	winErrorDiskFull   syscall.Errno = 112 // ERROR_DISK_FULL
	winErrorHandleFull syscall.Errno = 39  // ERROR_HANDLE_DISK_FULL
)

// isFatalLocalWriteError returns true when a local write failure is unrecoverable
// by retrying network chunks (such as out-of-space or permission denied).
func isFatalLocalWriteError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, winErrorDiskFull) || errors.Is(err, winErrorHandleFull)
}
