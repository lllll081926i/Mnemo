package netx

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type uploadResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	idle   time.Duration
}

func (body *uploadResponseBody) Read(data []byte) (int, error) {
	timer := time.AfterFunc(body.idle, body.cancel)
	defer timer.Stop()
	return body.ReadCloser.Read(data)
}

func (body *uploadResponseBody) Close() error {
	body.cancel()
	return body.ReadCloser.Close()
}

func boundedUploadResponse(response *http.Response, err error, cancel context.CancelFunc) (*http.Response, error) {
	if err != nil || response == nil || response.Body == nil {
		cancel()
		return response, err
	}
	response.Body = &uploadResponseBody{ReadCloser: response.Body, cancel: cancel, idle: 90 * time.Second}
	return response, nil
}

type uploadRequestKey struct{}

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

func BoundedUploadTransport(original *http.Transport, timeout time.Duration) *http.Transport {
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
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
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

type uploadRoutingTransport struct {
	control http.RoundTripper
	once    sync.Once
	upload  *http.Transport
}

func (transport *uploadRoutingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Context().Value(uploadRequestKey{}) == true {
		if base, ok := transport.control.(*http.Transport); ok {
			transport.once.Do(func() { transport.upload = BoundedUploadTransport(base, 90*time.Second) })
			return transport.upload.RoundTrip(request)
		}
	}
	return transport.control.RoundTrip(request)
}

func (transport *uploadRoutingTransport) CloseIdleConnections() {
	if closer, ok := transport.control.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	transport.once.Do(func() {
		if base, ok := transport.control.(*http.Transport); ok {
			transport.upload = BoundedUploadTransport(base, 90*time.Second)
		}
	})
	if transport.upload != nil {
		transport.upload.CloseIdleConnections()
	}
}
