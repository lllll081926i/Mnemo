package webdav

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadTransportStopsBlockedWrite(testContext *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		clientConn, serverConn := net.Pipe()
		transport := boundedUploadTransport(&http.Transport{
			DialContext: func(context.Context, string, string) (net.Conn, error) { return clientConn, nil },
		}, 100*time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		headersRead := make(chan struct{})
		go func() {
			_, _ = http.ReadRequest(bufio.NewReader(serverConn))
			close(headersRead)
		}()
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://example.test/file", strings.NewReader(strings.Repeat("x", 128*1024)))
		if err != nil {
			testContext.Fatal(err)
		}
		completed := make(chan error, 1)
		go func() {
			response, err := (&http.Client{Transport: transport}).Do(request)
			if response != nil {
				response.Body.Close()
			}
			completed <- err
		}()
		select {
		case <-headersRead:
		case <-time.After(3 * time.Second):
			cancel()
			serverConn.Close()
			testContext.Fatal("未收到请求头")
		}
		if cancelFirst {
			cancel()
		}
		select {
		case err := <-completed:
			var networkError net.Error
			if cancelFirst && !errors.Is(err, context.Canceled) {
				testContext.Error("取消未生效", err)
			} else if !cancelFirst && (!errors.As(err, &networkError) || !networkError.Timeout()) {
				testContext.Error("阻塞写入未超时", err)
			}
		case <-time.After(3 * time.Second):
			testContext.Error("上传仍然阻塞")
		}
		cancel()
		serverConn.Close()
		clientConn.Close()
		transport.CloseIdleConnections()
	}
}

type delayedUploadReader struct {
	delivered bool
}

func (reader *delayedUploadReader) Read(buffer []byte) (int, error) {
	if reader.delivered {
		return 0, io.EOF
	}
	time.Sleep(150 * time.Millisecond)
	reader.delivered = true
	buffer[0] = 'x'
	return 1, io.EOF
}

func TestUploadTransportExcludesSourceWaitAndUsesHTTP1OverTLS(testContext *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.ProtoMajor != 1 {
			testContext.Error("专用上传连接必须使用 HTTP/1，避免 HTTP/2 流控绕过写期限")
		}
		data, err := io.ReadAll(request.Body)
		if err != nil || string(data) != "x" {
			testContext.Error("正文错误", err)
		}
		writer.WriteHeader(http.StatusCreated)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	transport := boundedUploadTransport(server.Client().Transport.(*http.Transport), 100*time.Millisecond)
	defer transport.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodPut, server.URL, &delayedUploadReader{})
	if err != nil {
		testContext.Fatal(err)
	}
	request.ContentLength = 1
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		testContext.Fatal("慢源读取不应被网络期限中断", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		testContext.Fatal(response.Status)
	}
}
