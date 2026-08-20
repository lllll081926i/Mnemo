//go:build !windows

package vault

import "os"

// Native key storage is intentionally left to platform-specific adapters.
// Returning os.ErrNotExist keeps the existing encrypted-file format working
// on macOS/Linux without claiming that a process-local file is OS-protected.
func readOSProtectedKey(string) ([]byte, error) { return nil, os.ErrNotExist }

func writeOSProtectedKey(string, []byte) error { return os.ErrNotExist }
