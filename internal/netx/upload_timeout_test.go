package netx

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadControlTimeoutDoesNotLimitBody(testContext *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		time.Sleep(60 * time.Millisecond)
		writer.WriteHeader(201)
	}))
	defer server.Close()
	client := NewClient(10 * time.Millisecond)
	response, err := client.DoUpload(testContext.Context(), http.MethodPut, server.URL, nil, strings.NewReader("payload"))
	if err != nil {
		testContext.Fatalf("上传被控制请求总超时中止: %v", err)
	}
	response.Body.Close()
	if _, err := client.Do(testContext.Context(), http.MethodGet, server.URL, nil, nil); err == nil {
		testContext.Fatal("控制请求丢失原有超时")
	}
}

func TestUploadNetworkWriteIsBounded(testContext *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	connection := &uploadWriteConn{Conn: local, timeout: 20 * time.Millisecond}
	if _, err := connection.Write([]byte("blocked")); err == nil {
		testContext.Fatal("阻塞网络写入没有退出")
	}
}

func TestUploadResponseBodyReadHasIdleCancellation(testContext *testing.T) {
	ctx, cancel := context.WithCancel(testContext.Context())
	defer cancel()
	body := &uploadResponseBody{ReadCloser: &cancelableResponseReader{ctx: ctx}, cancel: cancel, idle: 20 * time.Millisecond}
	if _, err := body.Read(make([]byte, 1)); err == nil {
		testContext.Fatal("响应正文无限等待")
	}
}

type cancelableResponseReader struct{ ctx context.Context }

func (reader *cancelableResponseReader) Read([]byte) (int, error) {
	<-reader.ctx.Done()
	return 0, reader.ctx.Err()
}

func (*cancelableResponseReader) Close() error { return nil }
