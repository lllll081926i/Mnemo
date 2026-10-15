//go:build !windows

package vault

import "runtime"

func readOSProtectedKey(dir string) ([]byte, error) {
	return (desktopKeyring{platform: runtime.GOOS, run: runKeyringCommand}).read(dir)
}

func writeOSProtectedKey(dir string, key []byte) error {
	return (desktopKeyring{platform: runtime.GOOS, run: runKeyringCommand}).write(dir, key)
}
