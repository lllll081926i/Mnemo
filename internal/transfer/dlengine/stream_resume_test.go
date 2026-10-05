package dlengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestSmallStreamResumesAfterSignedURLRotation(t *testing.T) {
	payload := bytes.Repeat([]byte("small-file"), 4096)
	const prefix = 4096
	var interrupted atomic.Bool
	interrupted.Store(true)
	var resumedStart atomic.Int64
	var resumedRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"stable"`)
		if request.Header.Get("User-Agent") != "provider-agent" || request.Header.Get("Referer") != "https://provider.test/" || request.Header.Get("X-Request-Auth") != "signed" {
			t.Error("续传未保留网盘请求头和请求鉴权")
		}
		if start, end, ok := parseTestRange(request.Header.Get("Range")); ok {
			if end != 0 {
				resumedStart.CompareAndSwap(0, start)
				resumedRequests.Add(1)
				end = min(end, start+4096-1)
				if request.Header.Get("If-Range") != `"stable"` {
					t.Error("续传缺少资源身份条件")
				}
			}
			writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
			return
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if interrupted.Load() {
			_, _ = writer.Write(payload[:prefix])
			return
		}
		_, _ = writer.Write(payload)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "small.bin")
	opts := Options{ResourceID: "account/drive/file", Headers: map[string]string{"User-Agent": "provider-agent", "Referer": "https://provider.test/"}, RequestAuth: func(request *http.Request) error { request.Header.Set("X-Request-Auth", "signed"); return nil }}
	if err := Download(t.Context(), opts, server.URL+"?sig=old", path, nil); err == nil {
		t.Fatal("中断的单流下载未报错")
	}
	if progress, ok := CheckpointProgress(path, opts.ResourceID); !ok || progress.Downloaded != prefix {
		t.Fatalf("单流字节检查点错误：%+v, %v", progress, ok)
	}
	interrupted.Store(false)
	var first Progress
	count := 0
	if err := Download(t.Context(), opts, server.URL+"?sig=new", path, func(progress Progress) {
		if count == 0 {
			first = progress
		}
		count++
	}); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, payload) || resumedStart.Load() != prefix || first.Downloaded != prefix || resumedRequests.Load() <= 1 {
		t.Fatalf("单流续传内容或偏移错误：%v, start=%d, first=%+v", err, resumedStart.Load(), first)
	}
	if _, err := os.Stat(path + ".state.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("下载成功后未清理检查点")
	}
}

func TestStreamResumeRejectsUnsafeResponses(t *testing.T) {
	for _, sample := range []struct {
		name        string
		etag        string
		status      int
		rangeOffset int64
		totalOffset int64
		partLength  int
		resourceID  string
		wantRange   bool
		wantErr     error
	}{
		{name: "条件范围被忽略时完整重下", etag: `"stable"`, status: 200, partLength: 4096, resourceID: "file", wantRange: true},
		{name: "身份变化且返回完整资源时重下", etag: `"changed"`, status: 200, partLength: 4096, resourceID: "file", wantRange: true},
		{name: "错误偏移拒绝追加", etag: `"stable"`, status: 206, rangeOffset: -1, partLength: 4096, resourceID: "file", wantRange: true, wantErr: errInvalidRangeResponse},
		{name: "错误总长度拒绝追加", etag: `"stable"`, status: 206, totalOffset: 1, partLength: 4096, resourceID: "file", wantRange: true, wantErr: errInvalidRangeResponse},
		{name: "身份变化拒绝追加", etag: `"changed"`, status: 206, partLength: 4096, resourceID: "file", wantRange: true, wantErr: errResourceChanged},
		{name: "身份缺失拒绝追加", status: 206, partLength: 4096, resourceID: "file", wantRange: true, wantErr: errResourceChanged},
		{name: "服务端拒绝资源条件", etag: `"stable"`, status: 412, partLength: 4096, resourceID: "file", wantRange: true, wantErr: errResourceChanged},
		{name: "临时文件被截断时重下", etag: `"stable"`, status: 200, partLength: 100, resourceID: "file"},
		{name: "其他账号资源不能复用", etag: `"stable"`, status: 200, partLength: 4096, resourceID: "other-file"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("safe-stream"), 2048)
			var ranged atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("ETag", `"stable"`)
				if request.Header.Get("Range") == "bytes=0-0" {
					writeTestRange(writer, 0, 0, int64(len(payload)), payload[:1])
					return
				}
				writer.Header().Set("ETag", sample.etag)
				start, end, ok := parseTestRange(request.Header.Get("Range"))
				ranged.Store(ok)
				if sample.status == 206 {
					writeTestRange(writer, start+sample.rangeOffset, end, int64(len(payload))+sample.totalOffset, payload[start:end+1])
					return
				}
				writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
				writer.WriteHeader(sample.status)
				_, _ = writer.Write(payload)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(path+".part", payload[:sample.partLength], 0600); err != nil {
				t.Fatal(err)
			}
			saved := &state{Stream: true, Total: int64(len(payload)), StreamBytes: 4096, URLHash: urlFingerprint(server.URL), ResourceHash: urlFingerprint("file"), ETag: `"stable"`}
			if err := persistState(path+".state.json", saved); err != nil {
				t.Fatal(err)
			}
			err := Download(t.Context(), Options{ResourceID: sample.resourceID, MinSize: 1}, server.URL, path, nil)
			if (sample.wantErr == nil && err != nil) || (sample.wantErr != nil && !errors.Is(err, sample.wantErr)) || ranged.Load() != sample.wantRange {
				t.Fatalf("续传安全判断错误：%v, range=%v", err, ranged.Load())
			}
			if sample.wantErr != nil {
				actual, readErr := os.ReadFile(path + ".part")
				if readErr != nil || !bytes.Equal(actual, payload[:sample.partLength]) {
					t.Fatal("未经校验的正文修改了旧断点文件")
				}
				if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatal("校验失败却提交了最终文件")
				}
			} else if actual, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(actual, payload) {
				t.Fatal("完整重下内容错误")
			}
		})
	}
}

func TestPeriodicCheckpointPreservesInFlightPrefix(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			payload := bytes.Repeat([]byte("checkpoint"), 4096)
			const prefix = 4096
			var block atomic.Bool
			block.Store(true)
			var resumedStart atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("ETag", `"stable"`)
				start, end, ranged := parseTestRange(request.Header.Get("Range"))
				if ranged && end == 0 {
					writeTestRange(writer, 0, 0, int64(len(payload)), payload[:1])
					return
				}
				if !ranged {
					end = int64(len(payload)) - 1
				}
				if block.Load() {
					if ranged {
						writePartialTestRange(writer, start, end, int64(len(payload)), payload[:prefix])
					} else {
						writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
						_, _ = writer.Write(payload[:prefix])
						writer.(http.Flusher).Flush()
					}
					<-request.Context().Done()
					return
				}
				resumedStart.Store(start)
				writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "original")
			opts := Options{Concurrency: 1, ChunkSize: int64(len(payload)), MinSize: 1, ResourceID: "file"}
			if stream {
				opts.MinSize = int64(len(payload)) + 1
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- Download(ctx, opts, server.URL, path, nil) }()
			deadline := time.Now().Add(5 * time.Second)
			var checkpoint []byte
			for time.Now().Before(deadline) {
				data, err := os.ReadFile(path + ".state.json")
				var saved state
				if err == nil && json.Unmarshal(data, &saved) == nil && (saved.StreamBytes == prefix || len(saved.PartialBytes) == 1 && saved.PartialBytes[0] == prefix) {
					checkpoint = data
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			part, readErr := os.ReadFile(path + ".part")
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) || readErr != nil || len(checkpoint) == 0 {
				t.Fatalf("运行中未保存有效前缀：%v, %v, checkpoint=%s", err, readErr, checkpoint)
			}
			copyPath := filepath.Join(t.TempDir(), "restarted")
			if err := os.WriteFile(copyPath+".part", part, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(copyPath+".state.json", checkpoint, 0600); err != nil {
				t.Fatal(err)
			}
			block.Store(false)
			if err := Download(t.Context(), opts, server.URL, copyPath, nil); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(copyPath)
			if err != nil || !bytes.Equal(actual, payload) || resumedStart.Load() != prefix {
				t.Fatalf("异常退出检查点未恢复：%v, start=%d", err, resumedStart.Load())
			}
		})
	}
}
