//go:build windows

package vault

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const protectedKeyfile = "vault.key.dpapi"

func protectedKeyPath(dir string) string {
	return filepath.Join(dir, protectedKeyfile)
}

func readOSProtectedKey(dir string) ([]byte, error) {
	raw, err := os.ReadFile(protectedKeyPath(dir))
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("vault: empty DPAPI key")
	}
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	if out.Size == 0 || out.Data == nil {
		return nil, errors.New("vault: DPAPI returned empty key")
	}
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func writeOSProtectedKey(dir string, key []byte) error {
	if len(key) != 32 {
		return errors.New("vault: invalid master key length")
	}
	in := windows.DataBlob{Size: uint32(len(key)), Data: &key[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	if out.Size == 0 || out.Data == nil {
		return errors.New("vault: DPAPI returned empty key")
	}
	return writePrivateFileAtomically(protectedKeyPath(dir), unsafe.Slice(out.Data, out.Size))
}
