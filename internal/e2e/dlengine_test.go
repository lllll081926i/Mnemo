package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/transfer/dlengine"
)

// TestSegmentedDownload verifies the native multi-connection downloader against
// a local HTTP server with Range support.
func TestSegmentedDownload(t *testing.T) {
	payload := make([]byte, 10<<20) // 10 MiB of random-ish data
	for i := range payload {
		payload[i] = byte(i*7 + 13)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"payload-v1"`)
		rang := r.Header.Get("Range")
		if rang == "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
			return
		}
		var start, end int64
		fmt.Sscanf(rang, "bytes=%d-%d", &start, &end)
		if rang != "bytes=0-0" && r.Header.Get("If-Range") != `"payload-v1"` {
			http.Error(w, "missing If-Range", http.StatusPreconditionRequired)
			return
		}
		if end >= int64(len(payload)) {
			end = int64(len(payload)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "downloaded.bin")

	// Download with seg engine (4 segments, 4MiB chunks, 10MiB file → enters multi-chunk)
	err := dlengine.Download(context.Background(), dlengine.Options{
		Concurrency: 4,
		ChunkSize:   4 << 20,
		MinSize:     1 << 20, // small min to force multi-chunk
	}, srv.URL+"/test.bin", dest, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	// Verify content
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("size mismatch: %d vs %d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("byte %d mismatch: %d vs %d", i, got[i], payload[i])
		}
	}

	// Verify .part and .state files are cleaned up
	if _, err := os.Stat(dest + ".part"); err == nil {
		t.Fatal(".part file should be removed")
	}
	if _, err := os.Stat(dest + ".state.json"); err == nil {
		t.Fatal(".state.json should be removed")
	}

	// Test resume: download again with existing complete file → should succeed
	err = dlengine.Download(context.Background(), dlengine.Options{
		Concurrency: 4,
		ChunkSize:   4 << 20,
		MinSize:     1 << 20,
	}, srv.URL+"/test.bin", dest, nil)
	if err != nil {
		t.Fatalf("resume download: %v", err)
	}

	// Test single-stream (no Range headers)
	srvNoRange := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srvNoRange.Close()
	dest2 := filepath.Join(t.TempDir(), "noslice.bin")
	err = dlengine.Download(context.Background(), dlengine.Options{}, srvNoRange.URL+"/full.bin", dest2, nil)
	if err != nil {
		t.Fatalf("single-stream: %v", err)
	}
	got2, _ := os.ReadFile(dest2)
	if len(got2) != len(payload) {
		t.Fatalf("single-stream size: %d", len(got2))
	}
}

func TestSegmentedDownloadRejectsInvalidProbeRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Range", "bytes 1-1/64")
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{1})
	}))
	defer srv.Close()

	err := dlengine.Download(context.Background(), dlengine.Options{ChunkSize: 16, MinSize: 1}, srv.URL, filepath.Join(t.TempDir(), "invalid.bin"), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid probe Content-Range") {
		t.Fatalf("expected invalid probe range error, got %v", err)
	}
}

func TestSegmentedDownloadAppliesRequestAuthToEveryRequest(t *testing.T) {
	payload := make([]byte, 64)
	for i := range payload {
		payload[i] = byte(i)
	}
	var mu sync.Mutex
	seen := make(map[string]bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonceCount := r.Header.Get("X-Test-Nonce-Count")
		if nonceCount == "" {
			http.Error(w, "missing request authentication", http.StatusUnauthorized)
			return
		}
		mu.Lock()
		duplicate := seen[nonceCount]
		seen[nonceCount] = true
		mu.Unlock()
		if duplicate {
			http.Error(w, "replayed request authentication", http.StatusUnauthorized)
			return
		}
		var start, end int64
		_, _ = fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		if end >= int64(len(payload)) {
			end = int64(len(payload)) - 1
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	var next int
	err := dlengine.Download(context.Background(), dlengine.Options{
		Concurrency: 1,
		ChunkSize:   16,
		MinSize:     1,
		RequestAuth: func(req *http.Request) error {
			next++
			req.Header.Set("X-Test-Nonce-Count", fmt.Sprintf("%08x", next))
			return nil
		},
	}, srv.URL, filepath.Join(t.TempDir(), "authenticated.bin"), nil)
	if err != nil {
		t.Fatalf("authenticated download: %v", err)
	}
	mu.Lock()
	count := len(seen)
	mu.Unlock()
	if count < 2 {
		t.Fatalf("request auth applied %d times, want probe plus download request", count)
	}
}

func TestSegmentedDownloadRejectsMismatchedChunkRange(t *testing.T) {
	payload := make([]byte, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		_, _ = fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		if start == 0 && end == 0 {
			w.Header().Set("Content-Range", "bytes 0-0/64")
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
			return
		}
		length := end - start + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/64", start+1, end+1))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[:length])
	}))
	defer srv.Close()

	err := dlengine.Download(context.Background(), dlengine.Options{Concurrency: 2, ChunkSize: 16, MinSize: 1}, srv.URL, filepath.Join(t.TempDir(), "mismatch.bin"), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid range response") {
		t.Fatalf("expected mismatched range error, got %v", err)
	}
}

func TestSegmentedDownloadRejectsResourceChangedAfterProbe(t *testing.T) {
	payload := make([]byte, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("Content-Range", "bytes 0-0/64")
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
			return
		}
		if r.Header.Get("If-Range") != `"v1"` {
			http.Error(w, "missing validator", http.StatusBadRequest)
			return
		}
		w.Header().Set("ETag", `"v2"`)
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	err := dlengine.Download(context.Background(), dlengine.Options{Concurrency: 2, ChunkSize: 16, MinSize: 1}, srv.URL, filepath.Join(t.TempDir(), "changed.bin"), nil)
	if err == nil || !strings.Contains(err.Error(), "remote resource changed") {
		t.Fatalf("expected resource changed error, got %v", err)
	}
}

var _ = io.Copy

func TestDownloadSizeFallbacks(t *testing.T) {
	payload := strings.Repeat("完整文件", 128)
	for _, test := range []struct {
		name          string
		payload       string
		expectedSize  int64
		probeRange    bool
		probeFullBody bool
		chunked       bool
	}{
		{name: "无长度但有文件元数据", payload: payload, expectedSize: int64(len(payload)), chunked: true},
		{name: "无长度也无文件元数据", payload: payload, chunked: true},
		{name: "无长度但有探测总大小", payload: payload, probeRange: true, chunked: true},
		{name: "探测总大小优先于错误元数据", payload: payload, expectedSize: 1, probeRange: true, chunked: true},
		{name: "忽略Range并返回完整ContentRange", payload: payload, probeRange: true, probeFullBody: true},
		{name: "真实的一字节文件", payload: "X"},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Range") == "bytes=0-0" {
					length := 1
					if test.probeFullBody {
						length = len(test.payload)
					}
					if test.probeRange {
						writer.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", length-1, len(test.payload)))
					}
					writer.Header().Set("Content-Length", fmt.Sprintf("%d", length))
					_, _ = io.WriteString(writer, test.payload[:length])
					return
				}
				if test.chunked {
					writer.WriteHeader(http.StatusOK)
					writer.(http.Flusher).Flush()
				} else {
					writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(test.payload)))
				}
				_, _ = io.WriteString(writer, test.payload)
			}))
			defer srv.Close()
			var last dlengine.Progress
			path := filepath.Join(t.TempDir(), "fallback.bin")
			err := dlengine.Download(context.Background(), dlengine.Options{ExpectedSize: test.expectedSize}, srv.URL, path, func(progress dlengine.Progress) {
				if progress.Percent < 0 || progress.Percent > 100 || progress.Total != 0 && progress.Total != int64(len(test.payload)) {
					t.Errorf("错误的下载进度: %+v", progress)
				}
				last = progress
			})
			if err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != test.payload {
				t.Fatalf("下载内容不一致: %v", err)
			}
			if last.Total != int64(len(test.payload)) || last.Downloaded != last.Total || last.Percent != 100 {
				t.Fatalf("下载完成进度不正确: %+v", last)
			}
		})
	}
}

func TestDownloadUsesFullResponseSizeAfterOneByteProbe(t *testing.T) {
	for _, probeRange := range []bool{false, true} {
		t.Run(fmt.Sprintf("content_range_%t", probeRange), func(t *testing.T) {
			payload := strings.Repeat("光鸭下载内容", 128)
			srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Range") == "bytes=0-0" {
					if probeRange {
						writer.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(payload)))
					}
					writer.Header().Set("Content-Length", "1")
					writer.WriteHeader(http.StatusOK)
					_, _ = writer.Write([]byte(payload[:1]))
					return
				}
				writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
				_, _ = io.WriteString(writer, payload)
			}))
			defer srv.Close()
			var snapshots []dlengine.Progress
			path := filepath.Join(t.TempDir(), "download.bin")
			err := dlengine.Download(context.Background(), dlengine.Options{}, srv.URL, path, func(progress dlengine.Progress) {
				snapshots = append(snapshots, progress)
			})
			if err != nil {
				t.Fatalf("探测响应的 1 字节不能当作完整文件大小: %v", err)
			}
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != payload {
				t.Fatalf("下载内容不一致: %v", err)
			}
			if len(snapshots) == 0 {
				t.Fatal("下载没有报告进度")
			}
			for _, progress := range snapshots {
				if progress.Total != int64(len(payload)) || progress.Percent < 0 || progress.Percent > 100 {
					t.Fatalf("错误的下载大小或百分比: %+v", progress)
				}
			}
			last := snapshots[len(snapshots)-1]
			if last.Downloaded != int64(len(payload)) || last.Percent != 100 {
				t.Fatalf("下载完成进度不正确: %+v", last)
			}
		})
	}
}
