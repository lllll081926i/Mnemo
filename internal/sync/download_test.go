package sync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestSyncUsesInjectedDownloadLimiter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.ServeContent(writer, request, "file", time.Time{}, strings.NewReader("payload"))
	}))
	defer server.Close()
	limiter := &rejectingDownloadLimiter{}
	engine := NewEngine(nil, WithDownloadOptions(func() (dlengine.Options, error) {
		return dlengine.Options{Concurrency: 1, Limiter: limiter}, nil
	}))
	err := engine.downloadTo(t.Context(), &model.DownloadURL{URL: server.URL, DownloadMode: "proxy"}, filepath.Join(t.TempDir(), "file"))
	if !errors.Is(err, context.Canceled) || limiter.bytes != 7 {
		t.Fatalf("同步绕过全局限速器: %v，字节 %d", err, limiter.bytes)
	}
}

func TestSyncPropagatesLastModifiedRecoveryPolicy(t *testing.T) {
	payload := strings.Repeat("guangya", 1024)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Last-Modified", "Sun, 04 Oct 2026 01:02:03 GMT")
		if request.Header.Get("Range") == "" {
			if request.Header.Get("If-Unmodified-Since") != "" {
				t.Error("同步恢复仍发送过期修改时间")
			}
			_, _ = writer.Write([]byte(payload))
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(request.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Error(err)
			writer.WriteHeader(400)
			return
		}
		if end == 0 {
			writer.Header().Set("Last-Modified", "Mon, 05 Oct 2026 02:37:34 GMT")
		}
		writer.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write([]byte(payload[start : end+1]))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	err := downloadToWithOptions(t.Context(), &model.DownloadURL{URL: server.URL, AllowLastModifiedFallback: true, ChunkSize: 1024}, path, dlengine.Options{Concurrency: 8, MinSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != payload {
		t.Fatalf("同步恢复内容不正确: %v", err)
	}
}
