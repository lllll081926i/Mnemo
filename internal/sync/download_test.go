package sync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
