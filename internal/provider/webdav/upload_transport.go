package webdav

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
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
	transport := original.Clone()
	transport.ResponseHeaderTimeout = timeout
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	transport.ForceAttemptHTTP2 = false
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	dial := transport.DialContext
	if dial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		dial = dialer.DialContext
	}
	wrap := func(connect func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := connect(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return &uploadWriteConn{Conn: connection, timeout: timeout}, nil
		}
	}
	transport.DialContext = wrap(dial)
	if transport.DialTLSContext != nil {
		transport.DialTLSContext = wrap(transport.DialTLSContext)
	}
	return transport
}
