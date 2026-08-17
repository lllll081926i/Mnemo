//go:build windows

package player

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func newIPCAddress() (string, func()) {
	return fmt.Sprintf(`\\.\pipe\mnemo-mpv-%d-%d`, os.Getpid(), time.Now().UnixNano()), func() {}
}

func dialIPC(ctx context.Context, address string) (io.ReadWriteCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), address)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("player: create named pipe file")
	}
	return file, nil
}
