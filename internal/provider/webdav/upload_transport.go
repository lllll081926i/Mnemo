package webdav

import (
	"net"
	"net/http"
	"time"

	"mnemo-go/internal/netx"
)

type uploadWriteConn struct {
	net.Conn
	timeout time.Duration
}

func (connection *uploadWriteConn) Write(data []byte) (int, error) {
	if err := connection.Conn.SetWriteDeadline(time.Now().Add(connection.timeout)); err != nil {
		return 0, err
	}
	defer connection.Conn.SetWriteDeadline(time.Time{})
	return connection.Conn.Write(data)
}

func boundedUploadTransport(original *http.Transport, timeout time.Duration) *http.Transport {
	return netx.BoundedUploadTransport(original, timeout)
}
