package migrate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/transfer/dlengine"
)

type rejectingDownloadLimiter struct {
	bytes int64
}

func (limiter *rejectingDownloadLimiter) Wait(ctx context.Context, count int64) error {
	limiter.bytes += count
	return context.Canceled
}

func TestMigrationUsesInjectedDownloadLimiter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "payload")
	}))
	defer server.Close()
	limiter := &rejectingDownloadLimiter{}
	var output strings.Builder
	job := &Job{}
	err := downloadToCounted(t.Context(), &model.DownloadURL{URL: server.URL}, &output, job, dlengine.Options{Limiter: limiter})
	if !errors.Is(err, context.Canceled) || output.Len() != 0 || job.ProcessedBytes != 0 || limiter.bytes != 7 {
		t.Fatalf("迁移绕过全局限速器: %v，字节 %d", err, limiter.bytes)
	}
}

func TestMigrationConsumerWaitDoesNotTriggerReadIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "payload")
	}))
	defer server.Close()
	var content []byte
	err := streamMigration(t.Context(), &model.DownloadURL{URL: server.URL}, &Job{}, func(reader io.Reader) error {
		time.Sleep(100 * time.Millisecond)
		var err error
		content, err = io.ReadAll(reader)
		return err
	}, dlengine.Options{ReadIdleTimeout: 20 * time.Millisecond})
	if err != nil || string(content) != "payload" {
		t.Fatalf("上传方等待被误判为下载空闲: %q %v", content, err)
	}
}

func TestMigrationRejectsMetadataSizeMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "short")
	}))
	defer server.Close()
	var output strings.Builder
	if err := downloadTo(t.Context(), &model.DownloadURL{URL: server.URL, Size: 100}, &output); err == nil {
		t.Fatal("HTTP 成功但文件长度不符时不应提交成品")
	}
}
