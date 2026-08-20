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
