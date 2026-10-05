package dlengine

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingleConnectionFallbackPreservesCompletedChunks(t *testing.T) {
	t.Parallel()
	payload := bytes.Repeat([]byte("thunder"), 16000)
	chunk := int64(32000)
	var probes, firstChunks, active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "Dalvik/thunder" || request.Header.Get("Authorization") != "" {
			t.Errorf("下载头未隔离: %+v", request.Header)
		}
		var start, end int64
		if _, err := fmt.Sscanf(request.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Error(err)
			writer.WriteHeader(400)
			return
		}
		if start == 0 && end == 0 {
			probes.Add(1)
		} else if probes.Load() == 1 && start > 0 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		} else {
			if start == 0 {
				firstChunks.Add(1)
			}
			if probes.Load() > 1 {
				count := active.Add(1)
				defer active.Add(-1)
				for previous := peak.Load(); count > previous && !peak.CompareAndSwap(previous, count); previous = peak.Load() {
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		writer.Header().Set("ETag", `"stable"`)
		writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	var completed Progress
	err := Download(t.Context(), Options{Concurrency: 2, ChunkSize: chunk, MinSize: 1, RetrySingleConnection: true, ResourceID: "account/drive/file", Headers: map[string]string{"User-Agent": "Dalvik/thunder"}}, server.URL, path, func(progress Progress) { completed = progress })
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, payload) || probes.Load() != 2 || firstChunks.Load() != 1 || peak.Load() != 1 || completed.Downloaded != int64(len(payload)) || completed.Percent != 100 {
		t.Fatalf("降级未完整续传或未限制并发: %v, probes=%d first=%d peak=%d progress=%+v", err, probes.Load(), firstChunks.Load(), peak.Load(), completed)
	}
}

func TestSingleConnectionFallbackIsLimitedToSafeFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 408, 412, 416, 429, 500, 501, 502, 503, 504} {
		err := fmt.Errorf("chunk: %w", &httpStatusError{stage: "range", status: status})
		want := status == 408 || status == 500 || status == 502 || status == 503 || status == 504
		if got := canRetrySingleConnection(err); got != want {
			t.Errorf("HTTP %d 降级=%v，期望 %v", status, got, want)
		}
	}
	for _, err := range []error{errors.New("local disk full"), errResourceChanged, errInvalidRangeResponse, &httpStatusError{stage: "probe", status: 503}, &httpStatusError{stage: "stream", status: 503}, &httpStatusError{stage: "range", status: 503, retryAfter: time.Hour}, errors.Join(&httpStatusError{stage: "range", status: 503}, &os.PathError{Op: "write", Path: "file.part", Err: os.ErrPermission})} {
		if canRetrySingleConnection(err) {
			t.Fatalf("不安全错误不能自动降级: %v", err)
		}
	}
}

func TestSingleConnectionFallbackDoesNotIgnoreLongRetryAfter(t *testing.T) {
	t.Parallel()
	var probes, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Range") == "bytes=0-0" {
			probes.Add(1)
			writeTestRange(writer, 0, 0, 16, []byte{'x'})
			return
		}
		writer.Header().Set("Retry-After", "3600")
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	err := Download(t.Context(), Options{Concurrency: 2, ChunkSize: 16, MinSize: 1, RetrySingleConnection: true}, server.URL, filepath.Join(t.TempDir(), "file"), nil)
	if err == nil || probes.Load() != 1 || calls.Load() != 2 {
		t.Fatalf("长时间 Retry-After 被绕过: %v, probes=%d calls=%d", err, probes.Load(), calls.Load())
	}
}

func TestSingleConnectionFallbackStopsAfterOneRecovery(t *testing.T) {
	t.Parallel()
	var probes, ranges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Range") == "bytes=0-0" {
			probes.Add(1)
			writeTestRange(writer, 0, 0, 16, []byte{'x'})
			return
		}
		ranges.Add(1)
		if probes.Load() > 1 {
			writer.Header().Set("Retry-After", "3600")
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	err := Download(t.Context(), Options{Concurrency: 2, ChunkSize: 16, MinSize: 1, RetrySingleConnection: true}, server.URL, filepath.Join(t.TempDir(), "file"), nil)
	var status *httpStatusError
	if !errors.As(err, &status) || status.status != 503 || probes.Load() != 2 || ranges.Load() != MaxRetriesPerChunk+1 {
		t.Fatalf("单连接失败后不应再次恢复: %v, probes=%d ranges=%d", err, probes.Load(), ranges.Load())
	}
}
