package guangya

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestMultipartCancellationWaitsForPartsAndAborts(t *testing.T) {
	previous := netx.TestTransportHook
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseParts := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseParts()
	var uploads, aborts, commits atomic.Int32
	netx.TestTransportHook = guangyaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := ""
		switch {
		case request.Method == http.MethodPost && request.URL.Query().Has("uploads"):
			body = `<InitiateMultipartUploadResult><Bucket>bucket</Bucket><Key>file</Key><UploadId>session</UploadId></InitiateMultipartUploadResult>`
		case request.Method == http.MethodPut:
			uploads.Add(1)
			started <- struct{}{}
			<-request.Context().Done()
			<-release
			return nil, request.Context().Err()
		case request.Method == http.MethodDelete:
			aborts.Add(1)
		case request.Method == http.MethodPost:
			commits.Add(1)
		default:
			return nil, fmt.Errorf("意外请求 %s", request.Method)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	t.Cleanup(func() { netx.TestTransportHook = previous })
	path := filepath.Join(t.TempDir(), "file")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const size = 10*1024*1024 + 1
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- ossMultipart(ctx, &model.UploadingUI{Info: model.UploadInfo{Size: size}}, file, "https://oss-cn-test.aliyuncs.com", "bucket", "file", "access", "secret", "token", nil, "")
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("并发分片未启动")
		}
	}
	cancel()
	select {
	case err := <-result:
		t.Fatalf("分片收尾前返回：%v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseParts()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("取消被视为成功")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("分片结束后未返回")
	}
	if uploads.Load() != 2 || aborts.Load() != 1 || commits.Load() != 0 {
		t.Fatalf("取消后仍调度或提交：uploads=%d, aborts=%d, commits=%d", uploads.Load(), aborts.Load(), commits.Load())
	}
}
