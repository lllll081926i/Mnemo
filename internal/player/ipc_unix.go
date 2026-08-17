//go:build !windows

package player

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

func newIPCAddress() (string, func()) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("mnemo-mpv-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(path)
	return path, func() { _ = os.Remove(path) }
}

func dialIPC(ctx context.Context, address string) (io.ReadWriteCloser, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", address)
}
