//go:build !windows

package dlengine

import (
	"errors"
	"syscall"
)

// isFatalLocalWriteError returns true when a local write failure is unrecoverable
// by retrying network chunks (such as ENOSPC or EDQUOT).
func isFatalLocalWriteError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}
